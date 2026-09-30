import { createKey } from "./api.js";
import { encryptMessage } from "./crypto.js";
import { buildShareURL, estimateLinkLength, qrCapacity } from "./link.js";
import { copyText, formatDuration, plural, renderAnatomy, setStatus } from "./ui.js";

const defaultTTLSeconds = 24 * 60 * 60;
const maxTTLSeconds = 7 * 24 * 60 * 60;
const softPlaintextLimitBytes = 100 * 1024;
const textEncoder = new TextEncoder();

function byteLength(value) {
  return textEncoder.encode(value).length;
}

function describeMessage(plaintext) {
  if (plaintext === "") {
    return "Short notes make short links.";
  }

  const bytes = byteLength(plaintext);
  const linkLength = estimateLinkLength(bytes);
  const qr = linkLength <= qrCapacity ? "fits in a QR code" : "too long for a QR code";
  return `${plural(bytes, "byte")} · link ≈ ${plural(linkLength, "character")} · ${qr}`;
}

function expiryPresets(form) {
  return Array.from(form.querySelectorAll('input[name="expiryPreset"]'));
}

function isCustomSelected(form) {
  return expiryPresets(form).some((input) => input.checked && input.value === "custom");
}

function selectedTTL(form, maxTTL) {
  const selected = form.querySelector('input[name="expiryPreset"]:checked');
  if (!selected || selected.value === "") {
    return Math.min(defaultTTLSeconds, maxTTL);
  }

  if (selected.value !== "custom") {
    const ttl = Number(selected.value);
    if (ttl > maxTTL) {
      throw new Error("That expiry exceeds this server's configured limit.");
    }
    return ttl;
  }

  const minutes = Number(form.querySelector("#custom-ttl").value);
  if (!Number.isInteger(minutes) || minutes < 1) {
    throw new Error("Enter a custom expiry in whole minutes (at least 1).");
  }

  const ttl = minutes * 60;
  if (ttl > maxTTL) {
    throw new Error(`Custom expiry exceeds this server's limit of ${formatDuration(maxTTL)}.`);
  }
  return ttl;
}

// Disables presets longer than the server allows and caps the custom input to match.
function configureExpiryOptions(form, customInput, maxTTL) {
  const presets = expiryPresets(form);
  for (const input of presets) {
    if (input.value === "custom") {
      continue;
    }
    input.disabled = Number(input.value) > maxTTL;
    if (input.disabled) {
      input.checked = false;
    }
  }

  const maxMinutes = Math.max(1, Math.floor(maxTTL / 60));
  customInput.max = String(maxMinutes);
  if (Number(customInput.value) > maxMinutes) {
    customInput.value = String(maxMinutes);
  }

  if (!presets.some((input) => input.checked && !input.disabled)) {
    const longestAvailable = presets.filter((input) => input.value !== "custom" && !input.disabled).at(-1);
    (longestAvailable ?? presets.find((input) => input.value === "custom")).checked = true;
  }
}

async function confirmLargeMessage(dialog, bytes) {
  if (typeof dialog?.showModal !== "function") {
    return window.confirm("This note is large. Continue anyway?");
  }

  dialog.querySelector("[data-size-warning-text]").textContent =
    `This note is ${plural(bytes, "byte")} once encoded as UTF-8.`;

  return new Promise((resolve) => {
    dialog.addEventListener("close", () => resolve(dialog.returnValue === "confirm"), { once: true });
    dialog.showModal();
  });
}

function renderQRCode(container, value) {
  container.replaceChildren();
  if (typeof window.QRCode !== "function") {
    container.textContent = "QR code unavailable. Copy the share URL instead.";
    return false;
  }

  try {
    new window.QRCode(container, {
      text: value,
      width: 176,
      height: 176,
      colorDark: "#0b1220",
      colorLight: "#ffffff",
      correctLevel: window.QRCode.CorrectLevel.M
    });
    return true;
  } catch {
    container.replaceChildren();
    container.textContent = "This link is too long for a QR code. Copy the share URL instead.";
    return false;
  }
}

export function initCreatePage(root, ledger) {
  const form = root.querySelector("[data-create-form]");
  const messageInput = form.querySelector("#message");
  const messageMeta = form.querySelector("[data-message-meta]");
  const customInput = form.querySelector("#custom-ttl");
  const customField = form.querySelector("[data-custom-ttl]");
  const submitButton = form.querySelector('button[type="submit"]');
  const status = root.querySelector("#create-status");
  const result = root.querySelector("#create-result");
  const resultTitle = result.querySelector("#result-title");
  const resultExpiry = result.querySelector("[data-result-expiry]");
  const shareURL = result.querySelector("[data-share-url]");
  const copyButton = result.querySelector("[data-copy-link]");
  const nativeShareButton = result.querySelector("[data-native-share]");
  const openShareLink = result.querySelector("[data-open-share]");
  const newNoteButton = result.querySelector("[data-new-note]");
  const qrContainer = result.querySelector("[data-qr-code]");
  const sizeWarning = root.querySelector("[data-size-warning]");

  const configuredMaxTTL = Number(root.dataset.maxTtlSeconds);
  const maxTTL = Number.isFinite(configuredMaxTTL) && configuredMaxTTL >= 60
    ? Math.min(configuredMaxTTL, maxTTLSeconds)
    : maxTTLSeconds;

  const syncCustomInput = () => {
    const custom = isCustomSelected(form);
    customInput.disabled = !custom;
    customField.hidden = !custom;
  };

  configureExpiryOptions(form, customInput, maxTTL);
  syncCustomInput();
  for (const input of expiryPresets(form)) {
    input.addEventListener("change", () => {
      syncCustomInput();
      if (input.value === "custom" && input.checked) {
        customInput.focus();
      }
    });
  }

  messageInput.addEventListener("input", () => {
    messageMeta.textContent = describeMessage(messageInput.value);
  });

  copyButton.addEventListener("click", async () => {
    try {
      const copied = await copyText(copyButton, shareURL.value);
      if (!copied) {
        shareURL.select();
      }
      setStatus(status, copied ? "Share URL copied to your clipboard." : "Copy the share URL manually from the box above.", "success");
    } catch {
      shareURL.select();
      setStatus(status, "Copy failed. Select the share URL and copy it manually.", "error");
    }
  });

  nativeShareButton.hidden = typeof navigator.share !== "function";
  nativeShareButton.addEventListener("click", () => {
    navigator.share({ title: "A one-time note", url: shareURL.value }).catch(() => {});
  });

  newNoteButton.addEventListener("click", () => {
    form.reset();
    configureExpiryOptions(form, customInput, maxTTL);
    syncCustomInput();
    messageMeta.textContent = describeMessage("");
    result.hidden = true;
    shareURL.value = "";
    qrContainer.replaceChildren();
    setStatus(status, "Ready for a new note.", "idle");
    messageInput.focus();
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
      ttl = selectedTTL(form, maxTTL);
    } catch (error) {
      setStatus(status, error.message, "error");
      return;
    }

    const bytes = byteLength(plaintext);
    if (bytes > softPlaintextLimitBytes && !(await confirmLargeMessage(sizeWarning, bytes))) {
      setStatus(status, "Link generation cancelled.", "idle");
      return;
    }

    result.hidden = true;
    submitButton.disabled = true;
    setStatus(status, "Requesting a one-time keypair and encrypting locally…", "working");

    try {
      const { id, pub_pem: pubPem } = await createKey(ledger, ttl);
      const url = buildShareURL(id, await encryptMessage(pubPem, plaintext));

      shareURL.value = url;
      openShareLink.href = url;
      resultExpiry.textContent = `It opens once, and expires in ${formatDuration(ttl)} if nobody opens it.`;
      renderAnatomy(result, url);
      result.hidden = false;
      const qrRendered = renderQRCode(qrContainer, url);
      resultTitle.focus();

      setStatus(
        status,
        qrRendered
          ? "Link generated. Your message never left this browser; it is encrypted inside the link."
          : "Link generated. This link is too long for a QR code, so copy the share URL instead.",
        "success"
      );
    } catch (error) {
      setStatus(status, error instanceof Error ? error.message : "Link generation failed.", "error");
    } finally {
      submitButton.disabled = false;
    }
  });
}
