import { useState, type ReactNode } from "react";
import { Link, NavLink, Outlet, useNavigate } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Bell, LogOut, Menu, X } from "lucide-react";
import { Logo } from "@/components/brand/Logo";
import { useSession, homeFor, type Role } from "@/api/session";
import { api, unwrap } from "@/api/client";

export type NavItem = { to: string; label: string; icon?: ReactNode; end?: boolean };

/** Каркас кабинета: тёмная шапка, боковая навигация (на мобильном — выезжающая), колокольчик уведомлений. */
export function AppShell({ nav, roleLabel }: { nav: NavItem[]; roleLabel: string }) {
  const { user, logout } = useSession();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const unread = useQuery({
    queryKey: ["notifications", "unread"],
    queryFn: async () => unwrap(await api.GET("/notifications", { params: { query: { unread: true, limit: 1 } } })).unread_count,
    refetchInterval: 30_000,
    enabled: !!user,
  });

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="sticky top-0 z-40 bg-ink text-paper">
        <div className="flex h-14 items-center gap-3 px-4 sm:px-6">
          <button type="button" className="rounded-sm p-1.5 hover:bg-white/10 md:hidden" onClick={() => setOpen((v) => !v)} aria-label="Меню" aria-expanded={open}>
            {open ? <X className="size-5" /> : <Menu className="size-5" />}
          </button>
          <Link to={homeFor(user?.role)} className="flex items-center gap-3 no-underline">
            <Logo tone="dark" height={26} />
            <span className="label-mono !text-paper/60">{roleLabel}</span>
          </Link>
          <div className="ml-auto flex items-center gap-1">
            <NavLink to="/notifications" className="relative rounded-sm p-2 hover:bg-white/10" aria-label="Уведомления">
              <Bell className="size-5" />
              {!!unread.data && (
                <span className="mono absolute -right-0.5 -top-0.5 min-w-[18px] rounded-full bg-brand-red px-1 text-center text-[10px] font-bold leading-[18px] text-white">{unread.data > 99 ? "99+" : unread.data}</span>
              )}
            </NavLink>
            <span className="mono hidden max-w-[220px] truncate px-2 text-[12px] text-paper/70 sm:block" title={user?.email}>
              {user?.email}
            </span>
            <button
              type="button"
              className="rounded-sm p-2 hover:bg-white/10"
              aria-label="Выйти"
              onClick={async () => {
                await logout();
                navigate("/");
              }}
            >
              <LogOut className="size-5" />
            </button>
          </div>
        </div>
        <div className="hr-strip opacity-80" />
      </header>
      <div className="mx-auto flex w-full max-w-[1360px] flex-1">
        <aside className={`${open ? "block" : "hidden"} fixed inset-x-0 top-[62px] z-30 border-b border-line bg-paper p-3 md:static md:block md:w-60 md:shrink-0 md:border-0 md:bg-transparent md:p-4`}>
          <nav className="flex flex-col gap-0.5" aria-label="Разделы кабинета">
            {nav.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className="nav-link" onClick={() => setOpen(false)}>
                {n.icon}
                {n.label}
              </NavLink>
            ))}
          </nav>
        </aside>
        <main className="min-w-0 flex-1 px-4 py-6 sm:px-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

export function PageHeader({ title, description, actions, eyebrow }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; eyebrow?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        {eyebrow && <div className="label-mono mb-1">{eyebrow}</div>}
        <h1>{title}</h1>
        {description && <p className="mt-1 max-w-2xl text-[14px] text-muted">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 gap-2">{actions}</div>}
    </div>
  );
}

export function roleLabel(role: Role | undefined): string {
  return role === "employer" ? "кабинет работодателя" : role === "admin" ? "администрирование" : "кабинет кандидата";
}
