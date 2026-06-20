package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"GoMailServer/internal/config"
	"GoMailServer/internal/mailer"
	"GoMailServer/internal/server"
	"GoMailServer/internal/service"
	"GoMailServer/internal/store"
)

var Version = "0.1.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.Load()

	codeStore := store.NewCodeStore(cfg.Code.TTL, cfg.Code.ResendCooldown, cfg.Code.Length)
	m := mailer.New(cfg.SMTP)
	svc := service.NewMailService(codeStore, m, cfg.Code, Version)
	grpcSrv := server.NewGRPCServer(cfg, svc)

	// 启动验证码过期清理协程，生命周期随 ctx；退出时 cancel 以优雅停止
	ctx, cancel := context.WithCancel(context.Background())
	codeStore.StartCleanup(ctx, time.Minute)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("收到退出信号")
		cancel()       // 停止后台清理协程
		grpcSrv.Stop() // 优雅停止 gRPC（等待在途请求结束）
	}()

	slog.Info("GoMailServer 启动", "version", Version, "grpc", cfg.Server.Address())
	if cfg.SMTP.Host == "" {
		slog.Warn("SMTP 未配置 (SMTP_HOST 为空)，发送验证码将失败")
	}

	if err := grpcSrv.Start(); err != nil {
		slog.Error("gRPC 服务异常", "error", err)
		os.Exit(1)
	}
}
