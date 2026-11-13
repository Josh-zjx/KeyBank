# Current roadmap

## API v1 extraction

- Independently run and deploy the API and secret-note demo.
- Create/fetch X25519 ECC and random AES-256 keys, with optional passphrase gates.
- Authenticate before atomic compare-and-consume; preserve TTL and one-time access.
- Return key material in JSON over TLS; replace the browser fetch-sealing flow.
- Publish OpenAPI and update the demo with both algorithms and protected links.
- Validate all four combinations, storage races/expiry, browser flows, and images.

Rate limiting, accounts, failed-attempt lockout, additional curves, and legacy
link compatibility are deferred. Future authentication or persistence changes
must preserve atomic consume-or-fail behavior and update the documented threat
model before implementation.

Earlier milestone/review reports are historical and describe superseded APIs.
Current contracts are in `docs/openapi.yaml` and `docs/protocol.md`.
