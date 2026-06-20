package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileStore 本地文件头像存储(实现 Storage 接口)
type FileStore struct {
	dir        string // 绝对路径目录
	httpURL    string // 公网访问前缀，如 http://192.168.100.128:8080
	maxSize    int64
	maxW, maxH int
}

// NewFileStore 创建本地文件存储。
// dir 若为相对路径，则基于"可执行文件所在目录"解析为绝对路径，
// 杜绝从不同工作目录启动导致头像落在不同 uploads/ 而"重启丢失"的问题。
func NewFileStore(dir, httpURL string, maxSizeMB, maxW, maxH int) (*FileStore, error) {
	absDir := resolveDir(dir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("create avatar dir: %w", err)
	}
	return &FileStore{
		dir:     absDir,
		httpURL: strings.TrimRight(httpURL, "/"),
		maxSize: int64(maxSizeMB) * 1024 * 1024,
		maxW:    maxW,
		maxH:    maxH,
	}, nil
}

// resolveDir 相对路径→可执行文件目录下的绝对路径；绝对路径原样返回。
func resolveDir(dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), dir)
	}
	return dir
}

// Dir 返回绝对存储目录(供 HTTP 静态文件服务使用)。
func (s *FileStore) Dir() string { return s.dir }

func (s *FileStore) url(filename string) string {
	return fmt.Sprintf("%s/avatars/%s", s.httpURL, filename)
}

// Save 缩放后保存为 PNG（覆盖旧头像），返回文件名与URL。
func (s *FileStore) Save(id int32, data []byte, format string) (string, string, error) {
	pngData, err := processImage(data, s.maxW, s.maxH, s.maxSize)
	if err != nil {
		return "", "", err
	}

	// 删除旧头像，避免同一 id 文件堆积
	s.deleteFiles(id)

	filename := avatarFilename(id)
	path := filepath.Join(s.dir, filename)
	if err := os.WriteFile(path, pngData, 0644); err != nil {
		return "", "", fmt.Errorf("写入文件失败: %w", err)
	}
	return filename, s.url(filename), nil
}

func (s *FileStore) Delete(id int32) error {
	s.deleteFiles(id)
	return nil
}

func (s *FileStore) deleteFiles(id int32) {
	prefix := avatarPrefix(id)
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}

func (s *FileStore) Find(id int32) (string, string) {
	prefix := avatarPrefix(id)
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			return e.Name(), s.url(e.Name())
		}
	}
	return "", ""
}
