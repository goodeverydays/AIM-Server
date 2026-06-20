package rag

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ============================================================
// Retriever — RAG 检索编排
// ------------------------------------------------------------
// 流程：EnsureIndexed(从 MySQL 拉该用户文本消息 → 对未索引的增量 embedding → 写入向量库)
//      Retrieve(embedding 查询语句 → 向量库 top-k → 格式化为可读片段)
// 作用域 = 请求用户(owner)涉及的全部单聊/群聊文本历史。
// ============================================================

type Retriever struct {
	db        *sql.DB
	emb       Embedder
	store     VectorStore
	topK      int
	maxCorpus int

	mu sync.Mutex // 串行化索引，避免并发重复 embedding
}

// NewRetriever 创建检索器。
func NewRetriever(db *sql.DB, emb Embedder, store VectorStore, topK, maxCorpus int) *Retriever {
	if topK <= 0 {
		topK = 8
	}
	if maxCorpus <= 0 {
		maxCorpus = 1000
	}
	return &Retriever{db: db, emb: emb, store: store, topK: topK, maxCorpus: maxCorpus}
}

const embedBatch = 32 // 单次 embedding 批量大小

// EnsureIndexed 确保 owner 的语料已索引(增量：只 embedding 向量库里还没有的消息)。
func (r *Retriever) EnsureIndexed(ctx context.Context, owner int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	msgs, err := loadUserTextMessages(ctx, r.db, owner, r.maxCorpus)
	if err != nil {
		return err
	}

	// 收集内容非空的候选，批量问向量库哪些已索引(远程后端一次往返)
	candIDs := make([]int64, 0, len(msgs))
	msgByID := make(map[int64]dbMessage, len(msgs))
	for _, m := range msgs {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		candIDs = append(candIDs, m.ID)
		msgByID[m.ID] = m
	}
	if len(candIDs) == 0 {
		return nil
	}
	existing, err := r.store.HasIDs(ctx, owner, candIDs)
	if err != nil {
		return err
	}
	var pending []dbMessage
	for _, id := range candIDs {
		if !existing[id] {
			pending = append(pending, msgByID[id])
		}
	}
	if len(pending) == 0 {
		return nil
	}

	// 分批 embedding 并写入
	for start := 0; start < len(pending); start += embedBatch {
		end := start + embedBatch
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[start:end]
		texts := make([]string, len(batch))
		for i, m := range batch {
			texts[i] = m.Content
		}
		vecs, err := r.emb.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("索引 embedding 失败: %w", err)
		}
		items := make([]Item, 0, len(batch))
		for i, m := range batch {
			if i >= len(vecs) || len(vecs[i]) == 0 {
				continue
			}
			items = append(items, Item{
				ID:   m.ID,
				Text: m.Content,
				Vec:  vecs[i],
				Ts:   m.Ts,
				Mine: m.SenderID == owner,
			})
		}
		if err := r.store.Upsert(ctx, owner, items); err != nil {
			return err
		}
	}
	return nil
}

// Retrieve 检索与 query 最相关的片段，返回用于喂给 LLM 的文本行(按时间升序)。
func (r *Retriever) Retrieve(ctx context.Context, owner int32, query string) ([]string, error) {
	if err := r.EnsureIndexed(ctx, owner); err != nil {
		return nil, err
	}

	qv, err := r.emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("查询 embedding 失败: %w", err)
	}
	if len(qv) == 0 || len(qv[0]) == 0 {
		return nil, fmt.Errorf("查询向量为空")
	}

	hits, err := r.store.TopK(ctx, owner, qv[0], r.topK)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// 命中结果按时间升序排列，更符合阅读/时间线习惯
	sortByTime(hits)

	lines := make([]string, 0, len(hits))
	for _, h := range hits {
		who := "对方"
		if h.Mine {
			who = "我"
		}
		when := time.Unix(h.Ts, 0).Format("2006-01-02 15:04")
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", when, who, h.Text))
	}
	return lines, nil
}

func sortByTime(items []Item) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j-1].Ts > items[j].Ts; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}
