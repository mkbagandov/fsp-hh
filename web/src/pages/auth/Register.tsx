import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { AuthLayout } from "./AuthLayout";
import { Button, Field, Input, Checkbox, SegmentedControl } from "@/components/ui";
import { api, unwrap, isApiError, errorMessage } from "@/api/client";

type Role = "candidate" | "employer";

export function Register() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [role, setRole] = useState<Role>(params.get("role") === "employer" ? "employer" : "candidate");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [consent, setConsent] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!consent) {
      setError("Без согласия на обработку персональных данных аккаунт создать нельзя (152-ФЗ)");
      return;
    }
    setBusy(true);
    setError(null);
    setFields({});
    try {
      const r = unwrap(await api.POST("/auth/register", { body: { email: email.trim(), password, role, consents: { processing: true } } }));
      navigate(r.verification_required ? `/verify?email=${encodeURIComponent(email.trim())}` : "/login", { replace: true });
    } catch (err) {
      if (isApiError(err)) setFields(err.fields);
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout
      title="Регистрация"
      subtitle="Код подтверждения придёт на e-mail (на стенде — в /dev/outbox)"
      footer={
        <>
          Уже есть аккаунт? <Link to="/login">Войти</Link>
        </>
      }
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Кто вы">
          {() => (
            <SegmentedControl<Role>
              value={role}
              onChange={setRole}
              ariaLabel="Роль"
              options={[
                { value: "candidate", label: "Кандидат" },
                { value: "employer", label: "Работодатель" },
              ]}
            />
          )}
        </Field>
        <Field label="E-mail" error={fields.email}>
          {(id, by) => <Input id={id} aria-describedby={by} type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} invalid={!!fields.email} required />}
        </Field>
        <Field label="Пароль" hint="Не короче 8 символов" error={fields.password}>
          {(id, by) => <Input id={id} aria-describedby={by} type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} invalid={!!fields.password} required minLength={8} />}
        </Field>
        <Checkbox
          checked={consent}
          onChange={(e) => setConsent(e.target.checked)}
          label="Согласен на обработку персональных данных"
          description="Нужно для создания аккаунта. Показ профиля работодателям и получение достижений ФСП — отдельные согласия, их вы дадите позже."
        />
        {error && (
          <p role="alert" className="rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">
            {error}
          </p>
        )}
        <Button type="submit" loading={busy} className="w-full">
          {role === "candidate" ? "Создать аккаунт кандидата" : "Создать аккаунт работодателя"}
        </Button>
      </form>
    </AuthLayout>
  );
}
