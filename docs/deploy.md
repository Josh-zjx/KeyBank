# Deploy the API and demo

Requires Docker Engine, Compose, and a DNS name pointing at your host for public
TLS. The stack contains independent API and demo containers, private memory-only
Redis, and an optional Caddy TLS proxy.

## Local development

```sh
cp .env.example .env
docker compose up -d --build
```

Open http://localhost:14001. API v1 is at http://localhost:14000/api/v1.
Both published container ports bind to host loopback only. Redis is not published.
For API-only use, run `docker compose up -d --build api`.

Native development uses Go 1.27.4 or a newer patched toolchain; container and CI
builds pin Go 1.27.4. The module retains Go 1.24 source compatibility.
Without Redis, use two terminals:

```sh
KEYBANK_STORE=mem go run ./cmd/keybank
API_BASE_URL=http://localhost:14000/api/v1 go run ./cmd/demo
```

Native `PORT` defaults are 14000 for API and 14001 for demo. Configuration:

| Variable | Default | Meaning |
|---|---|---|
| KEYBANK_STORE | redis | API persistence: redis or mem |
| REDIS_ADDR | localhost:6379 | API Redis address; redis:6379 in Compose |
| MAX_TTL | 168h | TTL ceiling, 60s–168h; set consistently in API and demo |
| API_BASE_URL | http://localhost:14000/api/v1 | Browser-facing API URL, not a Docker service hostname |
| CORS_ALLOWED_ORIGINS | localhost/127.0.0.1 HTTP origins on port 14001 | Comma-separated exact origins; empty disables cross-origin access |
| AUTO_HIDE_SECONDS | 60 | Demo plaintext display duration |
| API_PORT / DEMO_PORT | 14000 / 14001 | Compose loopback host ports; update API URL/CORS if changed |
| REDIS_MAXMEMORY | 256mb | Compose Redis memory cap |
| SITE_ADDRESS | localhost | Caddy public hostname |

Malformed settings fail startup. No rate-limiter settings are used.

## Public TLS on one origin

Set these in `.env`:

```dotenv
SITE_ADDRESS=notes.example.com
API_BASE_URL=/api/v1
CORS_ALLOWED_ORIGINS=
```

Then start:

```sh
docker compose --profile caddy up -d --build
```

Caddy obtains TLS certificates and routes `/api/*` to the API and everything
else to the demo. Open ports 80 and 443 publicly; keep API/demo host ports and
Redis private. Same-origin requests do not need a cross-origin allowlist.

For different API/frontend domains, set API_BASE_URL to the public HTTPS API
URL including `/api/v1`, and CORS_ALLOWED_ORIGINS to the demo's exact HTTPS
origin. Configure the external reverse proxies for the corresponding services.
The demo automatically adds its configured API origin to CSP connect-src.

## Trust and operations

Keys and passphrases are JSON over TLS, without application-level fetch sealing.
A TLS-terminating CDN/proxy or decrypted traffic recording can expose usable
keys and credentials. Terminate TLS on infrastructure you trust. Keep
request/response body logging, HAR collection, and proxy capture disabled for
these endpoints. The service does not log key IDs, material, or passphrases.

Redis uses no RDB snapshots, no AOF, and no data volume. Restarting it destroys
unread keys. Keep Redis private: its records contain raw private/symmetric keys
and salted passphrase verifiers. The noeviction policy refuses new creates when
full; conditional deletion can still free existing records. Size memory to your
workload rather than relying on historical per-note estimates.

```sh
docker compose ps
curl --fail http://localhost:14000/healthz
curl --fail http://localhost:14001/healthz
docker compose logs api demo
```

Each image runs `/app/service healthcheck`, probing its own `/healthz`.
Health probes establish process liveness, not Redis readiness. Caddy waits for
both services' health checks; the API depends on Redis's health check at startup.

Build images separately with `docker build --target api -t keybank-api .` and
`docker build --target demo -t keybank-demo .`. The default Docker target is API.
Rebuild with `docker compose --profile caddy up -d --build` to update services.
Restarting only the API/demo preserves keys held in a running Redis.

## Breaking migration

Stop/remove the old combined `app` service when upgrading (`docker compose
--profile caddy up -d --build --remove-orphans`). The new services replace its
root executable, endpoints, and deployment settings. Old share links are
unsupported. New records use a separate Redis namespace; old records expire
without conversion. Ask senders to create new links.

If an older stack used a Redis data volume, it may retain deleted key material.
Inspect old volumes and remove obsolete Redis backups according to your
retention policy. Caddy's certificate/configuration volumes remain in use.
