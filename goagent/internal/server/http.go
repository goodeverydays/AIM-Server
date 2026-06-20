package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"goagent/internal/llm"
	"goagent/internal/middleware"
	"goagent/internal/ratelimit"
)

// HTTPServer HTTP REST 服务器，为 Qt 客户端提供轻量调用方式
type HTTPServer struct {
	svc     *httpService
	limiter *ratelimit.Limiter
	log     middleware.Logger
	address string
}

// httpService 封装 Agent 业务逻辑供 HTTP 使用
type httpService struct {
	provider llm.Provider
	version  string
}

// chatRequest HTTP 请求
type chatRequest struct {
	UserID   int32    `json:"user_id"`
	TargetID int32    `json:"target_id"`
	Content  string   `json:"content"`
	ChatType int32    `json:"chat_type"`
	Skill    string   `json:"skill"`   // 技能标识，空=普通对话
	Context  []string `json:"context"` // 技能上下文（如待总结的聊天记录）
}

// chatResponse HTTP 响应
type chatResponse struct {
	Code   int32  `json:"code"`
	Msg    string `json:"msg"`
	Reply  string `json:"reply"`
	Model  string `json:"model"`
}

// healthResponse 健康检查响应
type healthResponse struct {
	Healthy   bool   `json:"healthy"`
	Version   string `json:"version"`
	LlmStatus string `json:"llm_status"`
}

// vipRequest VIP 升级请求
type vipRequest struct {
	UserID       int32  `json:"user_id"`
	PaymentToken string `json:"payment_token"`
	AmountCents  int32  `json:"amount_cents"`
}

// vipResponse VIP 状态响应
type vipResponse struct {
	Code  int32  `json:"code"`
	Msg   string `json:"msg"`
	IsVip bool   `json:"is_vip"`
}

// NewHTTPServer 创建 HTTP 服务器
func NewHTTPServer(address string, provider llm.Provider, version string, limiter *ratelimit.Limiter, log middleware.Logger) *HTTPServer {
	return &HTTPServer{
		svc: &httpService{
			provider: provider,
			version:  version,
		},
		limiter: limiter,
		log:     log,
		address: address,
	}
}

// Start 启动 HTTP 服务器（在 goroutine 中运行）
func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/vip/upgrade", s.handleVipUpgrade)
	mux.HandleFunc("/api/vip/status", s.handleVipStatus)

	s.log.Info("HTTP REST 服务器启动", "address", s.address)
	go func() {
		if err := http.ListenAndServe(s.address, mux); err != nil {
			s.log.Error("HTTP 服务器退出", "error", err)
		}
	}()
	return nil
}

// handleChat POST /api/chat
func (s *HTTPServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, chatResponse{Code: 1, Msg: "请求解析失败"})
		return
	}

	if req.Content == "" && len(req.Context) == 0 {
		writeJSON(w, http.StatusBadRequest, chatResponse{Code: 1, Msg: "消息内容不能为空"})
		return
	}

	chatReq := &llm.ChatRequest{
		UserID:   req.UserID,
		TargetID: req.TargetID,
		Content:  req.Content,
		ChatType: req.ChatType,
		Skill:    req.Skill,
		Context:  req.Context,
	}

	resp, err := s.svc.provider.Chat(r.Context(), chatReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, chatResponse{Code: 2, Msg: "LLM调用失败: " + err.Error()})
		return
	}

	s.log.Info("HTTP chat 请求处理完成",
		"user_id", req.UserID,
		"content_len", len(req.Content),
		"reply_len", len(resp.Reply),
	)

	writeJSON(w, http.StatusOK, chatResponse{
		Code:  0,
		Msg:   "成功",
		Reply: resp.Reply,
		Model: resp.Model,
	})
}

// handleHealth GET /api/health
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	healthy := s.svc.provider.IsHealthy(r.Context())
	llmStatus := "active"
	if !healthy {
		llmStatus = "error"
	}

	writeJSON(w, http.StatusOK, healthResponse{
		Healthy:   healthy,
		Version:   s.svc.version,
		LlmStatus: llmStatus,
	})
}

// handleVipUpgrade POST /api/vip/upgrade —— 模拟支付后升级 VIP
func (s *HTTPServer) handleVipUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.limiter == nil {
		writeJSON(w, http.StatusOK, vipResponse{Code: 2, Msg: "会员服务不可用"})
		return
	}
	var req vipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, vipResponse{Code: 1, Msg: "请求解析失败"})
		return
	}
	if req.UserID <= 0 {
		writeJSON(w, http.StatusBadRequest, vipResponse{Code: 1, Msg: "无效用户"})
		return
	}
	// —— 模拟支付校验（真实支付在此对接网关）——
	const vipPriceCents = 500
	if req.PaymentToken == "" {
		writeJSON(w, http.StatusOK, vipResponse{Code: 3, Msg: "支付凭证缺失"})
		return
	}
	if req.AmountCents < vipPriceCents {
		writeJSON(w, http.StatusOK, vipResponse{Code: 4, Msg: "支付金额不足（需 $5）"})
		return
	}
	ctx := r.Context()
	if s.limiter.IsVIP(ctx, req.UserID) {
		writeJSON(w, http.StatusOK, vipResponse{Code: 0, Msg: "您已是 VIP", IsVip: true})
		return
	}
	if err := s.limiter.MarkVIP(ctx, req.UserID); err != nil {
		writeJSON(w, http.StatusOK, vipResponse{Code: 5, Msg: "升级失败"})
		return
	}
	s.log.Info("VIP 升级成功", "user_id", req.UserID, "amount_cents", req.AmountCents)
	writeJSON(w, http.StatusOK, vipResponse{Code: 0, Msg: "升级成功，已享 VIP 配额", IsVip: true})
}

// handleVipStatus GET /api/vip/status?user_id=N
func (s *HTTPServer) handleVipStatus(w http.ResponseWriter, r *http.Request) {
	var uid int32
	if v := r.URL.Query().Get("user_id"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		uid = int32(n)
	}
	isVip := false
	if s.limiter != nil {
		isVip = s.limiter.IsVIP(context.Background(), uid)
	}
	writeJSON(w, http.StatusOK, vipResponse{Code: 0, IsVip: isVip})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
