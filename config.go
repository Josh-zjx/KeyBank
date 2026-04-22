package main

import "os"

type Config struct {
	Port      string
	RedisAddr string
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
		Port:      port,
		RedisAddr: redisAddr,
	}
}
