# Copilot instructions

Read `AGENTS.md` and `CLAUDE.md` for current repository conventions.

- Use Go 1.27.4 or a newer patched toolchain; the module retains Go 1.24 source compatibility.
- Build with `go build ./...`; verify with `go test ./... -race -cover` and `go vet ./...`.
- Run `KEYBANK_STORE=mem go run ./cmd/keybank` (API :14000) and `go run ./cmd/demo` (:14001).
- Run `npm ci`, `npm run playwright:install`, and `npm run test:e2e` for both browser services.
- API v1: POST `/api/v1/keys` with type ecc/aes256, optional TTL/passphrase;
  POST `/api/v1/keys/{id}/fetch` with optional passphrase. JSON only.
- ECC uses X25519 raw keys; AES uses random 256-bit keys. Wire encoding is unpadded base64url.
- Store immutable snapshots through `handlers.Store`. Verify credentials before
  conditional atomic consumption. Never read then unconditionally delete.
- `internal/store` is the single persistence implementation; `internal/demo`
  owns frontend assets/pages; `internal/service` shares process setup.
- Keep note plaintext/ciphertext client-side. Keys/passphrases are JSON under TLS,
  without fetch sealing; never log or render these secrets in the request ledger.
- No rate limiter is active. `internal/rate` is unused legacy code.
- Protocol changes require `docs/protocol.md` and `docs/openapi.yaml` updates.
- Demo links use fragment v3. Legacy endpoints and links are intentionally unsupported.
- Do not edit vendored dependencies. Preserve TTL, concurrency, and one-time tests.
