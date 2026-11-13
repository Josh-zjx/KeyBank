# Technical stack

- Go 1.24 source compatibility; Go 1.27.4 container/CI toolchain. Standard HTTP server, embedded demo assets, html/template, structured logs.
- Independent `cmd/keybank` and `cmd/demo` processes, with shared startup helpers.
- X25519 via crypto/ecdh; random 256-bit AES keys via crypto/rand.
- Salted PBKDF2-HMAC-SHA256 (600,000 iterations), crypto/pbkdf2, constant-time match.
- Redis 7 / go-redis v8 or mutex-protected memory. Immutable payloads with TTL;
  snapshot comparison plus conditional atomic deletion replaces GETDEL.
- Vanilla browser modules and Web Crypto X25519/HKDF-SHA256/AES-256-GCM. No bundler.
- Demo fragment v3; API JSON v1. No application-level fetch sealing.
- Exact-origin CORS without credentials; CSP permits the configured API origin.
- Vendored QR renderer with SRI; Go dependencies remain vendored unchanged.
- Go table-driven/race tests, miniredis, Playwright Chromium, gosec, govulncheck.
- Separate distroless nonroot Docker targets, Compose, optional Caddy TLS proxy.

No rate limiter is active. Legacy internal/rate code is retained unused.
The OpenAPI schema and protocol document define encodings and trust boundaries.
