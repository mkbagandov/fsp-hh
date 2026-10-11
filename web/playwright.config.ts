import { defineConfig, devices } from "@playwright/test";

// E2E против изолированного стенда: e2e/server.mjs пересоздаёт базу *_e2e, заливает сид и поднимает API с web/dist.
// E2E_BASE_URL — прогон против уже запущенного стенда (сервер не стартует).
// E2E_VIDEO=1 — запись видео и замедление действий (для демо-ролика).
const port = process.env.E2E_PORT || "18080";
const external = process.env.E2E_BASE_URL;
const baseURL = external || `http://127.0.0.1:${port}`;
const demo = process.env.E2E_VIDEO === "1";

export default defineConfig({
  testDir: "./e2e",
  timeout: 90_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: demo ? 1 : 2,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : [["list"]],
  use: {
    baseURL,
    locale: "ru-RU",
    timezoneId: "Europe/Moscow",
    viewport: { width: 1280, height: 820 },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: demo ? "on" : "off",
    launchOptions: demo ? { slowMo: 250 } : undefined,
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 820 } } }],
  webServer: external
    ? undefined
    : {
        command: "node e2e/server.mjs",
        url: `${baseURL}/healthz`,
        timeout: 240_000,
        reuseExistingServer: !process.env.CI && process.env.E2E_REUSE === "1",
        stdout: "pipe",
        stderr: "pipe",
      },
});
