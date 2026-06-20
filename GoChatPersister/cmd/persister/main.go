package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"GoChatPersister/internal/config"
	"GoChatPersister/internal/consumer"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.Load()
	slog.Info("GoChatPersister 启动", "rabbit", cfg.RabbitURL, "queue", cfg.Queue)

	p, err := consumer.New(cfg)
	if err != nil {
		slog.Error("初始化失败", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		slog.Info("收到退出信号，停止消费")
		cancel()
	}()

	p.Run(ctx)
	slog.Info("GoChatPersister 已退出")
}
