package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 服务全局配置
type Config struct {
	Server ServerConfig
	SMTP   SMTPConfig
	Code   CodeConfig
}

// ServerConfig gRPC服务器配置
type ServerConfig struct {
	Host string // 监听地址
	Port int    // gRPC监听端口
}

// SMTPConfig 邮件发送配置
type SMTPConfig struct {
	Host     string // SMTP服务器地址，如 smtp.qq.com
	Port     int    // SMTP端口，如 465(SSL) / 587(STARTTLS)
	Username string // 登录账号（通常即发件邮箱）
	Password string // 授权码/密码
	From     string // 发件人地址，留空则使用 Username
	UseTLS   bool   // 是否使用 TLS（465端口通常为隐式TLS）
}

// CodeConfig 验证码策略
type CodeConfig struct {
	TTL            time.Duration // 验证码有效期
	ResendCooldown time.Duration // 同一邮箱重发冷却时间
	Length         int           // 验证码位数
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 19531,
		},
		SMTP: SMTPConfig{
			Host:     "",
			Port:     465,
			Username: "",
			Password: "",
			From:     "",
			UseTLS:   true,
		},
		Code: CodeConfig{
			TTL:            5 * time.Minute,
			ResendCooldown: 60 * time.Second,
			Length:         6,
		},
	}
}

// Load 从环境变量加载配置
func Load() *Config {
	// 先加载工作目录下的 .env（含 SMTP 授权码，已 gitignore，不会提交）。
	// 不覆盖已存在的真实环境变量（系统环境变量优先）。
	loadDotEnv(".env")

	cfg := DefaultConfig()

	if v := os.Getenv("MAIL_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("MAIL_GRPC_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}

	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = p
		}
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		cfg.SMTP.Username = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		cfg.SMTP.From = v
	}
	if v := os.Getenv("SMTP_USE_TLS"); v != "" {
		cfg.SMTP.UseTLS = (v == "true" || v == "1")
	}
	if cfg.SMTP.From == "" {
		cfg.SMTP.From = cfg.SMTP.Username
	}

	if v := os.Getenv("MAIL_CODE_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Code.TTL = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("MAIL_CODE_COOLDOWN_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Code.ResendCooldown = time.Duration(n) * time.Second
		}
	}

	return cfg
}

// loadDotEnv 读取简单的 KEY=VALUE 文件并注入环境变量（仅在变量未设置时）。
// 支持 # 注释、空行、可选引号；文件不存在则静默跳过。不依赖第三方库。
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
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
			_ = os.Setenv(key, val)
		}
	}
}

// Address 返回gRPC监听地址
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
