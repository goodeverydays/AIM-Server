package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Server  ServerConfig
	Storage StorageConfig
}

type ServerConfig struct {
	Host       string // 监听地址（绑定用，如 0.0.0.0）
	PublicHost string // 对外可访问地址（拼接头像URL用，如 192.168.100.128）
	GRPCPort   int    // gRPC 端口（供 IMServer 调用）
	HTTPPort   int    // HTTP 端口（静态文件 + 上传 API）
}

type StorageConfig struct {
	Backend   string // 存储后端: "local"(本地文件) | "s3"(对象存储)
	AvatarDir string // 本地头像存储目录(local 后端)
	MaxSizeMB int    // 最大文件大小（MB）
	MaxWidth  int    // 缩放宽（px）
	MaxHeight int    // 缩放高（px）
	S3        S3Config
}

// S3Config 对象存储配置(S3 兼容: 阿里云OSS/腾讯云COS/AWS S3/MinIO)
type S3Config struct {
	Endpoint   string // 如 oss-cn-hangzhou.aliyuncs.com (不含 scheme)
	Bucket     string // 存储桶名
	AccessKey  string // 访问密钥ID
	SecretKey  string // 访问密钥
	Region     string // 区域(部分厂商需要)
	UseSSL     bool   // 是否 https
	PublicBase string // 公网/CDN 访问前缀(缺省回退 endpoint/bucket)
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:       "0.0.0.0",
			PublicHost: "192.168.100.128",
			GRPCPort:   19529,
			HTTPPort:   8080,
		},
		Storage: StorageConfig{
			Backend:   "local",
			AvatarDir: "uploads/avatars",
			MaxSizeMB: 5,
			MaxWidth:  256,
			MaxHeight: 256,
			S3: S3Config{
				UseSSL: true,
			},
		},
	}
}

func Load() *Config {
	loadDotEnv() // 优先加载 .env(不覆盖已存在的真实环境变量)
	cfg := DefaultConfig()
	if v := os.Getenv("AVATAR_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("AVATAR_PUBLIC_HOST"); v != "" {
		cfg.Server.PublicHost = v
	}
	if v := os.Getenv("AVATAR_GRPC_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.GRPCPort = p
		}
	}
	if v := os.Getenv("AVATAR_HTTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.HTTPPort = p
		}
	}
	if v := os.Getenv("AVATAR_MAX_SIZE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Storage.MaxSizeMB = n
		}
	}

	// ── 存储后端 ──
	if v := os.Getenv("STORAGE_BACKEND"); v != "" {
		cfg.Storage.Backend = v
	}
	if v := os.Getenv("AVATAR_DIR"); v != "" {
		cfg.Storage.AvatarDir = v // 本地后端: 建议设为绝对持久路径，避免 cwd 陷阱
	}
	// ── 对象存储(S3 兼容) ──
	if v := os.Getenv("S3_ENDPOINT"); v != "" {
		cfg.Storage.S3.Endpoint = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		cfg.Storage.S3.Bucket = v
	}
	if v := os.Getenv("S3_ACCESS_KEY"); v != "" {
		cfg.Storage.S3.AccessKey = v
	}
	if v := os.Getenv("S3_SECRET_KEY"); v != "" {
		cfg.Storage.S3.SecretKey = v
	}
	if v := os.Getenv("S3_REGION"); v != "" {
		cfg.Storage.S3.Region = v
	}
	if v := os.Getenv("S3_PUBLIC_BASE_URL"); v != "" {
		cfg.Storage.S3.PublicBase = v
	}
	if v := os.Getenv("S3_USE_SSL"); v != "" {
		cfg.Storage.S3.UseSSL = (v == "1" || strings.EqualFold(v, "true"))
	}
	return cfg
}

// loadDotEnv 依次尝试加载 .env(当前工作目录 → 可执行文件所在目录)，
// 已存在的真实环境变量优先(不覆盖)，避免"工作目录陷阱"。
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
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
	return true
}

func (s ServerConfig) GRPCAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.GRPCPort)
}

func (s ServerConfig) HTTPAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.HTTPPort)
}

func (s ServerConfig) PublicHTTPURL() string {
	return fmt.Sprintf("http://%s:%d", s.PublicHost, s.HTTPPort)
}
