package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"GoImageServer/internal/storage"
)

// HTTPServer HTTP 服务器（上传 + 静态文件）
type HTTPServer struct {
	store     storage.Storage
	addr      string
	staticDir string // 本地静态文件目录(绝对路径)；为空=对象存储模式，不提供静态服务
}

func NewHTTPServer(addr string, store storage.Storage, staticDir string) *HTTPServer {
	return &HTTPServer{
		store:     store,
		addr:      addr,
		staticDir: staticDir,
	}
}

func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// API: 上传头像
	mux.HandleFunc("/api/avatar/upload", s.handleUpload)

	// API: 健康检查
	mux.HandleFunc("/api/health", s.handleHealth)

	// 静态文件: /avatars/xxx.png —— 仅本地后端提供；对象存储由 OSS/CDN 直供
	if s.staticDir != "" {
		_ = os.MkdirAll(s.staticDir, 0755)
		fs := http.FileServer(http.Dir(s.staticDir))
		mux.Handle("/avatars/", http.StripPrefix("/avatars/", fs))
	}

	// CORS（允许客户端跨域访问）
	wrapped := corsMiddleware(mux)

	slog.Info("HTTP 服务器启动", "addr", s.addr, "static", s.staticDir != "")
	return http.ListenAndServe(s.addr, wrapped)
}

func (s *HTTPServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 限制 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": 1, "msg": "请求太大或格式错误"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": 1, "msg": "未找到文件字段"})
		return
	}
	defer file.Close()

	// 校验格式
	ext := strings.ToLower(filepath.Ext(header.Filename))
	format := strings.TrimPrefix(ext, ".")
	if format != "png" && format != "jpg" && format != "jpeg" && format != "gif" && format != "webp" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": 1, "msg": "不支持的格式: " + format})
		return
	}
	if format == "jpeg" {
		format = "jpg"
	}

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": 2, "msg": "读取失败"})
		return
	}

	// 使用默认 userID=0（HTTP 模式下由客户端指定）
	userID := int32(0)
	filename, url, err := s.store.Save(userID, data, format)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": 2, "msg": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":     0,
		"msg":      "上传成功",
		"filename": filename,
		"url":      url,
	})
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"healthy": true, "version": "0.1.0"})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}
		next.ServeHTTP(w, r)
	})
}
