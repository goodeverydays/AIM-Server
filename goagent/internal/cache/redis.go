package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache Redis 缓存实现
type RedisCache struct {
	client     *redis.Client
	keyPrefix  string
}

// NewRedisCache 创建 Redis 缓存客户端
//
// addr: Redis 地址，如 "192.168.100.128:6379"
// password: 密码，无密码传 ""
// db: 数据库编号，通常 0
func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	// 启动时验证连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connect failed: %w", err)
	}

	return &RedisCache{
		client:    client,
		keyPrefix: "goagent:",
	}, nil
}

// Close 关闭 Redis 连接
func (r *RedisCache) Close() error {
	return r.client.Close()
}

// fullKey 拼接完整 key（加统一前缀）
func (r *RedisCache) fullKey(key string) string {
	return r.keyPrefix + key
}

// historyKey 生成对话历史 key
func (r *RedisCache) historyKey(userID int64) string {
	return fmt.Sprintf("%schat:history:%d", r.keyPrefix, userID)
}

func (r *RedisCache) Name() string {
	return "redis"
}

// ---- 多轮对话上下文 ----

func (r *RedisCache) GetHistory(ctx context.Context, userID int64, limit int) ([]Message, error) {
	key := r.historyKey(userID)
	vals, err := r.client.LRange(ctx, key, -int64(limit), -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis lrange: %w", err)
	}

	msgs := make([]Message, 0, len(vals))
	for _, v := range vals {
		msg, err := UnmarshalMessage(v)
		if err != nil {
			continue // 跳过损坏的数据
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func (r *RedisCache) AppendHistory(ctx context.Context, userID int64, msg Message, maxLen int) error {
	key := r.historyKey(userID)
	data, err := MarshalMessage(msg)
	if err != nil {
		return err
	}

	pipe := r.client.TxPipeline()
	pipe.RPush(ctx, key, data)
	// 裁剪到 maxLen 条
	pipe.LTrim(ctx, key, -int64(maxLen), -1)
	// 设置过期时间（7天无活动自动清理）
	pipe.Expire(ctx, key, 7*24*time.Hour)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis append history: %w", err)
	}
	return nil
}

func (r *RedisCache) ClearHistory(ctx context.Context, userID int64) error {
	return r.client.Del(ctx, r.historyKey(userID)).Err()
}

// ---- 基础 KV ----

func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, r.fullKey(key)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (r *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, r.fullKey(key), value, ttl).Err()
}

func (r *RedisCache) Del(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.fullKey(key)).Err()
}

// ---- 限流 ----

func (r *RedisCache) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	fullKey := r.fullKey(key)
	val, err := r.client.Incr(ctx, fullKey).Result()
	if err != nil {
		return 0, err
	}
	// 首次设置过期时间
	if val == 1 {
		r.client.Expire(ctx, fullKey, ttl)
	}
	return val, nil
}

// ---- 健康检查 ----

func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// ---- 辅助 ----

// GetUserSession 获取用户会话状态（JSON 存储）
func (r *RedisCache) GetUserSession(ctx context.Context, userID int64) (map[string]string, error) {
	key := fmt.Sprintf("%ssession:%d", r.keyPrefix, userID)
	result, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SetUserSession 设置用户会话字段
func (r *RedisCache) SetUserSession(ctx context.Context, userID int64, fields map[string]string) error {
	key := fmt.Sprintf("%ssession:%d", r.keyPrefix, userID)
	return r.client.HSet(ctx, key, fields).Err()
}

// GetCounter 读取计数器
func (r *RedisCache) GetCounter(ctx context.Context, key string) (int64, error) {
	val, err := r.client.Get(ctx, r.fullKey(key)).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}
