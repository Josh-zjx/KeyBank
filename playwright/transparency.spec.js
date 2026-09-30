const { expect, test } = require("@playwright/test");

const secret = "the plaintext must never reach the server";

function recordRequests(page) {
  const requests = [];
  page.on("request", (request) => {
    requests.push({ method: request.method(), url: request.url(), body: request.postData() ?? "" });
  });
  return requests;
}

test("never sends the message or the fragment to the server", async ({ browser }) => {
  const sender = await browser.newPage();
  const senderRequests = recordRequests(sender);
  await sender.goto("/");
  await sender.getByLabel("Message").fill(secret);
  await sender.getByRole("button", { name: "Generate link" }).click();
  await expect(sender.locator("#create-status")).toHaveText(/Link generated\./);
  const shareURL = await sender.locator("[data-share-url]").inputValue();

  const recipient = await browser.newPage();
  const recipientRequests = recordRequests(recipient);
  await recipient.goto(shareURL);
  await recipient.getByRole("button", { name: "Decrypt" }).click();
  await expect(recipient.locator("[data-share-plaintext]")).toHaveText(secret);

  const fragment = new URL(shareURL).hash.slice(1);
  for (const { url, body } of [...senderRequests, ...recipientRequests]) {
    expect(url).not.toContain(fragment);
    expect(body).not.toContain(secret);
    expect(body).not.toContain(fragment);
  }
});

test("lists each API request in the ledger exactly as sent", async ({ page }) => {
  const requests = recordRequests(page);
  await page.goto("/");
  await page.getByLabel("Message").fill(secret);
  await page.locator('input[name="expiryPreset"][value="3600"]').check();
  await page.getByRole("button", { name: "Generate link" }).click();
  await expect(page.locator("#create-status")).toHaveText(/Link generated\./);

  const create = requests.find(({ method, url }) => method === "POST" && url.endsWith("/api/keys"));
  const entry = page.locator("[data-ledger] .ledger-entry").filter({ hasText: "/api/keys" });
  await expect(entry.locator("[data-sent]")).toHaveText(create.body);
  await expect(entry.locator("[data-received]")).toHaveText(/^201 /);
  await expect(page.locator("[data-result-expiry]")).toContainText("1 hour");
});

test("refuses to spend the key when the link has no fragment", async ({ page }) => {
  const requests = recordRequests(page);
  await page.goto("/share/0123456789abcdef0123456789abcdef");

  await expect(page.locator("#share-status")).toHaveText(/missing its encrypted fragment/);
  await expect(page.getByRole("button", { name: "Decrypt" })).toBeDisabled();
  expect(requests.some(({ url }) => url.includes("/api/keys/"))).toBe(false);
});

test("explains a spent link and retires the decrypt button", async ({ browser }) => {
  const sender = await browser.newPage();
  await sender.goto("/");
  await sender.getByLabel("Message").fill(secret);
  await sender.getByRole("button", { name: "Generate link" }).click();
  await expect(sender.locator("#create-status")).toHaveText(/Link generated\./);
  const shareURL = await sender.locator("[data-share-url]").inputValue();

  const first = await browser.newPage();
  await first.goto(shareURL);
  await first.getByRole("button", { name: "Decrypt" }).click();
  await expect(first.locator("[data-share-plaintext]")).toHaveText(secret);
  await expect(first.getByRole("button", { name: "Decrypt" })).toBeHidden();

  const second = await browser.newPage();
  await second.goto(shareURL);
  await second.getByRole("button", { name: "Decrypt" }).click();
  await expect(second.locator("#share-status")).toHaveText(/already been opened or expired/);
  await expect(second.locator("[data-ledger]")).toContainText("404");
  await expect(second.getByRole("button", { name: "Decrypt" })).toBeHidden();
});
