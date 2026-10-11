import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { AuthLayout } from "./AuthLayout";
import { Spinner } from "@/components/ui";
import { getSession, refreshSession } from "@/api/client";
import { useSession, homeFor } from "@/api/session";

const errors: Record<string, string> = {
  state_mismatch: "Сессия входа устарела. Попробуйте ещё раз.",
  consent_required: "Для создания аккаунта нужно согласие на обработку данных.",
  fsp_id_disabled: "Вход через ФСП ID не настроен.",
  access_denied: "Вы отменили вход в ФСП ID.",
};

/** Сюда ФСП ID возвращает браузер после callback: refresh-cookie уже стоит, остаётся получить access-токен. */
export function FspDone() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { reload } = useSession();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const code = params.get("fsp_error");
    if (code) {
      setError(errors[code] ?? "Не удалось войти через ФСП ID. Попробуйте ещё раз.");
      return;
    }
    (async () => {
      const ok = await refreshSession();
      if (!ok) {
        setError("Не удалось получить сессию после входа через ФСП ID");
        return;
      }
      await reload();
      if (params.get("fsp_linked")) {
        navigate("/candidate/profile?fsp=linked", { replace: true });
        return;
      }
      const res = await fetch("/api/v1/auth/me", { credentials: "include", headers: { Authorization: `Bearer ${getSession()?.accessToken ?? ""}` } });
      const me = res.ok ? ((await res.json()) as { user: { role: "candidate" | "employer" | "admin" } }) : null;
      navigate(homeFor(me?.user.role), { replace: true });
    })();
  }, [params, navigate, reload]);

  return (
    <AuthLayout title="ФСП ID" subtitle="Завершаем вход">
      {error ? (
        <div className="flex flex-col gap-4">
          <p role="alert" className="rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">
            {error}
          </p>
          <Link to="/login" className="btn btn-secondary no-underline">
            Вернуться ко входу
          </Link>
        </div>
      ) : (
        <Spinner label="Получаем данные профиля…" />
      )}
    </AuthLayout>
  );
}
