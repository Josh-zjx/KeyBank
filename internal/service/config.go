// Package service shares process configuration, timeouts, health probes, and
// graceful shutdown between the independent API and demo executables.
package service

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains environment configuration shared by the two entrypoints.
type Config struct {
	Port            string
	RedisAddr       string
	StoreBackend    string
	MaxTTL          time.Duration
	APIBaseURL      string
	AutoHideSeconds int
	AllowedOrigins  []string
}

// LoadConfig applies the caller's default port and rejects unsafe settings.
func LoadConfig(defaultPort string) (Config, error) {
	cfg := Config{
		Port:            envString("PORT", defaultPort),
		RedisAddr:       envString("REDIS_ADDR", "localhost:6379"),
		StoreBackend:    envString("KEYBANK_STORE", "redis"),
		APIBaseURL:      envString("API_BASE_URL", "http://localhost:14000/api/v1"),
		AutoHideSeconds: 60,
		MaxTTL:          7 * 24 * time.Hour,
	}
	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		return cfg, fmt.Errorf("PORT must be between 1 and 65535")
	}
	if cfg.StoreBackend != "mem" && cfg.StoreBackend != "redis" {
		return cfg, fmt.Errorf("KEYBANK_STORE must be mem or redis")
	}
	if value := os.Getenv("MAX_TTL"); value != "" {
		cfg.MaxTTL, err = time.ParseDuration(value)
		if err != nil || cfg.MaxTTL < time.Minute || cfg.MaxTTL > 7*24*time.Hour {
			return cfg, fmt.Errorf("MAX_TTL must be between 60s and 168h")
		}
	}
	if value := os.Getenv("AUTO_HIDE_SECONDS"); value != "" {
		cfg.AutoHideSeconds, err = strconv.Atoi(value)
		if err != nil || cfg.AutoHideSeconds <= 0 {
			return cfg, fmt.Errorf("AUTO_HIDE_SECONDS must be positive")
		}
	}
	origins := envString("CORS_ALLOWED_ORIGINS", "http://localhost:14001,http://127.0.0.1:14001")
	// An explicitly empty value disables cross-origin access.
	if value, ok := os.LookupEnv("CORS_ALLOWED_ORIGINS"); ok {
		origins = value
	}
	for _, value := range strings.Split(origins, ",") {
		origin := strings.TrimSpace(value)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.Contains(u.Host, "*") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return cfg, fmt.Errorf("CORS_ALLOWED_ORIGINS must contain exact HTTP(S) origins")
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, origin)
	}
	return cfg, nil
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
