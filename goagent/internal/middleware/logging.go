package middleware

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// ============================================================
// gRPC拦截器 — 日志、panic恢复、请求耗时统计
// ============================================================

// Logger 日志接口，解耦具体日志实现
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// UnaryServerInterceptor 一元RPC拦截器链
func UnaryServerInterceptor(log Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Recovery from panic
		defer func() {
			if r := recover(); r != nil {
				log.Error("gRPC handler panic recovered",
					"method", info.FullMethod,
					"panic", r,
				)
			}
		}()

		// 记录请求日志
		log.Info("gRPC request",
			"method", info.FullMethod,
		)

		// 执行实际处理
		resp, err := handler(ctx, req)

		// 记录耗时
		duration := time.Since(start)
		if err != nil {
			log.Warn("gRPC request completed with error",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
				"error", err.Error(),
			)
		} else {
			log.Info("gRPC request completed",
				"method", info.FullMethod,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return resp, err
	}
}
