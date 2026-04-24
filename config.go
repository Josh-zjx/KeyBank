package main

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            string
	RedisAddr       string
	StoreBackend    string
	AutoHideSeconds int
	RateLimitCreate int
	RateLimitFetch  int
	MaxTTL          time.Duration
	TrustXFF        bool
}

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "14000"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	return Config{
		Port:            port,
		RedisAddr:       redisAddr,
		StoreBackend:    envString("KEYBANK_STORE", "redis"),
		AutoHideSeconds: envInt("AUTO_HIDE_SECONDS", 60),
		RateLimitCreate: envInt("RATE_LIMIT_CREATE", 30),
		RateLimitFetch:  envInt("RATE_LIMIT_FETCH", 60),
		MaxTTL:          envDuration("MAX_TTL", 7*24*time.Hour),
		TrustXFF:        envBool("TRUST_XFF", false),
	}
}

// envInt reads an env var as an integer, returning def if absent or unparseable.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool reads an env var as a boolean, accepting "1", "true", "TRUE", etc.
// Returns def if absent or unparseable.
func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || v == "true" || v == "True" || v == "TRUE"
	}
	return def
}

// envDuration reads an env var as a Go duration string (e.g., "168h", "24h30m").
// Returns def if absent, unparseable, or below 60s (logs warning). Minimum enforced: 60s.
func envDuration(key string, def time.Duration) time.Duration {
	minDuration := 60 * time.Second
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			if d < minDuration {
				slog.Warn("duration below minimum, using default", "key", key, "value", v, "min", minDuration, "default", def)
				return def
			}
			return d
		}
		slog.Warn("failed to parse duration, using default", "key", key, "value", v, "default", def)
	}
	return def
}
