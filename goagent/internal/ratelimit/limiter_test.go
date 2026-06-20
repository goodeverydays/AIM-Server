package ratelimit

import (
	"context"
	"testing"

	"goagent/internal/cache"
)

// 普通用户超过每日额度后被拦截。
func TestLimiter_FreeUserBlockedAfterLimit(t *testing.T) {
	c := cache.NewMemoryCache()
	l := New(c, true, 2, 100, nil) // 普通2条/天
	ctx := context.Background()
	const uid = int32(1001)

	for i := 1; i <= 2; i++ {
		if r := l.Allow(ctx, uid); !r.Allowed {
			t.Fatalf("第 %d 次应放行，却被拦截 (used=%d limit=%d)", i, r.Used, r.Limit)
		}
	}
	if r := l.Allow(ctx, uid); r.Allowed {
		t.Fatalf("第 3 次应被拦截，却放行了 (used=%d limit=%d)", r.Used, r.Limit)
	}
}

// 升级 VIP 后，额度从普通提升到 VIP，被拦的用户重新可用。
func TestLimiter_UpgradeVipRaisesQuota(t *testing.T) {
	c := cache.NewMemoryCache()
	l := New(c, true, 2, 5, nil) // 普通2/天，VIP5/天
	ctx := context.Background()
	const uid = int32(2002)

	// 先用满普通额度并触发一次拦截（used 累到 3）
	l.Allow(ctx, uid)
	l.Allow(ctx, uid)
	if r := l.Allow(ctx, uid); r.Allowed {
		t.Fatalf("升级前第 3 次应被拦截 (used=%d limit=%d)", r.Used, r.Limit)
	}

	// 升级为 VIP
	if err := l.MarkVIP(ctx, uid); err != nil {
		t.Fatalf("MarkVIP 失败: %v", err)
	}
	if !l.IsVIP(ctx, uid) {
		t.Fatal("MarkVIP 后 IsVIP 应为 true")
	}

	// 现在 limit=5，已用 3，应继续放行到第 5 次
	r := l.Allow(ctx, uid)
	if !r.Allowed || r.Limit != 5 || !r.IsVIP {
		t.Fatalf("升级后应放行且 limit=5/IsVIP=true，实际 allowed=%v limit=%d isVIP=%v",
			r.Allowed, r.Limit, r.IsVIP)
	}
}

// 静态名单（环境变量）用户直接享 VIP 额度。
func TestLimiter_StaticVipList(t *testing.T) {
	c := cache.NewMemoryCache()
	l := New(c, true, 2, 100, map[int32]bool{3003: true})
	ctx := context.Background()

	r := l.Allow(ctx, 3003)
	if !r.IsVIP || r.Limit != 100 {
		t.Fatalf("静态名单用户应为 VIP/limit=100，实际 isVIP=%v limit=%d", r.IsVIP, r.Limit)
	}
}

// 关闭限流时一律放行。
func TestLimiter_Disabled(t *testing.T) {
	c := cache.NewMemoryCache()
	l := New(c, false, 1, 5, nil)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if r := l.Allow(ctx, 4004); !r.Allowed {
			t.Fatal("关闭限流时应一律放行")
		}
	}
}
