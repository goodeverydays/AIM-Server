package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MemoryCache 内存缓存（Redis 不可用时的降级实现）
// 使用 sync.Map 存储，进程重启后数据丢失
type MemoryCache struct {
	mu       sync.RWMutex
	store    map[string]*memEntry
	done     chan struct{}
}

type memEntry struct {
	value  string
	expire time.Time // 零值表示永不过期
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache() *MemoryCache {
	m := &MemoryCache{
		store: make(map[string]*memEntry),
		done:  make(chan struct{}),
	}
	// 每秒清理过期 key
	go m.cleanExpired()
	return m
}

func (m *MemoryCache) cleanExpired() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for k, v := range m.store {
				if !v.expire.IsZero() && v.expire.Before(now) {
					delete(m.store, k)
				}
			}
			m.mu.Unlock()
		case <-m.done:
			return
		}
	}
}

func (m *MemoryCache) Name() string {
	return "memory"
}

// ---- 多轮对话上下文 ----
// Memory 实现使用 list key 模拟 Redis LIST

func (m *MemoryCache) GetHistory(ctx context.Context, userID int64, limit int) ([]Message, error) {
	key := fmt.Sprintf("chat:history:%d", userID)
	m.mu.RLock()
	entry, ok := m.store[key]
	m.mu.RUnlock()
	if !ok {
		return nil, nil
	}

	// JSON array of messages
	msgs := make([]Message, 0)
	_ = UnmarshalList(entry.value, &msgs)
	if len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

func (m *MemoryCache) AppendHistory(ctx context.Context, userID int64, msg Message, maxLen int) error {
	msgs, _ := m.GetHistory(ctx, userID, maxLen)
	msgs = append(msgs, msg)
	if len(msgs) > maxLen {
		msgs = msgs[len(msgs)-maxLen:]
	}
	data, _ := MarshalList(msgs)

	key := fmt.Sprintf("chat:history:%d", userID)
	m.mu.Lock()
	m.store[key] = &memEntry{value: data}
	m.mu.Unlock()
	return nil
}

func (m *MemoryCache) ClearHistory(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("chat:history:%d", userID)
	m.mu.Lock()
	delete(m.store, key)
	m.mu.Unlock()
	return nil
}

// ---- 基础 KV ----

func (m *MemoryCache) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	entry, ok := m.store[key]
	m.mu.RUnlock()
	if !ok {
		return "", nil
	}
	if !entry.expire.IsZero() && entry.expire.Before(time.Now()) {
		m.mu.Lock()
		delete(m.store, key)
		m.mu.Unlock()
		return "", nil
	}
	return entry.value, nil
}

func (m *MemoryCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	m.mu.Lock()
	m.store[key] = &memEntry{
		value:  value,
		expire: time.Now().Add(ttl),
	}
	m.mu.Unlock()
	return nil
}

func (m *MemoryCache) Del(ctx context.Context, key string) error {
	m.mu.Lock()
	delete(m.store, key)
	m.mu.Unlock()
	return nil
}

// ---- 限流 ----

func (m *MemoryCache) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.store[key]
	if !ok || (!entry.expire.IsZero() && entry.expire.Before(time.Now())) {
		m.store[key] = &memEntry{value: "1", expire: time.Now().Add(ttl)}
		return 1, nil
	}
	var count int64
	fmt.Sscanf(entry.value, "%d", &count)
	count++
	entry.value = fmt.Sprintf("%d", count)
	entry.expire = time.Now().Add(ttl)
	return count, nil
}

// ---- 健康检查 ----

func (m *MemoryCache) Ping(ctx context.Context) error {
	return nil // 内存缓存始终可用
}

// ---- 序列化辅助 ----

func MarshalList(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func UnmarshalList(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}
