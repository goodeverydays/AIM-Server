package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 服务全局配置
type Config struct {
	Server    ServerConfig
	LLM       LLMConfig
	Redis     RedisConfig
	Log       LogConfig
	RateLimit RateLimitConfig
	RAG       RAGConfig
}

// RAGConfig 基于聊天记录的检索增强(rag_qa 技能)配置。
// 默认关闭：不配置则 rag_qa 返回"未启用"提示，不影响其余功能。
type RAGConfig struct {
	Enabled       bool   // 是否启用 RAG
	MySQLDSN      string // 读 t_chatmsg 的 DSN: user:pass@tcp(host:3306)/db?charset=utf8mb4&parseTime=true
	TopK          int    // 检索返回片段数
	MaxCorpus     int    // 单用户最多索引的最近消息条数
	VectorBackend string // 向量库后端: "memory"(默认/回退) 或 "qdrant"
	Embed         EmbedConfig
	Qdrant        QdrantConfig
}

// QdrantConfig 外部向量数据库 Qdrant(REST)配置。
type QdrantConfig struct {
	URL        string // 如 http://localhost:6333
	APIKey     string // 可选
	Collection string // 集合名
}

// EmbedConfig embedding 提供商(OpenAI 兼容 /embeddings)。与对话 LLM 解耦。
type EmbedConfig struct {
	APIKey  string
	BaseURL string // 如 https://api.siliconflow.cn/v1
	Model   string // 如 BAAI/bge-m3
}

// RateLimitConfig AI 用量配额配置
type RateLimitConfig struct {
	Enabled   bool           // 是否启用限流
	FreeDaily int            // 普通用户每日可用次数
	VIPDaily  int            // VIP 用户每日可用次数
	VIPUsers  map[int32]bool // VIP 用户ID集合
}

// ServerConfig gRPC服务器配置
type ServerConfig struct {
	Host     string // 监听地址
	Port     int    // gRPC监听端口
	HTTPPort int    // HTTP REST监听端口
}

// LLMConfig 大模型配置
type LLMConfig struct {
	Provider string // 当前使用的provider: "empty" / "openai" / "anthropic"

	// 具体provider的配置（后续按需扩展）
	OpenAI    OpenAIConfig
	Anthropic AnthropicConfig
}

type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type AnthropicConfig struct {
	APIKey string
	Model  string
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Enabled  bool   // 是否启用 Redis
	Addr     string // 地址，如 "192.168.100.128:6379"
	Password string // 密码，无密码留空
	DB       int    // 数据库编号
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string // debug / info / warn / error
	Format string // json / text
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			// 仅绑回环：gRPC 由同机 IMServer 经 127.0.0.1 调用，
			// HTTP REST 仅作本机调试/探活旁路，二者均不对外暴露。
			// 如确需对外，可通过环境变量 AGENT_HOST 覆盖。
			Host:     "127.0.0.1",
			Port:     19527,
			HTTPPort: 19528,
		},
		LLM: LLMConfig{
			Provider: "openai",
			OpenAI: OpenAIConfig{
				// 默认指向 OpenAI；对接国内厂商时用 OPENAI_BASE_URL/OPENAI_MODEL 覆盖即可，
				// 例如 DeepSeek: BASE_URL=https://api.deepseek.com/v1 MODEL=deepseek-chat
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4o-mini",
			},
		},
		Redis: RedisConfig{
			Enabled:  true,
			Addr:     "192.168.100.128:6379",
			Password: "",
			DB:       0,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		RateLimit: RateLimitConfig{
			Enabled:   true,
			FreeDaily: 5,  // 普通用户每天 5 条
			VIPDaily:  20, // VIP 每天 100 条
			VIPUsers:  map[int32]bool{},
		},
		RAG: RAGConfig{
			Enabled:       false, // 默认关闭，配置 RAG_ENABLED=1 后启用
			TopK:          8,
			MaxCorpus:     1000,
			VectorBackend: "memory", // 默认内存；设 RAG_VECTOR_BACKEND=qdrant 切换
			Embed: EmbedConfig{
				// 推荐硅基流动(国内直连、免费 bge-m3)。也可换 Ollama 本地或 OpenAI。
				BaseURL: "https://api.siliconflow.cn/v1",
				Model:   "BAAI/bge-m3",
			},
			Qdrant: QdrantConfig{
				URL:        "http://localhost:6333",
				Collection: "im_chat_rag",
			},
		},
	}
}

// Load 从环境变量加载配置（后续可扩展为配置文件加载）
func Load() *Config {
	// 先尝试加载工作目录下的 .env（含密钥，已 gitignore，不会被提交）。
	// 不覆盖已存在的真实环境变量，便于部署时用系统环境变量优先。
	loadDotEnv(".env")

	cfg := DefaultConfig()

	// 从环境变量覆盖
	if v := os.Getenv("AGENT_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("AGENT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("AGENT_HTTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.HTTPPort = p
		}
	}
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.LLM.OpenAI.APIKey = v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.LLM.OpenAI.BaseURL = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" {
		cfg.LLM.OpenAI.Model = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.LLM.Anthropic.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_MODEL"); v != "" {
		cfg.LLM.Anthropic.Model = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.Log.Format = v
	}

	// Redis 环境变量
	if v := os.Getenv("REDIS_ENABLED"); v != "" {
		cfg.Redis.Enabled = (v == "true" || v == "1")
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if db, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = db
		}
	}

	// 限流环境变量
	if v := os.Getenv("AGENT_RATELIMIT_ENABLED"); v != "" {
		cfg.RateLimit.Enabled = (v == "true" || v == "1")
	}
	if v := os.Getenv("AGENT_FREE_DAILY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.FreeDaily = n
		}
	}
	if v := os.Getenv("AGENT_VIP_DAILY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.VIPDaily = n
		}
	}
	// AGENT_VIP_USERS: 逗号分隔的用户ID，如 "1001,1002,1003"
	if v := os.Getenv("AGENT_VIP_USERS"); v != "" {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if id, err := strconv.Atoi(part); err == nil {
				cfg.RateLimit.VIPUsers[int32(id)] = true
			}
		}
	}

	// RAG 环境变量
	if v := os.Getenv("RAG_ENABLED"); v != "" {
		cfg.RAG.Enabled = (v == "true" || v == "1")
	}
	if v := os.Getenv("RAG_MYSQL_DSN"); v != "" {
		cfg.RAG.MySQLDSN = v
	}
	if v := os.Getenv("RAG_TOPK"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RAG.TopK = n
		}
	}
	if v := os.Getenv("RAG_MAX_CORPUS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RAG.MaxCorpus = n
		}
	}
	if v := os.Getenv("EMBED_API_KEY"); v != "" {
		cfg.RAG.Embed.APIKey = v
	}
	if v := os.Getenv("EMBED_BASE_URL"); v != "" {
		cfg.RAG.Embed.BaseURL = v
	}
	if v := os.Getenv("EMBED_MODEL"); v != "" {
		cfg.RAG.Embed.Model = v
	}
	if v := os.Getenv("RAG_VECTOR_BACKEND"); v != "" {
		cfg.RAG.VectorBackend = v
	}
	if v := os.Getenv("QDRANT_URL"); v != "" {
		cfg.RAG.Qdrant.URL = v
	}
	if v := os.Getenv("QDRANT_API_KEY"); v != "" {
		cfg.RAG.Qdrant.APIKey = v
	}
	if v := os.Getenv("QDRANT_COLLECTION"); v != "" {
		cfg.RAG.Qdrant.Collection = v
	}

	return cfg
}

// loadDotEnv 读取简单的 KEY=VALUE 文件并注入进程环境变量。
// 仅在变量尚未设置时写入（系统环境变量优先）；文件不存在则静默跳过。
// 支持 # 注释、空行、可选的两侧引号；不依赖第三方库。
// loadDotEnv 依次在多个位置查找并加载 .env，加载到第一个存在的即停止。
// 解决"工作目录陷阱"：无论从项目根、build/ 还是用 systemd 启动都能读到。
// 查找顺序：当前工作目录 → 可执行文件所在目录 → 可执行文件上级目录。
func loadDotEnv(filename string) {
	candidates := []string{filename}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, filename),                  // 与二进制同目录
			filepath.Join(filepath.Dir(exeDir), filename))    // 二进制上级（如 build/ 的上级=项目根）
	}
	for _, p := range candidates {
		if tryLoadEnvFile(p) {
			return
		}
	}
}

// tryLoadEnvFile 读取并解析单个 .env 文件，文件不存在返回 false。
func tryLoadEnvFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return true
}

// Address 返回gRPC监听地址
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// HTTPAddress 返回HTTP监听地址
func (s ServerConfig) HTTPAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.HTTPPort)
}
