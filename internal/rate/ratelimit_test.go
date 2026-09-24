package rate

import (
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/go-redis/redis/v8"
)

func newTestRateLimiter(t *testing.T, bucket string, limit int) (*RedisRateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return NewRedisRateLimiter(client, bucket, limit), mr
}

func TestRedisRateLimiterAllowsUpToLimit(t *testing.T) {
	const limit = 5
	rl, _ := newTestRateLimiter(t, "create", limit)

	for i := 1; i <= limit; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Fatalf("request %d: expected Allow=true, got false", i)
		}
	}
}

func TestRedisRateLimiterBlocksAfterLimit(t *testing.T) {
	const limit = 3
	rl, _ := newTestRateLimiter(t, "create", limit)

	for i := 0; i < limit; i++ {
		rl.Allow("10.0.0.2")
	}

	if rl.Allow("10.0.0.2") {
		t.Fatal("N+1 request: expected Allow=false (blocked), got true")
	}
}

func TestRedisRateLimiterResetsAfterTTL(t *testing.T) {
	const limit = 2
	rl, mr := newTestRateLimiter(t, "create", limit)

	// Exhaust the limit.
	for i := 0; i < limit; i++ {
		rl.Allow("10.0.0.3")
	}
	if rl.Allow("10.0.0.3") {
		t.Fatal("should be blocked before TTL expires")
	}

	// Advance the miniredis clock past the 1-hour TTL.
	mr.FastForward(3601 * time.Second)

	// Counter should have reset — first request allowed again.
	if !rl.Allow("10.0.0.3") {
		t.Fatal("should be allowed after TTL reset")
	}
}

func TestRedisRateLimiterDifferentIPsAreIndependent(t *testing.T) {
	const limit = 1
	rl, _ := newTestRateLimiter(t, "fetch", limit)

	// Exhaust IP-A.
	rl.Allow("192.168.0.1")

	// IP-A is now blocked.
	if rl.Allow("192.168.0.1") {
		t.Fatal("IP-A: should be blocked after limit")
	}

	// IP-B has its own counter — still allowed.
	if !rl.Allow("192.168.0.2") {
		t.Fatal("IP-B: should be allowed (independent counter)")
	}
}

func TestRedisRateLimiterBucketIsolation(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	createLimiter := NewRedisRateLimiter(client, "create", 1)
	fetchLimiter := NewRedisRateLimiter(client, "fetch", 5)

	// Exhaust the create bucket for an IP.
	createLimiter.Allow("10.1.1.1")
	if createLimiter.Allow("10.1.1.1") {
		t.Fatal("create bucket: should be blocked")
	}

	// The fetch bucket for the same IP is independent.
	if !fetchLimiter.Allow("10.1.1.1") {
		t.Fatal("fetch bucket: should still be allowed")
	}
}

func TestRedisRateLimiterFailsClosedOnError(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	rl := NewRedisRateLimiter(client, "create", 5)

	// Close Redis to simulate an outage.
	mr.Close()

	// Should fail closed (block the request) rather than allowing.
	if rl.Allow("10.2.2.2") {
		t.Fatal("expected fail-closed (Allow=false) when Redis is down")
	}
}

func TestRedisRateLimiterKeyFormat(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	rl := NewRedisRateLimiter(client, "create", 10)
	ip := "203.0.113.1"
	rl.Allow(ip)

	expectedKey := fmt.Sprintf("rl:create:%s", ip)
	if !mr.Exists(expectedKey) {
		t.Errorf("expected key %q to exist in Redis", expectedKey)
	}
}
