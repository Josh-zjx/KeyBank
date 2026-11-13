const { expect, test } = require("@playwright/test");

for (const type of ["ecc", "aes256"]) {
  for (const protectedKey of [false, true]) {
    test(`${type} ${protectedKey ? "protected" : "unprotected"} note round-trip`, async ({ browser }) => {
      const secret = "a Unicode note: 🔑 密碼";
      const passphrase = " separately shared 🔐 ";
      const sender = await browser.newPage();
      const requests = [];
      sender.on("request", (r) => requests.push(r));
      await sender.goto("/");
      await sender.getByLabel("Message", { exact: true }).fill(secret);
      await sender.getByLabel("Key type", { exact: true }).selectOption(type);
      if (protectedKey) {
        await sender.getByLabel("Passphrase (optional)", { exact: true }).fill(passphrase);
      }
      const creation = sender.waitForResponse((r) => r.url().endsWith("/api/v1/keys") && r.request().method() === "POST");
      await sender.getByRole("button", { name: "Generate link" }).click();
      const created = await (await creation).json();
      await expect(sender.locator("#create-status")).toHaveText(/Link generated/);
      const link = await sender.locator("[data-share-url]").inputValue();
      const fragment = new URL(link).hash;
      expect(fragment).toContain(`v=3&t=${type}&p=${protectedKey ? "1" : "0"}`);
      expect(decodeURIComponent(fragment)).not.toContain(passphrase);
      expect(fragment).not.toContain(created.key ?? "no-symmetric-key");
      expect(Object.keys(created).sort()).toEqual(type === "ecc" ? ["id", "public_key", "type"] : ["id", "key", "type"]);
      const recipient = await browser.newPage();
      recipient.on("request", (r) => requests.push(r));
      await recipient.goto(link);
      if (protectedKey) {
        await expect(recipient.getByLabel("Passphrase", { exact: true })).toBeVisible();
        await recipient.getByLabel("Passphrase", { exact: true }).fill("incorrect");
        await recipient.getByRole("button", { name: "Decrypt" }).click();
        await expect(recipient.locator("#share-status")).toHaveText(/incorrect.*key has not been consumed/);
        await expect(recipient.getByRole("button", { name: "Decrypt" })).toBeEnabled();
        await recipient.getByLabel("Passphrase", { exact: true }).fill(passphrase);
      }
      await recipient.getByRole("button", { name: "Decrypt" }).click();
      await expect(recipient.locator("[data-share-plaintext]")).toHaveText(secret);
      for (const page of [sender, recipient]) {
        const ledger = await page.locator("[data-ledger]").textContent();
        expect(ledger).not.toContain(passphrase);
        if (protectedKey) { expect(ledger).toContain("[redacted]"); }
      }
      for (const request of requests) {
        expect(request.postData() ?? "").not.toContain(secret);
        expect(request.postData() ?? "").not.toContain(fragment.slice(1));
        expect(request.url()).not.toContain(fragment.slice(1));
      }
      await recipient.reload();
      if (protectedKey) { await recipient.getByLabel("Passphrase", { exact: true }).fill(passphrase); }
      await recipient.getByRole("button", { name: "Decrypt" }).click();
      await expect(recipient.locator("#share-status")).toHaveText(/already been opened or expired/);
    });
  }
}

test("rejects old and malformed fragments without fetching", async ({ page }) => {
  let fetches = 0;
  page.on("request", (r) => { if (r.url().endsWith("/fetch")) { fetches += 1; } });
  for (const fragment of [
    "v=2&e=AAAA&i=AAAA&c=AAAA",
    "v=3&t=ecc&p=0&e=AAAA&i=AAAA&c=AAAA",
    "v=3&t=aes256&p=0&i=AAAAAAAAAAAAAAAA&c=AAAA",
    "v=3&t=aes256&p=0&p=1&i=AAAAAAAAAAAAAAAA&c=AAAAAAAAAAAAAAAAAAAAAA"
  ]) {
    await page.goto(`/share/0123456789abcdef0123456789abcdef#${fragment}`);
    await expect(page.getByRole("button", { name: "Decrypt" })).toBeDisabled();
  }
  expect(fetches).toBe(0);
});

test("rejects low-order ECC points before consuming a key", async ({ page }) => {
  let fetches = 0;
  page.on("request", (r) => { if (r.url().endsWith("/fetch")) { fetches += 1; } });
  await page.goto(`/share/0123456789abcdef0123456789abcdef#v=3&t=ecc&p=0&e=${"A".repeat(43)}&i=${"A".repeat(16)}&c=${"A".repeat(22)}`);
  await page.getByRole("button", { name: "Decrypt" }).click();
  await expect(page.locator("[data-share-card]")).toHaveAttribute("data-state", "invalid");
  expect(fetches).toBe(0);
});
