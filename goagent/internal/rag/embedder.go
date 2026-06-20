package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// Embedder — 文本向量化抽象
// ------------------------------------------------------------
// 只依赖标准库，走 OpenAI 兼容的 /embeddings 接口，按 BaseURL+Model 切换厂商：
//   · SiliconFlow(硅基流动) https://api.siliconflow.cn/v1  model=BAAI/bge-m3   (国内可直连、免费额度，推荐)
//   · Ollama(本地离线)      http://localhost:11434/v1     model=bge-m3 / nomic-embed-text
//   · OpenAI               https://api.openai.com/v1     model=text-embedding-3-small
// DeepSeek 不提供 embeddings 接口，故 embedding 单独配置，与对话 LLM 解耦。
// ============================================================

// Embedder 把文本批量转成向量。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// OpenAIEmbedder 调用 OpenAI 兼容 /embeddings 接口。
type OpenAIEmbedder struct {
	apiKey  string
	baseURL string // 形如 https://api.siliconflow.cn/v1，不带末尾斜杠
	model   string
	client  *http.Client
}

// NewOpenAIEmbedder 创建 embedding 客户端。
func NewOpenAIEmbedder(apiKey, baseURL, model string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedRsp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed 批量向量化。返回顺序与输入一致。
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if e.apiKey == "" {
		return nil, fmt.Errorf("embedding APIKey 未配置")
	}

	body, _ := json.Marshal(embedReq{Model: e.model, Input: texts})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造 embedding 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用 embedding 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var parsed embedRsp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析 embedding 响应失败(status=%d): %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("embedding 返回错误: %s", parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("embedding 异常(status=%d, got=%d want=%d): %s",
			resp.StatusCode, len(parsed.Data), len(texts), truncate(string(raw), 200))
	}

	// data 可能乱序，按 index 还原
	out := make([][]float32, len(texts))
	for _, d := range parsed.Data {
		if d.Index >= 0 && d.Index < len(out) {
			out[d.Index] = d.Embedding
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
