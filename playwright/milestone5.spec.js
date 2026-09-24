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

test("round-trips messages across chunk-size cascade boundaries", async ({ browser }) => {
  const cases = [
    "a".repeat(0x80),
    "b".repeat(0x800),
    "c".repeat(0x801)
  ];

  const sender = await browser.newPage();
  for (const message of cases) {
    await sender.goto("/");
    await sender.getByLabel("Message").fill(message);
    await sender.getByRole("button", { name: "Generate link" }).click();
    await expect(sender.locator("#create-status")).toHaveText(/Link generated/);
    const shareURL = await sender.locator("[data-share-url]").inputValue();

    const recipient = await browser.newPage();
    await recipient.goto(shareURL);
    await recipient.getByRole("button", { name: "Decrypt" }).click();
    await recipient.getByRole("button", { name: "Keep visible" }).click();
    await expect(recipient.locator("[data-share-plaintext]")).toHaveText(message);
    await recipient.close();
  }
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
  await expect(autoHideRecipient.locator("[data-hide-countdown]")).toHaveText("1");
  await expect(autoHideRecipient.locator("[data-hide-countdown]")).toHaveAttribute("aria-label", "1 second remaining");
  await expect(autoHideRecipient.locator("[data-share-result]")).toHaveAttribute("hidden", "", { timeout: 4_000 });
  await expect(autoHideRecipient.locator("#share-status")).toHaveText(/hidden again after 1 second/);

  const keepVisibleURL = await createShareURL(sender, "stay visible");
  const keepVisibleRecipient = await browser.newPage();
  await keepVisibleRecipient.goto(keepVisibleURL);
  await keepVisibleRecipient.getByRole("button", { name: "Decrypt" }).click();
  await keepVisibleRecipient.locator("[data-share-result] [data-keep-visible]").click();

  await expect(keepVisibleRecipient.locator("[data-share-plaintext]")).toHaveText("stay visible");
  await expect(keepVisibleRecipient.locator("[data-hide-countdown]")).toHaveText("∞");
  await expect(keepVisibleRecipient.locator("[data-hide-countdown]")).toHaveAttribute("aria-label", "No time limit");
  await expect(keepVisibleRecipient.locator(".plaintext-toolbar")).toContainText("Visible in ∞ seconds");
  await keepVisibleRecipient.waitForTimeout(1_500);
  await expect(keepVisibleRecipient.locator("[data-share-result]")).toBeVisible();
  await expect(keepVisibleRecipient.locator("#share-status")).toHaveText(/stay visible until you leave or reload/);
});

test("keeps long share links usable when they exceed QR capacity", async ({ browser }) => {
  const sender = await browser.newPage();
  const message = "x".repeat(3000);

  await sender.goto("/");
  await sender.getByLabel("Message").fill(message);
  await sender.getByRole("button", { name: "Generate link" }).click();

  await expect(sender.locator("#create-status")).toHaveText(/Link generated.*too long for a QR code/);
  await expect(sender.locator("[data-qr-code]")).toHaveText(/Copy the share URL instead/);
  const shareURL = await sender.locator("[data-share-url]").inputValue();

  const recipient = await browser.newPage();
  await recipient.goto(shareURL);
  await recipient.getByRole("button", { name: "Decrypt" }).click();
  await expect(recipient.locator("[data-share-plaintext]")).toHaveText(message);
});

test("rejects fractional custom minutes before requesting a key", async ({ page }) => {
  let createRequests = 0;
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/api/keys")) {
      createRequests += 1;
    }
  });

  await page.goto("/");
  await page.getByLabel("Message").fill("fractional expiry");
  await page.locator('input[name="expiryPreset"][value="custom"]').check();
  await page.getByLabel("Custom expiry in minutes").fill("1.01");
  await page.getByRole("button", { name: "Generate link" }).click();

  await expect(page.locator("#create-status")).toHaveText(/whole minutes/);
  expect(createRequests).toBe(0);
});

test("renders a placeholder page for unknown routes", async ({ page }) => {
  const response = await page.goto("/does-not-exist");
  expect(response.status()).toBe(404);
  await expect(page.getByRole("heading", { name: "Page not found" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Create a secure note" })).toHaveAttribute("href", "/");
});
