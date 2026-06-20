package service

import (
	"context"
	"regexp"

	"GoMailServer/internal/config"
	"GoMailServer/internal/mailer"
	"GoMailServer/internal/store"
	"GoMailServer/pkg/pb"
)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// MailService 实现 pb.MailServiceServer
type MailService struct {
	pb.UnimplementedMailServiceServer

	store   *store.CodeStore
	mailer  *mailer.Mailer
	cfg     config.CodeConfig
	version string
}

// NewMailService 创建邮件服务
func NewMailService(codeStore *store.CodeStore, m *mailer.Mailer, cfg config.CodeConfig, version string) *MailService {
	return &MailService{
		store:   codeStore,
		mailer:  m,
		cfg:     cfg,
		version: version,
	}
}

func (s *MailService) SendCode(ctx context.Context, req *pb.SendCodeReq) (*pb.SendCodeRsp, error) {
	email := req.Email
	if !emailPattern.MatchString(email) {
		return &pb.SendCodeRsp{Code: 2, Msg: "邮箱格式不正确"}, nil
	}

	code, ok, cooldown := s.store.Generate(email)
	if !ok {
		return &pb.SendCodeRsp{Code: 1, Msg: "发送过于频繁，请稍后再试", CooldownSeconds: int32(cooldown)}, nil
	}

	ttlMinutes := int(s.cfg.TTL.Minutes())
	if err := s.mailer.SendCode(email, code, ttlMinutes); err != nil {
		return &pb.SendCodeRsp{Code: 3, Msg: "邮件发送失败: " + err.Error()}, nil
	}

	return &pb.SendCodeRsp{Code: 0, Msg: "验证码已发送"}, nil
}

func (s *MailService) VerifyCode(ctx context.Context, req *pb.VerifyCodeReq) (*pb.VerifyCodeRsp, error) {
	if !s.store.Verify(req.Email, req.Code) {
		return &pb.VerifyCodeRsp{Code: 1, Msg: "验证码错误或已过期"}, nil
	}
	return &pb.VerifyCodeRsp{Code: 0, Msg: "ok"}, nil
}

func (s *MailService) HealthCheck(ctx context.Context, req *pb.HealthCheckReq) (*pb.HealthCheckRsp, error) {
	return &pb.HealthCheckRsp{Healthy: true, Version: s.version}, nil
}
