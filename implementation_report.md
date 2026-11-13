# KeyBank Implementation Report

> Historical report: this document describes the combined application before
> the API v1 / demo v3 split. Paths, line numbers, and findings below refer
> to that earlier code. See [README](README.md) and
> [the current protocol](docs/protocol.md) for the current implementation.

Independent review of the KeyBank codebase against `mission.md`,
`techstack.md`, and `roadmap.md` (milestones M2 and M8 excluded per request).
Every finding is tagged by severity — **HIGH**, **MEDIUM**, **LOW** —
and by category: *security*, *performance*, *design*, or *idiomatic*.

This report focuses on issues that should change the code. Praise is
deliberately terse; the goal is a punch list of improvements.

---

## Executive summary

The v0.1 critical path is implemented end-to-end and substantially matches
the stated design: zero-knowledge split, atomic `GETDEL` consume-on-read,
RSA-OAEP + AES-GCM hybrid on the browser, strict CSP and security headers,
panic recovery, max-body cap, rate limiting, Docker + Caddy deploy, and CI
with `gosec` / `govulncheck` / Playwright smoke.

The two issues most worth fixing before v0.1 tag are:

1. **Duplicate, divergent storage/rate-limit packages** exist both in the
   repository root (`package main`, actually used) and under
   `internal/{store,rate}` (unused, subtly different implementations). This
   directly undermines the mission statement that the project can be
   audited in an afternoon.
2. **The shipped deploy default silently collapses rate limiting to a
   single bucket.** `TRUST_XFF=false` behind the recommended Caddy profile
   means every request looks like it comes from Caddy's container IP, so
   the global create/fetch budget is 30/60 requests per hour for the whole
   internet.

A handful of other medium-severity items (blocking keygen on the hot path,
fixed-window rate limiter mis-advertised as a token bucket, missing
request-scoped contexts, silently swallowed JSON decode errors) are listed
in detail below.

---

## 1. HIGH-severity findings

### 1.1 Duplicate source trees — `internal/store`, `internal/rate` (design)

**Where:** `internal/store/store.go`, `internal/store/record.go`,
`internal/rate/ratelimit.go` vs. `store.go`, `record.go`, `ratelimit.go` at
the repository root.

**What:** The `internal/` tree is a near-copy of the root files with
diverged behavior:

- `internal/store/store.go` uses `SetNX` with a 3-attempt retry loop and
  uses `errors.Is(err, goredis.Nil)` (idiomatic). The root `store.go` uses
  plain `SET` and `err == goredis.Nil` (pre-1.13 style).
- `internal/rate/ratelimit.go` is essentially identical to
  `ratelimit.go` except for exported identifier names.
- `main.go` imports only the root (`package main`) versions. The
  `internal/*` packages are dead code.

**Why it matters:** The mission statement is explicit — "security can be
audited by reading the code in an afternoon." Two copies of the storage
and rate-limit layers, one unused and subtly improved, is exactly the kind
of trap that sends auditors in circles. It also means future bug fixes
will likely land in only one of the two copies.

**Recommendation:** Pick one. Either delete `internal/*` outright, or
promote it to the production path (moving storage, records, and rate
limiting out of `package main`). The latter also solves the coverage
target gap (see 4.1) by giving those concerns their own test-coverable
package boundaries.

### 1.2 Rate limiter collapses to a single bucket behind the recommended deploy (security)

**Where:** `handlers/middleware.go:113-131` (`remoteIP`), `.env.example`,
`docker-compose.yml`, `docs/deploy.md`.

**What:** The default configuration sets `TRUST_XFF=false`. In the Caddy
compose profile, the KeyBank container only ever sees requests from the
Caddy container's internal Docker IP, so every external client shares the
same rate-limit bucket.

With `RATE_LIMIT_CREATE=30` per hour, the deployed service allows 30
create requests total, globally, per hour — not per IP. The same applies
to fetches.

**Why it matters:** A single noisy client trivially DoSes every other
user. This is the opposite of what the token-bucket description in
`techstack.md` promises.

**Recommendation:**

- Set `TRUST_XFF=true` by default in `.env.example` **when** the Caddy
  profile is enabled, or
- Document loudly in `docs/deploy.md` that `TRUST_XFF=true` is required
  with any reverse proxy, or
- Better: auto-detect the `X-Forwarded-For` header only when the request
  arrives on a private / loopback address and use the last (right-most)
  IP that the proxy wrote, not the left-most client-supplied value (see
  also 2.5 below).

---

## 2. MEDIUM-severity findings

### 2.1 Private key generation is on the request path (performance)

**Where:** `handlers/handlers.go:115` calling `generateKey()` →
`rsa.GenerateKey(rand.Reader, 4096)` at `handlers.go:210`.

RSA-4096 generation takes 50 ms–1 s on typical single-core VPS hardware.
At the default `RATE_LIMIT_CREATE=30/hour`, a single worker can saturate
one CPU core with valid traffic; a small set of IPs can exhaust all
cores. The server's `WriteTimeout=15s` (`main.go:57`) is a secondary
guard, but there is no global keygen concurrency cap.

**Recommendations (ranked):**

1. Fill a small bounded channel with pre-generated keypairs in a
   background goroutine, and pop from it on each create. Refills happen
   under a single semaphore, so peak CPU stays bounded.
2. Re-evaluate RSA-4096. Mission says "at most 7 days." RSA-2048 is
   comfortably secure against a week-long attacker and generates
   ~8–10× faster. `techstack.md` prescribes RSA-4096 — so this is a
   spec discussion, not a unilateral change.
3. The roadmap "stretch" bullet about migrating to X25519 +
   XChaCha20-Poly1305 would obsolete both of the above; it is worth
   promoting from stretch to a concrete post-v0.1 milestone.

### 2.2 Rate limiter is fixed-window, not token-bucket (design / spec drift)

**Where:** `ratelimit.go:22-28` (the Lua script does `INCR` +
`EXPIRE 3600`), `techstack.md:16` ("token bucket"), `roadmap.md:82` ("per-IP
token bucket backed by Redis").

**What:** The script is a classic fixed-window counter with a 1-hour
window. At window boundaries, an IP can burst `2×limit` requests (30 in
the last second of one hour, 30 in the first second of the next). A token
bucket smooths this out.

**Why it matters:** Either the docs are wrong or the implementation is.
Both lie to the reader.

**Recommendation:** Either update `techstack.md`/`roadmap.md` to say
"fixed-window counter," or reimplement with a token bucket (either a Redis
Lua bucket or `golang.org/x/time/rate.Limiter` in-process as the
hybrid-local cache that `techstack.md` already describes).

### 2.3 Silent JSON decode errors on `POST /api/keys` (security / correctness)

**Where:** `handlers/handlers.go:94-102`.

**What:** Only `*http.MaxBytesError` is distinguished. Every other decode
failure (malformed JSON, wrong JSON type for `ttl`, trailing garbage)
falls through to the default TTL. The API silently produces a working
keypair for an obviously broken request.

**Why it matters:** Two concerns. First, it's surprising behavior for API
consumers, who will swear they sent `{"ttl": "1h"}` and got a 24 h key.
Second, it delays debugging of client bugs and hides genuine abuse (e.g.,
scanners probing the endpoint).

**Recommendation:** Return `400 bad_request` on any decode error other
than `io.EOF` (empty body, which intentionally means "default TTL").

### 2.4 `os.Exit(1)` from the listener goroutine bypasses shutdown (design)

**Where:** `main.go:67-70`.

**What:** `os.Exit` inside the goroutine short-circuits the shutdown
defers in `main()` and any slog-handler flushing. If the listener errors
at boot (port in use, permission denied), logs may never appear.

**Recommendation:** Send the error onto a channel that `main()` selects
alongside `sigCtx.Done()`. Exit via `main()` so deferred cleanup runs.

### 2.5 `X-Forwarded-For` uses the left-most IP when trusted (security)

**Where:** `handlers/middleware.go:113-131`.

**What:** When `trustXFF=true`, the code takes the first (left-most)
value. That value is *client-supplied* unless the proxy rewrites it —
most proxies append. An abusive client can prefix fake IPs to defeat the
limiter.

**Recommendation:** Take the right-most IP the proxy added, or configure
a trusted-proxy allowlist. Caddy writes `X-Forwarded-For: <client>[,
<upstream>...]` — in that shape the right-most "near-hop" IP added by
Caddy is the safest value to key on. Document the assumption explicitly.

### 2.6 No health / readiness endpoint (design)

**Where:** `server.go`, `docker-compose.yml`.

**What:** The Compose healthcheck is defined only on Redis. The KeyBank
service has no `GET /healthz` or `GET /readyz`, so `docker compose ps`
cannot tell when the app is actually serving, and any orchestrator (K8s,
systemd's `ExecStartPost`, a front-end LB) needs to infer readiness from
`/` returning 200.

**Recommendation:** Add `GET /healthz` that returns 200 when the handler
is up, and `GET /readyz` that additionally pings Redis with a short
timeout. Neither should be rate-limited.

### 2.7 Request-scoped context is never threaded into storage or rate limiting (design)

**Where:** `store.go:89,96`, `ratelimit.go:43` — all use
`context.Background()`.

**What:** A hung Redis call blocks the goroutine for the full
`WriteTimeout`. Client disconnects don't cancel Redis work. This is also
why the `KeyStore` interface cannot be stubbed with a context-aware mock.

**Recommendation:** Extend the interface to `Save(ctx, rec, ttl)` /
`Load(ctx, id)` / `Allow(ctx, ip)` and pass `r.Context()` from the
handler. The `internal/store` copy already acknowledges this with a TODO
comment; only the adoption is missing.

### 2.8 Playwright smoke runs against `KEYBANK_STORE=mem` (design / testing)

**Where:** `.github/workflows/ci.yml:88`.

**What:** CI spins up the container with the in-memory store and no
Redis. The Redis-backed store, the Redis rate limiter, and the Lua
counter script never run end-to-end in CI.

**Why it matters:** The production code path is specifically the one
that's not smoke-tested. Mission principle 5 ("fail closed") is tested in
isolation but not end-to-end.

**Recommendation:** Add a `redis:7-alpine` service to the job and run
Playwright against `KEYBANK_STORE=redis`, mirroring `docker-compose.yml`.

### 2.9 Redis-store `Save` can theoretically overwrite an existing ID (security, very-low-probability)

**Where:** `store.go:89`.

**What:** Uses `SET` unconditionally. With 128-bit `crypto/rand` IDs the
collision probability is astronomical, but a plain `SET` is
structurally weaker than the `SetNX` + retry approach already implemented
in the unused `internal/store/store.go`.

**Recommendation:** Fold the `internal/store` `SetNX`-with-retry approach
back into the production path (this is a side effect of fixing 1.1).

### 2.10 `envInt` silently ignores negative / non-positive values (idiomatic)

**Where:** `config.go:45-52`.

**What:** Unlike `envDuration`, `envInt` does not log a warning when the
parsed value is ≤ 0. `RATE_LIMIT_CREATE=-1` silently becomes the default
30, not a loud error.

**Recommendation:** Mirror `envDuration` — log a `slog.Warn` and fall
back. Better still, refuse to start.

### 2.11 HSTS header omits `preload`, CSP omits `form-action` for share page specifics (security)

**Where:** `handlers/middleware.go:49-54`.

The CSP shown is reasonable, but some hardening could be considered:

- `Strict-Transport-Security` lacks `preload`. For a privacy-critical
  site hosted on a known domain, opting in is cheap.
- `Permissions-Policy` is not set; consider disabling unused powerful
  features (`camera=(), geolocation=(), microphone=(), ...`) since the
  app uses none of them.
- `Cross-Origin-Opener-Policy: same-origin` and
  `Cross-Origin-Resource-Policy: same-origin` would further reduce the
  side-channel surface; CORP is especially useful on the
  `/api/keys/{id}` PEM response.

These are hardening, not bug fixes.

### 2.12 Rate-limit window is not configurable; only the budget is (design)

**Where:** `ratelimit.go:22-28`.

The Lua script hard-codes `3600`. Only the `limit` count is an env knob
(`RATE_LIMIT_CREATE`, `RATE_LIMIT_FETCH`). Operators who want 60/minute
or 1000/day cannot express it without editing code.

**Recommendation:** Pass the window size as the second Lua argument, or
as a config field parallel to `RATE_LIMIT_CREATE`.

---

## 3. LOW-severity findings

### 3.1 Raw PEM responses lack `Content-Type` (idiomatic)

`handlers/handlers.go:158` writes the PEM body without setting a
`Content-Type`. Go infers `text/plain; charset=utf-8` via sniffing,
blocked by `X-Content-Type-Options: nosniff`. Set
`application/x-pem-file` explicitly for clarity and for any CLI clients.

### 3.2 `err == goredis.Nil` not idiomatic (idiomatic)

`store.go:97` — use `errors.Is(err, goredis.Nil)`. The `internal/store`
copy already does this.

### 3.3 `context.Background()` usage at call sites (idiomatic)

See 2.7; this is both a design and an idiomatic issue.

### 3.4 `Record.CreatedAt` is persisted but unused (design / privacy)

`record.go:6-9`, `store.go:115`. Mission principle: "store only ephemeral
private keys, never the message." Storing a timestamp the server never
reads and never exposes is harmless but contradicts the minimal-data
principle. Either wire it to something (slow-key detection, metrics), or
drop it.

### 3.5 `VendorScript` array is populated only for the create page (design)

`handlers.go:169-173` hard-codes the QR vendor script onto the create
page. Fine, but the `vendorScripts` abstraction is overkill for a single
entry. Inline the script tag in `create.html`, drop the helper type.

### 3.6 `autoHide` setter vs. constructor argument (idiomatic)

`handlers.New(...)` takes dependencies; `SetAutoHideSeconds` is a
post-hoc mutator. Either make it an argument to `New` (or a functional
option) so the struct is immutable after construction.

### 3.7 Template parse paths are stringly-typed and fail only at runtime (idiomatic)

`handlers/handlers.go:47-53`. Moving the template filenames into
constants and `go:embed`-listing them would surface broken paths at
compile time. Minor.

### 3.8 `StoreBackend` falls through to Redis on unknown values (idiomatic)

`main.go:32-40`. `KEYBANK_STORE=memory` (typo) silently starts the Redis
client and blocks on connect. Prefer an explicit whitelist with a fatal
error on unknown values.

### 3.9 `envBool` accepts only a small subset of truthy strings (idiomatic)

`config.go:63-68`. `yes`, `on`, `y` are rejected. Consider
`strconv.ParseBool`, which accepts the standard set; log a warning on
parse failure.

### 3.10 `var _ *http.MaxBytesError` compile-time assertion is obscure (idiomatic)

`ratelimit.go:60`. The intent ("pin Go ≥ 1.19") is opaque without a
comment. A one-line doc comment or — better — a `go.mod` Go-version
directive (already present) is sufficient.

### 3.11 CSS gradient backgrounds do not respect `prefers-color-scheme` (design)

`static/style.css:3`, `:15-26`. `color-scheme: light;` forces a light
palette; dark-mode users get a bright flash. Supporting dark mode is
outside v0.1 but worth a note.

### 3.12 Auto-hide timer does not null plaintext in JS memory (security, informational)

`static/app.js:246-280`. Clearing `output.textContent` does not purge the
plaintext variable from the closure. V8 won't zero memory in any case, so
this is an unfixable-in-JS limitation; mention it in the threat model.

### 3.13 `AUTO_HIDE_SECONDS` has no upper bound (idiomatic)

`setTimeout` with very large values is clamped by the browser. Document
an upper bound of roughly `2^31 / 1000` seconds (~24.8 days) or cap the
parsed value in `config.go`.

### 3.14 README.md grammar and missing "how does it compare" section (cosmetic)

`README.md:2` — "An one-time…" should be "A one-time…". The README
mostly points at itself; a brief "Differences from X / Y" section would
help reviewers who arrive from the competitor services mentioned in
`mission.md`.

### 3.15 `.env.example` defaults diverge from `config.go` defaults (idiomatic)

`config.go` defaults `REDIS_ADDR=localhost:6379`; `.env.example` sets
`redis:6379`. The compose stack overrides this, so runtime is correct,
but a reader of `config.go` who sets up outside Docker gets a different
default than the example file implies. Reconcile and comment.

### 3.16 Dockerfile pins Go via `golang:1.22-bookworm` but does not verify `go.sum` outside the vendored path (design)

`Dockerfile:6` uses `-mod=vendor`. Good. Consider also running
`go mod verify` as a defensive belt-and-braces check during build.

---

## 4. Alignment with the design documents

### 4.1 Coverage target not met on `package main` (spec drift)

`mission.md` and `roadmap.md M1` require ≥ 85 % coverage on `store`,
`record`, and `handlers`. `handlers` is at ~85 %. `store` and `record`
live inside `package main`, whose coverage is 55.8 %. Moving them out
(see 1.1) satisfies the target without writing new tests.

### 4.2 Roadmap says "panic-recover middleware emits a structured log, never leaks the stack" — ✅ implemented

`handlers/middleware.go:26-41`. Stack goes to the logger, the response
body is a bare "internal server error". Matches the spec.

### 4.3 Roadmap says "per-IP token bucket" — ❌ implementation is fixed-window

See 2.2.

### 4.4 Mission principle 2 ("one-time by construction; no background cleanup sweepers") — ✅ upheld

`GETDEL` in `store.go:96`; memory store deletes then checks TTL
(`store.go:53-56`) — correct order to prevent double-reads.

### 4.5 Mission principle 5 ("fail closed" on Redis down) — ✅ upheld in rate limiter

`ratelimit.go:45-46`. Covered by
`TestRedisRateLimiterFailsClosedOnError`.

### 4.6 Mission: "server never touches AES, plaintext, AES key, or salt" — ✅ upheld

No AES code path on the server. All wrapping/unwrapping is in
`static/crypto.js`; the server writes only the private PEM it generated.

### 4.7 `techstack.md` claim: "Hand-rolled Base64URL, roughly ten lines of JS" — closer to 30 lines (cosmetic)

`static/crypto.js:6-40`. Not a problem; just update the number or trim
the helpers.

---

## 5. Test-surface gaps

- No test exercises the `/api/keys/{id}` endpoint's error content-type
  (relates to 3.1).
- No test exercises the `nosniff` + PEM combination; a future browser
  change could break this interaction without warning.
- Playwright's smoke path in CI uses the memory store (2.8). No CI
  coverage of the Redis rate-limit Lua script against a real Redis.
- `handlers/handlers_test.go` does not test malformed-JSON input (see
  2.3) — adding this would both lock the current (lenient) behavior and,
  if 2.3 is fixed, document the strict behavior.

---

## 6. Priority-ordered action list

1. **Resolve the duplicate `internal/*` tree** (finding 1.1). Delete or
   adopt; do not leave both.
2. **Fix the default deploy's rate-limit bucket collapse** (finding 1.2).
   Either flip `TRUST_XFF` under Caddy or surface the requirement in the
   deploy doc and example env.
3. **Reconcile the rate-limiter docs with the implementation** (finding
   2.2). Pick token-bucket or fixed-window and say so everywhere.
4. **Cap keygen concurrency or pre-generate keys** (finding 2.1) — the
   biggest latency and DoS-resilience knob.
5. **Return 400 on malformed create-body JSON** (finding 2.3).
6. **Thread `r.Context()` through the store and rate limiter** (finding
   2.7). Preparatory work for the refactor in 1.1.
7. **Add `/healthz` and `/readyz`** (finding 2.6) so Compose and any LB
   can tell when the app is actually serving.
8. **Switch Playwright smoke to Redis-backed** (finding 2.8) before
   tagging v0.1.
9. **Harden HSTS/CSP/COOP-CORP/Permissions-Policy** (finding 2.11) for
   an audit-pass at launch.
10. Clean up idiomatic nits (`errors.Is`, explicit Content-Type, envBool,
    StoreBackend whitelist) — mostly one-liners, high signal per
    character.

---

*End of report.*
