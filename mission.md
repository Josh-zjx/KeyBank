# Mission

KeyBank provides a small, independently deployable API for one-time cryptographic
keys. Clients can create/fetch X25519 ECC or AES-256 keys, optionally gated by a
passphrase, without storing application messages on the API server.

The secret-note demo is a separate client: it encrypts notes locally and carries
ciphertext in share URL fragments. Both services are self-hostable behind TLS.

Principles:

- Generate independent random material per key; never derive it from passphrases.
- Authenticate before atomic conditional consumption, with at most one winner.
- Keep note plaintext and ciphertext in the browser, not on either service.
- Expire unread keys; use private memory-only Redis in the supplied deployment.
- Document trust boundaries honestly: keys/passphrases traverse TLS directly,
  and operators, compromised servers, devices, or TLS termination may expose them.
- Keep the API usable independently, with a documented versioned HTTP contract.

Accounts, message storage, reusable keys, rate limiting, failed-attempt lockout,
and legacy protocol compatibility are outside this version's scope.

See `docs/openapi.yaml`, `docs/protocol.md`, and `docs/deploy.md` for the current
contract, security model, and operational assumptions.
