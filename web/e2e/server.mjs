// Стенд для E2E: собирает API, пересоздаёт базу *_e2e с сидом и запускает API с раздачей web/dist.
// Запускается Playwright'ом (webServer в playwright.config.ts) и останавливается вместе с ним.
//
// Переменные: E2E_PORT (18080), E2E_DATABASE_URL (по умолчанию DATABASE_URL из app/.env с базой fsp_platform_e2e),
// E2E_CANDIDATES (60) — сколько синтетических кандидатов создать.
import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const app = path.resolve(here, "..", "..");
const web = path.resolve(here, "..");
const port = process.env.E2E_PORT || "18080";
const base = `http://127.0.0.1:${port}`;

function dbFromEnvFile() {
  const envPath = path.join(app, ".env");
  if (!fs.existsSync(envPath)) throw new Error("нет app/.env и не задан E2E_DATABASE_URL");
  const line = fs
    .readFileSync(envPath, "utf8")
    .split(/\r?\n/)
    .find((l) => l.startsWith("DATABASE_URL="));
  if (!line) throw new Error("в app/.env нет DATABASE_URL");
  const u = new URL(line.slice("DATABASE_URL=".length).trim());
  u.pathname = "/fsp_platform_e2e";
  return u.toString();
}

const dbURL = process.env.E2E_DATABASE_URL || dbFromEnvFile();
if (!new URL(dbURL).pathname.endsWith("_e2e")) {
  console.error("E2E: база должна оканчиваться на _e2e — стенд её пересоздаёт");
  process.exit(2);
}
if (!fs.existsSync(path.join(web, "dist", "index.html"))) {
  console.error("E2E: нет web/dist — сначала `npm run build` (task e2e делает это сам)");
  process.exit(2);
}

const env = {
  ...process.env,
  APP_ENV: "dev",
  DATABASE_URL: dbURL,
  HTTP_ADDR: `127.0.0.1:${port}`,
  WEB_DIR: path.join(web, "dist"),
  BASE_URL: base,
  FRONTEND_URL: base,
  CORS_ORIGINS: "",
  COOKIE_SECURE: "false",
  RATE_LIMIT_PER_MINUTE: "0",
  LOG_LEVEL: process.env.E2E_LOG_LEVEL || "warn",
};

function run(cmd, args) {
  const r = spawnSync(cmd, args, { cwd: app, env, stdio: "inherit", shell: false });
  if (r.status !== 0) {
    console.error(`E2E: команда завершилась с ошибкой: ${cmd} ${args.join(" ")}`);
    process.exit(r.status ?? 1);
  }
}

const bin = path.join(os.tmpdir(), `fsp-api-e2e${process.platform === "win32" ? ".exe" : ""}`);
run("go", ["build", "-o", bin, "./cmd/api"]);
run("go", ["run", "./cmd/seed", "-recreate-db", "-candidates", process.env.E2E_CANDIDATES || "60"]);

const api = spawn(bin, [], { cwd: app, env, stdio: "inherit" });
const stop = () => {
  if (!api.killed) api.kill();
};
for (const sig of ["SIGINT", "SIGTERM", "SIGHUP"]) process.on(sig, () => (stop(), process.exit(0)));
process.on("exit", stop);
api.on("exit", (code) => process.exit(code ?? 0));
