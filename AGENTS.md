# Repository Guidelines

## Project Structure & Module Organization

KeyBank is a Go 1.22 service. The root `main` package contains startup, configuration, HTTP assembly, storage, and rate limiting. Request handlers and middleware live in `handlers/`; related implementations are under `internal/store/` and `internal/rate/`. HTML templates are in `templates/`, while browser JavaScript and CSS are in `static/`. Unit tests sit beside Go source as `*_test.go`; Playwright tests live in `playwright/`. Deployment files include `Dockerfile`, `docker-compose.yml`, `Caddyfile`, and `docs/deploy.md`. Do not edit dependencies in `vendor/`.

## Build, Test, and Development Commands

- `go run .` starts the service on port 14000; use `KEYBANK_STORE=mem go run .` to develop without Redis.
- `go build ./...` compiles every Go package.
- `go test ./...` runs all unit and integration tests.
- `go test ./... -race -cover` matches the primary CI test check.
- `npm ci` installs the pinned Playwright tooling.
- `npm run playwright:install` installs Chromium; `npm run test:e2e` runs browser tests with a temporary in-memory server.
- `docker compose up --build` runs the application with Redis.

## Coding Style & Naming Conventions

Format Go with `gofmt`; use tabs, idiomatic mixed-cap identifiers, and short lower-case package names. Document exported APIs and handle errors at their boundary. JavaScript uses two-space indentation, semicolons, and `camelCase`. Prefer small consumer-side interfaces, as shown by `handlers.Store`, and keep memory and Redis implementations behaviorally aligned.

## Testing Guidelines

Name Go tests `TestBehavior` and place them beside the code under test. Use table-driven cases, `httptest` for HTTP behavior, and `miniredis` instead of live Redis. Playwright files use `*.spec.js`. Storage or fetch changes must test TTL expiry, concurrency, and exactly-once retrieval. No coverage threshold is configured, but new behavior should include regression coverage.

## Commit & Pull Request Guidelines

History favors concise, imperative subjects such as `Implement milestone 5 client crypto flow`; conventional prefixes like `feat:` are also accepted. Keep each commit focused. Pull requests should explain behavior and security impact, link relevant issues, list verification commands, and include screenshots for UI changes. Ensure CI tests, gosec, govulncheck, and Playwright checks pass.

## Security & Configuration

Copy settings from `.env.example`; never commit `.env` files or secrets. Preserve the core privacy model: plaintext and ciphertext stay client-side, the server stores only private keys, and reads consume keys atomically. Enable `TRUST_XFF` only behind a trusted proxy.
