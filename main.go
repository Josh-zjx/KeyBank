package main

import (
	"embed"
	"log/slog"
	"net/http"
	"os"

	"github.com/josh-zjx/keybank/handlers"
)

//go:embed templates/*.html static/*
var assets embed.FS

func main() {
	cfg := loadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	var (
		store         KeyStore
		createLimiter handlers.RateLimiter = noopRateLimiter{}
		fetchLimiter  handlers.RateLimiter = noopRateLimiter{}
	)

	switch cfg.StoreBackend {
	case "mem":
		store = newMemKeyStore()
	default:
		redisStore := newRedisKeyStore(cfg.RedisAddr)
		store = redisStore
		createLimiter = newRedisRateLimiter(redisStore.client, "create", cfg.RateLimitCreate)
		fetchLimiter = newRedisRateLimiter(redisStore.client, "fetch", cfg.RateLimitFetch)
	}

	app, err := newAppHandler(assets, store, logger, createLimiter, fetchLimiter, cfg.AutoHideSeconds)
	if err != nil {
		panic(err)
	}

	addr := ":" + cfg.Port
	logger.Info("server starting", "addr", addr)
	if err := http.ListenAndServe(addr, app); err != nil {
		panic(err)
	}
}
