import type { ReactNode } from "react";
import { createBrowserRouter, Navigate, Outlet, RouterProvider, useLocation } from "react-router";
import { Activity, Bell, Briefcase, Building2, ClipboardList, Database, LayoutDashboard, Mail, Send, ShieldAlert, Target, UserRound, Users } from "lucide-react";
import { useSession, homeFor, type Role } from "@/api/session";
import { PublicLayout } from "@/components/layout/PublicLayout";
import { AppShell, roleLabel } from "@/components/layout/AppShell";
import { Spinner } from "@/components/ui";
import { Landing } from "@/pages/Landing";
import { PublicVacancies } from "@/pages/vacancies/PublicList";
import { VacancyDetail } from "@/pages/vacancies/Detail";
import { Login } from "@/pages/auth/Login";
import { Register } from "@/pages/auth/Register";
import { Verify } from "@/pages/auth/Verify";
import { Forgot, Reset } from "@/pages/auth/Password";
import { FspDone } from "@/pages/auth/FspDone";
import { UiKit } from "@/pages/dev/UiKit";
import { Outbox } from "@/pages/dev/Outbox";
import { Notifications } from "@/pages/Notifications";
import { VerifyCategory } from "@/pages/VerifyCategory";
import { CandidateOverview } from "@/pages/candidate/Overview";
import { Onboarding } from "@/pages/candidate/Onboarding";
import { TestPage } from "@/pages/candidate/Test";
import { CandidateProfile } from "@/pages/candidate/Profile";
import { CandidateInvitations } from "@/pages/candidate/Invitations";
import { CandidateApplications } from "@/pages/candidate/Applications";
import { EmployerOverview } from "@/pages/employer/Overview";
import { CompanyPage } from "@/pages/employer/Company";
import { NeedsList } from "@/pages/employer/Needs";
import { NeedForm } from "@/pages/employer/NeedForm";
import { NeedDetail } from "@/pages/employer/NeedDetail";
import { CandidateSearch } from "@/pages/employer/Candidates";
import { EmployerCandidateCard } from "@/pages/employer/CandidateCard";
import { VacanciesList, VacancyForm, VacancyDetailEmployer } from "@/pages/employer/Vacancies";
import { EmployerInvitations } from "@/pages/employer/Invitations";
import { useCompany } from "@/api/employer";
import { AdminOverview } from "@/pages/admin/Overview";
import { AdminUsers } from "@/pages/admin/Users";
import { AdminItems } from "@/pages/admin/Items";
import { AdminEmployers, AdminVacancies, AdminSessions } from "@/pages/admin/Moderation";
import { AdminAudit } from "@/pages/admin/Audit";
import { NotFound } from "@/pages/NotFound";

/** Пускает только пользователей с нужной ролью; анонимов — на вход с возвратом. */
function RequireRole({ roles, children }: { roles: Role[]; children: ReactNode }) {
  const { ready, user } = useSession();
  const loc = useLocation();
  if (!ready) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <Spinner label="Проверяем сессию…" />
      </div>
    );
  }
  if (!user) return <Navigate to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />;
  if (!roles.includes(user.role)) return <Navigate to={homeFor(user.role)} replace />;
  return <>{children}</>;
}

/** Гостевые страницы: вошедшего отправляем в кабинет. */
function GuestOnly() {
  const { ready, user } = useSession();
  if (ready && user) return <Navigate to={homeFor(user.role)} replace />;
  return <Outlet />;
}

function CandidateShell() {
  return (
    <AppShell
      roleLabel={roleLabel("candidate")}
      nav={[
        { to: "/candidate", label: "Обзор", icon: <LayoutDashboard className="size-4" />, end: true },
        { to: "/candidate/profile", label: "Профиль", icon: <UserRound className="size-4" /> },
        { to: "/candidate/invitations", label: "Приглашения", icon: <Mail className="size-4" /> },
        { to: "/candidate/applications", label: "Отклики", icon: <Send className="size-4" /> },
        { to: "/vacancies", label: "Вакансии", icon: <Briefcase className="size-4" /> },
        { to: "/notifications", label: "Уведомления", icon: <Bell className="size-4" /> },
      ]}
    />
  );
}

function EmployerShell() {
  return (
    <AppShell
      roleLabel={roleLabel("employer")}
      nav={[
        { to: "/employer", label: "Обзор", icon: <LayoutDashboard className="size-4" />, end: true },
        { to: "/employer/needs", label: "Потребности", icon: <Target className="size-4" /> },
        { to: "/employer/candidates", label: "Банк кандидатов", icon: <Users className="size-4" />, end: true },
        { to: "/employer/invitations", label: "Приглашения", icon: <Mail className="size-4" /> },
        { to: "/employer/vacancies", label: "Вакансии", icon: <Briefcase className="size-4" /> },
        { to: "/employer/company", label: "Компания", icon: <Building2 className="size-4" /> },
        { to: "/notifications", label: "Уведомления", icon: <Bell className="size-4" /> },
      ]}
    />
  );
}

/** Без компании работодателю нечего делать в кабинете: ведём на форму компании. */
function RequireCompany() {
  const company = useCompany();
  const loc = useLocation();
  if (company.isLoading) {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <Spinner label="Загружаем компанию…" />
      </div>
    );
  }
  if (company.data === null && !company.isFetching && loc.pathname !== "/employer/company") return <Navigate to="/employer/company?setup=1" replace />;
  return <Outlet />;
}

function AdminShell() {
  return (
    <AppShell
      roleLabel={roleLabel("admin")}
      nav={[
        { to: "/admin", label: "Сводка", icon: <Activity className="size-4" />, end: true },
        { to: "/admin/users", label: "Пользователи", icon: <Users className="size-4" /> },
        { to: "/admin/items", label: "Банк заданий", icon: <Database className="size-4" /> },
        { to: "/admin/sessions", label: "Сессии с флагами", icon: <ShieldAlert className="size-4" /> },
        { to: "/admin/employers", label: "Работодатели", icon: <Building2 className="size-4" /> },
        { to: "/admin/vacancies", label: "Модерация вакансий", icon: <Briefcase className="size-4" /> },
        { to: "/admin/audit", label: "Аудит", icon: <ClipboardList className="size-4" /> },
        { to: "/notifications", label: "Уведомления", icon: <Bell className="size-4" /> },
      ]}
    />
  );
}

function NotificationsShell() {
  const { user } = useSession();
  if (user?.role === "admin") return <AdminShell />;
  return user?.role === "employer" ? <EmployerShell /> : <CandidateShell />;
}

const router = createBrowserRouter([
  {
    element: <PublicLayout />,
    children: [
      { path: "/", element: <Landing /> },
      { path: "/vacancies", element: <PublicVacancies /> },
      { path: "/vacancies/:id", element: <VacancyDetail /> },
      { path: "/verify/:id", element: <VerifyCategory /> },
      { path: "/dev/ui", element: <UiKit /> },
      { path: "/dev/outbox", element: <Outbox /> },
    ],
  },
  {
    element: <GuestOnly />,
    children: [
      { path: "/login", element: <Login /> },
      { path: "/register", element: <Register /> },
      { path: "/verify", element: <Verify /> },
      { path: "/forgot", element: <Forgot /> },
      { path: "/reset", element: <Reset /> },
    ],
  },
  { path: "/auth/fsp/done", element: <FspDone /> },
  {
    path: "/candidate",
    element: (
      <RequireRole roles={["candidate"]}>
        <CandidateShell />
      </RequireRole>
    ),
    children: [
      { index: true, element: <CandidateOverview /> },
      { path: "onboarding", element: <Onboarding /> },
      { path: "tests/:id", element: <TestPage /> },
      { path: "profile", element: <CandidateProfile /> },
      { path: "invitations", element: <CandidateInvitations /> },
      { path: "applications", element: <CandidateApplications /> },
    ],
  },
  {
    path: "/employer",
    element: (
      <RequireRole roles={["employer"]}>
        <EmployerShell />
      </RequireRole>
    ),
    children: [
      {
        element: <RequireCompany />,
        children: [
          { index: true, element: <EmployerOverview /> },
          { path: "company", element: <CompanyPage /> },
          { path: "needs", element: <NeedsList /> },
          { path: "needs/new", element: <NeedForm /> },
          { path: "needs/:id", element: <NeedDetail /> },
          { path: "needs/:id/edit", element: <NeedForm /> },
          { path: "candidates", element: <CandidateSearch /> },
          { path: "candidates/:id", element: <EmployerCandidateCard /> },
          { path: "invitations", element: <EmployerInvitations /> },
          { path: "vacancies", element: <VacanciesList /> },
          { path: "vacancies/new", element: <VacancyForm /> },
          { path: "vacancies/:id", element: <VacancyDetailEmployer /> },
          { path: "vacancies/:id/edit", element: <VacancyForm /> },
        ],
      },
    ],
  },
  {
    path: "/admin",
    element: (
      <RequireRole roles={["admin"]}>
        <AdminShell />
      </RequireRole>
    ),
    children: [
      { index: true, element: <AdminOverview /> },
      { path: "users", element: <AdminUsers /> },
      { path: "items", element: <AdminItems /> },
      { path: "sessions", element: <AdminSessions /> },
      { path: "employers", element: <AdminEmployers /> },
      { path: "vacancies", element: <AdminVacancies /> },
      { path: "audit", element: <AdminAudit /> },
    ],
  },
  {
    path: "/notifications",
    element: (
      <RequireRole roles={["candidate", "employer", "admin"]}>
        <NotificationsShell />
      </RequireRole>
    ),
    children: [{ index: true, element: <Notifications /> }],
  },
  { path: "*", element: <PublicLayout />, children: [{ path: "*", element: <NotFound /> }] },
]);

export function App() {
  return <RouterProvider router={router} />;
}
