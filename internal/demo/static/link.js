// Demo v3 fragments contain encrypted notes and public metadata, never secret keys.
import { base64URLToBytes } from "./crypto.js";

// Byte capacity of the largest QR code at error-correction level M.
export const qrCapacity = 2331;

// Encoded sizes of the fixed-length parts of a link.
const idLength = 32; // 128-bit hex ID
const ephemeralKeyLength = 43; // X25519 public key, base64url
const ivLength = 16; // 96-bit IV, base64url
const gcmTagBytes = 16;

function base64URLLength(bytes) {
  return Math.ceil((bytes * 4) / 3);
}

export function buildShareURL(id, encrypted, protectedKey = false) {
  const fragment = new URLSearchParams({
    v: "3",
    t: encrypted.type,
    p: protectedKey ? "1" : "0",
    i: encrypted.iv,
    c: encrypted.ct
  });

  if (encrypted.type === "ecc") {
    fragment.set("e", encrypted.epk);
  }
  return `${window.location.origin}/share/${encodeURIComponent(id)}#${fragment}`;
}

// Predicts the length of the link a plaintext of the given UTF-8 size produces.
export function estimateLinkLength(plaintextBytes, type = "ecc") {
  return `${window.location.origin}/share/`.length + idLength
    + `#v=3&t=${type}&p=0`.length + (type === "ecc" ? "&e=".length + ephemeralKeyLength : 0)
    + "&i=".length + ivLength
    + "&c=".length + base64URLLength(plaintextBytes + gcmTagBytes);
}

// Splits a link into what reaches the server and what stays in the browser.
export function splitShareURL(href) {
  const url = new URL(href);
  return { sent: `${url.origin}${url.pathname}`, kept: url.hash };
}

export function parseFragment(hash) {
  const raw = hash.startsWith("#") ? hash.slice(1) : hash;
  if (raw === "") {
    throw new Error("This share link is missing its encrypted fragment. Ask the sender for the complete link.");
  }
  const params = new URLSearchParams(raw);
  if (["1", "2"].includes(params.get("v"))) {
    throw new Error("This link was made by an older version of KeyBank that this server can no longer open. Nothing was fetched, so ask the sender for a new link.");
  }
  if (params.get("v") !== "3") {
    throw new Error("This share link uses an unsupported fragment version.");
  }
  const type = params.get("t");
  const protectedFlag = params.get("p");
  if (!["ecc", "aes256"].includes(type) || !["0", "1"].includes(protectedFlag)) {
    throw new Error("This share link has invalid key metadata.");
  }
  const fields = type === "ecc" ? ["v", "t", "p", "e", "i", "c"] : ["v", "t", "p", "i", "c"];
  if ([...params.keys()].length !== fields.length || fields.some((field) => params.getAll(field).length !== 1)) {
    throw new Error("This share link is missing encrypted data or contains unexpected fields.");
  }
  const epk = params.get("e");
  const iv = params.get("i");
  const ct = params.get("c");
  try {
    if (base64URLToBytes(iv).length !== 12 || base64URLToBytes(ct).length < 16 || (type === "ecc" && base64URLToBytes(epk).length !== 32)) {
      throw new Error("invalid length");
    }
  } catch {
    throw new Error("The encrypted fragment in this share link is malformed.");
  }
  return { type, protectedKey: protectedFlag === "1", epk, iv, ct };
}
