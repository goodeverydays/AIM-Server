package cache

import (
	"context"
	"encoding/json"
	"time"
)

// Message 对话消息
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
	Time    int64  `json:"time"`
}

// Cache 缓存抽象接口
// 提供多轮对话上下文、会话状态、限流计数等能力
type Cache interface {
	// ---- 多轮对话上下文 ----

	// GetHistory 获取用户的最近 N 条对话历史
	GetHistory(ctx context.Context, userID int64, limit int) ([]Message, error)

	// AppendHistory 追加一条对话记录（保留最近 limit 条）
	AppendHistory(ctx context.Context, userID int64, msg Message, maxLen int) error

	// ClearHistory 清除用户对话历史
	ClearHistory(ctx context.Context, userID int64) error

	// ---- 基础 KV ----

	// Get 读取 key
	Get(ctx context.Context, key string) (string, error)

	// Set 写入 key，可选 TTL
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Del 删除 key
	Del(ctx context.Context, key string) error

	// ---- 限流 ----

	// IncrWithTTL 原子递增并设置过期时间，返回递增后的值
	// 用于实现 API 限流计数器
	IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error)

	// ---- 健康检查 ----

	// Ping 检查缓存是否可用
	Ping(ctx context.Context) error

	// Name 返回缓存实现名称
	Name() string
}

// MarshalMessage 序列化消息
func MarshalMessage(msg Message) (string, error) {
	b, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalMessage 反序列化消息
func UnmarshalMessage(data string) (Message, error) {
	var msg Message
	err := json.Unmarshal([]byte(data), &msg)
	return msg, err
}
