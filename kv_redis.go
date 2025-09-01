package taskbus

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisKV 适配 github.com/redis/go-redis 以满足 KV 接口。
type RedisKV struct{ R *redis.Client }

func (r RedisKV) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return r.R.SetNX(ctx, key, value, ttl).Result()
}
