import { Link, NavLink, Outlet } from "react-router";
import { Logo } from "@/components/brand/Logo";
import { LinkButton } from "@/components/ui";
import { useSession, homeFor } from "@/api/session";

export function PublicLayout() {
  const { user, ready } = useSession();
  return (
    <div className="flex min-h-dvh flex-col">
      <header className="sticky top-0 z-40 border-b border-line-soft bg-paper/85 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-[1200px] items-center gap-6 px-4 sm:px-6">
          <Link to="/" className="flex items-center gap-3 no-underline">
            <Logo height={30} />
            <span className="hidden text-[12px] leading-tight text-muted sm:block">
              платформа
              <br />
              найма
            </span>
          </Link>
          <nav className="ml-auto flex items-center gap-1 text-[14px]" aria-label="Основная навигация">
            <NavLink to="/vacancies" className="nav-link">
              Вакансии
            </NavLink>
            <a href="/#how" className="nav-link hidden sm:flex">
              Как это работает
            </a>
            {ready && user ? (
              <LinkButton to={homeFor(user.role)} size="sm" variant="dark" className="ml-2">
                В кабинет
              </LinkButton>
            ) : (
              <>
                <NavLink to="/login" className="nav-link">
                  Войти
                </NavLink>
                <LinkButton to="/register" size="sm" className="ml-1">
                  Регистрация
                </LinkButton>
              </>
            )}
          </nav>
        </div>
      </header>
      <main className="flex-1">
        <Outlet />
      </main>
      <footer className="border-t border-line-soft bg-ink text-paper">
        <div className="hr-strip opacity-90" />
        <div className="mx-auto flex max-w-[1200px] flex-col gap-6 px-4 py-10 sm:flex-row sm:items-start sm:justify-between sm:px-6">
          <div>
            <Logo tone="dark" height={28} />
            <p className="mt-3 max-w-sm text-[13px] text-paper/70">
              Платформа найма ИТ-специалистов с обратной механикой. Сделано для Федерации спортивного программирования в рамках «Лидеров цифровой трансформации — 2026».
            </p>
          </div>
          <div className="mono text-[12px] text-paper/60">
            <div>api → /api/v1/docs</div>
            <div>status → /healthz</div>
          </div>
        </div>
      </footer>
    </div>
  );
}
