# Copilot Instructions

## Build and test commands

- Build everything with `go build ./...`.
- Run the full test suite with `go test ./...`.
- Run a single test with `go test ./... -run '^TestFetchEndpointIsOneTime$'`. Replace the regex with another test name as needed.
- Run the server with `go run main.go`.
- Override runtime defaults with environment variables, for example: `PORT=8080 REDIS_ADDR=localhost:6379 go run main.go`.
- There is no checked-in lint command or lint configuration in the current repository state. `techstack.md` mentions future linting and CI tooling, but those files are not present yet.

## High-level architecture

- The current implementation is a small `main` package split across `main.go`, `store.go`, and `main_test.go`.
- `main()` reads `REDIS_ADDR` and `PORT`, constructs `Server{store: newRedisKeyStore(...)}`, and serves all traffic through `Server.webServer`.
- `Server.webServer` is the only HTTP handler. It currently serves three `GET` routes:
  - `/create` generates an RSA-4096 keypair, stores the private key through the storage abstraction, and returns the generated ID plus the PEM-encoded public key in a plain-text response.
  - `/fetch/{id}` loads and returns the private key for that ID.
  - `/page/{id}` is still a placeholder response.
- `KeyStore` in `store.go` is the main seam between HTTP behavior and persistence. Production uses `redisKeyStore` in `main.go`; tests use `memKeyStore` in `store.go`.
- One-time retrieval is enforced in the storage layer, not in the handler: `redisKeyStore.Load` uses Redis `GETDEL`, while `memKeyStore.Load` deletes the entry from its in-memory map before returning it.
- `randomID()` lives in `store.go` and is shared by both store implementations. It generates 16 random bytes and hex-encodes them, so IDs are 32-character lowercase hex strings.
- `main_test.go` focuses on the one-time contract at two levels: directly against the in-memory store and through the `/fetch/{id}` handler with `net/http/httptest`.

## Key conventions

- Keep the privacy model intact: the server stores only private keys, while ciphertext is expected to stay client-side and travel outside the server in the share URL. `README.md`, `mission.md`, and the current handlers all assume that design.
- When changing persistence behavior, keep `memKeyStore` and `redisKeyStore` behaviorally aligned. The test store is the fast substitute for production semantics, especially the delete-on-read contract.
- Prefer the existing small-surface approach: standard-library HTTP and crypto packages plus the Redis client. The docs explicitly position auditability and minimal dependencies as project goals.
- Treat `roadmap.md` and `techstack.md` as target-state guidance, not as a description of the code that exists today. They describe future packages, routes, UI, CI, and lint tooling that have not been implemented yet.
- Current endpoint behavior is intentionally simple and test-coupled: routes are string-switched in `webServer`, requests are `GET` only, and `/create` responds with plain text rather than JSON.
