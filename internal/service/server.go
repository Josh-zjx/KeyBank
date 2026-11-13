package service

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Healthcheck returns an exit status for the container's local health endpoint.
func Healthcheck(port string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", port)+"/healthz", nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// Run serves until SIGTERM/SIGINT, allowing active requests to finish.
func Run(handler http.Handler, port string, logger *slog.Logger) error {
	srv := &http.Server{
		Addr: ":" + port, Handler: handler,
		ReadTimeout: 10 * time.Second, ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	failed := make(chan error, 1)
	go func() { failed <- srv.ListenAndServe() }()
	logger.Info("server starting", "port", port)
	select {
	case err := <-failed:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		return srv.Shutdown(shutdown)
	}
}
