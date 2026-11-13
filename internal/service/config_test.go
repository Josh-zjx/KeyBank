package service

import (
	"testing"
	"time"
)

func TestConfiguration(t *testing.T) {
	for _, key := range []string{"PORT", "KEYBANK_STORE", "REDIS_ADDR", "MAX_TTL", "API_BASE_URL", "AUTO_HIDE_SECONDS"} {
		t.Setenv(key, "")
	}
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	cfg, err := LoadConfig("14001")
	if err != nil || cfg.Port != "14001" || cfg.MaxTTL != 168*time.Hour || len(cfg.AllowedOrigins) != 0 {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	for _, tc := range []struct{ key, value string }{
		{"PORT", "0"}, {"PORT", "65536"}, {"PORT", "abc"},
		{"KEYBANK_STORE", "other"}, {"MAX_TTL", "59s"}, {"MAX_TTL", "169h"}, {"MAX_TTL", "oops"},
		{"AUTO_HIDE_SECONDS", "0"}, {"AUTO_HIDE_SECONDS", "no"},
		{"CORS_ALLOWED_ORIGINS", "*"}, {"CORS_ALLOWED_ORIGINS", "https://*.example"},
		{"CORS_ALLOWED_ORIGINS", "https://example/path"}, {"CORS_ALLOWED_ORIGINS", "https://u:p@example"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := LoadConfig("14000"); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://notes.example,https://other.example ")
	t.Setenv("KEYBANK_STORE", "mem")
	t.Setenv("MAX_TTL", "2h")
	t.Setenv("AUTO_HIDE_SECONDS", "1")
	cfg, err = LoadConfig("14000")
	if err != nil || cfg.MaxTTL != 2*time.Hour || cfg.AutoHideSeconds != 1 || len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("overrides: %+v %v", cfg, err)
	}
}
