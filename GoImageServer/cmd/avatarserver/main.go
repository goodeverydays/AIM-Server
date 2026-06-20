package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"GoImageServer/internal/config"
	"GoImageServer/internal/server"
	"GoImageServer/internal/service"
	"GoImageServer/internal/storage"
)

var Version = "0.2.0"

func main() {
	// 结构化日志(标准库 log/slog)，设为全局默认供各包复用
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.Load()

	// 按配置选择存储后端：local(本地文件) / s3(对象存储)
	var store storage.Storage
	var staticDir string // 本地后端需要 HTTP 静态服务目录；对象存储为空
	var err error

	switch cfg.Storage.Backend {
	case "s3":
		store, err = storage.NewObjectStore(
			cfg.Storage.S3.Endpoint,
			cfg.Storage.S3.Bucket,
			cfg.Storage.S3.AccessKey,
			cfg.Storage.S3.SecretKey,
			cfg.Storage.S3.Region,
			cfg.Storage.S3.PublicBase,
			cfg.Storage.S3.UseSSL,
			cfg.Storage.MaxSizeMB,
			cfg.Storage.MaxWidth,
			cfg.Storage.MaxHeight,
		)
		if err != nil {
			slog.Error("对象存储初始化失败", "error", err)
			os.Exit(1)
		}
		slog.Info("存储后端: 对象存储(S3)",
			"bucket", cfg.Storage.S3.Bucket, "endpoint", cfg.Storage.S3.Endpoint)
	default: // "local"
		fileStore, ferr := storage.NewFileStore(
			cfg.Storage.AvatarDir,
			cfg.Server.PublicHTTPURL(),
			cfg.Storage.MaxSizeMB,
			cfg.Storage.MaxWidth,
			cfg.Storage.MaxHeight,
		)
		if ferr != nil {
			slog.Error("本地存储初始化失败", "error", ferr)
			os.Exit(1)
		}
		store = fileStore
		staticDir = fileStore.Dir()
		slog.Info("存储后端: 本地文件", "dir", staticDir)
	}

	// gRPC 服务
	svc := service.NewAvatarService(store, Version)
	grpcSrv := server.NewGRPCServer(cfg, svc)

	// HTTP 服务（上传API + 本地静态文件；对象存储模式不提供静态服务）
	httpSrv := server.NewHTTPServer(cfg.Server.HTTPAddress(), store, staticDir)

	// 优雅退出
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("收到退出信号")
		grpcSrv.Stop()
	}()

	// HTTP 在 goroutine 中启动
	go func() {
		if err := httpSrv.Start(); err != nil {
			slog.Error("HTTP 服务异常", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("GoImageServer 启动", "version", Version)
	slog.Info("监听地址",
		"grpc", cfg.Server.GRPCAddress(), "http", cfg.Server.HTTPAddress())

	if err := grpcSrv.Start(); err != nil {
		slog.Error("gRPC 服务异常", "error", err)
		os.Exit(1)
	}
}
