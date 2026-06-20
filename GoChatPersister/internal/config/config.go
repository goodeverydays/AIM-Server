package config

import "os"

// Config 聊天消息持久化消费者配置(全部环境变量驱动)。
type Config struct {
	RabbitURL  string // amqp://user:pass@host:5672/
	Exchange   string // 交换机名
	Queue      string // 队列名
	RoutingKey string // 绑定/路由键
	MySQLDSN   string // user:pass@tcp(host:3306)/db?charset=utf8mb4&parseTime=true
	Prefetch   int    // QoS 预取(未 ack 的最大投递数)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load 从环境变量加载配置。
func Load() *Config {
	return &Config{
		RabbitURL:  env("RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/"),
		Exchange:   env("RABBITMQ_EXCHANGE", "im.events"),
		Queue:      env("RABBITMQ_QUEUE", "im.chat.persist"),
		RoutingKey: env("RABBITMQ_ROUTING_KEY", "chat"),
		MySQLDSN:   env("MYSQL_DSN", "root:qaz000999plm@tcp(127.0.0.1:3306)/myim?charset=utf8mb4&parseTime=true"),
		Prefetch:   16,
	}
}
