# KeyBank
An one-time Cryptography Key generation and retrieval service

# Note Sharing
Traditional secure note sharing service store the message on their servers and promise to destroy it after one fetch.

KeyBank applies an opposite model.

KeyBank generate the cryptography key pair for you, sends you the public key to encrypt the message. The ciphertext is held in the share url directly.

When fetching, KeyBank would fetch the private key and decrypt the message on the web interface.

The message is never sent to the server, therefore reducing the legal problem on storing and hosting user data, at the price of limited message length.

# Feature
- Zero user data on server
	- No legal problem for server
	- No Data Leak even when server hacked
- Fully encrypted procedure
	- Public key and private key are sent in two different access
	- No Eavesdropping 
- Every time one new keypair
	- No ciphertext only attack on consecutive use

# API

## POST /api/keys

Generates an RSA-4096 keypair. Stores the private key server-side and returns the public key to the caller. The caller uses the public key to encrypt a message client-side; the ciphertext is never sent to the server.

**Response `201 Created`:**
```json
{
  "id": "3f2a1b4c...",
  "pub_pem": "-----BEGIN RSA PUBLIC KEY-----\n..."
}
```

| Field | Description |
|---|---|
| `id` | 128-bit random hex ID used to retrieve the private key |
| `pub_pem` | RSA-4096 public key in PKCS#1 PEM format |

## GET /api/keys/{id}

Retrieves and **permanently deletes** the private key for the given ID (one-time read). Returns `404` if the key has already been fetched or never existed.

**Response `200 OK`:** private key PEM bytes (used by the browser to decrypt the message).

## GET /

Create page (shell — UI added in Milestone 4).

## GET /share/{id}

Decryption page for share URL (shell — UI added in Milestone 4).

# Credit
This project is inspired by many secure note sharing app on the Internet, especially the [ tutorial ](https://dusted.codes/building-a-secure-note-sharing-service-in-go) made by [Dusted Codes Limited](https://dusted.codes/about) .
	
