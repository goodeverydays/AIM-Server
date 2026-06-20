package server

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"goagent/internal/config"
	"goagent/internal/middleware"
	"goagent/internal/service"
	"goagent/pkg/pb"
)

// ============================================================
// GRPCServer — 封装gRPC服务器的启动、注册、优雅关闭
// ============================================================

// GRPCServer gRPC服务器
type GRPCServer struct {
	cfg    *config.Config
	log    middleware.Logger
	server *grpc.Server
}

// NewGRPCServer 创建gRPC服务器
func NewGRPCServer(cfg *config.Config, log middleware.Logger, svc *service.AgentService) *GRPCServer {
	// 拦截器链
	interceptor := middleware.UnaryServerInterceptor(log)

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptor),
	)

	// 注册服务
	pb.RegisterAgentServiceServer(s, svc)

	// 注册服务反射（便于grpcurl等调试工具）
	reflection.Register(s)

	return &GRPCServer{
		cfg:    cfg,
		log:    log,
		server: s,
	}
}

// Start 启动gRPC服务器，阻塞直到Stop或错误
func (s *GRPCServer) Start() error {
	addr := s.cfg.Server.Address()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	s.log.Info("gRPC服务器启动", "address", addr)

	if err := s.server.Serve(lis); err != nil {
		return fmt.Errorf("gRPC服务异常退出: %w", err)
	}
	return nil
}

// Stop 优雅停止gRPC服务器
func (s *GRPCServer) Stop() {
	s.log.Info("gRPC服务器正在停止...")
	s.server.GracefulStop()
	s.log.Info("gRPC服务器已停止")
}
