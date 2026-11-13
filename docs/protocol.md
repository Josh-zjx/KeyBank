# API v1 and demo fragment v3

The API is independent of the note-sharing demo. Its authoritative wire contract
is [openapi.yaml](openapi.yaml). API handlers are in `handlers/`; browser crypto
is in `internal/demo/static/crypto.js`.

## Key API

`POST /api/v1/keys` accepts an object with required `type` (`ecc` or `aes256`),
optional `ttl` (seconds), and optional `passphrase`. A nonempty passphrase gates
retrieval. Generate keys using `crypto/rand`, never from the passphrase.

| Type | Generated material | Create (`201`) | Fetch (`200`) |
|---|---|---|---|
| ecc | X25519 pair | id, type, public_key | id, type, public_key, private_key |
| aes256 | random 32 bytes | id, type, key | id, type, identical key |

All key fields are raw 32-byte encodings represented as canonical unpadded
base64url (43 characters), including X25519 private scalars (not PEM/PKCS#8).
IDs are random 128-bit values encoded as 32 lowercase hexadecimal characters.

`POST /api/v1/keys/{id}/fetch` accepts `{}` or `{"passphrase":"..."}`. Matching
uses the exact UTF-8 string, with no whitespace trimming or Unicode
normalization. Empty/omitted passphrases mean no gate on creation. Passphrases
are limited to 1,024 UTF-8 bytes. Unprotected records do not check credentials.

Both endpoints require `application/json` (optional charset parameters allowed)
and cap bodies at 16 KiB. Exactly one JSON object is required; unknown fields,
duplicate fields, null values, wrong types, invalid UTF-8, and trailing data are
rejected. TTL omitted/zero defaults to min(24 hours, MAX_TTL); explicit nonzero
TTL is 60 seconds through MAX_TTL, whose ceiling is seven days.

Errors are `{"error":"code"}`: 400 invalid_request/invalid_type/invalid_ttl,
403 invalid_passphrase/origin_not_allowed, 404 not_found, 405 method_not_allowed,
413 request_body_too_large, 415 unsupported_media_type, or 500 internal_error.
Key responses are never cached. GET never fetches or consumes material.

## Persistence and consumption

Redis uses `keybank:v1:keys:{id}` with SET NX and TTL. An immutable JSON record
contains key type, secret bytes, creation time, and optional verifier metadata.
Protected records store a random 16-byte salt, PBKDF2-HMAC-SHA256 iteration
count (600,000), and 32-byte verifier. They never store the passphrase itself.
This is a retrieval gate, not encryption at rest.

Fetch reads a snapshot without deleting it, validates persisted state, and
compares a derived verifier in constant time. It prepares the JSON response
before attempting consumption. A Lua script then compares the current payload
with the exact snapshot and deletes only on equality. In-memory storage performs
the same comparison/deletion under a mutex. Only a successful consumer receives
the response. Expiry or a concurrent winner causes 404, never release of a stale
snapshot. No expensive password hashing runs inside a Redis script or memory
store lock. Bad credentials do not delete records or refresh their TTL.

Failures do not roll back successful consumption. Disconnects or uncertain Redis
outcomes may lose a key; the API promises at most one successful consumer, not
guaranteed delivery. No rate limits, failed-attempt counters, or lockout exist.

## Browser transport

The demo configures an API base URL, either `/api/v1` behind the shared proxy or
an absolute HTTP(S) URL ending in `/api/v1`. CSP permits that origin. Browser
requests omit credentials and use JSON POST. Cross-origin access requires an
exact origin in CORS_ALLOWED_ORIGINS; no wildcard or credentialed CORS is used.
Same-host requests also work behind the TLS proxy. CLI clients may omit Origin.
Unknown origins are rejected before accessing keys. Browser JSON POST preflights
prevent unrelated websites from consuming keys using HTML forms or simple fetch.

Keys and passphrases are returned/sent directly under TLS. There is no fetch
sealing handshake. Anyone who can record decrypted API traffic can recover key
material; TLS termination and request/response logging are trust boundaries.

## Demo note encryption

The fragment is:

```
ECC:    #v=3&t=ecc&p=0&i={iv}&c={ciphertext}&e={sender_public}
AES:    #v=3&t=aes256&p=0&i={iv}&c={ciphertext}
```

`p=1` requests a passphrase prompt; it is only a UI hint and cannot bypass the
API gate. No private/symmetric key, passphrase, or verifier appears in the link.
Each field must appear exactly once; unexpected fields are rejected. Binary
fields use canonical unpadded base64url. IVs are 12 bytes; ciphertext includes a
16-byte GCM tag. The sender public key is 32 bytes in ECC mode only.

For ECC, the API generates S and the sender generates a throwaway E:

```
shared = X25519(E_private, S_public)
info   = UTF8("keybank/demo/v3/note") || 0x00 || E_public || S_public
K      = HKDF-SHA256(shared, salt=empty, info=info, length=32)
```

The sender encrypts UTF-8 plaintext with AES-256-GCM(K, random 12-byte IV).
The recipient imports the fetched S_private/S_public as a nonexportable JWK,
computes X25519(S_private, E_public), derives K identically, and decrypts locally.
For AES mode, K is the API's raw AES-256 key, imported nonexportably into Web
Crypto. Every generated key encrypts only one note. There is no additional data.

The browser validates fragments, decoded lengths, supported crypto, and ECC
low-order points before fetching. Syntactically valid but tampered ciphertext
can still consume a key before GCM detects tampering. Note plaintext and
ciphertext are never sent to either service. The request ledger redacts
passphrases and describes returned secrets without rendering them.

## Limits and migration

Anyone possessing an unprotected key ID can consume it; an ID is a bearer
capability. Full links plus a successful fetch reveal notes. Correct
passphrases do not prove recipient identity. Server/Redis access, a malicious
frontend, endpoint malware, or TLS termination can compromise confidentiality.
Deletion cannot prevent a caller from copying key material or plaintext.

API v1 does not implement the old `/api/keys` endpoints. Demo v3 rejects v1/v2
links before fetching. Old Redis records remain isolated and expire naturally;
there is no automatic migration. Redis restart invalidates all unread keys when
using the supplied memory-only deployment.
