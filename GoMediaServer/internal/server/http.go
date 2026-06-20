package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"GoMediaServer/internal/storage"
)

// HTTPServer 媒体上传 + 静态文件服务
type HTTPServer struct {
	store     storage.Storage
	addr      string
	maxBytes  int64
	staticDir string // 本地后端静态目录；为空=对象存储模式不提供静态服务
}

func NewHTTPServer(addr string, store storage.Storage, maxSizeMB int, staticDir string) *HTTPServer {
	return &HTTPServer{
		store:     store,
		addr:      addr,
		maxBytes:  int64(maxSizeMB) * 1024 * 1024,
		staticDir: staticDir,
	}
}

func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/media/upload", s.handleUpload)
	mux.HandleFunc("/api/health", s.handleHealth)

	if s.staticDir != "" {
		_ = os.MkdirAll(s.staticDir, 0755)
		fs := http.FileServer(http.Dir(s.staticDir))
		mux.Handle("/media/", http.StripPrefix("/media/", fs))
	}

	slog.Info("HTTP 媒体服务启动", "addr", s.addr, "static", s.staticDir != "")
	return http.ListenAndServe(s.addr, corsMiddleware(mux))
}

func (s *HTTPServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// 限制请求体大小(上限 + 1MB 余量给 multipart 边界)
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBytes+(1<<20))
	if err := r.ParseMultipartForm(s.maxBytes + (1 << 20)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": 1, "msg": "请求过大或格式错误"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": 1, "msg": "未找到文件字段 file"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": 2, "msg": "读取失败"})
		return
	}
	if int64(len(data)) > s.maxBytes {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": 1, "msg": "文件超过大小上限"})
		return
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	filename, url, err := s.store.Save(data, ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": 2, "msg": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":      0,
		"msg":       "上传成功",
		"filename":  filename,
		"url":       url,
		"file_size": len(data),
	})
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"healthy": true})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
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
