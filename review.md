# KeyBank Implementation Review

> Historical report: this document describes the combined application before
> the API v1 / demo v3 split. Paths, line numbers, and findings below refer
> to that earlier code. See [README](README.md) and
> [the current protocol](docs/protocol.md) for the current implementation.

Reviewed milestones: **M0, M1, M3, M4, M5** against `mission.md`, `techstack.md`,
and `roadmap.md`.

The implementation is close to the spec and the v0.1 happy path works
end-to-end (confirmed by `go test ./...` and the Playwright spec). The findings
below are organized by severity, then by milestone.

---

## Summary

- **High-impact issues:** 3 — rate-limiter fails open on Redis outage
  (violates `mission.md` "Fail closed" principle), record IDs are logged in
  violation of the stated logging policy, and no HTTP server timeouts leave the
  process exposed to slow-loris.
- **Medium-impact:** rate-limiter uses a fixed window (not the token bucket
  the spec calls for), `store` and `record` live in `package main` rather
  than dedicated packages (defeats the 85 % coverage target per-package),
  no SRI/hash for vendored `qrcode.js`, client-side PKCS#1 parsing is a
  self-inflicted complexity.
- **Low-impact:** style nits, missing `MAX_TTL` env var, 308-line CSS vs.
  200-line target, reverse-proxy / XFF handling not configurable, minor
  error-handling choices.

Tests pass; handler package coverage is 85.7 % (meets target);
`package main` coverage is 63.0 % (misses target because `store`/`record`
are not separate packages).

---

## Milestone 0 — Foundation

### Aligned with spec
- URL surface matches: `POST /api/keys`, `GET /api/keys/{id}`, `GET /`,
  `GET /share/{id}` (`server.go:25-34`).
- `handlers/` package extracted, with `handlers.Store` as a narrow
  interface; `storeAdapter` (`store.go:110-125`) bridges to avoid an import
  cycle — good pattern.
- `record.go` and `config.go` extracted from the previous `main.go`.
- `POST /api/keys` returns JSON `{id, pub_pem}` with `201 Created`
  (`handlers/handlers.go:122-131`).
- `log/slog` with JSON handler wired in `main.go:17-18`.

### Deviations / nits
- **README.md** has a few rough edges (e.g., "An one-time …"). Not blocking,
  but the mission says the project's value is trust, so surface polish matters.
- `GET /api/keys/{id}` responds with **raw PEM bytes and no `Content-Type`**
  (`handlers/handlers.go:150-151`). Go will infer `text/plain`. Consider
  setting `application/x-pem-file` (or `application/octet-stream`) and/or
  wrapping in JSON for consistency with the create endpoint — the README
  already documents "PEM bytes" so either direction requires a doc update.
- `main.go:38-46` uses `panic(err)` for startup failures. `log.Fatal` (or a
  structured slog error + `os.Exit(1)`) communicates intent better.

### Code style
- Handler methods are cleanly organized; the `pageData` struct threads a
  single shape through templates.
- `generateKey` (`handlers/handlers.go:199-215`) uses `PKCS1PublicKey`. See
  M5 — this drives the heavy ASN.1 parsing on the client. PKIX/SPKI +
  PKCS#8 would cut hundreds of lines of JS.

---

## Milestone 1 — Record model and expiry

### Aligned with spec
- `Record{PrivPEM, CreatedAt}` with JSON tags, zero-value safe
  (`record.go:5-9`). Round-trip test + zero-value test in
  `record_test.go`.
- `KeyStore` interface is `Save(Record, ttl) (id, error)` /
  `Load(id) (*Record, error)` (`store.go:14-17`). Both `memKeyStore` and
  `redisKeyStore` implement it.
- TTL validation: `60s ≤ ttl ≤ 7d`, otherwise
  `400 {"error":"invalid_ttl"}` (`handlers/handlers.go:102-107`). Boundary
  tests at 59, 60, 604800, 604801 seconds are present
  (`handlers/handlers_test.go:258-302`).
- Atomic consume-on-read via `GETDEL` in `redisKeyStore.Load`
  (`store.go:96`). `miniredis` test confirms it.
- Concurrent `Load` test asserts exactly one winner across 20 goroutines on
  `memKeyStore` (`store_test.go:68-91`).

### Deviations / nits
- **Coverage target missed because of packaging.** The roadmap requires
  ≥85 % on `store` and `record` packages; both live in `package main`
  alongside `main.go`, `config.go`, `ratelimit.go`, and `server.go`. Overall
  `package main` coverage is **63.0 %**. Moving `store.go` + `record.go`
  into `internal/store` (and their tests with them) would both meet the
  target and give the package the encapsulation the spec implies.
- `redisKeyStore.Save` uses `SET` (`store.go:89`). ID collisions on a
  128-bit space are astronomically unlikely, but `SETNX` would make
  clobbering structurally impossible — a cheap defence.
- `redisKeyStore.Load` compares error with `err == goredis.Nil` instead of
  `errors.Is(err, goredis.Nil)` (`store.go:97`). Minor style.
- `redisKeyStore` uses `context.Background()` everywhere. Request-scoped
  contexts (`r.Context()`) would let a handler cancel Redis calls if the
  client disconnects and bound the blast radius of a hanging Redis.
- `memKeyStore.Load` deletes the entry **before** checking expiry
  (`store.go:48-56`). The behavior is correct (expired → return nil, gone
  for good), just worth knowing: an expired-but-not-yet-read entry is
  consumed by the first reader even though it would have failed to
  decrypt.
- `CreateKey` silently falls back to default TTL on any JSON decode error
  other than `MaxBytesError` (`handlers/handlers.go:88-96`). The code
  comment is honest about this, but trailing garbage or an invalid shape
  arguably deserves `400 bad_request`.
- `MAX_TTL` is hard-coded (`handlers/handlers.go:19-22`). `techstack.md`
  lists `MAX_TTL` as an expected `.env.example` variable; either wire it
  through `config.go` or drop it from the spec.

---

## Milestone 3 — Middleware and hardening

### Aligned with spec
- `PanicRecover` logs with stack, returns 500, never leaks the trace to
  the client (`handlers/middleware.go:25-40`, covered by
  `TestPanicRecoverReturns500`).
- `SecurityHeaders` sets every header in the techstack table with the
  exact strings specified (`handlers/middleware.go:43-57`). Every
  endpoint is covered by `TestSecurityHeadersOnAllEndpoints`.
- `MaxBodySize(64*1024)` wraps `POST /api/keys`
  (`server.go:28-30`), and `CreateKey` specifically handles
  `*http.MaxBytesError` → `413` (`handlers/handlers.go:89-93`,
  `TestMaxBodySizeWith413ViaCreateKey`).
- Redis Lua `INCR` + `EXPIRE` script gives per-IP counters with 1-hour
  windows (`ratelimit.go:22-28`). Defaults (30/60/hour) are wired through
  env vars (`config.go:33-34`).

### Security / spec deviations
- **HIGH: Rate limiter fails OPEN on Redis error.** `ratelimit.go:44-47`:
  ```go
  if err != nil {
      // Fail open: don't block requests if Redis is unavailable.
      return true
  }
  ```
  This directly contradicts `mission.md` principle 5 ("Fail closed. If
  Redis is down … return a clean error"). Because the key store is also
  Redis-backed, the practical blast radius is small — `Save`/`Load` will
   500 anyway — but fail-closed is a stated invariant. A fail-closed
  default with an explicit `FAIL_OPEN_ON_REDIS_ERROR` escape hatch would
  match the spec.
- **HIGH: No HTTP server timeouts.** `main.go:44` uses
  `http.ListenAndServe` with the default `*http.Server{}` (no
  `ReadTimeout`, `WriteHeaderTimeout`, `IdleTimeout`). A slow-loris
  client can hold connections indefinitely. Construct an `http.Server`
  explicitly and set sane timeouts (e.g. 10s read/write, 120s idle). This
  is trivially exploitable for a DoS against a self-hoster — mission
  principle 5 again.
- **MEDIUM: Fixed window, not token bucket.** Roadmap M3: "per-IP token
  bucket backed by Redis." The Lua script is a fixed-window counter, which
  allows ~2× the limit at window boundaries. For the stated scale this is
  unlikely to matter, but the spec also promises
  `golang.org/x/time/rate` + Redis counters (`techstack.md:15`). Only the
  Redis half is implemented; no in-process bucket.
- **MEDIUM: Reverse-proxy IP handling.** `remoteIP`
  (`handlers/middleware.go:92-98`) explicitly ignores `X-Forwarded-For` to
  avoid spoofing. Correct default, but the recommended deploy is behind
  Caddy — in that topology every request carries `RemoteAddr = 127.0.0.1`
  and **all clients share one rate-limit bucket**. Add a
  `TRUSTED_PROXY_CIDR` (or `TRUST_XFF=true`) switch so an operator can opt
  in once TLS termination is in front.
- **LOW:** `SecurityHeaders` applies `Cache-Control: no-store` to **every**
  response, including `/static/*`. The spec language is "on all share and
  retrieval responses" — static assets (CSS, `qrcode.js`, `app.js`) are
  reasonable to cache. Split the middleware, or attach `no-store` only to
  the HTML/API routes.

### Code style
- `Chain` iterating backwards to wrap in apply-order is fine; the test
  documents the semantics.
- `remoteIP` comment is precise; good defensive default.

---

## Milestone 4 — Frontend shells

### Aligned with spec
- `templates/base.html` has no inline scripts: only external `<script
  src="…" defer>` / `<script type="module" src="…">` tags
  (`templates/base.html:28-29`). Compatible with
  `script-src 'self'`.
- `create.html` has the textarea + preset/custom expiry picker (1 h / 1 d /
  7 d / custom minutes) + "Generate link" button.
- `share.html` has the Decrypt button, status area, and Keep-Visible
  hook.
- `style.css` is hand-written, single file.
- Routes wired: `GET /` → `HomePage`, `GET /share/{id}` → `SharePage`
  (`server.go:33-34`).
- `Cache-Control: no-store` is observed on share responses via the
  global middleware (tested).

### Deviations / nits
- **CSS length.** 308 lines (`static/style.css`) vs. the stated
  ~200-line target. Not alarming — it's still a single hand-written
  file — but worth a pass for dead rules / consolidation.
- **Responsive breakpoint.** Only one `@media (max-width: 480px)` block
  (`style.css:292-308`). The roadmap says "responsive down to 320px".
  Layouts appear to hold at 320 px because of `width: min(100%, …)` and
  `grid-template-columns: 1fr`, but there is no explicit validation. A
  Playwright smoke at 320 × 568 would close this.
- **Template wiring is tight.** `pageData.BodyTemplate` is compared
  against two literal strings inside `base.html`. A typo in either side
  (`create-body` / `share-body`) silently renders an empty body. A named
  constant or a unit test that asserts both pages render a non-empty body
  (you do this on the rendered HTML — good) covers this.

### Code style
- `data-*` attributes drive JS binding — clean separation.
- `{{range .VendorScripts}}` is a nice hook for the vendored qrcode.js.
- Templates use `html/template` correctly; no `safehtml`/`template.HTML`
  escape hatches.

---

## Milestone 5 — Client crypto

### Aligned with spec
- `crypto.js` is DOM-free: it only touches `crypto`, `TextEncoder`,
  `TextDecoder`, `btoa`, `atob`. No `document`/`window` / no side effects.
  Exports the three functions the spec lists, with the exact signatures.
- Primitives match `techstack.md`: RSA-OAEP (SHA-256) for the key wrap,
  AES-256-GCM with a 96-bit IV, hand-rolled Base64URL.
- `app.js` orchestrates both flows, reads/writes `location.hash` via
  `URLSearchParams`, renders the QR code, enforces the 100 KB soft cap
  with a `<dialog>` modal, auto-hides plaintext after N seconds with a
  "Keep visible" override, and shows an HTTP warning banner
  (`maybeShowHTTPWarning`).
- Playwright covers the happy path, tampered-fragment rejection, and
  auto-hide vs. keep-visible (`playwright/milestone5.spec.js`).

### Security / spec deviations
- **HIGH: record IDs logged.** `handlers.go:122`, `:140`, `:149` log
  `"id", id` at Info/Error level. `techstack.md:16` explicitly states:
  "No request bodies, **no record IDs**, no secrets. Client IP is hashed
  or omitted by default." This is a direct policy violation. If the log
  stream is ever shipped off-box (operator mistake, SIEM pipeline,
  docker-driver default) an attacker with log access can replay any
  unopened ID — which, because `GETDEL` is one-time, silently burns the
  record and the recipient gets a 404. Drop the ID from the log or hash
  it (first-6-hex of HMAC-SHA256).
- **MEDIUM: No Subresource Integrity / pinned hash on `qrcode.js`.**
  `techstack.md:52` says "pinned SHA-256". The file header only names a
  source URL; no `integrity=` on the `<script>` tag and no checksum in
  the repo. Add an `integrity="sha384-…"` attribute **and** a
  `SECURITY.md` line with the expected hash, or a CI check that re-hashes
  on every push. Current SHA-256 of the vendored file is
  `02cf1c7a09475938ac07ebfb3e3c88e7be20998d0207f934beca1b40f8b6bf97`.
- **MEDIUM: Manual PKCS#1 ASN.1 parsing on the client.** `crypto.js`
  spends ~140 lines reconstructing a JWK from the server's PKCS#1 PEM
  because Web Crypto doesn't natively import PKCS#1. Server-side
  generation is already under your control. Emit SPKI (`x509.
  MarshalPKIXPublicKey`) for the public key and PKCS#8
  (`x509.MarshalPKCS8PrivateKey`) for the private key; Web Crypto
  imports them directly with `"spki"` / `"pkcs8"`, and most of
  `crypto.js` goes away. This is a mission-aligned simplification
  ("security can be audited by reading the code in an afternoon").
- **LOW:** `app.js` doesn't disable the "Decrypt" button after a
  successful decrypt. A second click starts a second fetch → 404
  → error banner overwrites the (still-visible) plaintext status. Cosmetic.
- **LOW:** `consumePlaintext` stacks `setTimeout`s if ever called twice
  on the same page (not reachable today because the button isn't
  disabled, but the guard is one line).

### Code style
- `crypto.js` ASN.1 reader is defensive (explicit tag + length checks,
  rejects trailing data). Good.
- `app.js` cleanly separates create/share init and uses `data-*` hooks —
  easy to follow.
- `parseFragment` pre-validates Base64URL regex before handing bytes to
  Web Crypto. Nice: better error messages without weakening the
  GCM auth-tag check that actually matters.

---

## Cross-cutting observations

1. **Logging policy drift.** M5 flagged record IDs; audit every
   `h.logger.*` call against the "no bodies, no IDs, no secrets" line in
   `techstack.md`. Consider a small slog wrapper that strips or hashes
   sensitive fields so the rule is enforced at the boundary.
2. **Fail-closed consistency.** The rate limiter is the explicit
   violation, but the pattern also shows up as "default to allow" on
   malformed request bodies (M1). Grep for `return true` and `falls
   through` in error paths and decide case-by-case whether fail-closed
   is cheap to adopt.
3. **HTTP server hardening.** No timeouts, no graceful shutdown, no
   TLS-redirect middleware (fine if Caddy handles it, but worth a README
   note). A `*http.Server` with `ReadTimeout: 10s`, `WriteTimeout: 15s`,
   `IdleTimeout: 120s`, plus `signal.NotifyContext(SIGINT, SIGTERM)` +
   `Server.Shutdown` closes this.
4. **Packaging for coverage.** Moving `store` + `record` (and `ratelimit`)
   into sub-packages would both satisfy the coverage target and make
   the dependency graph visible at a glance — helpful for the
   auditability goal.
5. **Config completeness.** `MAX_TTL`, a trusted-proxy toggle, and
   optional `HASH_CLIENT_IP=true` for rate-limit key hashing would close
   the gap between `techstack.md`'s promises and `config.go`.

---

## Where the implementation shines

- The atomic `GETDEL` + minimal record struct keeps the one-time
  guarantee at the storage layer instead of in handler logic.
- The `handlers.Store` / `storeAdapter` split is a clean response to the
  Go import-cycle constraint and keeps the handler package testable with
  a fake store.
- Middleware composition is a few dozen lines total and fully covered —
  exactly the "handful of switch cases, every middleware is code the
  project owns" posture the techstack promises.
- Playwright spec exercises the threat model it has to (tamper
  rejection, auto-hide), not just the happy path.
- The HTML/CSS deliberately avoids frameworks; the `data-page` dispatch
  in `app.js` is the right amount of client-side routing for two pages.

Addressing the three HIGH items (fail-open limiter, logged record IDs,
missing server timeouts) plus the client-side PKCS#1 simplification would
leave the v0.1 path spec-aligned and materially easier to audit.
