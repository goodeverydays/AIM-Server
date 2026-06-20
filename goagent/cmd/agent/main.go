package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"goagent/internal/cache"
	"goagent/internal/config"
	"goagent/internal/llm"
	"goagent/internal/rag"
	"goagent/internal/ratelimit"
	"goagent/internal/server"
	"goagent/internal/service"
)

// 版本号，构建时通过 -ldflags 注入
var Version = "0.1.0"

func main() {
	// 结构化日志(标准库 log/slog)；设为全局默认，供包级 slog.Info 复用同一 handler。
	// *slog.Logger 的 Info/Warn/Error 签名恰好满足 middleware.Logger 接口，可直接注入。
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// 加载配置
	cfg := config.Load()

	logger.Info("goagent 启动中...",
		"version", Version,
		"address", cfg.Server.Address(),
		"llm_provider", cfg.LLM.Provider,
	)

	// 初始化缓存层 —— Redis 优先，不可用时降级到 Memory
	var c cache.Cache
	if cfg.Redis.Enabled {
		redisCache, err := cache.NewRedisCache(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			logger.Warn("Redis 连接失败，降级到内存缓存", "addr", cfg.Redis.Addr, "error", err)
			c = cache.NewMemoryCache()
		} else {
			logger.Info("Redis 已连接", "addr", cfg.Redis.Addr, "db", cfg.Redis.DB)
			c = redisCache
		}
	} else {
		logger.Info("Redis 未启用，使用内存缓存")
		c = cache.NewMemoryCache()
	}

	logger.Info("缓存层就绪", "backend", c.Name())

	// 创建LLM Provider
	provider, err := createProvider(cfg, c)
	if err != nil {
		logger.Error("创建Provider失败", "error", err)
		os.Exit(1)
	}

	// 配额限流器（普通/VIP 每日额度）
	limiter := ratelimit.New(c,
		cfg.RateLimit.Enabled,
		cfg.RateLimit.FreeDaily,
		cfg.RateLimit.VIPDaily,
		cfg.RateLimit.VIPUsers)
	logger.Info("配额限流",
		"enabled", cfg.RateLimit.Enabled,
		"free_daily", cfg.RateLimit.FreeDaily,
		"vip_daily", cfg.RateLimit.VIPDaily,
		"vip_users", len(cfg.RateLimit.VIPUsers))

	// RAG 检索器（默认关闭；启用需配置 MySQL DSN + embedding 厂商）
	retriever := buildRetriever(cfg, logger)

	// 创建gRPC服务实现（注入 Cache + 限流器 + RAG 检索器）
	svc := service.NewAgentService(provider, Version, c, limiter, retriever)

	// HTTP REST 服务器
	httpSrv := server.NewHTTPServer(cfg.Server.HTTPAddress(), provider, Version, limiter, logger)
	if err := httpSrv.Start(); err != nil {
		logger.Error("HTTP服务器启动失败", "error", err)
		os.Exit(1)
	}

	// gRPC 服务器
	srv := server.NewGRPCServer(cfg, logger, svc)

	// 优雅退出
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("收到信号，准备退出", "signal", sig.String())
		srv.Stop()
	}()

	logger.Info("goagent 已启动",
		"version", Version,
		"grpc_port", cfg.Server.Port,
		"http_port", cfg.Server.HTTPPort,
		"cache", c.Name())

	if err := srv.Start(); err != nil {
		logger.Error("服务器退出", "error", err)
		os.Exit(1)
	}

	logger.Info("goagent 已退出")
}

// buildRetriever 按配置构建 RAG 检索器；未启用或配置不全时返回 nil（rag_qa 技能会提示未启用）。
func buildRetriever(cfg *config.Config, logger *slog.Logger) *rag.Retriever {
	if !cfg.RAG.Enabled {
		logger.Info("RAG 未启用（设置 RAG_ENABLED=1 启用记忆问答）")
		return nil
	}
	if cfg.RAG.MySQLDSN == "" || cfg.RAG.Embed.APIKey == "" {
		logger.Warn("RAG 已启用但缺少 RAG_MYSQL_DSN 或 EMBED_API_KEY，已禁用记忆问答")
		return nil
	}
	db, err := rag.OpenMySQL(cfg.RAG.MySQLDSN)
	if err != nil {
		logger.Warn("RAG 连接 MySQL 失败，已禁用记忆问答", "error", err)
		return nil
	}
	emb := rag.NewOpenAIEmbedder(cfg.RAG.Embed.APIKey, cfg.RAG.Embed.BaseURL, cfg.RAG.Embed.Model)

	// 向量库后端：qdrant(外部向量数据库) 或 memory(进程内回退)
	var store rag.VectorStore
	switch cfg.RAG.VectorBackend {
	case "qdrant":
		store = rag.NewQdrantStore(cfg.RAG.Qdrant.URL, cfg.RAG.Qdrant.APIKey, cfg.RAG.Qdrant.Collection)
		logger.Info("RAG 向量后端 = Qdrant",
			"url", cfg.RAG.Qdrant.URL, "collection", cfg.RAG.Qdrant.Collection)
	default:
		store = rag.NewMemoryStore()
		logger.Info("RAG 向量后端 = 内存(MemoryStore)")
	}

	logger.Info("RAG 已启用",
		"embed_base", cfg.RAG.Embed.BaseURL,
		"embed_model", cfg.RAG.Embed.Model,
		"topk", cfg.RAG.TopK,
		"max_corpus", cfg.RAG.MaxCorpus)
	return rag.NewRetriever(db, emb, store, cfg.RAG.TopK, cfg.RAG.MaxCorpus)
}

func createProvider(cfg *config.Config, c cache.Cache) (llm.Provider, error) {
	switch cfg.LLM.Provider {
	case "empty":
		slog.Info("使用 EmptyProvider（LLM未配置，返回占位回复）")
		return llm.NewEmptyProvider(c), nil

	case "openai":
		// OpenAI 兼容接口：OpenAI / DeepSeek / Kimi / 智谱 等均可，按 BaseURL+Model 切换
		if cfg.LLM.OpenAI.APIKey == "" {
			slog.Warn("LLM_PROVIDER=openai 但 OPENAI_API_KEY 为空，回退到 EmptyProvider")
			return llm.NewEmptyProvider(c), nil
		}
		slog.Info("使用 OpenAI 兼容 Provider",
			"base", cfg.LLM.OpenAI.BaseURL, "model", cfg.LLM.OpenAI.Model)
		return llm.NewOpenAIProvider(
			cfg.LLM.OpenAI.APIKey, cfg.LLM.OpenAI.BaseURL, cfg.LLM.OpenAI.Model, c), nil

	default:
		slog.Warn("未知的 LLM Provider，回退到 EmptyProvider", "provider", cfg.LLM.Provider)
		return llm.NewEmptyProvider(c), nil
	}
}
