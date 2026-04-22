const { expect, test } = require("@playwright/test");

async function createShareURL(page, message) {
  await page.goto("/");
  await page.getByLabel("Message").fill(message);
  await page.getByRole("button", { name: "Generate link" }).click();

  await expect(page.locator("#create-status")).toHaveText(/Link generated\./);
  await expect(page.locator("[data-qr-code] canvas, [data-qr-code] img, [data-qr-code] svg").first()).toBeVisible();

  return page.locator("[data-share-url]").inputValue();
}

function tamperShareURL(shareURL) {
  const url = new URL(shareURL);
  const params = new URLSearchParams(url.hash.slice(1));
  const ciphertext = params.get("c");
  const replacement = ciphertext.endsWith("A") ? "B" : "A";
  params.set("c", `${ciphertext.slice(0, -1)}${replacement}`);
  url.hash = params.toString();
  return url.toString();
}

test("creates and decrypts a note in the browser", async ({ browser }) => {
  const sender = await browser.newPage();
  const shareURL = await createShareURL(sender, "launch code: delta-7");

  const recipientContext = await browser.newContext();
  const recipient = await recipientContext.newPage();
  await recipient.goto(shareURL);
  await recipient.getByRole("button", { name: "Decrypt" }).click();

  await expect(recipient.locator("[data-share-plaintext]")).toHaveText("launch code: delta-7");
  await expect(recipient.locator("#share-status")).toHaveText(/Plaintext decrypted locally/);
});

test("rejects a tampered fragment without rendering plaintext", async ({ browser }) => {
  const sender = await browser.newPage();
  const shareURL = await createShareURL(sender, "tamper detection");

  const recipient = await browser.newPage();
  await recipient.goto(tamperShareURL(shareURL));
  await recipient.getByRole("button", { name: "Decrypt" }).click();

  await expect(recipient.locator("#share-status")).toHaveText(/corrupted or tampered/);
  await expect(recipient.locator("[data-share-result]")).toHaveAttribute("hidden", "");
  await expect(recipient.locator("[data-share-plaintext]")).toHaveText("");
});

test("auto-hides plaintext and lets the user keep it visible", async ({ browser }) => {
  const sender = await browser.newPage();
  const autoHideURL = await createShareURL(sender, "auto hide me");

  const autoHideRecipient = await browser.newPage();
  await autoHideRecipient.goto(autoHideURL);
  await autoHideRecipient.getByRole("button", { name: "Decrypt" }).click();
  await expect(autoHideRecipient.locator("[data-share-plaintext]")).toHaveText("auto hide me");
  await expect(autoHideRecipient.locator("[data-share-result]")).toHaveAttribute("hidden", "", { timeout: 4_000 });
  await expect(autoHideRecipient.locator("#share-status")).toHaveText(/hidden again after 1 seconds/);

  const keepVisibleURL = await createShareURL(sender, "stay visible");
  const keepVisibleRecipient = await browser.newPage();
  await keepVisibleRecipient.goto(keepVisibleURL);
  await keepVisibleRecipient.getByRole("button", { name: "Decrypt" }).click();
  await keepVisibleRecipient.getByRole("button", { name: "Keep visible" }).click();

  await expect(keepVisibleRecipient.locator("[data-share-plaintext]")).toHaveText("stay visible");
  await keepVisibleRecipient.waitForTimeout(1_500);
  await expect(keepVisibleRecipient.locator("[data-share-result]")).toBeVisible();
  await expect(keepVisibleRecipient.locator("#share-status")).toHaveText(/stay visible until you leave or reload/);
});
