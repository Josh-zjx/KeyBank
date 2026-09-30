// The share link format: https://host/share/{id}#v=1&w={wrapped key}&i={iv}&c={ciphertext}
// Everything after "#" is the URL fragment, which browsers never send to a server.

const base64URLPattern = /^[A-Za-z0-9_-]+$/;

// Byte capacity of the largest QR code at error-correction level M.
export const qrCapacity = 2331;

// Encoded sizes of the fixed-length parts of a link.
const idLength = 32; // 128-bit hex ID
const wrappedKeyLength = 683; // RSA-4096 ciphertext, base64url
const ivLength = 16; // 96-bit IV, base64url
const gcmTagBytes = 16;

function base64URLLength(bytes) {
  return Math.ceil((bytes * 4) / 3);
}

export function buildShareURL(id, encrypted) {
  const fragment = new URLSearchParams({
    v: "1",
    w: encrypted.wrapped,
    i: encrypted.iv,
    c: encrypted.ct
  });

  return `${window.location.origin}/share/${encodeURIComponent(id)}#${fragment}`;
}

// Predicts the length of the link a plaintext of the given UTF-8 size produces.
export function estimateLinkLength(plaintextBytes) {
  return `${window.location.origin}/share/`.length + idLength
    + "#v=1&w=".length + wrappedKeyLength
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
  if (params.get("v") !== "1") {
    throw new Error("This share link uses an unsupported fragment version.");
  }

  const wrapped = params.get("w");
  const iv = params.get("i");
  const ct = params.get("c");
  if (!wrapped || !iv || !ct) {
    throw new Error("This share link is missing encrypted data. Ask the sender for the complete link.");
  }

  for (const [name, value] of [["wrapped key", wrapped], ["IV", iv], ["ciphertext", ct]]) {
    if (!base64URLPattern.test(value)) {
      throw new Error(`The ${name} in this share link is malformed.`);
    }
  }

  return { wrapped, iv, ct };
}
