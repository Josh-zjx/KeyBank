const { defineConfig } = require("@playwright/test");

const externalBaseURL = process.env.PLAYWRIGHT_BASE_URL;
const port = process.env.PLAYWRIGHT_PORT || "4173";
const baseURL = externalBaseURL || `http://127.0.0.1:${port}`;

const webServer = externalBaseURL
  ? undefined
  : {
      command: `PORT=${port} KEYBANK_STORE=mem AUTO_HIDE_SECONDS=1 go run .`,
      url: baseURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000
    };

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
  webServer
});
