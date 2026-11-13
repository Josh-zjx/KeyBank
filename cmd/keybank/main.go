package main

import (
	"log/slog"
	"os"

	"github.com/josh-zjx/keybank/handlers"
	"github.com/josh-zjx/keybank/internal/service"
	"github.com/josh-zjx/keybank/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := service.LoadConfig("14000")
	if err != nil {
		logger.Error("configuration", "err", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(service.Healthcheck(cfg.Port))
	}
	var backend handlers.Store
	if cfg.StoreBackend == "mem" {
		backend = store.NewMemKeyStore()
	} else {
		redis := store.NewRedisKeyStore(cfg.RedisAddr)
		defer func() { _ = redis.Close() }()
		backend = redis
	}
	api := handlers.New(backend, logger, cfg.MaxTTL).Handler(cfg.AllowedOrigins)
	if err := service.Run(api, cfg.Port, logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
