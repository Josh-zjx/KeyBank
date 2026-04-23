package main

import (
	"context"
	"embed"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		logger.Error("handler init", "err", err)
		os.Exit(1)
	}

	addr := ":" + cfg.Port
	logger.Info("server starting", "addr", addr)

	// Create HTTP server with timeouts
	srv := &http.Server{
		Addr:              addr,
		Handler:           app,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Setup graceful shutdown with signal handling
	sigCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-sigCtx.Done()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
