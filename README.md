# KeyBank

KeyBank is a one-time key generation and retrieval API. A separate secret-note
demo shows how to use it without sending messages or ciphertext to a server.

- Generate X25519 ECC keypairs or random AES-256 keys.
- Optionally require a passphrase to fetch a key.
- Atomically consume keys on successful fetch; expire unread keys automatically.
- Run the API independently of the demo, with Redis or an in-memory store.

## Run locally

Use Go 1.27.4 or a newer patched toolchain. The module retains Go 1.24 source
compatibility; container and CI builds use Go 1.27.4. Run these in separate terminals:

```sh
KEYBANK_STORE=mem go run ./cmd/keybank
API_BASE_URL=http://localhost:14000/api/v1 go run ./cmd/demo
```

Open <http://localhost:14001>. The API listens on port 14000. Native binaries
accept `PORT`; the default CORS allowlist permits the demo on localhost and
127.0.0.1 port 14001. In-memory storage is intended for development and tests.

## API v1

Both POST endpoints require `Content-Type: application/json`. Keys are raw
32-byte values encoded as unpadded base64url. IDs are 32 lowercase hexadecimal
characters. The [OpenAPI specification](docs/openapi.yaml) defines the contract.

Create an ECC keypair:

```sh
curl http://localhost:14000/api/v1/keys \
  -H 'Content-Type: application/json' \
  -d '{"type":"ecc","ttl":3600,"passphrase":"a separately shared passphrase"}'
```

The `201` response contains `id`, `type`, and `public_key`. Fetch using that ID:

```sh
curl http://localhost:14000/api/v1/keys/KEY_ID/fetch \
  -H 'Content-Type: application/json' \
  -d '{"passphrase":"a separately shared passphrase"}'
```

The `200` ECC response contains `id`, `type`, `public_key`, and `private_key`.
Use `"type":"aes256"` to create an AES key: both creation and fetch return
`id`, `type`, and the identical `key`. An omitted or empty passphrase disables
the gate; an unprotected fetch sends `{}`. Passphrases are matched exactly.

`ttl` is seconds: omitted or zero means 24 hours capped by `MAX_TTL`; explicit
nonzero values range from 60 seconds to `MAX_TTL` (at most seven days).

A missing or incorrect passphrase returns `403 invalid_passphrase` without
consuming the record. Missing, expired, and consumed keys return `404 not_found`.
Malformed requests never consume a key. All API errors are JSON; key responses
are marked `Cache-Control: no-store`. A successful fetch cannot be repeated,
including if the response is lost. GET requests never consume keys.

## Demo and security model

The demo offers ECC/AES selection and an optional passphrase. It encrypts notes
with AES-256-GCM locally and puts ciphertext in the share URL fragment. ECC mode
derives its encryption key using X25519 and HKDF-SHA256. Only the recipient's
explicit Decrypt action fetches the key. Wrong passphrases permit retry; opened
notes auto-hide. Passphrases belong in a separate communication channel.

The API stores private or symmetric keys plus salted passphrase verifiers. It
does not receive note plaintext or ciphertext. Passphrases gate retrieval; they
do not derive keys or encrypt stored records. API keys and passphrases travel
directly in JSON: production requires TLS with a trusted termination point.
Decrypted traffic recordings contain usable keys. Avoid body logging at proxies.
A compromised API/Redis, frontend, or endpoint device can compromise notes; this
service does not promise protection against its operator or server compromise.

API v1 and demo fragment v3 intentionally replace the previous protocol.
Existing links are unsupported; legacy records are left to expire in their old
namespace. See [protocol](docs/protocol.md), [deployment](docs/deploy.md), and
[security](SECURITY.md) for details. Rate limiting is currently disabled.

## Verification

```sh
go build ./...
go test ./... -race -cover
go vet ./...
npm ci
npm run playwright:install
npm run test:e2e
```

Playwright starts separate in-memory API and demo servers on ports 4174 and
4173. `PLAYWRIGHT_BASE_URL` instead tests an existing demo and its configured API.

```sh
docker compose up --build
# For public HTTPS, set SITE_ADDRESS and API_BASE_URL=/api/v1 in .env:
docker compose --profile caddy up --build
```

The Dockerfile defaults to an API-only image; `--target demo` builds the frontend.
Both binaries expose `/healthz` and a `healthcheck` subcommand.

## Credit

Inspired by the secure-note-sharing tutorial from [Dusted Codes Limited](https://dusted.codes/building-a-secure-note-sharing-service-in-go).
