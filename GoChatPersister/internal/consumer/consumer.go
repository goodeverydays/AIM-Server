package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
	amqp "github.com/rabbitmq/amqp091-go"

	"GoChatPersister/internal/config"
)

// ChatEvent 与 IMServer(C++) 发布的 JSON 结构一一对应。
type ChatEvent struct {
	SenderID  int32  `json:"senderid"`
	TargetID  int32  `json:"targetid"`
	Content   string `json:"content"`
	MsgType   int32  `json:"msgtype"`
	MediaURL  string `json:"media_url"`
	Duration  int32  `json:"duration"`
	ThumbURL  string `json:"thumb_url"`
	FileSize  int64  `json:"filesize"`
	FileName  string `json:"filename"`
	Timestamp int64  `json:"timestamp"`
}

// Persister 消费聊天事件并异步落库 t_chatmsg。
type Persister struct {
	cfg *config.Config
	db  *sql.DB
}

func New(cfg *config.Config) (*Persister, error) {
	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 失败: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	return &Persister{cfg: cfg, db: db}, nil
}

// save 将一条聊天事件写入 t_chatmsg；f_create_time 用发送端时间(与客户端去重时间戳对齐)。
func (p *Persister) save(ctx context.Context, ev *ChatEvent) error {
	const cols = "INSERT INTO t_chatmsg (f_senderid,f_targetid,f_msgcontent,f_msgtype," +
		"f_media_url,f_duration,f_thumb_url,f_filesize,f_filename,f_create_time) VALUES (?,?,?,?,?,?,?,?,?,"
	args := []interface{}{ev.SenderID, ev.TargetID, ev.Content, ev.MsgType,
		ev.MediaURL, ev.Duration, ev.ThumbURL, ev.FileSize, ev.FileName}
	var q string
	if ev.Timestamp > 0 {
		q = cols + "FROM_UNIXTIME(?))"
		args = append(args, ev.Timestamp)
	} else {
		q = cols + "NOW())"
	}
	_, err := p.db.ExecContext(ctx, q, args...)
	return err
}

// Run 建立 RabbitMQ 连接并持续消费;连接断开时自动重连。
func (p *Persister) Run(ctx context.Context) {
	for {
		if err := p.consumeOnce(ctx); err != nil {
			slog.Warn("消费中断，3 秒后重连", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (p *Persister) consumeOnce(ctx context.Context) error {
	conn, err := amqp.Dial(p.cfg.RabbitURL)
	if err != nil {
		return fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("打开 channel 失败: %w", err)
	}
	defer ch.Close()

	// 直连交换机 + 持久化队列 + 绑定;生产者与此声明保持一致。
	if err := ch.ExchangeDeclare(p.cfg.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明交换机失败: %w", err)
	}
	if _, err := ch.QueueDeclare(p.cfg.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明队列失败: %w", err)
	}
	if err := ch.QueueBind(p.cfg.Queue, p.cfg.RoutingKey, p.cfg.Exchange, false, nil); err != nil {
		return fmt.Errorf("绑定队列失败: %w", err)
	}
	if err := ch.Qos(p.cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("设置 QoS 失败: %w", err)
	}

	deliveries, err := ch.Consume(p.cfg.Queue, "", false /*手动ack*/, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("开始消费失败: %w", err)
	}
	slog.Info("已连接 RabbitMQ，开始消费",
		"queue", p.cfg.Queue, "exchange", p.cfg.Exchange, "key", p.cfg.RoutingKey)

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("投递通道已关闭")
			}
			var ev ChatEvent
			if err := json.Unmarshal(d.Body, &ev); err != nil {
				slog.Error("消息解析失败，丢弃", "error", err)
				_ = d.Nack(false, false) // 坏消息不重入队，避免毒丸循环
				continue
			}
			if err := p.save(ctx, &ev); err != nil {
				slog.Error("落库失败，重入队", "error", err)
				_ = d.Nack(false, true) // 落库失败重入队重试
				continue
			}
			_ = d.Ack(false)
		}
	}
}
