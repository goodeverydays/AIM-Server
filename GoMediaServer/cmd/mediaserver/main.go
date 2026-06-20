package main

import (
	"log/slog"
	"os"

	"GoMediaServer/internal/config"
	"GoMediaServer/internal/server"
	"GoMediaServer/internal/storage"
)

var Version = "0.1.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.Load()

	var store storage.Storage
	var staticDir string

	switch cfg.Backend {
	case "s3":
		objStore, err := storage.NewObjectStore(
			cfg.S3.Endpoint, cfg.S3.Bucket, cfg.S3.AccessKey, cfg.S3.SecretKey,
			cfg.S3.Region, cfg.S3.PublicBase, cfg.S3.UseSSL,
		)
		if err != nil {
			slog.Error("对象存储初始化失败", "error", err)
			os.Exit(1)
		}
		store = objStore
		slog.Info("媒体存储后端: 对象存储(S3)", "bucket", cfg.S3.Bucket, "endpoint", cfg.S3.Endpoint)
	default: // local
		fs, err := storage.NewFileStore(cfg.MediaDir, cfg.PublicURL())
		if err != nil {
			slog.Error("本地存储初始化失败", "error", err)
			os.Exit(1)
		}
		store = fs
		staticDir = fs.Dir()
		slog.Info("媒体存储后端: 本地文件", "dir", staticDir)
	}

	httpSrv := server.NewHTTPServer(cfg.Addr(), store, cfg.MaxSizeMB, staticDir)

	slog.Info("GoMediaServer 启动", "version", Version, "http", cfg.Addr())
	if err := httpSrv.Start(); err != nil {
		slog.Error("HTTP 服务异常", "error", err)
		os.Exit(1)
	}
}
