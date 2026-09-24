package rate

import (
	"context"
	"net/http"

	goredis "github.com/go-redis/redis/v8"
)

// RedisRateLimiter implements handlers.RateLimiter using a Redis fixed-window counter.
// The Lua script atomically increments the counter and sets a 1-hour TTL on first use,
// ensuring each IP gets exactly `limit` requests per hour window.
type RedisRateLimiter struct {
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

func NewRedisRateLimiter(client *goredis.Client, bucket string, limit int) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		bucket: bucket,
		limit:  limit,
		script: incrScript,
	}
}

// Allow returns true if the IP has not exceeded the per-hour limit.
// On Redis error it fails closed (blocks the request) per mission principle 5.
func (rl *RedisRateLimiter) Allow(ip string) bool {
	key := "rl:" + rl.bucket + ":" + ip
	count, err := rl.script.Run(context.Background(), rl.client, []string{key}).Int()
	if err != nil {
		// Fail closed: block requests if Redis is unavailable.
		return false
	}
	return count <= rl.limit
}

// NoopRateLimiter always allows requests. Used in tests and for routes with no limit.
type NoopRateLimiter struct{}

func (NoopRateLimiter) Allow(_ string) bool { return true }

// Ensure NoopRateLimiter satisfies the interface at compile time.
var _ interface{ Allow(string) bool } = NoopRateLimiter{}

// Ensure http.MaxBytesError is accessible at compile time (Go 1.19+).
var _ *http.MaxBytesError
