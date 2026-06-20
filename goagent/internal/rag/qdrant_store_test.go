package rag

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestQdrantStoreSmoke 用真实 Qdrant 验证 QdrantStore 的 REST 契约。
// Qdrant 不可达时自动跳过(不影响普通 CI)。用法：
//   docker compose up -d qdrant && go test ./internal/rag -run TestQdrant -v
func TestQdrantStoreSmoke(t *testing.T) {
	url := os.Getenv("QDRANT_URL")
	if url == "" {
		url = "http://localhost:6333"
	}
	hc := &http.Client{Timeout: 2 * time.Second}
	if resp, err := hc.Get(url + "/healthz"); err != nil {
		t.Skipf("Qdrant 不可达(%s)，跳过: %v", url, err)
	} else {
		resp.Body.Close()
	}

	ctx := context.Background()
	const coll = "im_rag_smoketest"
	store := NewQdrantStore(url, "", coll)
	defer store.do(ctx, http.MethodDelete, "/collections/"+coll, nil) // 清理

	owner := int32(1001)
	items := []Item{
		{ID: 1, Text: "明天上午十点开会讨论RAG方案", Vec: []float32{1, 0, 0, 0}, Ts: 1000, Mine: true},
		{ID: 2, Text: "周末一起去爬山吧", Vec: []float32{0, 1, 0, 0}, Ts: 2000, Mine: false},
	}
	if err := store.Upsert(ctx, owner, items); err != nil {
		t.Fatalf("Upsert 失败: %v", err)
	}

	// 增量去重：1、2 已存在，3 不存在
	has, err := store.HasIDs(ctx, owner, []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("HasIDs 失败: %v", err)
	}
	if !has[1] || !has[2] || has[3] {
		t.Fatalf("HasIDs 结果不对: %v", has)
	}

	// 检索：靠近 item1 的查询应命中 item1
	hits, err := store.TopK(ctx, owner, []float32{0.9, 0.1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("TopK 失败: %v", err)
	}
	if len(hits) == 0 || hits[0].ID != 1 {
		t.Fatalf("TopK 期望命中 ID=1，实际: %+v", hits)
	}
	if hits[0].Text == "" || hits[0].Mine != true {
		t.Fatalf("payload 回读不对: %+v", hits[0])
	}

	// owner 隔离：换一个 owner 应检索不到
	other, err := store.TopK(ctx, int32(2002), []float32{0.9, 0.1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("TopK(other) 失败: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("owner 隔离失效，越权检索到: %+v", other)
	}

	t.Logf("OK has=%v topID=%d topText=%q", has, hits[0].ID, hits[0].Text)
}
