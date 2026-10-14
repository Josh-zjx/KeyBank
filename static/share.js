import { fetchKey } from "./api.js";
import { decryptMessage } from "./crypto.js";
import { parseFragment } from "./link.js";
import { copyText, plural, renderAnatomy, setStatus } from "./ui.js";

const tickMs = 250;

// Shows the plaintext for a limited time, with the option to keep it on screen.
function reveal(root, plaintext, autoHideSeconds, status) {
  const result = root.querySelector("[data-share-result]");
  const output = result.querySelector("[data-share-plaintext]");
  const keepVisibleButton = result.querySelector("[data-keep-visible]");
  const copyButton = result.querySelector("[data-copy-plaintext]");
  const countdown = result.querySelector("[data-hide-countdown]");
  const countdownUnit = result.querySelector("[data-hide-countdown-unit]");
  const countdownBar = result.querySelector("[data-countdown-bar]");

  const renderRemaining = (seconds) => {
    countdown.textContent = String(seconds);
    countdown.dataset.infinite = "false";
    countdown.setAttribute("aria-label", `${plural(seconds, "second")} remaining`);
    countdownUnit.textContent = seconds === 1 ? " second" : " seconds";
  };

  const hide = () => {
    window.clearInterval(timer);
    output.textContent = "";
    result.hidden = true;
    setStatus(status, `Plaintext hidden again after ${plural(autoHideSeconds, "second")}. It cannot be shown again: the key no longer exists.`, "success");
  };

  const hideAt = Date.now() + autoHideSeconds * 1000;
  const timer = window.setInterval(() => {
    const remaining = Math.max(0, Math.ceil((hideAt - Date.now()) / 1000));
    renderRemaining(remaining);
    if (remaining === 0) {
      hide();
    }
  }, tickMs);

  keepVisibleButton.addEventListener("click", () => {
    window.clearInterval(timer);
    countdown.textContent = "∞";
    countdown.dataset.infinite = "true";
    countdown.setAttribute("aria-label", "No time limit");
    countdownUnit.textContent = " seconds";
    countdownBar.dataset.state = "stopped";
    keepVisibleButton.disabled = true;
    keepVisibleButton.textContent = "Visible until you leave this page";
    setStatus(status, "Plaintext will stay visible until you leave or reload this page.", "success");
  }, { once: true });

  copyButton.addEventListener("click", () => {
    copyText(copyButton, plaintext).catch(() => {
      setStatus(status, "Copy failed. Select the text and copy it manually.", "error");
    });
  });

  output.textContent = plaintext;
  renderRemaining(autoHideSeconds);
  countdownBar.style.setProperty("--autohide-duration", `${autoHideSeconds}s`);
  countdownBar.dataset.state = "running";
  keepVisibleButton.hidden = false;
  result.hidden = false;
}

export function initSharePage(root, ledger) {
  const card = root.querySelector("[data-share-card]");
  const button = card.querySelector("[data-decrypt-button]");
  const status = card.querySelector("#share-status");
  const shareID = root.dataset.shareId;
  const autoHideSeconds = Number(root.dataset.autohideSeconds) || 60;
  const readyMessage = status.textContent;

  // ready → working → open | spent; a malformed link is invalid and never touches the key.
  const setState = (state) => {
    card.dataset.state = state;
    button.disabled = state !== "ready";
  };

  let fragment = null;
  const inspectLink = () => {
    renderAnatomy(card, window.location.href);
    try {
      fragment = parseFragment(window.location.hash);
      setState("ready");
      setStatus(status, readyMessage, "idle");
    } catch (error) {
      fragment = null;
      setState("invalid");
      setStatus(status, error.message, "error");
    }
  };

  inspectLink();
  window.addEventListener("hashchange", () => {
    if (card.dataset.state === "ready" || card.dataset.state === "invalid") {
      inspectLink();
    }
  });

  button.addEventListener("click", async () => {
    if (fragment === null || !shareID) {
      return;
    }

    setState("working");
    setStatus(status, "Fetching the one-time private key…", "working");

    let privPem;
    try {
      privPem = await fetchKey(ledger, shareID);
    } catch (error) {
      setState("ready");
      setStatus(status, error.message, "error");
      return;
    }

    if (privPem === null) {
      setState("spent");
      setStatus(status, "This note has already been opened or expired. If you did not open it yourself, someone else may have: ask the sender for a new link.", "error");
      return;
    }

    let plaintext;
    try {
      plaintext = await decryptMessage(privPem, fragment.wrapped, fragment.iv, fragment.ct);
    } catch {
      setState("spent");
      setStatus(status, "Unable to decrypt this link. The encrypted fragment may be corrupted or tampered with.", "error");
      return;
    }

    setState("open");
    reveal(root, plaintext, autoHideSeconds, status);
    setStatus(status, "Plaintext decrypted locally. The private key has already been destroyed server-side.", "success");
  });
}
