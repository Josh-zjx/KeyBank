# Deploy KeyBank with Docker Compose

This milestone ships a single Compose stack for **KeyBank + Redis**, with an
optional **Caddy** profile for automatic HTTPS. The app itself is always bound
to `127.0.0.1:${PORT}` on the host, so enabling Caddy does not expose the app
directly to the internet.

## Prerequisites

- A Linux VPS with Docker Engine and the Docker Compose plugin installed
- A DNS record pointing your public host name at the VPS
- Ports **80** and **443** open to the internet if you want Caddy to obtain and
  renew public TLS certificates

## 1. Prepare the environment

Copy the example environment file and edit it for your host:

```bash
cp .env.example .env
```

At minimum, set:

- `SITE_ADDRESS` to your public DNS name, for example `notes.example.com`
- `PORT` only if you want the app's loopback-only host port to be something
  other than `14000`

Leave `REDIS_ADDR=redis:6379` and `KEYBANK_STORE=redis` when using the shipped
Compose stack. Leave `TRUST_XFF=true` when using Caddy so rate limits use each
client's forwarded address; set it to `false` if the app is exposed directly
without a reverse proxy.

## 2. Start the stack

### Local HTTP-only bring-up

This starts KeyBank and Redis without Caddy:

```bash
docker compose up -d --build
```

The app will be reachable on the VPS itself at:

```text
http://127.0.0.1:14000/
```

If you changed `PORT` in `.env`, use that value instead of `14000`.

### Public HTTPS deployment with Caddy

This enables the optional Caddy profile and lets Caddy terminate TLS:

```bash
docker compose --profile caddy up -d --build
```

With DNS pointed at the VPS and ports `80/443` open, the service should become
reachable at:

```text
https://your-hostname/
```

## 3. Verify the deployment

Check the service list:

```bash
docker compose ps
```

Check local app reachability on the host:

```bash
curl -I http://127.0.0.1:${PORT:-14000}/
```

If Caddy is enabled, check the public endpoint:

```bash
curl -I https://your-hostname/
```

You can also inspect logs:

```bash
docker compose logs -f app
docker compose logs -f redis
docker compose logs -f caddy
```

## 4. Persistent data

The Compose stack creates these named volumes:

- `redis_data` for Redis data
- `caddy_data` for certificates and ACME state
- `caddy_config` for Caddy runtime state

Back up the Redis and Caddy data volumes before major upgrades or host
migrations.

## 5. Updating the deployment

Pull the latest code, then rebuild and restart:

```bash
git pull
docker compose --profile caddy up -d --build
```

If you are not using Caddy, omit the profile flag:

```bash
docker compose up -d --build
```

## 6. Stopping the stack

Stop containers without deleting data:

```bash
docker compose down
```

To remove containers and named volumes together:

```bash
docker compose down -v
```
