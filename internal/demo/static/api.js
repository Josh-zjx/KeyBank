// API v1 client. Only this module sends key requests. Never display secrets in the ledger.

export function createLedger(root) {
  const list = root.querySelector("[data-ledger]");
  const template = root.querySelector("template[data-ledger-entry]");

  return {
    // Adds a request to the ledger and returns a callback that fills in the reply.
    // Request bodies redact passphrases; without a body, the note describes what was sent.
    record(method, path, { body, note = "Nothing but the address" } = {}) {
      if (!list || !template) {
        return () => {};
      }

      const entry = template.content.firstElementChild.cloneNode(true);
      entry.querySelector(".ledger-method").textContent = method;
      entry.querySelector(".ledger-path").textContent = path;
      const sent = entry.querySelector("[data-sent]");
      let shown = body;
      if (body !== undefined) {
        const parsed = JSON.parse(body);
        if (Object.hasOwn(parsed, "passphrase")) {
          parsed.passphrase = "[redacted]";
        }
        shown = JSON.stringify(parsed);
      }
      sent.textContent = shown ?? note;
      sent.classList.toggle("ledger-literal", body !== undefined);
      const received = entry.querySelector("[data-received]");
      received.textContent = "Waiting…";
      list.append(entry);

      return (reply) => {
        received.textContent = reply;
      };
    }
  };
}

const apiBase = () => document.querySelector("[data-api-base-url]").dataset.apiBaseUrl;

async function send(ledger, path, payload, description) {
  const url = `${apiBase()}${path}`;
  const body = JSON.stringify(payload);
  const settle = ledger.record("POST", url, { body });
  let response;
  try {
    response = await fetch(url, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body, credentials: "omit", cache: "no-store", referrerPolicy: "no-referrer"
    });
  } catch {
    settle("No response");
    throw new Error("Could not reach KeyBank. Check your connection and try again.");
  }
  settle(`${response.status} · ${response.ok ? description : "an error"}`);
  if (response.status === 404) {
    return null;
  }
  const reply = await response.json();
  if (!response.ok) {
    const messages = {
      invalid_passphrase: "The passphrase is missing or incorrect. Try again; the key has not been consumed.",
      invalid_ttl: "KeyBank rejected that expiry. Pick another and try again.",
      origin_not_allowed: "This demo's origin is not allowed by the API configuration.",
      request_body_too_large: "The request was too large."
    };
    const error = new Error(messages[reply.error] ?? "KeyBank could not complete the request. Try again shortly.");
    error.code = reply.error;
    throw error;
  }
  return reply;
}

export async function createKey(ledger, options) {
  const reply = await send(ledger, "/keys", options, options.type === "ecc" ? "a key ID and a public key" : "a key ID and an AES-256 key");
  if (!reply) {
    throw new Error("The configured KeyBank API endpoint was not found.");
  }
  return reply;
}

export function fetchKey(ledger, id, passphrase) {
  const payload = passphrase ? { passphrase } : {};
  return send(ledger, `/keys/${encodeURIComponent(id)}/fetch`, payload, "the key, now deleted from KeyBank");
}
