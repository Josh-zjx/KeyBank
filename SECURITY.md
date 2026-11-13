# Security

## Reporting a vulnerability

Please **do not** open a public issue for suspected security vulnerabilities.

Preferred reporting path:

1. Use GitHub's private vulnerability reporting flow for this repository when it
   is available in the GitHub UI.
2. If private reporting is not available for your account or this repository,
   contact the maintainer through GitHub without posting exploit details
   publicly.

When you report an issue, please include:

- the affected commit, tag, or deployment version if known;
- reproduction steps or a proof of concept;
- impact details and any suggested mitigation if you have them.

KeyBank aims to acknowledge security reports within 5 business days and will use
coordinated disclosure for fixes.

## Vendored assets

| File | SHA-256 | SRI |
|------|---------|-----|
| `internal/demo/static/qrcode.js` | `02cf1c7a09475938ac07ebfb3e3c88e7be20998d0207f934beca1b40f8b6bf97` | `sha384-ahLw45Nl1X/zUAno5v8A1m7qYEzDIOrQtPeIGkG/a+vFi9BC17OhLEhzDJZUHqN2` |

The SRI hash is enforced via the `integrity` attribute on the `<script>` tag in
`internal/demo/templates/base.html`. If the file on disk ever changes, the browser refuses to
execute it and the QR code feature breaks loudly.

To regenerate the SHA-256:
```
sha256sum internal/demo/static/qrcode.js
```

To regenerate the SRI (SHA-384, base64):
```
openssl dgst -sha384 -binary internal/demo/static/qrcode.js | openssl base64 -A
```

## API v1 trust boundaries

Production requires TLS with a trusted termination point. API responses carry
raw private/symmetric keys and protected requests carry passphrases. Decrypted
traffic recordings and body logging can expose them. There is no fetch sealing.

The API stores keys and salted passphrase verifiers; it receives no note data.
Passphrases gate retrieval, without encrypting stored keys. Atomic conditional
consumption permits at most one successful fetch, including concurrent requests.
Wrong credentials never delete a record. No rate limiter or attempt lockout is
active; deployments must account for online guessing and resource consumption.

The demo keeps plaintext/ciphertext in browser memory and link fragments, and
redacts credentials from its ledger. A compromised frontend, API, Redis,
recipient device, or trusted TLS terminator can compromise secrets. Do not claim
protection from server compromise or that decrypted traffic reveals nothing.
