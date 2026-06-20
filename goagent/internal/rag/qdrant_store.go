package rag

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================
// QdrantStore — 基于 Qdrant 向量数据库的 VectorStore 实现
// ------------------------------------------------------------
// 走 Qdrant REST API(仅标准库 net/http，零额外 Go 依赖)。
// 单集合 + payload 字段 owner 做作用域过滤(标准 Qdrant 用法，演示元数据过滤)。
// 点 id 由 (owner, msgID) 生成确定性 UUID：同一条消息在不同 owner 下是不同点，
// 既能多人各自检索到，又能按点 id 增量去重。
// ============================================================
type QdrantStore struct {
	baseURL    string
	apiKey     string
	collection string
	client     *http.Client

	mu      sync.Mutex
	ensured bool // collection 是否已确保存在
}

// NewQdrantStore 创建 Qdrant 向量库客户端。
func NewQdrantStore(baseURL, apiKey, collection string) *QdrantStore {
	return &QdrantStore{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		collection: collection,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// pointID 由 (owner,msgID) 生成确定性 UUID 字符串。
func pointID(owner int32, msgID int64) string {
	sum := md5.Sum([]byte(fmt.Sprintf("%d:%d", owner, msgID)))
	h := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// do 发起一次 REST 请求，返回响应体与状态码。
func (q *QdrantStore) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("访问 Qdrant 失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

// ensureCollection 确保集合存在(不存在则按给定维度+余弦距离创建)。
func (q *QdrantStore) ensureCollection(ctx context.Context, dim int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ensured {
		return nil
	}
	_, code, err := q.do(ctx, http.MethodGet, "/collections/"+q.collection, nil)
	if err != nil {
		return err
	}
	if code == http.StatusOK {
		q.ensured = true
		return nil
	}
	// 创建集合
	create := map[string]interface{}{
		"vectors": map[string]interface{}{"size": dim, "distance": "Cosine"},
	}
	raw, code, err := q.do(ctx, http.MethodPut, "/collections/"+q.collection, create)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("创建 Qdrant 集合失败(%d): %s", code, truncate(string(raw), 200))
	}
	q.ensured = true
	return nil
}

func (q *QdrantStore) HasIDs(ctx context.Context, owner int32, ids []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	uuidToID := make(map[string]int64, len(ids))
	pids := make([]string, 0, len(ids))
	for _, id := range ids {
		u := pointID(owner, id)
		uuidToID[u] = id
		pids = append(pids, u)
	}
	body := map[string]interface{}{"ids": pids, "with_payload": false, "with_vector": false}
	raw, code, err := q.do(ctx, http.MethodPost, "/collections/"+q.collection+"/points", body)
	if err != nil {
		return out, err
	}
	if code == http.StatusNotFound {
		return out, nil // 集合还不存在 = 都未索引
	}
	if code/100 != 2 {
		return out, fmt.Errorf("Qdrant 取点失败(%d): %s", code, truncate(string(raw), 200))
	}
	var resp struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return out, fmt.Errorf("解析 Qdrant 响应失败: %w", err)
	}
	for _, p := range resp.Result {
		if id, ok := uuidToID[p.ID]; ok {
			out[id] = true
		}
	}
	return out, nil
}

func (q *QdrantStore) Upsert(ctx context.Context, owner int32, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	if err := q.ensureCollection(ctx, len(items[0].Vec)); err != nil {
		return err
	}
	type point struct {
		ID      string                 `json:"id"`
		Vector  []float32              `json:"vector"`
		Payload map[string]interface{} `json:"payload"`
	}
	pts := make([]point, 0, len(items))
	for _, it := range items {
		pts = append(pts, point{
			ID:     pointID(owner, it.ID),
			Vector: it.Vec,
			Payload: map[string]interface{}{
				"owner": owner,
				"msgid": it.ID,
				"text":  it.Text,
				"ts":    it.Ts,
				"mine":  it.Mine,
			},
		})
	}
	body := map[string]interface{}{"points": pts}
	raw, code, err := q.do(ctx, http.MethodPut, "/collections/"+q.collection+"/points?wait=true", body)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("Qdrant 写入失败(%d): %s", code, truncate(string(raw), 200))
	}
	return nil
}

func (q *QdrantStore) TopK(ctx context.Context, owner int32, query []float32, k int) ([]Item, error) {
	body := map[string]interface{}{
		"vector":       query,
		"limit":        k,
		"with_payload": true,
		"filter": map[string]interface{}{
			"must": []interface{}{
				map[string]interface{}{
					"key":   "owner",
					"match": map[string]interface{}{"value": owner},
				},
			},
		},
	}
	raw, code, err := q.do(ctx, http.MethodPost, "/collections/"+q.collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, nil
	}
	if code/100 != 2 {
		return nil, fmt.Errorf("Qdrant 检索失败(%d): %s", code, truncate(string(raw), 200))
	}
	var resp struct {
		Result []struct {
			Score   float64 `json:"score"`
			Payload struct {
				MsgID int64  `json:"msgid"`
				Text  string `json:"text"`
				Ts    int64  `json:"ts"`
				Mine  bool   `json:"mine"`
			} `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析 Qdrant 检索结果失败: %w", err)
	}
	items := make([]Item, 0, len(resp.Result))
	for _, r := range resp.Result {
		items = append(items, Item{
			ID:   r.Payload.MsgID,
			Text: r.Payload.Text,
			Ts:   r.Payload.Ts,
			Mine: r.Payload.Mine,
		})
	}
	return items, nil
}
