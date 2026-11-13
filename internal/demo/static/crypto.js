const encoder = new TextEncoder();
const decoder = new TextDecoder();

const base64URLPattern = /^[A-Za-z0-9_-]+$/;

function chunkSizeForPlaintext(plaintextByteLength) {
  if (plaintextByteLength <= 0x80) {
    return 0x80;
  }
  if (plaintextByteLength <= 0x800) {
    return 0x800;
  }
  return 0x8000;
}

function bytesToBase64(bytes, chunkSize) {
  let binary = "";

  for (let i = 0; i < bytes.length; i += chunkSize) {
    const chunk = bytes.subarray(i, i + chunkSize);
    binary += String.fromCharCode(...chunk);
  }

  return btoa(binary);
}

function base64ToBytes(base64) {
  const normalized = base64.replace(/-/g, "+").replace(/_/g, "/");
  const padLength = normalized.length % 4 === 0 ? 0 : 4 - (normalized.length % 4);
  const binary = atob(normalized + "=".repeat(padLength));
  const bytes = new Uint8Array(binary.length);

  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }

  return bytes;
}

function bytesToBase64URL(bytes, chunkSize) {
  return bytesToBase64(bytes, chunkSize).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

export function base64URLToBytes(value) {
  if (!base64URLPattern.test(value)) {
    throw new Error("Invalid base64url data.");
  }
  const bytes = base64ToBytes(value);
  if (bytesToBase64URL(bytes, 0x8000) !== value) {
    throw new Error("Non-canonical base64url data.");
  }
  return bytes;
}

// Demo v3 uses raw API keys; only note encryption derives a key with HKDF.
const noteLabel = "keybank/demo/v3/note";
const x25519 = { name: "X25519" };
const x25519KeyBytes = 32;
const keyChunkSize = 0x80;

const unsupportedMessage = "This browser does not support X25519 encryption. Update it to a current version and try again.";

// Generates a throwaway keypair. Only the public half can ever be exported.
async function generateKeyPair() {
  try {
    return await crypto.subtle.generateKey(x25519, false, ["deriveBits"]);
  } catch (error) {
    if (error?.name === "NotSupportedError") {
      throw new Error(unsupportedMessage);
    }
    throw error;
  }
}

// Resolves if this browser can do X25519, otherwise rejects with a readable message.
export async function ensureCryptoSupport(type = "ecc") {
  if (!globalThis.crypto?.subtle) {
    throw new Error("This browser needs HTTPS or localhost to encrypt notes.");
  }
  await crypto.subtle.importKey("raw", new Uint8Array(32), "AES-GCM", false, ["encrypt", "decrypt"]);
  if (type === "ecc") {
    await generateKeyPair();
  }
}

async function exportPublicKey(keyPair) {
  return new Uint8Array(await crypto.subtle.exportKey("raw", keyPair.publicKey));
}

function decodePublicKey(value, name) {
  const raw = base64URLToBytes(value);
  if (raw.length !== x25519KeyBytes) {
    throw new Error(`The ${name} is not a valid X25519 public key.`);
  }
  return raw;
}

function concatBytes(...parts) {
  const out = new Uint8Array(parts.reduce((length, part) => length + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

// ECDH, then HKDF-SHA256 into a single-purpose AES-256-GCM key.
async function deriveAesKey(privateKey, peerPublic, label, ephemeralPublic, staticPublic, usage) {
  const peer = await crypto.subtle.importKey("raw", peerPublic, x25519, true, []);
  const shared = await crypto.subtle.deriveBits({ name: "X25519", public: peer }, privateKey, 256);
  const ikm = await crypto.subtle.importKey("raw", shared, "HKDF", false, ["deriveKey"]);
  const info = concatBytes(encoder.encode(label), new Uint8Array([0]), ephemeralPublic, staticPublic);
  return crypto.subtle.deriveKey(
    { name: "HKDF", hash: "SHA-256", salt: new Uint8Array(), info },
    ikm,
    { name: "AES-GCM", length: 256 },
    false,
    [usage]
  );
}

// Check the complete crypto path before the irreversible fetch. This also
// rejects low-order ECC points that would fail X25519 agreement.
export async function validateCryptoForFragment(fragment) {
  await ensureCryptoSupport(fragment.type);
  if (fragment.type === "ecc") {
    const probe = await generateKeyPair();
    const ownPublic = await exportPublicKey(probe);
    const peer = decodePublicKey(fragment.epk, "key in this link");
    await deriveAesKey(probe.privateKey, peer, noteLabel, peer, ownPublic, "decrypt");
  }
}

async function importAesKey(encoded, usage) {
  const raw = base64URLToBytes(encoded);
  if (raw.length !== 32) {
    throw new Error("KeyBank returned an invalid AES-256 key.");
  }
  return crypto.subtle.importKey("raw", raw, "AES-GCM", false, [usage]);
}

// The API reply is used locally; it is never added to a share link.
export async function encryptMessage(reply, plaintext) {
  let aesKey;
  let epk;
  if (reply.type === "ecc") {
    const notePublic = decodePublicKey(reply.public_key, "key KeyBank returned");
    const ephemeral = await generateKeyPair();
    const ephemeralPublic = await exportPublicKey(ephemeral);
    aesKey = await deriveAesKey(ephemeral.privateKey, notePublic, noteLabel, ephemeralPublic, notePublic, "encrypt");
    epk = bytesToBase64URL(ephemeralPublic, keyChunkSize);
  } else if (reply.type === "aes256") {
    aesKey = await importAesKey(reply.key, "encrypt");
  } else {
    throw new Error("KeyBank returned an unsupported key type.");
  }
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const plaintextBytes = encoder.encode(plaintext);
  const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, aesKey, plaintextBytes);
  return {
    type: reply.type,
    epk,
    iv: bytesToBase64URL(iv, keyChunkSize),
    ct: bytesToBase64URL(new Uint8Array(ciphertext), chunkSizeForPlaintext(plaintextBytes.byteLength))
  };
}

export async function decryptMessage(reply, fragment) {
  if (reply.type !== fragment.type) {
    throw new Error("The key type does not match this link.");
  }
  let aesKey;
  if (reply.type === "ecc") {
    const notePublic = decodePublicKey(reply.public_key, "public key KeyBank returned");
    const secret = base64URLToBytes(reply.private_key);
    if (secret.length !== 32) {
      throw new Error("KeyBank returned an invalid private key.");
    }
    const privateKey = await crypto.subtle.importKey(
      "jwk", { kty: "OKP", crv: "X25519", d: reply.private_key, x: reply.public_key },
      x25519, false, ["deriveBits"]
    );
    const ephemeral = decodePublicKey(fragment.epk, "key in this link");
    aesKey = await deriveAesKey(privateKey, ephemeral, noteLabel, ephemeral, notePublic, "decrypt");
    secret.fill(0);
  } else {
    aesKey = await importAesKey(reply.key, "decrypt");
  }
  const plaintext = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: base64URLToBytes(fragment.iv) },
    aesKey, base64URLToBytes(fragment.ct)
  );
  return decoder.decode(plaintext);
}
