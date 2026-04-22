# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

KeyBank is a one-time cryptographic key generation and retrieval service written in Go. It implements a privacy-first secure note sharing model: the server generates RSA key pairs and stores only the private key (in Redis), returning the public key to the client. The message ciphertext is embedded in the share URL client-side — the server never sees the plaintext.

## Commands

```bash
# Run the server (defaults to port 14000)
go run main.go

# Run with a custom port
PORT=8080 go run main.go

# Build
go build ./...

# Run tests
go test ./...

# Tidy dependencies
go mod tidy
```

## Architecture

The entire service lives in `main.go` as a single package. Key components:

- **`webServer`** — HTTP handler routing three endpoints:
  - `GET /create` — generates an RSA 4096-bit keypair, stores the private key, returns the ID and public key
  - `GET /fetch/{id}` — retrieves the private key by ID (stub: `loadPrivateKey` not yet implemented)
  - `GET /page/{id}` — serves the decryption page (stub)
- **`generateKey`** — produces RSA 4096-bit keypairs encoded as PEM
- **`savePrivateKey`** / **`loadPrivateKey`** — stubs for Redis persistence; `Server.RedisCache` (`go-redis/cache/v8`) is declared but not yet wired into the handler

## Incomplete / In-Progress

- `savePrivateKey` currently returns the private key bytes directly (no Redis write)
- `loadPrivateKey` is a passthrough stub
- The `Server` struct with `RedisCache` is declared but never instantiated or passed to `webServer`
- `/fetch/` and `/page/` routes log to stdout but return no response body
