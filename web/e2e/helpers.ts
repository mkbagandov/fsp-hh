import { expect, type APIRequestContext, type Browser, type BrowserContext, type Page, type TestInfo } from "@playwright/test";

export const PASSWORD = "E2e-Passw0rd!";
export const DEMO_PASSWORD = "Demo1234!";

/** Уникальный e-mail тестового пользователя (база e2e пересоздаётся перед прогоном). */
export function uniqueEmail(tag: string): string {
  return `ui-e2e-${tag}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}@test.local`;
}

/** Код подтверждения из консольного мейлера (/api/v1/dev/outbox доступен только на dev-стенде). */
export async function codeFor(request: APIRequestContext, email: string): Promise<string> {
  for (let i = 0; i < 40; i++) {
    const res = await request.get("/api/v1/dev/outbox");
    expect(res.ok()).toBeTruthy();
    const body = (await res.json()) as { messages: { to: string; text: string }[] | null };
    const msg = [...(body.messages ?? [])].reverse().find((m) => m.to === email);
    const code = msg?.text.match(/\b(\d{6})\b/)?.[1];
    if (code) return code;
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`нет письма с кодом для ${email}`);
}

type Json = Record<string, unknown>;

/** Тонкий клиент API для подготовки данных в тестах. */
export class Api {
  constructor(
    private readonly request: APIRequestContext,
    public token = "",
  ) {}

  private headers() {
    return this.token ? { Authorization: `Bearer ${this.token}` } : {};
  }

  async call<T = Json>(method: "GET" | "POST" | "PUT" | "DELETE", url: string, data?: unknown, expectStatus?: number): Promise<{ status: number; body: T }> {
    const res = await this.request.fetch(`/api/v1${url}`, { method, data, headers: this.headers() });
    const text = await res.text();
    const body = (text ? JSON.parse(text) : null) as T;
    if (expectStatus !== undefined) expect(res.status(), `${method} ${url}: ${text.slice(0, 300)}`).toBe(expectStatus);
    return { status: res.status(), body };
  }

  /** Регистрация с подтверждением e-mail; токен сохраняется в клиенте. */
  async signup(role: "candidate" | "employer", email: string): Promise<string> {
    await this.call("POST", "/auth/register", { email, password: PASSWORD, role, consents: { processing: true } }, 201);
    const code = await codeFor(this.request, email);
    const { body } = await this.call<{ access_token: string; user: { id: string } }>("POST", "/auth/verify", { email, code }, 200);
    this.token = body.access_token;
    return body.user.id;
  }

  async login(email: string, password = PASSWORD): Promise<void> {
    const { body } = await this.call<{ access_token: string }>("POST", "/auth/login", { email, password }, 200);
    this.token = body.access_token;
  }
}

export type Candidate = { api: Api; id: string; email: string; firstName: string; lastName: string; city: string; telegram: string };

/**
 * Кандидат, готовый к подбору: анкета с уникальным городом (по нему подборка фильтруется до одного человека),
 * согласие на публикацию, контакты, опрос и завершённый тест (ответы произвольные — категория будет не подтверждена).
 */
export async function readyCandidate(request: APIRequestContext, tag: string, opts: { withTest?: boolean } = {}): Promise<Candidate> {
  const api = new Api(request);
  const email = uniqueEmail(tag);
  const id = await api.signup("candidate", email);
  const suffix = Math.random().toString(36).slice(2, 7);
  // Telegram хранится без «@» (нормализация в сервисе кандидатов), в интерфейсе показывается с «@».
  const c: Candidate = { api, id, email, firstName: "Тест", lastName: `Е2е${suffix}`, city: `Е2Е-${suffix}`, telegram: `e2e_${suffix}` };
  await api.call("PUT", "/candidate/profile", {
    first_name: c.firstName,
    last_name: c.lastName,
    city: c.city,
    work_formats: ["remote"],
    salary_expected: 250000,
    visibility: "open",
    primary_specialization: "backend",
    headline: "Backend на Go, сквозной тест",
  }, 200);
  await api.call("POST", "/auth/consents", { kind: "publication" }, 200);
  await api.call("PUT", "/candidate/contacts", { telegram: c.telegram, other: {} }, 200);
  if (opts.withTest !== false) {
    await api.call("POST", "/candidate/survey", { specialization: "backend", claimed_grade: "middle" }, 201);
    const start = await api.call<{ session: { id: string } }>("POST", "/candidate/tests", { specialization: "backend", target_grade: "middle" });
    expect([200, 201]).toContain(start.status);
    await finishTest(api, start.body.session.id);
  }
  return c;
}

/** Отвечает на все оставшиеся задания сессии произвольными, но корректными по формату ответами. */
export async function finishTest(api: Api, sessionId: string): Promise<void> {
  for (let i = 0; i < 40; i++) {
    const { body } = await api.call<{ finished: boolean; item?: { id: string; format: string; options?: { key: string }[] | null } }>("GET", `/candidate/tests/${sessionId}/item`, undefined, 200);
    if (body.finished || !body.item) return;
    const it = body.item;
    const keys = (it.options ?? []).map((o) => o.key);
    const submission =
      it.format === "single_choice" || it.format === "multi_choice"
        ? { keys: keys.slice(0, 1) }
        : it.format === "ordering"
          ? { order: keys }
          : it.format === "numeric"
            ? { number: 1 }
            : it.format === "sql_result"
              ? { rows: [["1"]] }
              : { text: "1" };
    const res = await api.call<{ finished: boolean }>("POST", `/candidate/tests/${sessionId}/answer`, { item_id: it.id, submission }, 200);
    if (res.body.finished) return;
  }
}

/** Работодатель с компанией (через API). */
export async function readyEmployer(request: APIRequestContext, tag: string, company = "Компания Е2Е"): Promise<{ api: Api; email: string; companyId: string }> {
  const api = new Api(request);
  const email = uniqueEmail(tag);
  await api.signup("employer", email);
  const { body } = await api.call<{ id: string }>("PUT", "/employer/company", { name: `${company} ${Math.random().toString(36).slice(2, 6)}`, public_contacts: { site: "https://e2e.example/jobs" } }, 200);
  return { api, email, companyId: body.id };
}

/** Отдельный контекст браузера (вторая роль в сценарии); в демо-режиме пишет видео рядом с результатами теста. */
export async function roleContext(browser: Browser, testInfo: TestInfo, role: string): Promise<BrowserContext> {
  const video = process.env.E2E_VIDEO === "1";
  return browser.newContext({
    locale: "ru-RU",
    timezoneId: "Europe/Moscow",
    viewport: { width: 1280, height: 820 },
    recordVideo: video ? { dir: testInfo.outputPath(`video-${role}`), size: { width: 1280, height: 820 } } : undefined,
  });
}

/** Вход через форму; ждёт перехода в кабинет. */
export async function uiLogin(page: Page, email: string, password = PASSWORD): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("E-mail", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL(/\/(candidate|employer|admin)/);
}

/** Собирает ошибки консоли страницы (кроме ожидаемых сетевых 4xx, которые UI обрабатывает сам). */
export function collectConsoleErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error" && !/status of (401|403|404|409|422)/.test(msg.text())) errors.push(msg.text());
  });
  page.on("pageerror", (err) => errors.push(String(err)));
  return errors;
}
