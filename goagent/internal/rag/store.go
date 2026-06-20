package rag

import (
	"context"
	"math"
	"sort"
	"sync"
)

// ============================================================
// VectorStore — 向量存储/检索抽象
// ------------------------------------------------------------
// 两个实现：
//   · MemoryStore  进程内、暴力余弦，零外部依赖(默认/回退)
//   · QdrantStore  外部向量数据库 Qdrant(REST)，HNSW + payload 过滤(生产)
// 接口按批量设计：HasIDs 一次往返判重，避免远程后端逐条 RTT。
// ============================================================

// Item 一条可检索的记录(对应一条聊天消息)。
type Item struct {
	ID   int64     // 消息主键 f_id，用于增量去重
	Text string    // 文本内容
	Vec  []float32 // 向量
	Ts   int64     // 消息时间(秒)
	Mine bool      // 是否为 owner 本人发出
}

// VectorStore 向量存取接口。
type VectorStore interface {
	// HasIDs 返回候选 id 中【已索引】的集合(批量，便于远程后端一次往返)。
	HasIDs(ctx context.Context, owner int32, ids []int64) (map[int64]bool, error)
	// Upsert 批量写入/更新。
	Upsert(ctx context.Context, owner int32, items []Item) error
	// TopK 余弦 top-k 检索(限定 owner 作用域)。
	TopK(ctx context.Context, owner int32, query []float32, k int) ([]Item, error)
}

// ============================================================
// MemoryStore — 进程内向量库
// ============================================================
type MemoryStore struct {
	mu   sync.RWMutex
	data map[int32]map[int64]Item // owner -> (msgID -> item)
}

// NewMemoryStore 创建内存向量库。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[int32]map[int64]Item)}
}

func (s *MemoryStore) HasIDs(_ context.Context, owner int32, ids []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(ids))
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket, ok := s.data[owner]
	if !ok {
		return out, nil
	}
	for _, id := range ids {
		if _, exists := bucket[id]; exists {
			out[id] = true
		}
	}
	return out, nil
}

func (s *MemoryStore) Upsert(_ context.Context, owner int32, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.data[owner]
	if !ok {
		bucket = make(map[int64]Item)
		s.data[owner] = bucket
	}
	for _, it := range items {
		bucket[it.ID] = it
	}
	return nil
}

func (s *MemoryStore) TopK(_ context.Context, owner int32, query []float32, k int) ([]Item, error) {
	s.mu.RLock()
	bucket, ok := s.data[owner]
	if !ok || len(bucket) == 0 {
		s.mu.RUnlock()
		return nil, nil
	}
	type scored struct {
		item  Item
		score float64
	}
	list := make([]scored, 0, len(bucket))
	for _, it := range bucket {
		list = append(list, scored{item: it, score: cosine(query, it.Vec)})
	}
	s.mu.RUnlock()

	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	if k > len(list) {
		k = len(list)
	}
	out := make([]Item, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, list[i].item)
	}
	return out, nil
}

// cosine 计算两个向量的余弦相似度；长度不一致或零向量返回 0。
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
