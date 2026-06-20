package service

import (
	"context"
	"fmt"

	"goagent/internal/cache"
	"goagent/internal/llm"
	"goagent/internal/rag"
	"goagent/internal/ratelimit"
	"goagent/pkg/pb"
)

// AgentService 实现 pb.AgentServiceServer 接口
// 持有 LLM Provider 和 Cache，将gRPC请求转换为Provider调用
type AgentService struct {
	pb.UnimplementedAgentServiceServer

	provider  llm.Provider
	cache     cache.Cache
	limiter   *ratelimit.Limiter
	retriever *rag.Retriever // RAG 检索器，nil 表示未启用
	version   string
}

// NewAgentService 创建AgentService实例。retriever 为 nil 时 rag_qa 技能返回"未启用"。
func NewAgentService(provider llm.Provider, version string, c cache.Cache, limiter *ratelimit.Limiter, retriever *rag.Retriever) *AgentService {
	return &AgentService{
		provider:  provider,
		cache:     c,
		limiter:   limiter,
		retriever: retriever,
		version:   version,
	}
}

// ProcessMessage 处理用户消息
func (s *AgentService) ProcessMessage(
	ctx context.Context,
	req *pb.ProcessMessageReq,
) (*pb.ProcessMessageRsp, error) {
	// 输入校验：普通对话需要 content；总结/建议等技能可仅凭 context 触发。
	if req.Content == "" && len(req.Context) == 0 {
		return &pb.ProcessMessageRsp{
			Code: 1,
			Msg:  "消息内容不能为空",
		}, nil
	}

	// 配额限流：普通用户每日 N 条，VIP 更高额度
	if s.limiter != nil {
		r := s.limiter.Allow(ctx, req.UserId)
		if !r.Allowed {
			tier := "普通用户"
			if r.IsVIP {
				tier = "VIP用户"
			}
			return &pb.ProcessMessageRsp{
				Code: 4,
				Msg: fmt.Sprintf("今日 AI 额度已用完（%s每日 %d 条）。%s",
					tier, r.Limit,
					vipHint(r.IsVIP)),
			}, nil
		}
	}

	// 构建ChatRequest（透传技能与上下文，技能分发在 llm 层完成）
	chatReq := &llm.ChatRequest{
		UserID:   req.UserId,
		TargetID: req.TargetId,
		Content:  req.Content,
		ChatType: req.ChatType,
		Model:    req.Model,
		Skill:    req.Skill,
		Context:  req.Context,
	}

	// RAG 技能：先从该用户的历史聊天记录里向量检索相关片段，作为上下文喂给 LLM。
	if llm.NormalizeSkill(req.Skill) == llm.SkillRagQa {
		if s.retriever == nil {
			return &pb.ProcessMessageRsp{
				Code: 3,
				Msg:  "记忆问答未启用（服务端未配置 RAG）",
			}, nil
		}
		snippets, err := s.retriever.Retrieve(ctx, req.UserId, req.Content)
		if err != nil {
			return &pb.ProcessMessageRsp{
				Code: 2,
				Msg:  fmt.Sprintf("检索聊天记录失败: %s", err.Error()),
			}, nil
		}
		if len(snippets) == 0 {
			return &pb.ProcessMessageRsp{
				Code:  0,
				Msg:   "成功",
				Reply: "在你的聊天记录里没找到相关信息。",
			}, nil
		}
		chatReq.Context = snippets // 检索片段覆盖为上下文，交给 rag_qa 系统提示词处理
	}

	// 调用LLM Provider
	chatResp, err := s.provider.Chat(ctx, chatReq)
	if err != nil {
		return &pb.ProcessMessageRsp{
			Code: 2,
			Msg:  fmt.Sprintf("LLM调用失败: %s", err.Error()),
		}, nil
	}

	return &pb.ProcessMessageRsp{
		Code:  0,
		Msg:   "成功",
		Reply: chatResp.Reply,
		Model: chatResp.Model,
	}, nil
}

// UpgradeVip 升级 VIP：校验支付后将用户标记为 VIP。
// 当前为模拟支付——校验金额与支付令牌非空即视为支付成功；
// 接入真实支付网关时，只需把下面"模拟支付校验"替换为向网关核验 payment_token。
func (s *AgentService) UpgradeVip(
	ctx context.Context,
	req *pb.UpgradeVipReq,
) (*pb.VipStatusRsp, error) {
	if req.UserId <= 0 {
		return &pb.VipStatusRsp{Code: 1, Msg: "无效用户"}, nil
	}
	if s.limiter == nil {
		return &pb.VipStatusRsp{Code: 2, Msg: "限流/会员服务不可用"}, nil
	}

	// —— 模拟支付校验（真实支付在此对接网关核验）——
	const vipPriceCents = 500 // $5
	if req.PaymentToken == "" {
		return &pb.VipStatusRsp{Code: 3, Msg: "支付凭证缺失"}, nil
	}
	if req.AmountCents < vipPriceCents {
		return &pb.VipStatusRsp{Code: 4, Msg: "支付金额不足（需 $5）"}, nil
	}

	// 已是 VIP 则幂等返回
	if s.limiter.IsVIP(ctx, req.UserId) {
		return &pb.VipStatusRsp{Code: 0, Msg: "您已是 VIP", IsVip: true}, nil
	}

	if err := s.limiter.MarkVIP(ctx, req.UserId); err != nil {
		return &pb.VipStatusRsp{Code: 5, Msg: "升级失败，请稍后重试"}, nil
	}
	return &pb.VipStatusRsp{Code: 0, Msg: "升级成功，已享 VIP 配额", IsVip: true}, nil
}

// GetVipStatus 查询用户 VIP 状态。
func (s *AgentService) GetVipStatus(
	ctx context.Context,
	req *pb.VipStatusReq,
) (*pb.VipStatusRsp, error) {
	if s.limiter == nil {
		return &pb.VipStatusRsp{Code: 0, IsVip: false}, nil
	}
	return &pb.VipStatusRsp{Code: 0, IsVip: s.limiter.IsVIP(ctx, req.UserId)}, nil
}

// vipHint 给普通用户一句升级提示。
func vipHint(isVIP bool) string {
	if isVIP {
		return "请明日再试。"
	}
	return "升级 VIP 可享更高额度，或明日再试。"
}

// HealthCheck 健康检查
func (s *AgentService) HealthCheck(
	ctx context.Context,
	req *pb.HealthCheckReq,
) (*pb.HealthCheckRsp, error) {
	healthy := true
	llmStatus := "active"

	if !s.provider.IsHealthy(ctx) {
		// 服务自身正常但LLM不可用，整体仍返回healthy=false
		// 因为Agent服务的核心价值在于LLM回复
		healthy = false
		llmStatus = "error"
	}

	return &pb.HealthCheckRsp{
		Healthy:   healthy,
		Version:   s.version,
		LlmStatus: llmStatus,
	}, nil
}

// ListModels 获取模型列表
func (s *AgentService) ListModels(
	ctx context.Context,
	req *pb.ListModelsReq,
) (*pb.ListModelsRsp, error) {
	models, err := s.provider.ListModels(ctx)
	if err != nil {
		return &pb.ListModelsRsp{}, nil
	}

	pbModels := make([]*pb.ListModelsRsp_ModelInfo, 0, len(models))
	for _, m := range models {
		pbModels = append(pbModels, &pb.ListModelsRsp_ModelInfo{
			Name:      m.Name,
			Provider:  m.Provider,
			MaxTokens: m.MaxTokens,
		})
	}

	return &pb.ListModelsRsp{
		Models: pbModels,
	}, nil
}
