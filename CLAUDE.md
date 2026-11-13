# Repository working notes

Read `AGENTS.md` for conventions and required checks. KeyBank is a Go API
with a separate browser demo; Go 1.24 source compatibility is retained while
container and CI builds use the patched Go 1.27.4 toolchain; `README.md` is the current entrypoint guide.

- `cmd/keybank`: independent API on port 14000, Redis by default, mem for tests.
- `cmd/demo`: demo on port 14001, HTTP client configured by API_BASE_URL.
- `handlers`: JSON API v1, key generation, credential verification, CORS.
- `internal/store`: one immutable payload store with nondestructive Get and
  conditional atomic Consume. Redis compares exact bytes using Lua; memory
  compares/deletes under a mutex. Never use read followed by unconditional DEL.
- `internal/service`: shared environment config, timeouts, health probes, shutdown.
- `internal/demo`: embedded assets/templates and page rendering. Browser crypto
  uses X25519/HKDF and AES-GCM, or the generated AES key directly.
- `internal/rate`: retained legacy code, unused by the running services.

Run `KEYBANK_STORE=mem go run ./cmd/keybank` and `go run ./cmd/demo` separately.
Use `go build ./...`, `go test ./... -race -cover`, and `go vet ./...`.
`npm ci && npm run playwright:install && npm run test:e2e` starts both services.
Docker targets are `api` (default) and `demo`; Compose includes private Redis
and optional Caddy. See `docs/deploy.md` for configuration and TLS.

API contract: `docs/openapi.yaml`. Crypto, fragments, and trust model:
`docs/protocol.md`. API v1 and demo links v3 intentionally break legacy links.
ECC means X25519, AES means random 32-byte AES-256 material. Optional passphrases
gate retrieval and are stored as salted PBKDF2-SHA256 verifiers (600k iterations).
They do not derive the generated key or encrypt stored records. Wrong credentials
never consume keys. At most one authenticated consumer wins; delivery is not
recoverable after consumption. No rate limiting or lockout is active.

Note plaintext/ciphertext must remain client-side. The API sees keys and
passphrases; responses are raw JSON under TLS without fetch sealing. Never log
IDs, bodies, credentials, or key material. A decrypted traffic recording can
expose keys. Redis remains private, memory-only, noeviction, without a volume.

Do not edit vendor dependencies. The QR source and SRI live under
`internal/demo`; checksums and reporting instructions are in `SECURITY.md`.
