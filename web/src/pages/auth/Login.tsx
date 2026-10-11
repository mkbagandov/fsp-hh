import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { AuthLayout } from "./AuthLayout";
import { Button, Field, Input, Checkbox } from "@/components/ui";
import { api, unwrap, isApiError, errorMessage } from "@/api/client";
import { useSession, homeFor } from "@/api/session";
import { safeNext } from "@/lib/nav";

// Демо-доступы стенда: показываются на странице входа, чтобы жюри могли войти в один клик
// и проверить проект под разными ролями. Пароль общий — задаётся сидом (task seed).
const DEMO_PASSWORD = "Demo1234!";
const DEMO_ACCOUNTS: { role: string; email: string; hint: string }[] = [
  { role: "Работодатель", email: "demo-employer@seed.fsp.local", hint: "подбор по категориям, приглашения" },
  { role: "Кандидат", email: "demo-candidate@seed.fsp.local", hint: "профиль, категории, приглашения" },
  { role: "Кандидат (новый)", email: "demo-candidate-new@seed.fsp.local", hint: "пройти тест с нуля" },
  { role: "Администратор", email: "demo-admin@seed.fsp.local", hint: "пользователи, аудит, метрики" },
];

export function Login() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const { applySession } = useSession();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [demoBusy, setDemoBusy] = useState<string | null>(null);
  const [fspBusy, setFspBusy] = useState(false);
  const [fspConsent, setFspConsent] = useState(true);

  // doLogin — единый путь входа по e-mail и паролю (и для формы, и для демо-кнопок).
  async function doLogin(em: string, pw: string) {
    setError(null);
    setFields({});
    try {
      const s = unwrap(await api.POST("/auth/login", { body: { email: em.trim(), password: pw } }));
      await applySession(s);
      navigate(safeNext(params.get("next")) ?? homeFor(s.user.role), { replace: true });
    } catch (err) {
      if (isApiError(err) && err.is("email_not_verified")) {
        navigate(`/verify?email=${encodeURIComponent(em.trim())}`);
        return;
      }
      if (isApiError(err)) setFields(err.fields);
      setError(errorMessage(err));
    }
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    await doLogin(email, password);
    setBusy(false);
  }

  async function quickLogin(em: string) {
    setEmail(em);
    setPassword(DEMO_PASSWORD);
    setDemoBusy(em);
    await doLogin(em, DEMO_PASSWORD);
    setDemoBusy(null);
  }

  async function viaFsp() {
    setFspBusy(true);
    setError(null);
    try {
      const r = unwrap(await api.POST("/auth/fsp/start", { body: { mode: "login", consent_processing: fspConsent, return_to: "/auth/fsp/done" } }));
      window.location.assign(r.authorization_url);
    } catch (err) {
      setError(isApiError(err) && err.is("fsp_id_disabled") ? "Вход через ФСП ID пока не настроен на этом стенде" : errorMessage(err));
      setFspBusy(false);
    }
  }

  return (
    <AuthLayout
      title="Вход"
      subtitle="По e-mail и паролю или через ФСП ID"
      footer={
        <>
          Нет аккаунта? <Link to="/register">Зарегистрироваться</Link>
        </>
      }
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="E-mail" error={fields.email}>
          {(id, by) => <Input id={id} aria-describedby={by} type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} invalid={!!fields.email} required />}
        </Field>
        <Field label="Пароль" error={fields.password}>
          {(id, by) => <Input id={id} aria-describedby={by} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} invalid={!!fields.password} required />}
        </Field>
        {error && (
          <p role="alert" className="rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">
            {error}
          </p>
        )}
        <Button type="submit" loading={busy} className="w-full">
          Войти
        </Button>
        <div className="text-center text-[13px]">
          <Link to="/forgot">Забыли пароль?</Link>
        </div>
      </form>

      <section aria-label="Демо-доступы" className="mt-6 rounded-md border border-line bg-paper/60 p-4">
        <p className="font-mono text-[11px] uppercase tracking-[0.08em] text-muted">Демо-доступы стенда</p>
        <p className="mt-1 text-[12px] leading-relaxed text-muted">
          Войдите в один клик под любой ролью, чтобы проверить проект. Пароль у всех — <span className="font-mono text-ink">{DEMO_PASSWORD}</span>.
        </p>
        <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {DEMO_ACCOUNTS.map((a) => (
            <button
              key={a.email}
              type="button"
              onClick={() => quickLogin(a.email)}
              disabled={busy || demoBusy !== null}
              aria-busy={demoBusy === a.email}
              className="flex flex-col items-start rounded-sm border border-line bg-surface px-3 py-2 text-left transition hover:border-brand-blue hover:bg-brand-blue-soft/40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-blue disabled:cursor-not-allowed disabled:opacity-60"
            >
              <span className="font-mono text-[13px] font-semibold text-ink">
                {a.role}
                {demoBusy === a.email ? " — вход…" : ""}
              </span>
              <span className="mt-0.5 text-[11px] text-muted">{a.hint}</span>
              <span className="mt-1 font-mono text-[11px] text-brand-blue">{a.email}</span>
            </button>
          ))}
        </div>
      </section>

      <div className="my-5 flex items-center gap-3 text-[12px] text-muted">
        <span className="h-px flex-1 bg-line-soft" />
        или
        <span className="h-px flex-1 bg-line-soft" />
      </div>
      <div className="flex flex-col gap-3">
        <Button type="button" variant="dark" loading={fspBusy} onClick={viaFsp} className="w-full">
          <img src="/brand/fsp-zvezda-dark.svg" alt="" className="size-4" aria-hidden /> Войти через ФСП ID
        </Button>
        <Checkbox checked={fspConsent} onChange={(e) => setFspConsent(e.target.checked)} label="Согласен на обработку персональных данных при создании аккаунта" />
      </div>
    </AuthLayout>
  );
}
