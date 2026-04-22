# Roadmap

This roadmap follows a crypto-first approach: lock down server-side crypto
contracts and tests before building any UI. Each milestone ends in a
working, tested, committable state with green CI.

## Snapshot of the starting point

- `main.go` serves `/create`, `/fetch/{id}`, and a stub `/page/{id}`.
- `store.go` defines `KeyStore` with `memKeyStore` (tests) and
  `redisKeyStore` (production, atomic `GETDEL`).
- Tests cover one-time semantics for the store and the fetch endpoint.
- No web UI, no client JS, no password gate, no configurable TTL, no
  deployment artifacts.

## Milestone 0 — Foundation

**Goal:** clean working baseline to build on.

- Reshape the URL surface to the target API: `POST /api/keys`,
  `GET /api/keys/{id}`, `GET /`, `GET /share/{id}`.
- Extract `handlers/`, `config.go`, and `record.go` from the current
  monolithic `main.go` without changing behavior.
- Replace the plain-text response of key creation with JSON
  (`{id, pub_pem}`).
- Wire `log/slog` with JSON output and a minimal log format.

**Done when:** existing tests pass after the refactor; the new JSON shape is
documented in `README.md`.

## Milestone 1 — Record model and expiry

**Goal:** the storage layer understands the record shape and honors a
user-chosen TTL.

- Define `Record{PrivPEM, CreatedAt}` in `record.go` (JSON codec,
  zero-value safe). Keep the v0.1 record minimal; password-related
  fields are deferred to a post-v0.1 redesign.
- Update the `KeyStore` interface to `Save(Record, ttl) (id, error)` and
  `Load(id) (*Record, error)`.
- Migrate `memKeyStore` and `redisKeyStore` to the new shape.
- The create handler validates `ttl`: `60s ≤ ttl ≤ 7d`, otherwise
  `400 invalid_ttl`.
- Tests: codec round-trip, TTL boundary values, Redis TTL honored
  (miniredis or real), concurrent `GetDel` remains one-time.

**Done when:** `go test ./...` is green with ≥ 85% coverage on `store` and
`record`.

## Milestone 2 — Password gate (deferred)

**Status:** deferred until after v0.1 ships the core one-time share flow.

The original password-gate design adds too much risk to the first release:
it introduces extra state, complicates the record model, and is easy to get
wrong in a way that weakens the atomic consume-on-read guarantee. v0.1
therefore ships without server-side password validation.

- Keep password-derived state (`PwdHash`, failed-attempt counters, lockout
  behavior) out of the v0.1 record model and API.
- Remove password inputs and wrong-password flows from the v0.1 UI and test
  milestones below.
- Treat possession of the full share URL as sufficient to attempt decryption;
  document that explicitly in the threat model before revisiting this work.
- When this milestone is revived, redesign it around a single atomic
  Redis-side fetch/validate/consume operation. Do not split password checks
  and record deletion across separate storage round trips.

**Done when:** the v0.1 critical path no longer depends on password-gate
work, and any future password milestone starts from an atomic
consume-or-fail contract.

## Milestone 3 — Middleware and hardening

**Goal:** every response is production-safe.

- Panic-recover middleware — returns `500 internal`, emits a structured log,
  never leaks the stack.
- Security-headers middleware covering CSP, HSTS, Referrer-Policy,
  X-Content-Type-Options, X-Frame-Options, Cache-Control.
- Rate limiter: per-IP token bucket backed by Redis. Defaults:
  30 creates/hour, 60 fetches/hour. All knobs configurable through env.
- `http.MaxBytesReader` on POST bodies at 64 KB.
- Tests assert every header is present on every endpoint, and that the
  `N+1`-th request inside the limit window returns `429`.

**Done when:** a `curl -I` audit against a running instance matches the
security-headers table in `techstack.md`.

## Milestone 4 — Frontend shells

**Goal:** static HTML served end-to-end, no crypto logic yet.

- `templates/base.html` with strict CSP and no inline scripts.
- `templates/create.html`: textarea for the message, expiry picker
  (1h / 1d / 7d / custom), "Generate link" button.
- `templates/share.html`: "decrypt" button and status area for errors.
- `static/style.css`: clean minimal layout, responsive down to 320px width.
- Routes: `GET /` → create page, `GET /share/{id}` → share page.

**Done when:** pages render, a stubbed form submission reaches the API,
`Cache-Control: no-store` is confirmed on share responses.

## Milestone 5 — Client crypto

**Goal:** full encrypt / decrypt flow in the browser.

- `static/crypto.js` is DOM-free and exports:
  `generateAesKey()`, `encryptMessage(pubPem, plaintext) → {wrapped, iv, ct}`,
  `decryptMessage(privPem, wrapped, iv, ct) → plaintext`.
- `static/app.js` orchestrates the create and share flows, reads and writes
  `location.hash`, and renders a QR code via vendored `qrcode.js`.
- Soft plaintext cap at 100 KB with a warning modal beyond that.
- Auto-hide decrypted plaintext after 60 seconds (configurable, overridable
  by a "keep visible" click).
- Warn the user if the page is loaded over plain HTTP.
- Playwright tests: end-to-end round-trip, tampered-fragment rejected,
  auto-hide behavior.

**Done when:** a fresh browser context can create a link and decrypt it;
tampered URLs produce a clear error without leaking any plaintext.

## Milestone 6 — Dockerization and deploy docs

**Goal:** one-command self-host.

- Multi-stage `Dockerfile` → distroless final image.
- `docker-compose.yml` with app + Redis + optional Caddy service and a
  sample site file configured for automatic TLS.
- `.env.example` documenting every environment variable.
- `docs/deploy.md` walking through a fresh VPS with Caddy auto-TLS.

**Done when:** `docker compose up` on a fresh VM yields a working KeyBank
reachable at `https://host`.

## Milestone 7 — CI and security scans

**Goal:** every PR goes through the same gates.

- GitHub Actions workflow running on PR and on tag:
  - `go test ./... -race -cover`
  - `gosec ./...`
  - `govulncheck ./...`
  - Playwright smoke against a just-built container
  - Docker image build and tag on release tags
- Dependabot enabled on Go modules.
- `SECURITY.md` with a contact for responsible disclosure.

**Done when:** `main` is green; a red CI blocks merging.

## Milestone 8 — Polish and launch prep

**Goal:** a project a stranger can trust.

- `docs/threat-model.md` — what we defend against, what we do not.
- `CONTRIBUTING.md` with build, test, and audit instructions.
- Refresh `README.md`: install, usage, screenshots, links to `mission.md`,
  `techstack.md`, and the threat model.
- Tag `v0.1.0`.

**Done when:** a first-time visitor can install, use, and audit the project
from the README alone.

## Stretch (post-v0.1, unscheduled)

Revisit after launch feedback — deliberately not sequenced.

- Reintroduce an optional password gate only with an atomic Redis-side
  design (for example, a Lua-backed fetch/validate/consume step).
- Burn-after-reading receipt (a sender-facing notification endpoint that
  trades a small timing side channel for usability).
- If the password gate returns, evaluate Argon2id instead of PBKDF2 if the
  dependency cost is acceptable.
- Migration to X25519 + XChaCha20-Poly1305 for smaller keys and faster
  crypto.
- Optional Tor hidden-service deploy guide.

## Milestone dependencies

v0.1 critical path:

```
M0 → M1 → M3 → M4 → M5 → M6 → M7 → M8
```

Deferred until after v0.1: `M2`.

The v0.1 path stays strictly sequential. `M2` is intentionally out of band
until the core release is complete.
