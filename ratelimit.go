package main

import (
	"context"
	"net/http"

	goredis "github.com/go-redis/redis/v8"
)

// redisRateLimiter implements handlers.RateLimiter using a Redis fixed-window counter.
// The Lua script atomically increments the counter and sets a 1-hour TTL on first use,
// ensuring each IP gets exactly `limit` requests per hour window.
type redisRateLimiter struct {
	client *goredis.Client
	bucket string // key prefix, e.g. "create" or "fetch"
	limit  int
	script *goredis.Script
}

// incrScript atomically increments a Redis key and sets a 1-hour TTL on first use.
// Returns the new counter value.
var incrScript = goredis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("EXPIRE", KEYS[1], 3600)
end
return count
`)

func newRedisRateLimiter(client *goredis.Client, bucket string, limit int) *redisRateLimiter {
	return &redisRateLimiter{
		client: client,
		bucket: bucket,
		limit:  limit,
		script: incrScript,
	}
}

// Allow returns true if the IP has not exceeded the per-hour limit.
// On Redis error it fails closed (blocks the request) per mission principle 5.
func (rl *redisRateLimiter) Allow(ip string) bool {
	key := "rl:" + rl.bucket + ":" + ip
	count, err := rl.script.Run(context.Background(), rl.client, []string{key}).Int()
	if err != nil {
		// Fail closed: block requests if Redis is unavailable.
		return false
	}
	return count <= rl.limit
}

// noopRateLimiter always allows requests. Used in tests and for routes with no limit.
type noopRateLimiter struct{}

func (noopRateLimiter) Allow(_ string) bool { return true }

// Ensure noopRateLimiter satisfies the interface at compile time.
var _ interface{ Allow(string) bool } = noopRateLimiter{}

// Ensure http.MaxBytesError is accessible at compile time (Go 1.19+).
var _ *http.MaxBytesError
