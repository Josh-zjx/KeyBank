# Tech Stack

This document describes the technologies used by KeyBank, organized by
layer. Every choice is justified against the mission: zero knowledge, small
dependency surface, self-hostable.

## Server

| Layer          | Choice                                           | Why                                                                                                             |
| -------------- | ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------- |
| Language       | Go 1.22+                                         | Single static binary, first-class `crypto` library, low memory footprint, widely auditable.                     |
| HTTP           | `net/http` (standard library)                    | No framework; routing is a handful of `switch` cases. Every middleware is code the project owns.                |
| Templates      | `html/template` (standard library)               | Context-aware auto-escaping by default. No template-engine dependency.                                          |
| Persistence    | Redis 7+ via `github.com/go-redis/redis/v8`      | Already wired. `GETDEL` gives atomic read-and-delete. `EX` gives expiry for free.                               |
| Rate limiting  | `golang.org/x/time/rate` + Redis counters        | In-process limiter for per-instance fairness; Redis counters for consistency across replicas.                   |
| Logging        | `log/slog` (standard library, Go 1.21+)          | Structured JSON logs. No request bodies, no record IDs, no secrets. Client IP is hashed or omitted by default.  |

## Cryptography — server side

| Primitive                    | Use                                                      | Library                              |
| ---------------------------- | -------------------------------------------------------- | ------------------------------------ |
| RSA-4096 keypair             | Generated once per `/api/keys` request                   | `crypto/rsa`, `crypto/rand`          |
| PEM / PKCS#1                 | Wire format for public and private keys                  | `crypto/x509`, `encoding/pem`        |
| Constant-time compare        | Password hash check against the client-submitted hex     | `crypto/subtle`                      |
| CSPRNG                       | ID generation (128-bit hex), all randomness              | `crypto/rand`                        |

The server never touches AES, the user's plaintext, the unwrapped AES key,
the password, or the PBKDF2 salt. Password hashing (PBKDF2) runs
exclusively in the browser; the server only stores and compares the hex
digest.

## Cryptography — client side

| Primitive                    | Use                                                      | API                                            |
| ---------------------------- | -------------------------------------------------------- | ---------------------------------------------- |
| RSA-OAEP (SHA-256)           | Wrap the per-message AES key with the server's pub key   | `window.crypto.subtle.encrypt` / `.decrypt`    |
| AES-256-GCM                  | Encrypt the plaintext; GCM's auth tag detects tampering  | `window.crypto.subtle.encrypt` / `.decrypt`    |
| CSPRNG                       | AES-256 key, 96-bit IV                                   | `window.crypto.getRandomValues`                |
| PBKDF2-SHA256 (100k iters)   | Derive the password hash submitted to the server         | `window.crypto.subtle.deriveBits`              |
| Base64URL                    | Encode binary into the URL fragment                      | Hand-rolled, roughly ten lines of JS           |

All client cryptography is performed through the browser's Web Crypto API.
No JavaScript cryptographic library is bundled.

## Frontend

| Layer    | Choice                                                   | Why                                                                    |
| -------- | -------------------------------------------------------- | ---------------------------------------------------------------------- |
| HTML     | Go `html/template` serving two pages (`create`, `share`) | Server-rendered shells with no secret content.                         |
| JS       | Vanilla ES2020+, no bundler, no framework                | Every byte is first-party and auditable. No build step.                |
| CSS      | Hand-written, single `style.css`                         | No Tailwind, no PostCSS. Target ~200 lines for the whole app.          |
| QR codes | Vendored `qrcode.js` (MIT), pinned SHA-256               | Single-purpose, small, zero transitive dependencies.                   |

`static/crypto.js` is kept DOM-free and side-effect-free so it can be
exercised by a headless browser test in isolation from the rest of the UI.

## Transport and security

| Concern          | Mechanism                                                                                                                                     |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| TLS termination  | Caddy reverse proxy (recommended) or user-supplied nginx/Traefik. The app binds to `127.0.0.1`.                                               |
| HSTS             | `max-age=15552000; includeSubDomains`.                                                                                                        |
| CSP              | `default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'`. No inline scripts, no third-party origins.   |
| Referrer-Policy  | `no-referrer`. Prevents the `/share/{id}` URL from leaking to any outbound link the rendered plaintext might contain.                         |
| Cache-Control    | `no-store` on all share and retrieval responses.                                                                                              |
| Frame / sniffing | `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`.                                                                                   |
| Request size     | `http.MaxBytesReader` caps POST bodies at 64 KB.                                                                                              |
| Cookies          | None. No sessions. No CSRF tokens (no state-changing form posts; the JSON API is guarded by same-origin policy and CSP).                      |

## Deployment

| Artifact             | Role                                                                                           |
| -------------------- | ---------------------------------------------------------------------------------------------- |
| `Dockerfile`         | Multi-stage build producing a distroless final image.                                          |
| `docker-compose.yml` | Service plus Redis plus optional Caddy with a sample site config.                              |
| `.env.example`       | Documented environment variables: `PORT`, `REDIS_ADDR`, `MAX_TTL`, `RATE_LIMIT_CREATE`, etc.   |
| CI (GitHub Actions)  | `go test -race -cover`, `gosec`, `govulncheck`, Playwright smoke, Docker image build on tags.  |

## Testing

| Tier              | Tooling                                                                                                 |
| ----------------- | ------------------------------------------------------------------------------------------------------- |
| Unit (Go)         | Standard `testing`, table-driven, `net/http/httptest`.                                                  |
| Integration       | `miniredis` if its `GETDEL` support is verified on the pinned version; otherwise real Redis in compose. |
| Browser crypto    | Headless Playwright exercising `crypto.js` in isolation.                                                |
| End-to-end        | One Playwright spec: happy-path round-trip plus wrong-password negative case.                           |
| Security scanning | `gosec`, `govulncheck`. Vendored JS audited on upgrade.                                                 |

## Developer tooling

- `go fmt`, `go vet` enforced by CI.
- `golangci-lint` with a minimal, opinion-free configuration.
- `Makefile` with `run`, `test`, `lint`, `docker` targets.

## Deliberately out of scope

- Databases other than Redis.
- Front-end frameworks (React, Vue, Svelte) and their build toolchains.
- Bundlers (webpack, vite, esbuild).
- Telemetry, analytics, per-user metrics.
- Multi-region or multi-writer replication.
- Native mobile clients.
