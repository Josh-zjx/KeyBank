const { defineConfig } = require("@playwright/test");

const externalBaseURL = process.env.PLAYWRIGHT_BASE_URL;
const port = process.env.PLAYWRIGHT_PORT || "4173";
const apiPort = process.env.PLAYWRIGHT_API_PORT || "4174";
const baseURL = externalBaseURL || `http://127.0.0.1:${port}`;
const apiURL = `http://127.0.0.1:${apiPort}`;
const webServer = externalBaseURL ? undefined : [
  {
    command: `PORT=${apiPort} KEYBANK_STORE=mem CORS_ALLOWED_ORIGINS=${baseURL} go run ./cmd/keybank`,
    url: `${apiURL}/healthz`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  },
  {
    command: `PORT=${port} API_BASE_URL=${apiURL}/api/v1 AUTO_HIDE_SECONDS=1 go run ./cmd/demo`,
    url: `${baseURL}/healthz`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  }
];

module.exports = defineConfig({
  testDir: "./playwright",
  timeout: 30_000,
  expect: { timeout: 5_000 },
  use: { baseURL, browserName: "chromium", headless: true, trace: "on-first-retry" },
  webServer
});
