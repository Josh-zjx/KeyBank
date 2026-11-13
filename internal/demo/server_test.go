package demo

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDemoIndependentRoutes(t *testing.T) {
	app, err := NewHandler("https://keys.example/api/v1", 1, 2*time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, contains string
		status         int
	}{
		{"/", "data-api-base-url=\"https://keys.example/api/v1\"", 200},
		{"/share/0123456789abcdef0123456789abcdef", "fetch-passphrase", 200},
		{"/static/crypto.js", "keybank/demo/v3/note", 200},
		{"/healthz", `{"status":"ok"}`, 200},
		{"/unknown", "Page not found", 404},
		{"/api/v1/keys", "Page not found", 404},
	} {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
		if tc.path == "/" {
			if !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self' https://keys.example") {
				t.Fatal("CSP excludes API")
			}
			if !strings.Contains(w.Body.String(), "data-max-ttl-seconds=\"7200\"") {
				t.Fatal("max TTL missing")
			}
		}
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/keys", strings.NewReader(`{"type":"ecc"}`)))
	if w.Code == 201 {
		t.Fatal("demo implements key API")
	}
}

func TestAPIURLValidation(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "https://user:pass@keys.example/api/v1", "//keys.example/api/v1", "https://keys.example/api/v1?token=secret", "https://keys.example/api/v1#secret", "https://keys.example/other", "https://keys.example;evil/api/v1"} {
		if _, err := NewHandler(value, 60, time.Hour, slog.Default()); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	for _, value := range []string{"/api/v1", "http://127.0.0.1:14000/api/v1", "https://keys.example/api/v1/"} {
		if _, err := NewHandler(value, 60, time.Hour, slog.Default()); err != nil {
			t.Fatalf("rejected %s: %v", value, err)
		}
	}
}
