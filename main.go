package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/josh-zjx/keybank/handlers"
)

func main() {
	cfg := loadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	store := newRedisKeyStore(cfg.RedisAddr)
	h := handlers.New(&storeAdapter{ks: store}, logger)

	createLimiter := newRedisRateLimiter(store.client, "create", cfg.RateLimitCreate)
	fetchLimiter := newRedisRateLimiter(store.client, "fetch", cfg.RateLimitFetch)

	mux := http.NewServeMux()
	mux.Handle("POST /api/keys", handlers.Chain(
		handlers.MaxBodySize(64*1024),
		handlers.RateLimit(createLimiter),
	)(http.HandlerFunc(h.CreateKey)))
	mux.Handle("GET /api/keys/{id}",
		handlers.RateLimit(fetchLimiter)(http.HandlerFunc(h.FetchKey)))
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)

	globalChain := handlers.Chain(
		handlers.PanicRecover(logger),
		handlers.SecurityHeaders(),
	)

	addr := ":" + cfg.Port
	logger.Info("server starting", "addr", addr)
	if err := http.ListenAndServe(addr, globalChain(mux)); err != nil {
		panic(err)
	}
}
