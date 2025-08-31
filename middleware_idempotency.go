package taskbus

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"
)

// KV 是幂等中间件依赖的最小键值接口，便于单元测试注入 mock。
type KV interface { SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) }

// IdempotencyConfig 配置幂等中间件。
// Key 计算顺序：优先 Message.Key；若为空且提供 KeyFunc，则使用 KeyFunc。
// 最终存储 key 为 Prefix + ":" + sha1(keyRaw)。
type IdempotencyConfig struct {
	KV      KV            // 可选：键值存储（生产用 RedisKV），若为 nil 且提供 Redis* 或使用 MQ.Redis，则自动创建
	// 可选 Redis 连接参数（若 KV 为空则使用这些参数自动启用）
	RedisAddr     string
	RedisUsername string
	RedisPassword string
	RedisDB       int

	Prefix  string        // key 前缀，如 "tq:idem"
	TTL     time.Duration // 幂等键过期时间
	KeyFunc func(ctx context.Context, m Message) (string, error) // 可选：自定义业务唯一键
}

// NewIdempotencyMiddleware 生成通用 MQ Middleware。
func NewIdempotencyMiddleware(cfg IdempotencyConfig) Middleware {
	if cfg.KV == nil { panic("IdempotencyMiddleware requires KV") }
	prefix := cfg.Prefix
	if prefix == "" { prefix = "tq:idem" }
	if cfg.TTL <= 0 { cfg.TTL = 24 * time.Hour }
	return func(next Handler) Handler {
		return func(ctx context.Context, m Message) error {
			keyRaw := m.Key
			if keyRaw == "" && cfg.KeyFunc != nil {
				if s, err := cfg.KeyFunc(ctx, m); err == nil { keyRaw = s }
			}
			if keyRaw == "" { return next(ctx, m) }
			// sha1 规整 key
			h := sha1.Sum([]byte(keyRaw))
			storeKey := fmt.Sprintf("%s:%s", prefix, hex.EncodeToString(h[:]))
			ok, err := cfg.KV.SetNX(ctx, storeKey, "1", cfg.TTL)
			if err != nil { return err }
			if !ok { return nil } // 已处理，直接跳过
			return next(ctx, m)
		}
	}
}

// NewJobIdempotencyMiddleware 生成 JobMiddleware。
// Job 场景下，构造 Message 计算 key（jobName+payload 或自定义）。
func NewJobIdempotencyMiddleware(cfg IdempotencyConfig) JobMiddleware {
	mw := NewIdempotencyMiddleware(cfg)
	return func(next JobHandler) JobHandler {
		return func(ctx context.Context, jobName string, payload []byte) error {
			m := Message{Topic: "job." + jobName, Key: "", Body: payload}
			// 允许 KeyFunc 使用 Message 内容来定制幂等 key
			h := func(context.Context, Message) error { return next(ctx, jobName, payload) }
			return mw(h)(ctx, m)
		}
	}
}

