package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileStore 本地文件媒体存储(实现 Storage)
type FileStore struct {
	dir     string // 绝对路径
	httpURL string // 公网前缀，如 http://192.168.100.128:8090
}

// NewFileStore 创建本地存储。dir 相对路径会基于可执行文件目录解析为绝对路径，
// 避免从不同工作目录启动导致媒体落在不同目录而"丢失"。
func NewFileStore(dir, httpURL string) (*FileStore, error) {
	absDir := resolveDir(dir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("create media dir: %w", err)
	}
	return &FileStore{dir: absDir, httpURL: strings.TrimRight(httpURL, "/")}, nil
}

func resolveDir(dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), dir)
	}
	return dir
}

// Dir 返回绝对存储目录(供 HTTP 静态服务)。
func (s *FileStore) Dir() string { return s.dir }

func (s *FileStore) Save(data []byte, ext string) (string, string, error) {
	filename := uniqueFilename(ext)
	if err := os.WriteFile(filepath.Join(s.dir, filename), data, 0644); err != nil {
		return "", "", fmt.Errorf("写入文件失败: %w", err)
	}
	return filename, fmt.Sprintf("%s/media/%s", s.httpURL, filename), nil
}
