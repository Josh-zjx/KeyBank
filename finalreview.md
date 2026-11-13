# KeyBank Implementation Review — Complete Audit

> Historical report: this document describes the combined application before
> the API v1 / demo v3 split. Paths, line numbers, and findings below refer
> to that earlier code. See [README](README.md) and
> [the current protocol](docs/protocol.md) for the current implementation.

**Reviewed milestones:** M0, M1, M3, M4, M5, M6, M7 against `mission.md`,
`techstack.md`, and `roadmap.md`.

**Baseline:** supersedes the partial review in `review.md` (M0–M5 only).

---

## Summary

The implementation has reached a mature state. The three HIGH-severity issues
flagged in `review.md` have all been **fixed**:

1. Rate limiter now fails closed (`ratelimit.go:45-46` returns `false` on Redis error).
2. Record IDs no longer logged — handlers emit only generic strings.
3. HTTP server has timeouts and graceful shutdown (`main.go:52-59`).

Several MEDIUM items also resolved:

4. Server now emits SPKI/PKCS#8 (`handlers/handlers.go:215-231`); `static/crypto.js` imports directly with `"spki"` / `"pkcs8"`. The ASN.1 parsing is gone — `crypto.js` is 108 lines (was ~250).
5. Vendored `qrcode.js` is integrity-pinned in `SECURITY.md` and enforced via `integrity=` on the `<script>` tag in `templates/base.html:28`.

No residual HIGH issues. Remaining gaps are spec-alignment refinements.

Coverage snapshot:

- `handlers`: **84.7 %** (target ≥ 85 %, narrowly missed)
- `package main`: **55.8 %** (target not met because `store`/`record`/`ratelimit` are not separate packages)

---

## Milestone 0 — Foundation

### Aligned with spec
- URL surface matches: `POST /api/keys`, `GET /api/keys/{id}`, `GET /`, `GET /share/{id}` (`server.go:25-34`).
- `handlers/` extracted with a narrow `handlers.Store` interface; `storeAdapter` bridges main.
- `POST /api/keys` returns JSON `{id, pub_pem}` with `201` (`handlers/handlers.go:129-137`).
- `log/slog` JSON handler wired in `main.go:23-24`.

### Deviations / nits
- **LOW:** `GET /api/keys/{id}` returns raw PEM with no explicit `Content-Type` (Go infers `text/plain`). Consider `application/x-pem-file`.
- **LOW:** README.md grammar ("An one-time …"). Cosmetic.

---

## Milestone 1 — Record model and expiry

### Aligned with spec
- `Record{PrivPEM, CreatedAt}` with JSON codec, zero-value safe (`record.go:5-9`).
- `KeyStore` interface: `Save(Record, ttl) (id, error)` / `Load(id) (*Record, error)`.
- TTL validation: `60s ≤ ttl ≤ 7d` → `400 invalid_ttl` (`handlers/handlers.go:108-112`). Boundary tests at 59/60/604800/604801 seconds present.
- Atomic `GETDEL` in `redisKeyStore.Load` (`store.go:96`); concurrent Load test confirms one-time semantics (`store_test.go:68-91`).

### Deviations / nits
- **MEDIUM (unresolved):** `store.go`/`record.go`/`ratelimit.go` sit in `package main`. The roadmap's ≥ 85 % coverage target per package can't be measured against them; `package main` is at 55.8 %. Moving them under `internal/store` and `internal/rate` would both meet the target and cleanly expose dependencies.
- **LOW:** `redisKeyStore.Save` uses `SET` (`store.go:89`) rather than `SETNX`. 128-bit collisions are astronomical but `SETNX` is structurally stronger.
- **LOW:** `redisKeyStore.Load` compares `err == goredis.Nil` (`store.go:97`); idiomatic Go prefers `errors.Is`.
- **LOW:** `redisKeyStore` uses `context.Background()` everywhere. Request-scoped context would allow client cancellation and bound a hung Redis.
- **LOW:** `CreateKey` falls through to default TTL on non-`MaxBytesError` JSON decode errors (`handlers/handlers.go:94-102`) — arguably should `400 bad_request`.
- **LOW (unresolved):** `MAX_TTL` is hard-coded (`handlers/handlers.go:20`). `techstack.md` documents it as an env var; either wire it or drop it from the spec.

---

## Milestone 3 — Middleware and hardening

### Aligned with spec
- Panic-recover middleware returns 500, never leaks stack (`handlers/middleware.go:25-40`).
- Security headers match the techstack table exactly (`handlers/middleware.go:43-57`), covered on every endpoint.
- `MaxBodySize(64 KiB)` on `POST /api/keys` (`server.go:28`); `CreateKey` handles `MaxBytesError` → `413`.
- Rate limiter via Redis Lua `INCR`+`EXPIRE`, defaults 30 creates/hr and 60 fetches/hr, env-tunable.
- **FIXED:** fail-closed rate limiter.
- **FIXED:** HTTP server timeouts — `ReadTimeout: 10s`, `ReadHeaderTimeout: 5s`, `WriteTimeout: 15s`, `IdleTimeout: 120s`, with `signal.NotifyContext` + `Server.Shutdown`.

### Deviations / nits
- **MEDIUM (unresolved):** Fixed-window counter, not token bucket. Roadmap M3 says "per-IP token bucket"; techstack also mentions in-process `golang.org/x/time/rate`. Only the Redis half is implemented and as a fixed window, which allows ~2× at window boundaries.
- **MEDIUM (unresolved):** Reverse-proxy IP trust. `remoteIP` (`handlers/middleware.go:92-98`) ignores `X-Forwarded-For`. Correct default, but behind Caddy every request sees `RemoteAddr = 127.0.0.1` and all clients share one bucket. Needs `TRUSTED_PROXY_CIDR` or `TRUST_XFF` opt-in.
- **LOW:** `Cache-Control: no-store` is applied globally, including `/static/*`. Spec language is "all share and retrieval responses"; static assets can cache.

---

## Milestone 4 — Frontend shells

### Aligned with spec
- `templates/base.html` has no inline scripts; external `<script defer>` / `<script type="module">` only. Compatible with `script-src 'self'`.
- `create.html` has textarea + expiry picker (presets + custom minutes) + generate button.
- `share.html` has Decrypt button + keep-visible toggle + status area.
- `static/style.css` single hand-written file.
- Routes wired; `Cache-Control: no-store` observed on share responses.

### Deviations / nits
- **LOW:** CSS at 308 lines vs. ~200-line target. Readable, but consolidation pass could help.
- **LOW:** Only one `@media (max-width: 480px)` block. Roadmap says "responsive down to 320px" — holds through `min()` / `1fr`, but no Playwright smoke at 320×568.
- **LOW:** `pageData.BodyTemplate` is compared against literal strings inside `base.html`. A typo silently yields an empty body; a unit test that both pages render non-empty body covers it (exists).

---

## Milestone 5 — Client crypto

### Aligned with spec
- `crypto.js` DOM-free, exports `generateAesKey`, `encryptMessage`, `decryptMessage` with spec signatures.
- Primitives match: RSA-OAEP (SHA-256), AES-256-GCM (96-bit IV), hand-rolled Base64URL.
- `app.js` orchestrates both flows, reads/writes `location.hash`, renders QR, enforces 100 KB soft cap, auto-hides plaintext after N seconds with keep-visible override, warns on plain HTTP.
- Playwright covers happy path, tampered-fragment rejection, and auto-hide (`playwright/milestone5.spec.js`).
- **FIXED:** server emits SPKI/PKCS#8, client imports with `"spki"`/`"pkcs8"`; no more hand-rolled ASN.1.
- **FIXED:** `qrcode.js` integrity documented and enforced via SRI.

### Deviations / nits
- **LOW:** Decrypt button is not disabled after a successful decrypt; second click re-fetches and errors, overwriting the still-visible plaintext status. Cosmetic race, one-line fix.
- **LOW:** `consumePlaintext` stacks `setTimeout`s if ever called twice. Not currently reachable; a flag guard is one line.

---

## Milestone 6 — Dockerization and deploy docs

### Aligned with spec
- **Dockerfile:** multi-stage (build on Go 1.22 → `gcr.io/distroless/static-debian12:nonroot`), `CGO_ENABLED=0`, `-trimpath -ldflags='-s -w'`, vendored deps (`-mod=vendor`), nonroot user, sane ENV defaults, `EXPOSE 14000`.
- **docker-compose.yml:** three services — `app` (bound to `127.0.0.1:${PORT}`, depends-on-redis-healthy), `redis` (7-alpine, persistent volume, healthcheck), `caddy` (opt-in via `profiles: ["caddy"]`, ports 80/443, site address env-driven).
- **Caddyfile:** minimal, clean — `reverse_proxy app:{$PORT}` + compression.
- **.dockerignore:** excludes `.git`, `.github`, `node_modules`, test outputs, local binaries.
- **docs/deploy.md:** walks fresh VPS bring-up (env, `docker compose up`, Caddy profile, verification, persistent volumes, upgrade path).

### Deviations / nits
- **LOW (unresolved):** `.env.example` does not document `MAX_TTL` (because the code doesn't read it). Either wire the env var and document it or drop it from the techstack list.
- **INFO:** Dockerfile ENV defaults are redundant with explicit compose `environment:` entries. Harmless; compose wins.
- **INFO:** Deploy guide could call out more prominently that the app binds loopback-only without Caddy.

---

## Milestone 7 — CI and security scans

### Aligned with spec
- **`.github/workflows/ci.yml`:** triggers on pull_request and tag pushes with concurrency cancellation. Jobs:
  1. `go test ./... -race -cover`
  2. `gosec ./...` (securego/gosec@v2.25.0)
  3. `govulncheck ./...` (golang/govulncheck-action@v1)
  4. Docker build + Playwright smoke against the container (in-memory store)
  5. Release tagging step (conditional on `refs/tags/`)
- **`.github/dependabot.yml`:** gomod + npm, weekly, max 5 open.
- **SECURITY.md:** reporting channel (GitHub private reporting + contact), SLAs, vendored asset table with SHA-256 and SRI.

### Deviations / nits
- **MEDIUM (unresolved):** `techstack.md` says "`go fmt`, `go vet` enforced by CI". No job runs either. Pass locally, but not gated.
- **LOW:** No explicit coverage threshold gate in CI. `go test -cover` runs but doesn't fail on regressions.
- **INFO:** Release step tags the image but doesn't push to a registry. Intentional for self-hosted; a clarifying comment would help.

---

## Cross-cutting observations

1. **Logging policy:** re-audited. No record IDs, plaintexts, or secrets logged. Panic recovery logs `"panic"` and `"stack"`, which is fine.
2. **Fail-closed posture:** rate limiter now fails closed; server has timeouts. The only residual fall-through is malformed-JSON → default TTL in `CreateKey`, which is low risk.
3. **Packaging:** `store.go`, `record.go`, `ratelimit.go` still in `package main`. This blocks per-package coverage measurement and obscures the dependency graph.
4. **Config completeness:** `MAX_TTL`, `TRUSTED_PROXY_CIDR` / `TRUST_XFF`, and optional client-IP hashing are not wired. Other env vars (`PORT`, `REDIS_ADDR`, `KEYBANK_STORE`, `AUTO_HIDE_SECONDS`, `RATE_LIMIT_CREATE/FETCH`) are plumbed correctly.
5. **Frontend polish:** CSS lightly over the target, 320 px breakpoint untested, decrypt button double-click guard missing — all cosmetic.
6. **Deployment experience:** `docker compose up -d --build` gives a working stack; Caddy profile adds TLS automation. Clear, concise.
7. **Security scanning:** `gosec` and `govulncheck` in CI; vendored JS audited and SRI-pinned.

---

## Status of prior review's findings

| Issue | Status |
|---|---|
| Rate-limiter fail-open | FIXED |
| Record IDs logged | FIXED |
| No HTTP timeouts | FIXED |
| Manual PKCS#1 ASN.1 on client | FIXED (SPKI/PKCS#8) |
| `qrcode.js` no SRI | FIXED |
| Fixed-window limiter (not token bucket) | Unresolved |
| Reverse-proxy IP trust knob missing | Unresolved |
| `Cache-Control: no-store` on statics | Unresolved |
| CSS > 200 lines | Unresolved |
| 320 px responsiveness unvalidated | Unresolved |
| `MAX_TTL` hard-coded | Unresolved |
| Decrypt button not disabled | Unresolved |
| `consumePlaintext` reentrancy | Unresolved |
| `store`/`record` in `package main` | Unresolved |
| `go fmt` / `go vet` not CI-gated | Unresolved |

---

## Sub-tasks for outstanding work

### HIGH (spec-alignment; recommended before v0.1)

- **T1 — Wire `MAX_TTL` env var.** Files: `config.go`, `handlers/handlers.go`, `.env.example`, `docker-compose.yml`. Add `MaxTTL time.Duration` to `Config`, default `7*24h`, parse a Go duration string from `MAX_TTL`. Pass to `Handlers` and use in `CreateKey` TTL validation. Document in `.env.example`.
- **T2 — Add `go fmt` / `go vet` gates to CI.** File: `.github/workflows/ci.yml`. Two jobs: `gofmt -l .` (fail if any output) and `go vet ./...`.
- **T3 — Move `store.go`/`record.go`/`ratelimit.go` into `internal/store` and `internal/rate`.** Updates imports in `main.go`, `server.go`, `config.go`, `handlers/*`. Move tests alongside. Verify `go test ./... -cover` shows ≥ 85 % on each new package.

### MEDIUM

- **T4 — Trusted-proxy config for rate limiter.** Add `TRUST_XFF` or `TRUSTED_PROXY_CIDR` to `Config`; extend `remoteIP` to read `X-Forwarded-For` only when trusted.
- **T5 — Split `Cache-Control: no-store` off static assets.** Apply security headers without `no-store` for `/static/*`, or move `no-store` to the HTML/API handlers.
- **T6 — Disable decrypt button after success; guard `consumePlaintext` reentrancy.** One-line changes in `static/app.js`.
- **T7 — CSS consolidation pass** toward the ~200-line target.
- **T8 — Playwright smoke at 320 px viewport** in `playwright/milestone5.spec.js`.

### LOW / STRETCH

- **T9 — Replace fixed-window limiter with token bucket** (Lua + `golang.org/x/time/rate`) once M-tier items settle.
- **T10 — Coverage threshold gate in CI.**
- **T11 — `redisKeyStore` request-scoped contexts + `errors.Is` + `SETNX`.**
- **T12 — `GET /api/keys/{id}` explicit `Content-Type`.**
- **T13 — Deploy docs callout: app binds loopback-only without Caddy.**

---

## Where the implementation excels

- Atomic `GETDEL` keeps the one-time guarantee at the storage layer rather than in handler logic.
- SPKI/PKCS#8 on the wire cut `crypto.js` to ~108 lines; the crypto surface is auditable in one sitting.
- Middleware stack is small, fully tested, and every middleware is project-owned.
- TLS via Caddy profile is one environment variable and one `docker compose --profile caddy up` away.
- Playwright exercises the adversarial cases (tampered fragment, auto-hide) that matter, not just the happy path.
- No frontend framework, no bundler; every byte is first-party and auditable.
- Graceful signal handling with bounded shutdown.

Remaining work is refinement. The v0.1 critical path is spec-aligned and the
code base is ready for audit.
