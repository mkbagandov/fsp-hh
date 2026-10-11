import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { AuthLayout } from "./AuthLayout";
import { Button, Field, Input } from "@/components/ui";
import { api, unwrap, errorMessage } from "@/api/client";

export function Forgot() {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      unwrap(await api.POST("/auth/password/forgot", { body: { email: email.trim() } }));
      navigate(`/reset?email=${encodeURIComponent(email.trim())}`);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout
      title="Восстановление пароля"
      subtitle="Пришлём код на e-mail, если такой аккаунт есть"
      footer={
        <>
          Вспомнили? <Link to="/login">Войти</Link>
        </>
      }
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="E-mail">{(id) => <Input id={id} type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />}</Field>
        {error && (
          <p role="alert" className="rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">
            {error}
          </p>
        )}
        <Button type="submit" loading={busy} className="w-full">
          Отправить код
        </Button>
      </form>
    </AuthLayout>
  );
}

export function Reset() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [email, setEmail] = useState(params.get("email") ?? "");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      unwrap(await api.POST("/auth/password/reset", { body: { email: email.trim(), code: code.trim(), new_password: password } }));
      navigate("/login?reset=1", { replace: true });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout title="Новый пароль" subtitle="Код из письма и новый пароль">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="E-mail">{(id) => <Input id={id} type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />}</Field>
        <Field label="Код">
          {(id) => <Input id={id} mono inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))} placeholder="000000" className="text-center !text-[22px] tracking-[0.3em]" required />}
        </Field>
        <Field label="Новый пароль" hint="Не короче 8 символов">
          {(id) => <Input id={id} type="password" autoComplete="new-password" minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} required />}
        </Field>
        {error && (
          <p role="alert" className="rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">
            {error}
          </p>
        )}
        <Button type="submit" loading={busy} className="w-full" disabled={code.length !== 6 || password.length < 8}>
          Сменить пароль
        </Button>
      </form>
    </AuthLayout>
  );
}
