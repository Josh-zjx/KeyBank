import { decryptMessage, encryptMessage } from "./crypto.js";

const defaultTTLSeconds = 24 * 60 * 60;
const maxTTLSeconds = 7 * 24 * 60 * 60;
const softPlaintextLimitBytes = 100 * 1024;
const base64URLPattern = /^[A-Za-z0-9_-]+$/;
const textEncoder = new TextEncoder();

function setStatus(element, message, state) {
  element.textContent = message;
  element.dataset.state = state;
}

function byteLength(value) {
  return textEncoder.encode(value).length;
}

function syncCustomInput(form, customInput) {
  const customSelected = Array.from(form.querySelectorAll('input[name="expiryPreset"]')).some(
    (input) => input.checked && input.value === "custom"
  );
  customInput.disabled = !customSelected;
}

function selectedTTL(form) {
  const selected = form.querySelector('input[name="expiryPreset"]:checked');
  if (!selected || selected.value === "") {
    return defaultTTLSeconds;
  }

  if (selected.value !== "custom") {
    return Number(selected.value);
  }

  const minutes = Number(form.querySelector("#custom-ttl").value);
  if (!Number.isFinite(minutes) || minutes < 1) {
    throw new Error("Enter a custom expiry of at least 1 minute.");
  }

  const ttl = minutes * 60;
  if (ttl > maxTTLSeconds) {
    throw new Error("Custom expiry must be 7 days or less.");
  }

  return ttl;
}

async function confirmLargeMessage(dialog, message, bytes) {
  if (!dialog || typeof dialog.showModal !== "function") {
    return window.confirm(message);
  }

  const text = dialog.querySelector("[data-size-warning-text]");
  if (text) {
    text.textContent = `This note is ${bytes.toLocaleString()} bytes once encoded as UTF-8. Large notes create long share URLs that are harder to copy or scan.`;
  }

  return new Promise((resolve) => {
    const handleClose = () => {
      dialog.removeEventListener("close", handleClose);
      resolve(dialog.returnValue === "confirm");
    };

    dialog.addEventListener("close", handleClose);
    dialog.showModal();
  });
}

function buildShareURL(id, encrypted) {
  const fragment = new URLSearchParams({
    v: "1",
    w: encrypted.wrapped,
    i: encrypted.iv,
    c: encrypted.ct
  });

  return `${window.location.origin}/share/${encodeURIComponent(id)}#${fragment.toString()}`;
}

async function copyToClipboard(value) {
  if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
    await navigator.clipboard.writeText(value);
    return true;
  }

  return false;
}

function renderQRCode(container, value) {
  if (!container) {
    return;
  }

  container.innerHTML = "";
  if (typeof window.QRCode !== "function") {
    return;
  }

  new window.QRCode(container, {
    text: value,
    width: 192,
    height: 192,
    colorDark: "#111827",
    colorLight: "#ffffff",
    correctLevel: window.QRCode.CorrectLevel.M
  });
}

function maybeShowHTTPWarning(root) {
  const warning = root.querySelector("[data-http-warning]");
  if (!warning) {
    return;
  }

  warning.hidden = window.location.protocol !== "http:";
}

async function initCreatePage(root) {
  const form = root.querySelector("[data-create-form]");
  if (!form) {
    return;
  }

  const messageInput = form.querySelector("#message");
  const customInput = form.querySelector("#custom-ttl");
  const status = root.querySelector("#create-status");
  const result = root.querySelector("#create-result");
  const shareURL = root.querySelector("[data-share-url]");
  const copyButton = root.querySelector("[data-copy-link]");
  const openShareLink = root.querySelector("[data-open-share]");
  const qrContainer = root.querySelector("[data-qr-code]");
  const sizeWarning = root.querySelector("[data-size-warning]");

  form.querySelectorAll('input[name="expiryPreset"]').forEach((input) => {
    input.addEventListener("change", () => syncCustomInput(form, customInput));
  });
  syncCustomInput(form, customInput);

  copyButton?.addEventListener("click", async () => {
    if (!shareURL.value) {
      return;
    }

    try {
      const copied = await copyToClipboard(shareURL.value);
      setStatus(status, copied ? "Share URL copied to your clipboard." : "Copy the share URL manually from the box below.", "success");
    } catch {
      setStatus(status, "Copy failed. Select the share URL and copy it manually.", "error");
    }
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();

    const plaintext = messageInput.value;
    if (plaintext.trim() === "") {
      setStatus(status, "Enter a message before generating a link.", "error");
      messageInput.focus();
      return;
    }

    let ttl;
    try {
      ttl = selectedTTL(form);
    } catch (error) {
      setStatus(status, error instanceof Error ? error.message : "Enter a valid expiry.", "error");
      return;
    }

    const bytes = byteLength(plaintext);
    if (bytes > softPlaintextLimitBytes) {
      const confirmed = await confirmLargeMessage(sizeWarning, "This note is large. Continue anyway?", bytes);
      if (!confirmed) {
        setStatus(status, "Link generation cancelled.", "idle");
        return;
      }
    }

    result.hidden = true;
    shareURL.value = "";
    if (openShareLink) {
      openShareLink.href = "";
    }
    setStatus(status, "Requesting a one-time keypair and encrypting locally...", "working");

    try {
      const response = await fetch("/api/keys", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({ ttl })
      });

      if (!response.ok) {
        const body = await response.text();
        throw new Error(body || "Key creation failed.");
      }

      const payload = await response.json();
      const encrypted = await encryptMessage(payload.pub_pem, plaintext);
      const url = buildShareURL(payload.id, encrypted);

      shareURL.value = url;
      result.hidden = false;
      renderQRCode(qrContainer, url);

      if (openShareLink) {
        openShareLink.href = url;
      }

      setStatus(status, "Link generated. The message stayed in this browser; only the encrypted fragment belongs in the URL.", "success");
    } catch (error) {
      setStatus(status, error instanceof Error ? error.message : "Link generation failed.", "error");
    }
  });
}

function parseFragment() {
  const raw = window.location.hash.startsWith("#") ? window.location.hash.slice(1) : window.location.hash;
  if (raw === "") {
    throw new Error("This share link is missing its encrypted fragment.");
  }

  const params = new URLSearchParams(raw);
  if (params.get("v") !== "1") {
    throw new Error("This share link uses an unsupported fragment version.");
  }

  const wrapped = params.get("w");
  const iv = params.get("i");
  const ciphertext = params.get("c");
  if (!wrapped || !iv || !ciphertext) {
    throw new Error("This share link is missing encrypted data.");
  }

  for (const [name, value] of [["wrapped key", wrapped], ["IV", iv], ["ciphertext", ciphertext]]) {
    if (!base64URLPattern.test(value)) {
      throw new Error(`The ${name} in this share link is malformed.`);
    }
  }

  return { wrapped, iv, ct: ciphertext };
}

function consumePlaintext(root, plaintext, autoHideSeconds, status) {
  const result = root.querySelector("[data-share-result]");
  const output = root.querySelector("[data-share-plaintext]");
  const keepVisibleButton = root.querySelector("[data-keep-visible]");

  let hideTimer = null;

  function clearHideTimer() {
    if (hideTimer !== null) {
      window.clearTimeout(hideTimer);
      hideTimer = null;
    }
  }

  output.textContent = plaintext;
  result.hidden = false;
  keepVisibleButton.hidden = false;
  keepVisibleButton.disabled = false;
  keepVisibleButton.textContent = "Keep visible";

  keepVisibleButton.onclick = () => {
    clearHideTimer();
    keepVisibleButton.disabled = true;
    keepVisibleButton.textContent = "Visible until you leave this page";
    setStatus(status, "Plaintext will stay visible until you leave or reload this page.", "success");
  };

  clearHideTimer();
  hideTimer = window.setTimeout(() => {
    output.textContent = "";
    result.hidden = true;
    keepVisibleButton.hidden = true;
    setStatus(status, `Plaintext hidden again after ${autoHideSeconds} seconds.`, "success");
  }, autoHideSeconds * 1000);
}

async function initSharePage(root) {
  const button = root.querySelector("[data-decrypt-button]");
  if (!button) {
    return;
  }

  const status = root.querySelector("#share-status");
  const shareID = root.dataset.shareId;
  const autoHideSeconds = Number(root.dataset.autohideSeconds) || 60;

  button.addEventListener("click", async () => {
    let fragment;
    try {
      fragment = parseFragment();
    } catch (error) {
      setStatus(status, error instanceof Error ? error.message : "This share link is invalid.", "error");
      return;
    }

    if (!shareID) {
      setStatus(status, "This share page is missing its record identifier.", "error");
      return;
    }

    setStatus(status, "Fetching the one-time private key...", "working");

    try {
      const response = await fetch(`/api/keys/${encodeURIComponent(shareID)}`, {
        method: "GET"
      });

      if (response.status === 404) {
        throw new Error("This note has already been opened or expired.");
      }
      if (!response.ok) {
        throw new Error("Fetching the private key failed.");
      }

      const privPem = await response.text();
      let plaintext;
      try {
        plaintext = await decryptMessage(privPem, fragment.wrapped, fragment.iv, fragment.ct);
      } catch {
        throw new Error("Unable to decrypt this link. The encrypted fragment may be corrupted or tampered with.");
      }

      consumePlaintext(root, plaintext, autoHideSeconds, status);
      setStatus(status, "Plaintext decrypted locally. The private key has already been destroyed server-side.", "success");
    } catch (error) {
      setStatus(status, error instanceof Error ? error.message : "Unable to decrypt this link.", "error");
    }
  });
}

const root = document.querySelector("[data-page]");
if (root instanceof HTMLElement) {
  maybeShowHTTPWarning(root);

  switch (root.dataset.page) {
    case "create":
      void initCreatePage(root);
      break;
    case "share":
      void initSharePage(root);
      break;
    default:
      break;
  }
}
