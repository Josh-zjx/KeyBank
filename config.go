package main

import (
	"os"
	"strconv"
)

type Config struct {
	Port             string
	RedisAddr        string
	RateLimitCreate  int
	RateLimitFetch   int
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
