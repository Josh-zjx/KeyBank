package main

import (
	"os"
	"strconv"
)

type Config struct {
	Port            string
	RedisAddr       string
	StoreBackend    string
	AutoHideSeconds int
	RateLimitCreate int
	RateLimitFetch  int
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
