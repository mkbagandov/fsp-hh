import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { AuthLayout } from "./AuthLayout";
import { Button, Field, Input } from "@/components/ui";
import { api, unwrap, isApiError, errorMessage } from "@/api/client";
import { useSession, homeFor } from "@/api/session";

export function Verify() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const { applySession } = useSession();
  const [email, setEmail] = useState(params.get("email") ?? "");
  const [code, setCode] = useState(params.get("code") ?? "");
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const s = unwrap(await api.POST("/auth/verify", { body: { email: email.trim(), code: code.trim() } }));
      await applySession(s);
      navigate(homeFor(s.user.role), { replace: true });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function resend() {
    setError(null);
    setInfo(null);
    try {
      unwrap(await api.POST("/auth/verify/resend", { body: { email: email.trim() } }));
      setInfo("Новый код отправлен");
    } catch (err) {
      setError(isApiError(err) && err.status === 429 ? "Слишком часто. Подождите минуту" : errorMessage(err));
    }
  }

  return (
    <AuthLayout
      title="Подтверждение e-mail"
      subtitle="Введите шестизначный код из письма"
      footer={
        <>
          На стенде письма видны на странице <Link to="/dev/outbox">/dev/outbox</Link>
        </>
      }
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="E-mail">{(id) => <Input id={id} type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />}</Field>
        <Field label="Код">
          {(id) => (
            <Input id={id} mono inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))} placeholder="000000" className="text-center !text-[22px] tracking-[0.3em]" required />
          )}
        </Field>
        {error && (
          <p role="alert" className="rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">
            {error}
          </p>
        )}
        {info && <p className="rounded-sm bg-ok-soft px-3 py-2 text-[13px] text-ok">{info}</p>}
        <Button type="submit" loading={busy} className="w-full" disabled={code.length !== 6}>
          Подтвердить
        </Button>
        <Button type="button" variant="ghost" onClick={resend} className="w-full">
          Отправить код ещё раз
        </Button>
      </form>
    </AuthLayout>
  );
}
