import { splitShareURL } from "./link.js";

const copiedFeedbackMs = 2000;

export function setStatus(element, message, state) {
  element.textContent = message;
  element.dataset.state = state;
}

export function plural(count, unit) {
  return `${count.toLocaleString()} ${unit}${count === 1 ? "" : "s"}`;
}

// Formats whole seconds using the largest unit that divides them evenly.
export function formatDuration(seconds) {
  for (const [unit, size] of [["day", 86400], ["hour", 3600], ["minute", 60]]) {
    if (seconds % size === 0) {
      return plural(seconds / size, unit);
    }
  }
  return plural(seconds, "second");
}

// Copies text and briefly confirms on the button itself. Resolves to whether it worked.
export async function copyText(button, text) {
  if (!navigator.clipboard?.writeText) {
    return false;
  }

  await navigator.clipboard.writeText(text);

  const label = button.querySelector("[data-copy-label]");
  const original = label.textContent;
  label.textContent = "Copied";
  button.dataset.copied = "";
  window.setTimeout(() => {
    label.textContent = original;
    delete button.dataset.copied;
  }, copiedFeedbackMs);
  return true;
}

// Fills the link anatomy figure: the part of the link sent to KeyBank and the part kept locally.
export function renderAnatomy(root, href) {
  const { sent, kept } = splitShareURL(href);
  root.querySelector("[data-anatomy-path]").textContent = sent;
  root.querySelector("[data-anatomy-fragment]").textContent = kept || "# (missing)";
  root.querySelector("[data-anatomy-size]").textContent = kept ? `, ${plural(kept.length, "character")}` : "";
}
