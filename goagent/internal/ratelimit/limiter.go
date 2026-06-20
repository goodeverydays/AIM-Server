package ratelimit

import (
	"context"
	"fmt"
	"time"

	"goagent/internal/cache"
)

// ============================================================
// Limiter — AI 用量按日配额限流
// ------------------------------------------------------------
// 基于 cache.IncrWithTTL 的原子计数实现：
//   key = "ratelimit:agent:{userID}:{YYYYMMDD}"
// 每个用户每天一个 key，首次自增时设置 TTL，次日自动用新 key。
// 普通用户 / VIP 用户用不同上限（VIP 仅是更高的 limit，无独立逻辑）。
// ============================================================

// Result 一次配额检查的结果。
type Result struct {
	Allowed bool // 是否放行
	Used    int  // 今日已用（含本次；-1 表示缓存不可用未计数）
	Limit   int  // 今日上限（-1 表示未启用限流）
	IsVIP   bool // 是否 VIP 用户
}

// Limiter 配额限流器。
type Limiter struct {
	cache     cache.Cache
	enabled   bool
	freeDaily int
	vipDaily  int
	vipUsers  map[int32]bool
}

// New 创建限流器。
func New(c cache.Cache, enabled bool, freeDaily, vipDaily int, vipUsers map[int32]bool) *Limiter {
	if vipUsers == nil {
		vipUsers = map[int32]bool{}
	}
	return &Limiter{
		cache:     c,
		enabled:   enabled,
		freeDaily: freeDaily,
		vipDaily:  vipDaily,
		vipUsers:  vipUsers,
	}
}

// vipTTL：VIP 标记有效期。memory 缓存 ttl=0 会被当作立即过期，
// 故用一个很长的 TTL（10年，对 demo 等同永久）；Redis 同样生效。
const vipTTL = 3650 * 24 * time.Hour

func vipKey(userID int32) string {
	return fmt.Sprintf("vip:%d", userID)
}

// MarkVIP 将用户标记为 VIP（写入缓存，供后续配额判定）。
func (l *Limiter) MarkVIP(ctx context.Context, userID int32) error {
	return l.cache.Set(ctx, vipKey(userID), "1", vipTTL)
}

// IsVIP 判断是否 VIP：静态名单（环境变量）或缓存标记（动态升级）。
func (l *Limiter) IsVIP(ctx context.Context, userID int32) bool {
	if l.vipUsers[userID] {
		return true
	}
	v, err := l.cache.Get(ctx, vipKey(userID))
	return err == nil && v == "1"
}

// Allow 进行一次配额检查并计数（原子自增，先占用再判断，并发安全）。
// 注意：失败的 LLM 调用也会消耗一次配额（按"尝试"计数）；
// 这是为并发安全做的权衡——若要按"成功"计费需改用补偿/回退逻辑。
func (l *Limiter) Allow(ctx context.Context, userID int32) Result {
	if !l.enabled {
		return Result{Allowed: true, Limit: -1, Used: -1}
	}

	isVIP := l.IsVIP(ctx, userID)
	limit := l.freeDaily
	if isVIP {
		limit = l.vipDaily
	}

	key := fmt.Sprintf("ratelimit:agent:%d:%s", userID, time.Now().Format("20060102"))
	// TTL 取 48h，确保按日 key 用完后能被清理；当日计数正确性由日期 key 保证。
	n, err := l.cache.IncrWithTTL(ctx, key, 48*time.Hour)
	if err != nil {
		// 缓存不可用时放行（fail open），避免限流组件故障拖垮主功能。
		return Result{Allowed: true, Limit: limit, Used: -1, IsVIP: isVIP}
	}

	return Result{
		Allowed: n <= int64(limit),
		Used:    int(n),
		Limit:   limit,
		IsVIP:   isVIP,
	}
}
