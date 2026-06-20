package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"
)

// maxVerifyAttempts 单个验证码允许的最大校验失败次数，超过即作废（防暴力穷举）
const maxVerifyAttempts = 5

// codeEntry 单个邮箱的验证码记录
type codeEntry struct {
	code     string
	expireAt time.Time
	lastSent time.Time
	attempts int // 已失败校验次数
}

// CodeStore 验证码内存存储，按邮箱地址索引
type CodeStore struct {
	mu     sync.Mutex
	codes  map[string]codeEntry
	ttl    time.Duration
	cool   time.Duration
	digits int
}

// NewCodeStore 创建验证码存储
func NewCodeStore(ttl, cooldown time.Duration, digits int) *CodeStore {
	return &CodeStore{
		codes:  make(map[string]codeEntry),
		ttl:    ttl,
		cool:   cooldown,
		digits: digits,
	}
}

// Generate 为邮箱生成新验证码
// 若距上次发送时间未超过冷却时间，返回 ok=false 及剩余冷却秒数
func (s *CodeStore) Generate(email string) (code string, ok bool, cooldownRemaining int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if entry, exists := s.codes[email]; exists {
		elapsed := now.Sub(entry.lastSent)
		if elapsed < s.cool {
			remaining := s.cool - elapsed
			return "", false, int(remaining.Seconds()) + 1
		}
	}

	code = randomDigits(s.digits)
	s.codes[email] = codeEntry{
		code:     code,
		expireAt: now.Add(s.ttl),
		lastSent: now,
	}
	return code, true, 0
}

// Verify 校验邮箱+验证码，成功后立即删除（一次性）
func (s *CodeStore) Verify(email, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.codes[email]
	if !exists {
		return false
	}
	if time.Now().After(entry.expireAt) {
		delete(s.codes, email)
		return false
	}
	if entry.code != code {
		// 失败计数，超过上限即作废该码，迫使用户重新获取（防暴力穷举6位码）
		entry.attempts++
		if entry.attempts >= maxVerifyAttempts {
			delete(s.codes, email)
		} else {
			s.codes[email] = entry // 回写更新后的失败次数（map 存的是值类型）
		}
		return false
	}
	delete(s.codes, email)
	return true
}

// cleanupExpired 删除所有已过期的验证码（必须在持有 s.mu 时调用）
func (s *CodeStore) cleanupExpired() int {
	now := time.Now()
	removed := 0
	for email, entry := range s.codes {
		if now.After(entry.expireAt) {
			delete(s.codes, email)
			removed++
		}
	}
	return removed
}

// StartCleanup 启动后台清理协程，按 interval 周期性回收过期验证码，
// 避免长期不验证的邮箱条目一直占用内存（Verify 的惰性删除只能清理被再次校验的邮箱）。
// 通过 ctx 控制生命周期：ctx 被取消时协程退出并停止 ticker，杜绝 goroutine 泄漏。
func (s *CodeStore) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("验证码清理协程退出")
				return
			case <-ticker.C:
				s.mu.Lock()
				n := s.cleanupExpired()
				s.mu.Unlock()
				if n > 0 {
					slog.Info("清理过期验证码", "count", n)
				}
			}
		}
	}()
}

// randomDigits 生成指定位数的纯数字验证码
func randomDigits(digits int) string {
	max := big.NewInt(1)
	for i := 0; i < digits; i++ {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		n = big.NewInt(0)
	}
	return fmt.Sprintf("%0*d", digits, n.Int64())
}
