package llm

import "context"

// ============================================================
// ChatRequest / ChatResponse — LLM调用的通用数据结构
// ============================================================

// ChatRequest 发送给LLM的请求
type ChatRequest struct {
	UserID    int32    // 发送者ID
	TargetID  int32    // 目标ID
	Content   string   // 用户输入
	ChatType  int32    // 1=单聊, 2=群聊
	Model     string   // 指定模型名称，为空则使用默认模型
	Skill     string   // 技能标识：空/"chat"=普通对话，其余触发对应技能
	Context   []string // 技能所需上下文消息（如待总结的历史聊天）
}

// ChatResponse LLM返回的响应
type ChatResponse struct {
	Reply string // 回复内容
	Model string // 实际使用的模型名称
}

// ModelInfo 模型信息
type ModelInfo struct {
	Name      string
	Provider  string
	MaxTokens int32
}

// ============================================================
// Provider — LLM提供商的抽象接口
// 新增LLM接入只需实现此接口并注册即可
// ============================================================
type Provider interface {
	// Name 返回Provider名称，如 "openai", "anthropic", "empty"
	Name() string

	// Chat 发送消息到LLM并获取回复
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// ListModels 返回此Provider支持的模型列表
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// IsHealthy 检查Provider是否健康可用
	// 返回false表示服务不可用（密钥无效、网络不通等）
	IsHealthy(ctx context.Context) bool
}
