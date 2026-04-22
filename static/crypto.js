const encoder = new TextEncoder();
const decoder = new TextDecoder();

const base64URLPattern = /^[A-Za-z0-9_-]+$/;

function bytesToBase64(bytes) {
  let binary = "";
  const chunkSize = 0x8000;

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

function bytesToBase64URL(bytes) {
  return bytesToBase64(bytes).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function base64URLToBytes(value) {
  if (!base64URLPattern.test(value)) {
    throw new Error("Invalid base64url data.");
  }
  return base64ToBytes(value);
}

function trimLeadingZero(bytes) {
  if (bytes.length > 1 && bytes[0] === 0) {
    return bytes.subarray(1);
  }
  return bytes;
}

function decodePemBlock(pem, label) {
  const beginMarker = `-----BEGIN ${label}-----`;
  const endMarker = `-----END ${label}-----`;
  const beginIndex = pem.indexOf(beginMarker);
  const endIndex = pem.indexOf(endMarker);

  if (beginIndex === -1 || endIndex === -1 || endIndex <= beginIndex) {
    throw new Error(`Missing ${label} PEM block.`);
  }

  const body = pem
    .slice(beginIndex + beginMarker.length, endIndex)
    .replace(/\s+/g, "");

  if (body === "") {
    throw new Error(`Empty ${label} PEM block.`);
  }

  return base64ToBytes(body);
}

function readLength(bytes, offset) {
  if (offset >= bytes.length) {
    throw new Error("Unexpected end of ASN.1 input.");
  }

  const first = bytes[offset];
  if ((first & 0x80) === 0) {
    return { length: first, offset: offset + 1 };
  }

  const byteCount = first & 0x7f;
  if (byteCount === 0 || byteCount > 4 || offset + 1 + byteCount > bytes.length) {
    throw new Error("Unsupported ASN.1 length.");
  }

  let length = 0;
  for (let i = 0; i < byteCount; i += 1) {
    length = (length << 8) | bytes[offset + 1 + i];
  }

  return { length, offset: offset + 1 + byteCount };
}

function createDerReader(bytes) {
  let offset = 0;

  return {
    readElement(expectedTag, name) {
      if (offset >= bytes.length) {
        throw new Error(`Unexpected end of ${name}.`);
      }

      const tag = bytes[offset];
      offset += 1;

      if (tag !== expectedTag) {
        throw new Error(`Unexpected ASN.1 tag while reading ${name}.`);
      }

      const { length, offset: valueOffset } = readLength(bytes, offset);
      const endOffset = valueOffset + length;
      if (endOffset > bytes.length) {
        throw new Error(`Truncated ASN.1 value while reading ${name}.`);
      }

      offset = endOffset;
      return bytes.subarray(valueOffset, endOffset);
    },
    done() {
      return offset === bytes.length;
    }
  };
}

function createSequenceReader(der) {
  const root = createDerReader(der);
  const content = root.readElement(0x30, "ASN.1 sequence");
  if (!root.done()) {
    throw new Error("Unexpected trailing ASN.1 data.");
  }
  return createDerReader(content);
}

function readInteger(reader, name) {
  return trimLeadingZero(reader.readElement(0x02, name));
}

function parsePkcs1PublicKey(pem) {
  const reader = createSequenceReader(decodePemBlock(pem, "RSA PUBLIC KEY"));
  const modulus = readInteger(reader, "RSA modulus");
  const exponent = readInteger(reader, "RSA public exponent");

  if (!reader.done()) {
    throw new Error("Unexpected trailing data in RSA public key.");
  }

  return {
    kty: "RSA",
    n: bytesToBase64URL(modulus),
    e: bytesToBase64URL(exponent),
    alg: "RSA-OAEP-256",
    ext: true,
    key_ops: ["encrypt"]
  };
}

function parsePkcs1PrivateKey(pem) {
  const reader = createSequenceReader(decodePemBlock(pem, "RSA PRIVATE KEY"));
  readInteger(reader, "RSA version");

  const parts = {
    n: readInteger(reader, "RSA modulus"),
    e: readInteger(reader, "RSA public exponent"),
    d: readInteger(reader, "RSA private exponent"),
    p: readInteger(reader, "RSA prime1"),
    q: readInteger(reader, "RSA prime2"),
    dp: readInteger(reader, "RSA exponent1"),
    dq: readInteger(reader, "RSA exponent2"),
    qi: readInteger(reader, "RSA coefficient")
  };

  if (!reader.done()) {
    throw new Error("Unexpected trailing data in RSA private key.");
  }

  return {
    kty: "RSA",
    n: bytesToBase64URL(parts.n),
    e: bytesToBase64URL(parts.e),
    d: bytesToBase64URL(parts.d),
    p: bytesToBase64URL(parts.p),
    q: bytesToBase64URL(parts.q),
    dp: bytesToBase64URL(parts.dp),
    dq: bytesToBase64URL(parts.dq),
    qi: bytesToBase64URL(parts.qi),
    alg: "RSA-OAEP-256",
    ext: true,
    key_ops: ["decrypt"]
  };
}

async function importRsaPublicKey(pem) {
  return crypto.subtle.importKey(
    "jwk",
    parsePkcs1PublicKey(pem),
    { name: "RSA-OAEP", hash: "SHA-256" },
    false,
    ["encrypt"]
  );
}

async function importRsaPrivateKey(pem) {
  return crypto.subtle.importKey(
    "jwk",
    parsePkcs1PrivateKey(pem),
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
  const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, aesKey, plaintextBytes);
  const rawKey = await crypto.subtle.exportKey("raw", aesKey);
  const wrapped = await crypto.subtle.encrypt({ name: "RSA-OAEP" }, publicKey, rawKey);

  return {
    wrapped: bytesToBase64URL(new Uint8Array(wrapped)),
    iv: bytesToBase64URL(iv),
    ct: bytesToBase64URL(new Uint8Array(ciphertext))
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
