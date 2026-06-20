package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"goagent/internal/cache"
)

// ============================================================
// OpenAIProvider — OpenAI 兼容 Provider
// ------------------------------------------------------------
// 走标准 /chat/completions 接口，只依赖标准库 net/http。
// 通过配置 BaseURL + Model + APIKey，可对接：
//   · OpenAI            (https://api.openai.com/v1)
//   · DeepSeek          (https://api.deepseek.com/v1)
//   · Kimi/Moonshot     (https://api.moonshot.cn/v1)
//   · 智谱/通义 等       (各自的 OpenAI 兼容地址)
// 即"一份实现，按配置切换厂商"。
// ============================================================

// OpenAIProvider 实现 Provider 接口。
type OpenAIProvider struct {
	apiKey  string
	baseURL string // 形如 https://api.openai.com/v1，不带末尾斜杠
	model   string
	client  *http.Client
	cache   cache.Cache
}

// NewOpenAIProvider 创建 OpenAI 兼容 Provider。
func NewOpenAIProvider(apiKey, baseURL, model string, c cache.Cache) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
		cache:   c,
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

// ---- /chat/completions 请求/响应结构 ----

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"` // 例如 "json_object"，强制模型只输出 JSON
}

type chatCompletionReq struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatCompletionRsp struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("OpenAI APIKey 未配置")
	}

	skill := NormalizeSkill(req.Skill)

	// 组装 messages：system 提示词 + （对话/社交意图才带历史）+ 当前 user 内容
	messages := []chatMessage{{Role: "system", Content: SystemPrompt(skill)}}

	// 普通对话与社交意图都带最近多轮历史（社交意图需历史以支持"加上面提到的人"等指代）
	withHistory := (skill == SkillChat || skill == SkillSocialAction)
	if withHistory && p.cache != nil {
		history, _ := p.cache.GetHistory(ctx, int64(req.UserID), 10)
		for _, m := range history {
			messages = append(messages, chatMessage{Role: m.Role, Content: m.Content})
		}
	}

	userContent := BuildUserContent(req)
	messages = append(messages, chatMessage{Role: "user", Content: userContent})

	// 社交意图技能强制 JSON 输出，提升结构化可靠性（DeepSeek/OpenAI 兼容）
	reqBody := chatCompletionReq{Model: p.model, Messages: messages}
	if skill == SkillSocialAction {
		reqBody.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	// 发请求
	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用 LLM 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var parsed chatCompletionRsp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析响应失败(status=%d): %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("LLM 返回错误: %s", parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("LLM 异常(status=%d): %s", resp.StatusCode, truncate(string(raw), 200))
	}

	reply := parsed.Choices[0].Message.Content
	usedModel := parsed.Model
	if usedModel == "" {
		usedModel = p.model
	}

	// 普通对话与社交意图写入多轮历史（assistant 端社交意图存的是 JSON，模型可自解析）；
	// summarize/suggest_reply 等一次性技能不写入，避免污染对话上下文
	if withHistory && p.cache != nil {
		_ = p.cache.AppendHistory(ctx, int64(req.UserID),
			cache.Message{Role: "user", Content: req.Content, Time: time.Now().Unix()}, 100)
		_ = p.cache.AppendHistory(ctx, int64(req.UserID),
			cache.Message{Role: "assistant", Content: reply, Time: time.Now().Unix()}, 100)
	}

	return &ChatResponse{Reply: reply, Model: usedModel}, nil
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	// 简单返回当前配置的模型；如需动态列举可调 /models 接口。
	return []ModelInfo{{Name: p.model, Provider: "openai", MaxTokens: 0}}, nil
}

func (p *OpenAIProvider) IsHealthy(ctx context.Context) bool {
	// 配置了密钥即视为可用；真实探活可发一次极小请求，但会产生费用，这里从简。
	return p.apiKey != ""
}
