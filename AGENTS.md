# Repository Guidelines

## Project Structure & Module Organization

KeyBank retains Go 1.24 source compatibility and uses Go 1.27.4 for container and CI builds, with independent API (`cmd/keybank`) and demo (`cmd/demo`) executables. `handlers/` implements API v1; `internal/store/` is the single persistence implementation. `internal/demo/` embeds its own `templates/` and `static/` assets. `internal/service/` shares process startup and configuration. `internal/rate/` remains unused legacy code. Tests sit beside Go source; browser tests are in `playwright/`. Do not edit dependencies in `vendor/`.

## Build, Test, and Development Commands

- `go run ./cmd/keybank` starts the API on port 14000; use `KEYBANK_STORE=mem` without Redis. Run `go run ./cmd/demo` separately on port 14001.
- `go build ./...` compiles every Go package.
- `go test ./...` runs all unit and integration tests.
- `go test ./... -race -cover` matches the primary CI test check.
- `npm ci` installs the pinned Playwright tooling.
- `npm run playwright:install` installs Chromium; `npm run test:e2e` runs browser tests with separate temporary API and demo servers.
- `docker compose up --build` runs the API, demo, and Redis.

## Coding Style & Naming Conventions

Format Go with `gofmt`; use tabs, idiomatic mixed-cap identifiers, and short lower-case package names. Document exported APIs and handle errors at their boundary. Always use structured logging when implementing logging functionality. JavaScript uses two-space indentation, semicolons, and `camelCase`. Prefer small consumer-side interfaces, as shown by `handlers.Store`, and keep memory and Redis implementations behaviorally aligned.

## Testing Guidelines

Name Go tests `TestBehavior` and place them beside the code under test. Use table-driven cases, `httptest` for HTTP behavior, and `miniredis` instead of live Redis. Playwright files use `*.spec.js`. Storage or fetch changes must test TTL expiry, concurrency, and exactly-once retrieval. No coverage threshold is configured, but new behavior should include regression coverage.

## Commit & Pull Request Guidelines

History favors concise, imperative subjects such as `Implement milestone 5 client crypto flow`; conventional prefixes like `feat:` are also accepted. Keep each commit focused. Pull requests should explain behavior and security impact, link relevant issues, list verification commands, and include screenshots for UI changes. Ensure CI tests, gosec, govulncheck, and Playwright checks pass.

## Security & Configuration

Copy settings from `.env.example`; never commit `.env` files or secrets. Preserve the core model: note plaintext and ciphertext stay client-side; the API stores private/symmetric keys and optional salted verifiers. Fetch authenticates before conditional atomic consumption. Keys and passphrases travel directly in JSON over TLS; never log bodies or claim recorded decrypted traffic is safe. Update `docs/protocol.md` and `docs/openapi.yaml` for API or crypto changes. Passphrases never belong in links. No rate limiting is wired into either service.
