// HTTP-клиент поверх openapi-fetch: типы из docs/api/openapi.yaml (npm run gen:api),
// bearer-токен из сессии, упреждающее обновление по refresh-cookie, разбор problem+json.
import createClient, { type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";

export type Schemas = components["schemas"];

/** Ошибка API в формате RFC 9457 (huma): код — в errors[].value. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly detail: string;
  readonly fields: Record<string, string>;

  constructor(status: number, body: unknown) {
    const p = (body ?? {}) as { title?: string; detail?: string; errors?: { message?: string; location?: string; value?: unknown }[] };
    const first = p.errors?.[0];
    super(p.detail || p.title || `HTTP ${status}`);
    this.status = status;
    this.detail = p.detail || p.title || "";
    this.code = typeof first?.value === "string" ? first.value : status === 401 ? "unauthorized" : status === 403 ? "forbidden" : "error";
    this.fields = {};
    for (const e of p.errors ?? []) {
      if (e.location && e.message) {
        this.fields[e.location.replace(/^body\./, "")] = e.message;
      }
    }
  }

  is(code: string): boolean {
    return this.code === code;
  }
}

type Session = { accessToken: string; expiresAt: number };
let session: Session | null = null;
let refreshing: Promise<boolean> | null = null;
const listeners = new Set<(s: Session | null) => void>();

export function getSession(): Session | null {
  return session;
}
const FLAG = "fsp_session";

export function setSession(next: Session | null): void {
  session = next;
  try {
    // Cookie HttpOnly не видна скрипту: флаг подсказывает, стоит ли пробовать refresh при загрузке.
    if (next) localStorage.setItem(FLAG, "1");
    else localStorage.removeItem(FLAG);
  } catch {
    /* приватный режим */
  }
  for (const l of listeners) l(next);
}

export function mayHaveSession(): boolean {
  try {
    return localStorage.getItem(FLAG) === "1";
  } catch {
    return false;
  }
}
export function onSession(l: (s: Session | null) => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** Обновляет access-токен по HttpOnly cookie fsp_refresh. Один запрос на все ожидающие. */
export function refreshSession(): Promise<boolean> {
  if (refreshing) return refreshing;
  refreshing = (async () => {
    try {
      const res = await fetch("/api/v1/auth/refresh", { method: "POST", credentials: "include", headers: { Accept: "application/json" } });
      if (!res.ok) {
        setSession(null);
        return false;
      }
      const body = (await res.json()) as { access_token: string; expires_at: string };
      setSession({ accessToken: body.access_token, expiresAt: Date.parse(body.expires_at) });
      return true;
    } catch {
      return false;
    } finally {
      refreshing = null;
    }
  })();
  return refreshing;
}

const AUTH_FREE = ["/auth/login", "/auth/register", "/auth/verify", "/auth/refresh", "/auth/password/forgot", "/auth/password/reset"];

const auth: Middleware = {
  async onRequest({ request, schemaPath }) {
    if (AUTH_FREE.some((p) => schemaPath.startsWith(p))) return request;
    if (session && session.expiresAt - Date.now() < 30_000) {
      await refreshSession();
    }
    if (session) request.headers.set("Authorization", `Bearer ${session.accessToken}`);
    return request;
  },
  async onResponse({ response, schemaPath }) {
    if (response.status === 401 && session && !AUTH_FREE.some((p) => schemaPath.startsWith(p))) {
      // Токен отозван или истёк между проверкой и ответом: сессии больше нет.
      setSession(null);
    }
    return response;
  },
};

export const api = createClient<paths>({ baseUrl: "/api/v1", credentials: "include" });
api.use(auth);

/** Разворачивает ответ openapi-fetch в данные или бросает ApiError. */
export function unwrap<T>(result: { data?: T; error?: unknown; response: Response }): T {
  if (result.error !== undefined || !result.response.ok) {
    throw new ApiError(result.response.status, result.error);
  }
  return result.data as T;
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

export function errorMessage(e: unknown): string {
  if (isApiError(e)) return e.detail || e.message;
  if (e instanceof Error) return e.message;
  return "Что-то пошло не так";
}
