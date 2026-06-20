package store

import (
	"testing"
	"time"
)

// 正确验证码应一次通过，且一次性消费（再验失败）
func TestVerifySuccessConsumesCode(t *testing.T) {
	s := NewCodeStore(5*time.Minute, time.Minute, 6)
	code, ok, _ := s.Generate("a@x.com")
	if !ok {
		t.Fatal("生成验证码失败")
	}
	if !s.Verify("a@x.com", code) {
		t.Fatal("正确验证码应通过")
	}
	if s.Verify("a@x.com", code) {
		t.Fatal("验证码应一次性消费，第二次须失败")
	}
}

// 连续错误达到上限后，验证码作废（即使之后输入正确也失败）
func TestVerifyAttemptsLimit(t *testing.T) {
	s := NewCodeStore(5*time.Minute, time.Minute, 6)
	code, _, _ := s.Generate("b@x.com")

	for i := 0; i < maxVerifyAttempts; i++ {
		if s.Verify("b@x.com", "000000xx-wrong") {
			t.Fatalf("第%d次错误验证码不应通过", i+1)
		}
	}
	// 达到失败上限后该码已被作废，正确码也无法通过
	if s.Verify("b@x.com", code) {
		t.Fatal("超过失败上限后，验证码应已作废")
	}
}

// 过期验证码应失败
func TestVerifyExpired(t *testing.T) {
	s := NewCodeStore(10*time.Millisecond, time.Minute, 6)
	code, _, _ := s.Generate("c@x.com")
	time.Sleep(20 * time.Millisecond)
	if s.Verify("c@x.com", code) {
		t.Fatal("过期验证码不应通过")
	}
}
