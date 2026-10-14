// The only module that talks to KeyBank. Every request is written to the page's
// ledger using the very same values handed to fetch, so what the ledger shows
// is exactly what went over the wire.

const failureMessages = {
  400: "KeyBank rejected that expiry. Pick another and try again.",
  413: "The request was too large.",
  429: "Too many requests. Wait a moment and try again."
};

export function createLedger(root) {
  const list = root.querySelector("[data-ledger]");
  const template = root.querySelector("template[data-ledger-entry]");

  return {
    // Adds a request to the ledger and returns a callback that fills in the reply.
    // A request body is shown verbatim; without one, the note describes what was sent.
    record(method, path, { body, note = "Nothing but the address" } = {}) {
      if (!list || !template) {
        return () => {};
      }

      const entry = template.content.firstElementChild.cloneNode(true);
      entry.querySelector(".ledger-method").textContent = method;
      entry.querySelector(".ledger-path").textContent = path;
      const sent = entry.querySelector("[data-sent]");
      sent.textContent = body ?? note;
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

async function send(ledger, method, path, { headers, body, describe }) {
  const settle = ledger.record(method, path, { body });

  let response;
  try {
    response = await fetch(path, { method, headers, body });
  } catch {
    settle("No response");
    throw new Error("Could not reach KeyBank. Check your connection and try again.");
  }

  settle(`${response.status} · ${describe(response.status)}`);
  return response;
}

// Requests a fresh keypair. Resolves to { id, pub_pem }.
export async function createKey(ledger, ttl) {
  const response = await send(ledger, "POST", "/api/keys", {
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ttl }),
    describe: (status) => (status === 201 ? "a key ID and a public key" : "an error, no key")
  });

  if (!response.ok) {
    throw new Error(failureMessages[response.status] ?? "KeyBank could not create a key. Try again shortly.");
  }
  return response.json();
}

// Fetches, and thereby destroys, a private key. Resolves to its PEM, or null
// if the key was already fetched or has expired.
export async function fetchKey(ledger, id) {
  const response = await send(ledger, "GET", `/api/keys/${encodeURIComponent(id)}`, {
    describe: (status) => {
      switch (status) {
        case 200:
          return "the private key, now deleted from KeyBank";
        case 404:
          return "nothing: already opened or expired";
        default:
          return "an error";
      }
    }
  });

  if (response.status === 404) {
    return null;
  }
  if (!response.ok) {
    throw new Error(failureMessages[response.status] ?? "Fetching the private key failed.");
  }
  return response.text();
}
