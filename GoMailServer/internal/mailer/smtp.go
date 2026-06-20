package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"GoMailServer/internal/config"
)

// dialTimeout SMTP 连接超时，避免服务器无响应时发信协程长时间挂起
const dialTimeout = 10 * time.Second

// Mailer 通过 SMTP 发送验证码邮件
type Mailer struct {
	cfg config.SMTPConfig
}

// New 创建 Mailer
func New(cfg config.SMTPConfig) *Mailer {
	return &Mailer{cfg: cfg}
}

// SendCode 发送验证码邮件到指定地址
func (m *Mailer) SendCode(to, code string, ttlMinutes int) error {
	if m.cfg.Host == "" {
		return fmt.Errorf("SMTP 未配置")
	}

	subject := "您的注册验证码"
	body := fmt.Sprintf("您的验证码是 %s，%d 分钟内有效，请勿泄露给他人。", code, ttlMinutes)

	msg := strings.Join([]string{
		"From: " + m.cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)

	if m.cfg.UseTLS {
		return m.sendTLS(addr, auth, to, []byte(msg))
	}
	return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, []byte(msg))
}

// sendTLS 通过隐式 TLS 连接发送（如 465 端口）
func (m *Mailer) sendTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: m.cfg.Host}

	// 带超时的拨号，避免无响应时永久阻塞
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("连接SMTP服务器失败: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return fmt.Errorf("创建SMTP客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP认证失败: %w", err)
	}
	if err := client.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
