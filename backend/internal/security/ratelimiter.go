package security

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, time.Duration, error)
}

type RedisRateLimiter struct {
	client *redis.Client
}

func NewRedisRateLimiter(addr string) *RedisRateLimiter {
	return &RedisRateLimiter{client: redis.NewClient(&redis.Options{Addr: addr})}
}

func (r *RedisRateLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, time.Duration, error) {
	if limit <= 0 {
		return true, 0, nil
	}

	lua := redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
return {current, ttl}
`)

	result, err := lua.Run(ctx, r.client, []string{key}, window.Milliseconds()).Result()
	if err != nil {
		return true, 0, fmt.Errorf("rate limit eval: %w", err)
	}

	values, ok := result.([]any)
	if !ok || len(values) != 2 {
		return true, 0, nil
	}

	count, ok := values[0].(int64)
	if !ok {
		if n, conv := values[0].(int); conv {
			count = int64(n)
		}
	}

	ttlMs, ok := values[1].(int64)
	if !ok {
		if n, conv := values[1].(int); conv {
			ttlMs = int64(n)
		}
	}

	if count > limit {
		retry := time.Duration(ttlMs) * time.Millisecond
		if retry < 0 {
			retry = window
		}
		return false, retry, nil
	}

	return true, 0, nil
}

func (r *RedisRateLimiter) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}