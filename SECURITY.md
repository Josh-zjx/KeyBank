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
| `static/qrcode.js` | `02cf1c7a09475938ac07ebfb3e3c88e7be20998d0207f934beca1b40f8b6bf97` | `sha384-ahLw45Nl1X/zUAno5v8A1m7qYEzDIOrQtPeIGkG/a+vFi9BC17OhLEhzDJZUHqN2` |

The SRI hash is enforced via the `integrity` attribute on the `<script>` tag in
`templates/base.html`. If the file on disk ever changes, the browser refuses to
execute it and the QR code feature breaks loudly.

To regenerate the SHA-256:
```
sha256sum static/qrcode.js
```

To regenerate the SRI (SHA-384, base64):
```
openssl dgst -sha384 -binary static/qrcode.js | openssl base64 -A
```
