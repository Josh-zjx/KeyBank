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

function base64URLToBytes(value) {
  if (!base64URLPattern.test(value)) {
    throw new Error("Invalid base64url data.");
  }
  return base64ToBytes(value);
}

function pemToBytes(pem, label) {
  const beginMarker = `-----BEGIN ${label}-----`;
  const endMarker = `-----END ${label}-----`;
  const beginIndex = pem.indexOf(beginMarker);
  const endIndex = pem.indexOf(endMarker);
  if (beginIndex === -1 || endIndex === -1 || endIndex <= beginIndex) {
    throw new Error(`Missing ${label} PEM block.`);
  }
  const body = pem.slice(beginIndex + beginMarker.length, endIndex).replace(/\s+/g, "");
  if (body === "") {
    throw new Error(`Empty ${label} PEM block.`);
  }
  return base64ToBytes(body);
}

async function importRsaPublicKey(pem) {
  return crypto.subtle.importKey(
    "spki",
    pemToBytes(pem, "PUBLIC KEY"),
    { name: "RSA-OAEP", hash: "SHA-256" },
    false,
    ["encrypt"]
  );
}

async function importRsaPrivateKey(pem) {
  return crypto.subtle.importKey(
    "pkcs8",
    pemToBytes(pem, "PRIVATE KEY"),
    { name: "RSA-OAEP", hash: "SHA-256" },
    false,
    ["decrypt"]
  );
}

export async function generateAesKey() {
  return crypto.subtle.generateKey({ name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]);
}

export async function encryptMessage(pubPem, plaintext) {
  const publicKey = await importRsaPublicKey(pubPem);
  const aesKey = await generateAesKey();
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const plaintextBytes = encoder.encode(plaintext);
  // Size the serialization chunks from the actual UTF-8 plaintext bytes.
  const chunkSize = chunkSizeForPlaintext(plaintextBytes.byteLength);
  const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, aesKey, plaintextBytes);
  const rawKey = await crypto.subtle.exportKey("raw", aesKey);
  const wrapped = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, publicKey, rawKey);

  return {
    wrapped: bytesToBase64URL(new Uint8Array(wrapped), chunkSize),
    iv: bytesToBase64URL(iv, chunkSize),
    ct: bytesToBase64URL(new Uint8Array(ciphertext), chunkSize)
  };
}

export async function decryptMessage(privPem, wrapped, iv, ct) {
  const privateKey = await importRsaPrivateKey(privPem);
  const rawKey = await crypto.subtle.decrypt({ name: "RSA-OAEP" }, privateKey, base64URLToBytes(wrapped));
  const aesKey = await crypto.subtle.importKey("raw", rawKey, { name: "AES-GCM" }, false, ["decrypt"]);
  const plaintext = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: base64URLToBytes(iv) },
    aesKey,
    base64URLToBytes(ct)
  );

  return decoder.decode(plaintext);
}
