# Mission

KeyBank is a one-time cryptographic key generation and retrieval service for
sharing encrypted, self-destructing notes. Unlike traditional secure-note
services that store the message on their servers and promise to destroy it
after one fetch, KeyBank inverts the model: **the server holds only ephemeral
private keys, never the message**.

## Goal

Let one person share a short secret with another — credentials, recovery
codes, medical notes, contract snippets — over any untrusted channel (email,
chat, ticketing system) such that:

- the plaintext is never transmitted to, stored by, or decryptable by the
  KeyBank operator;
- the recipient can read the message exactly once;
- the cryptographic material is automatically destroyed after first fetch or
  after a sender-chosen expiry (at most 7 days).

## Mission

Ship a self-hostable, minimally-dependent, zero-knowledge secure-note service
whose security can be audited by reading the code in an afternoon.

The project's value is inseparable from the trust its users place in the
code. Every design choice — small dependency set, no tracking, no plaintext
on the wire, open source — serves that trust. Simplicity and transparency
are first-class features, not nice-to-haves.

## Principles

1. **Zero knowledge.** The server must never see the plaintext, the symmetric
   key, or the ciphertext. Anything that violates this is a bug, not a
   tradeoff.
2. **One-time by construction.** Delete-on-read is atomic at the storage
   layer (`GETDEL`). No background cleanup sweepers, no consistency windows
   in which a second reader could win the race.
3. **Small dependency surface.** Every added library is an auditability
   cost. Prefer the standard library. Vendor any JS from source with pinned
   checksums.
4. **Self-hostable in ten minutes.** `docker compose up` on a fresh VM
   produces a working instance behind a reverse proxy with TLS.
5. **Fail closed.** If Redis is down, if a password is wrong, if a URL
   fragment is malformed — return a clean error and, when appropriate,
   destroy the record rather than leak behavior.

## Non-goals

- Persistent chat, channels, threaded conversations, or any multi-recipient
  workflow.
- Arbitrary file transfer. Messages are short-form text; large files belong
  on a different tool.
- Account systems, user profiles, or any persisted user identity.
- Horizontal scale beyond what a single Redis instance serves. This is a
  privacy tool, not a platform.
- Enterprise directory integration, SSO, or per-user audit logs (which
  would defeat the privacy posture).

## In-scope conveniences (v0.1)

Beyond the core share-and-burn flow, the first release adds:

- **Custom expiry** — sender picks a TTL between one minute and seven days.
- **Password gate** — an optional sender-chosen password derived client-side
  with PBKDF2-SHA256; the server stores only the derived hash and refuses to
  release the private key without a matching hash on retrieval. After five
  failed attempts the record is destroyed.
- **QR code of the share URL** — rendered client-side for device-to-device
  hand-off without copy-paste.

## Success criteria

- A third-party reviewer can read the entire server source in under an hour
  and satisfy themselves that no plaintext path exists.
- Creating and retrieving a note each take at most two clicks for a
  non-technical user.
- `go test ./...` passes with at least 85% coverage on the `crypto`, `store`,
  and `handlers` packages.
- `gosec` and `govulncheck` report clean on every release.
- Rolling upgrades are possible with zero downtime (all state is external in
  Redis).

## Audience

- **Senders** who need to hand a secret to one other person and do not want
  a third party (including the service operator) to ever hold the plaintext.
- **Self-hosters** — individuals, small teams, or organizations — who prefer
  to run their own privacy-critical tooling.
- **Reviewers and auditors** who need to satisfy themselves that the code
  does what the mission statement claims.

## Inspiration

The design of this service is inspired by the many secure-note-sharing apps
on the Internet, especially the tutorial made by Dusted Codes Limited. The
inversion — storing keys instead of messages — is the core departure.
