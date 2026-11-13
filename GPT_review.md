# KeyBank review

> Historical report: this document describes the combined application before
> the API v1 / demo v3 split. Paths, line numbers, and findings below refer
> to that earlier code. See [README](README.md) and
> [the current protocol](docs/protocol.md) for the current implementation.

This review focused on cryptographic guarantees, security posture, critical performance risks, and user availability. It is based on direct inspection of the current code plus parallel review threads for crypto, performance/availability, and documentation drift.

## Executive summary

The core split-knowledge design is sound in the happy path: the browser encrypts locally, the server stores only a one-time private key, and Redis `GETDEL` preserves atomic consume-on-read semantics. The biggest problems were around **how easy it was to invalidate that model operationally**: transport safety, cross-site note burning, reverse-proxy trust, and deployment defaults that did not match the documented guarantees.

The highest-value issues identified were:

| Severity | Area | Finding | Evidence |
| --- | --- | --- | --- |
| **Critical** | Security / availability | `GET /api/keys/{id}` let any third-party page, bot, or prefetch consume a note without reading it. | `server.go`, `handlers/handlers.go`, `static/app.js` |
| **Critical** | Cryptography / deployment | Docker Compose enabled Redis AOF persistence by default, writing one-time private keys to disk and increasing backup/remanence risk. | `docker-compose.yml` |
| **High** | Security | The browser flow only warned on plain HTTP, even though public HTTP allows active script tampering that defeats the crypto story. | `templates/base.html`, `static/app.js` |
| **High** | Security / availability | Reverse-proxy handling was incomplete: docs recommended Caddy, but rate limiting depended on `TRUST_XFF`, and trusting XFF blindly from arbitrary peers is unsafe. | `config.go`, `handlers/middleware.go`, `docs/deploy.md` |
| **Medium** | API hardening | Malformed JSON on `POST /api/keys` silently fell back to the default TTL instead of returning a client error. | `handlers/handlers.go` |
| **Medium** | Documentation drift | README and tech stack docs overstated guarantees and documented the wrong key format / fetch semantics. | `README.md`, `techstack.md` |
| **Medium** | Performance / availability | RSA-4096 generation happens on the request path and can become a CPU hotspot under sustained create traffic. | `handlers/handlers.go` |
| **Medium** | UX / availability | The 100 KB soft plaintext limit can still produce brittle share URLs for browsers and messaging clients. | `static/app.js` |

## Detailed findings

### 1. Cross-site note burning via bare fetch endpoint

`/api/keys/{id}` was exposed as a plain `GET`. Anyone who learned only the record ID could destroy the note by triggering the URL from another page, image tag, crawler, or prefetcher. They would not learn the plaintext, but they could still make the real recipient fail permanently.

This is a real availability and trust-boundary bug because the share ID is in the path, while the ciphertext lives in the fragment. The fragment never reaches the server, but the path does.

### 2. Public HTTP weakened the cryptographic guarantee too much

The app showed a warning banner over HTTP, but still allowed create/decrypt flows. On a public non-local HTTP deployment, any active network attacker can tamper with the served JavaScript and steal plaintext or the fetched private key. That is not a minor warning; it defeats the whole security model.

### 3. Redis persistence default was too durable for one-time private keys

The Compose stack configured Redis with `--appendonly yes`. That improves crash recovery, but it also means one-time private keys are written to disk and can survive deletion in AOF history, backups, or copied volumes. For this application, that is a meaningful cryptographic guarantee regression.

### 4. Reverse-proxy trust and rate limiting needed harder edges

The common deployment story is “run behind Caddy,” but per-client rate limiting only works correctly when the app trusts forwarded client IPs. At the same time, trusting `X-Forwarded-For` from arbitrary peers is unsafe. This needs both safer code and clearer operator guidance.

### 5. Bad JSON was accepted too leniently

`POST /api/keys` treated malformed JSON as “use the default TTL.” That hides client bugs and makes the API harder to reason about.

### 6. Docs overstated what “zero knowledge” means here

The implementation keeps plaintext and ciphertext fragments off the server in the normal flow, but the repo docs sometimes implied stronger guarantees such as “no data leak even when server hacked.” That is too strong. A compromised host can still steal pending private keys, tamper with JS, or benefit from deployment mistakes.

### 7. Remaining non-blocking risks

These are real but were not the first fixes I chose for this branch:

1. RSA-4096 key generation happens synchronously on each create request and can be a CPU hotspot.
2. The current soft plaintext limit can still generate fragile URLs.
3. The rate limiter is still a fixed-window counter, not a token bucket.

## Work dispatched

The review and remediation work was split across subagents:

1. `perf-review` — availability and performance findings
2. `docs-review` — documentation and guarantee drift
3. `crypto-review` — cryptography and security review
4. `runtime-hardening` — runtime/deployment default hardening implementation

## Status on this branch

This branch leaves the codebase safer than the review started with.

### Fixed on `copilot/review`

1. `/api/keys/{id}` now uses a same-origin `POST` plus `X-Keybank-Fetch: 1`, removing the bare cross-site `GET` burn path.
2. The browser app now refuses create/decrypt flows over plain HTTP on non-local addresses instead of only warning.
3. `POST /api/keys` now rejects malformed JSON with `400 {"error":"invalid_request"}` instead of silently defaulting TTL.
4. The handler now honors the configured max TTL and returns the private key with an explicit `application/x-pem-file` content type.
5. `TRUST_XFF` is now only honored from loopback/private proxy hops, reducing spoofing risk when operators enable it.
6. Bare/local runs now default to `BIND_ADDR=127.0.0.1`, while the container image and compose stack explicitly use `BIND_ADDR=0.0.0.0`.
7. Redis append-only persistence is now opt-in via `REDIS_APPENDONLY=yes` instead of on by default.
8. README / deploy / tech stack docs were updated to match the actual key formats, fetch semantics, transport requirements, and Redis durability tradeoff.

### Still worth future work

1. RSA-4096 key generation is still on the create request path and can become a CPU hotspot under load.
2. The current share-link size guidance is still soft; large notes can still generate brittle URLs.
3. The rate limiter remains a fixed-window counter rather than a token bucket.
