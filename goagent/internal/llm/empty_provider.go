package llm

import (
	"context"
	"fmt"
	"strings"

	"goagent/internal/cache"
)

// EmptyProvider 是一个LLM Provider的空实现
// 当LLM尚未配置时使用，返回友好的占位回复并记录对话历史
type EmptyProvider struct {
	cache cache.Cache
}

// NewEmptyProvider 创建空Provider
func NewEmptyProvider(c cache.Cache) *EmptyProvider {
	return &EmptyProvider{cache: c}
}

func (p *EmptyProvider) Name() string {
	return "empty"
}

func (p *EmptyProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	skill := NormalizeSkill(req.Skill)

	// 技能分支（summarize / suggest_reply 等）：展示分发已生效，
	// 接入真实 Provider 后将 BuildPrompt(req) 发给 LLM 即可。
	if skill != SkillChat {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("[Agent未配置] 已识别技能: %s\n", skill))
		if !IsKnownSkill(skill) {
			sb.WriteString("（该技能未注册，已回退为普通对话）\n")
		}
		sb.WriteString(fmt.Sprintf("收到上下文 %d 条。\n", len(req.Context)))
		sb.WriteString("\n--- 将发送给 LLM 的 prompt 预览 ---\n")
		sb.WriteString(BuildPrompt(req))
		sb.WriteString("--- 配置 llm.provider 后即可返回真实结果 ---")
		return &ChatResponse{Reply: sb.String(), Model: "none"}, nil
	}

	// 获取对话历史（多轮上下文）
	history, _ := p.cache.GetHistory(ctx, int64(req.UserID), 10)

	// 保存用户消息
	_ = p.cache.AppendHistory(ctx, int64(req.UserID),
		cache.Message{Role: "user", Content: req.Content, Time: 0}, 100)

	// 构建带有历史摘要的回复
	var sb strings.Builder
	sb.WriteString("[Agent未配置]\n")
	sb.WriteString(fmt.Sprintf("收到消息: \"%s\"\n", req.Content))

	if len(history) > 1 {
		sb.WriteString(fmt.Sprintf("\n对话历史 (%d条):\n", len(history)))
		for _, m := range history {
			role := "🧑"
			if m.Role == "assistant" {
				role = "🤖"
			}
			sb.WriteString(fmt.Sprintf("  %s %s\n", role, truncate(m.Content, 60)))
		}
	}

	sb.WriteString("\n当前LLM Provider未配置，请在配置中设置 llm.provider 以启用AI回复。\n")
	sb.WriteString("支持的Provider: openai, anthropic")

	reply := sb.String()

	// 保存 assistant 回复
	_ = p.cache.AppendHistory(ctx, int64(req.UserID),
		cache.Message{Role: "assistant", Content: reply, Time: 0}, 100)

	return &ChatResponse{Reply: reply, Model: "none"}, nil
}

func (p *EmptyProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return []ModelInfo{}, nil
}

func (p *EmptyProvider) IsHealthy(ctx context.Context) bool {
	if p.cache != nil {
		return p.cache.Ping(ctx) == nil
	}
	return false
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
