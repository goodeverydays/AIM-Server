package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config GoMediaServer 配置(聊天媒体：语音/视频/图片/文件)
type Config struct {
	Host       string // 监听地址
	Port       int    // HTTP 端口
	PublicHost string // 对外可访问地址(拼接媒体URL用)
	MaxSizeMB  int    // 单文件上限

	Backend  string // local | s3
	MediaDir string // 本地媒体目录(建议绝对持久路径)
	S3       S3Config
}

type S3Config struct {
	Endpoint   string
	Bucket     string
	AccessKey  string
	SecretKey  string
	Region     string
	UseSSL     bool
	PublicBase string
}

func Default() *Config {
	return &Config{
		Host:       "0.0.0.0",
		Port:       8090,
		PublicHost: "192.168.100.128",
		MaxSizeMB:  50,
		Backend:    "local",
		MediaDir:   "uploads/media",
		S3:         S3Config{UseSSL: true},
	}
}

func Load() *Config {
	loadDotEnv()
	cfg := Default()
	if v := os.Getenv("MEDIA_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("MEDIA_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("MEDIA_PUBLIC_HOST"); v != "" {
		cfg.PublicHost = v
	}
	if v := os.Getenv("MEDIA_MAX_SIZE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxSizeMB = n
		}
	}
	if v := os.Getenv("STORAGE_BACKEND"); v != "" {
		cfg.Backend = v
	}
	if v := os.Getenv("MEDIA_DIR"); v != "" {
		cfg.MediaDir = v
	}
	if v := os.Getenv("S3_ENDPOINT"); v != "" {
		cfg.S3.Endpoint = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		cfg.S3.Bucket = v
	}
	if v := os.Getenv("S3_ACCESS_KEY"); v != "" {
		cfg.S3.AccessKey = v
	}
	if v := os.Getenv("S3_SECRET_KEY"); v != "" {
		cfg.S3.SecretKey = v
	}
	if v := os.Getenv("S3_REGION"); v != "" {
		cfg.S3.Region = v
	}
	if v := os.Getenv("S3_PUBLIC_BASE_URL"); v != "" {
		cfg.S3.PublicBase = v
	}
	if v := os.Getenv("S3_USE_SSL"); v != "" {
		cfg.S3.UseSSL = (v == "1" || strings.EqualFold(v, "true"))
	}
	return cfg
}

func (c *Config) Addr() string      { return fmt.Sprintf("%s:%d", c.Host, c.Port) }
func (c *Config) PublicURL() string { return fmt.Sprintf("http://%s:%d", c.PublicHost, c.Port) }

// loadDotEnv 依次尝试 .env(cwd → 可执行文件目录)，真实环境变量优先。
func loadDotEnv() {
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), ".env"))
	}
	for _, p := range paths {
		if tryLoadEnvFile(p) {
			return
		}
	}
}

func tryLoadEnvFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		if _, ok := os.LookupEnv(key); !ok {
			os.Setenv(key, val)
		}
	}
	return true
}
