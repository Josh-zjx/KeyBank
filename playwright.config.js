const { defineConfig } = require("@playwright/test");

const port = process.env.PLAYWRIGHT_PORT || "4173";
const baseURL = `http://localhost:${port}`;

module.exports = defineConfig({
  testDir: "./playwright",
  timeout: 30_000,
  expect: {
    timeout: 5_000
  },
  use: {
    baseURL,
    browserName: "chromium",
    headless: true,
    trace: "on-first-retry"
  },
  webServer: {
    command: `PORT=${port} KEYBANK_STORE=mem AUTO_HIDE_SECONDS=1 go run .`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  }
});
