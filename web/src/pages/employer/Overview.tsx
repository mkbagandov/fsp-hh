import { Link } from "react-router";
import { ArrowRight, Briefcase, Mail, Target } from "lucide-react";
import { useCompany, useEmployerInvitations, useNeeds, useVacancies } from "@/api/employer";
import { PageHeader } from "@/components/layout/AppShell";
import { Card, CardBody, CardHeader, EmptyState, LinkButton, Skeleton, Stat, StatusBadge, SalaryRange } from "@/components/ui";
import { INVITATION_STATUS, NEED_STATUS, VACANCY_STATUS, category, grade, relative } from "@/lib/format";

export function EmployerOverview() {
  const company = useCompany();
  const needs = useNeeds();
  const vacancies = useVacancies();
  const invitations = useEmployerInvitations();
  const inv = invitations.data ?? [];
  const active = inv.filter((i) => i.status === "sent" || i.status === "viewed").length;
  const accepted = inv.filter((i) => i.status === "accepted").length;
  const answered = inv.filter((i) => i.status === "accepted" || i.status === "declined").length;
  const newApplications = (vacancies.data ?? []).reduce((n, v) => n + (v.applications?.new ?? 0), 0);

  return (
    <>
      <PageHeader
        eyebrow="кабинет работодателя"
        title={company.data?.name ?? "Обзор"}
        description="Опишите потребность — карта категорий покажет, сколько подтверждённых кандидатов есть и какая вилка их покрывает."
        actions={
          <>
            <LinkButton to="/employer/needs/new" size="sm">
              <Target className="size-4" /> Новая потребность
            </LinkButton>
            <LinkButton to="/employer/vacancies/new" size="sm" variant="secondary">
              <Briefcase className="size-4" /> Вакансия
            </LinkButton>
          </>
        }
      />
      <div className="grid gap-3 sm:grid-cols-4">
        <Stat label="активных приглашений" value={active} />
        <Stat label="принято" value={accepted} tone="blue" hint={answered ? `из ${answered} ответов` : "ответов пока нет"} />
        <Stat label="новых откликов" value={newApplications} tone={newApplications ? "red" : "ink"} />
        <Stat label="потребностей" value={(needs.data ?? []).filter((n) => n.status === "active").length} hint={`вакансий: ${(vacancies.data ?? []).filter((v) => v.status === "published").length}`} />
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader title="Потребности" description="Обратная механика: категория → подборка → приглашение" action={<Link to="/employer/needs" className="text-[13px]">все →</Link>} />
          <CardBody className="!p-0">
            {needs.isLoading && <Skeleton className="m-4 h-16" />}
            {needs.data && needs.data.length === 0 && (
              <div className="p-4">
                <EmptyState title="Потребностей пока нет" description="Опишите, кого ищете, обычным текстом — помощник разберёт это в структуру." action={<LinkButton to="/employer/needs/new">Описать потребность</LinkButton>} pattern={false} />
              </div>
            )}
            {(needs.data ?? []).slice(0, 5).map((n) => (
              <Link key={n.id} to={`/employer/needs/${n.id}`} className="flex items-center gap-3 border-b border-line-soft px-5 py-3 no-underline last:border-0 hover:bg-paper/60">
                <div className="min-w-0 flex-1">
                  <div className="truncate text-[14px] font-medium text-ink">{n.title}</div>
                  <div className="mono text-[12px] text-muted">
                    {category(n.specialization, n.grade_min)}
                    {n.grade_min !== n.grade_max ? ` – ${grade(n.grade_max)}` : ""}
                  </div>
                </div>
                <SalaryRange from={n.salary_from} to={n.salary_to} className="text-[13px] text-ink" />
                <StatusBadge status={n.status} label={NEED_STATUS[n.status] ?? n.status} />
                <ArrowRight className="size-4 text-muted" />
              </Link>
            ))}
          </CardBody>
        </Card>
        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader title="Последние приглашения" action={<Link to="/employer/invitations" className="text-[13px]">трекер →</Link>} />
            <CardBody className="!p-0">
              {invitations.isLoading && <Skeleton className="m-4 h-12" />}
              {invitations.data && inv.length === 0 && <p className="px-5 py-4 text-[14px] text-muted">Приглашений ещё не было.</p>}
              {inv.slice(0, 5).map((i) => (
                <Link key={i.id} to={`/employer/candidates/${i.candidate_id}`} className="flex items-center gap-3 border-b border-line-soft px-5 py-2.5 no-underline last:border-0 hover:bg-paper/60">
                  <Mail className="size-4 shrink-0 text-brand-blue" />
                  <div className="min-w-0 flex-1 text-[13px] text-ink">
                    <span className="text-muted">{relative(i.sent_at)}</span> · <SalaryRange from={i.salary_from} to={i.salary_to} className="text-[13px]" />
                  </div>
                  <StatusBadge status={i.status} label={INVITATION_STATUS[i.status] ?? i.status} />
                </Link>
              ))}
            </CardBody>
          </Card>
          <Card>
            <CardHeader title="Вакансии" action={<Link to="/employer/vacancies" className="text-[13px]">все →</Link>} />
            <CardBody className="!p-0">
              {vacancies.data && vacancies.data.length === 0 && <p className="px-5 py-4 text-[14px] text-muted">Классический путь: вакансия с вилкой и отклики.</p>}
              {(vacancies.data ?? []).slice(0, 4).map((v) => (
                <Link key={v.id} to={`/employer/vacancies/${v.id}`} className="flex items-center gap-3 border-b border-line-soft px-5 py-2.5 no-underline last:border-0 hover:bg-paper/60">
                  <div className="min-w-0 flex-1 truncate text-[13px] font-medium text-ink">{v.title}</div>
                  {v.applications && (
                    <span className="mono text-[12px] text-muted">
                      откликов {v.applications.total}
                      {v.applications.new ? <span className="text-brand-red"> · новых {v.applications.new}</span> : null}
                    </span>
                  )}
                  <StatusBadge status={v.status} label={VACANCY_STATUS[v.status] ?? v.status} />
                </Link>
              ))}
            </CardBody>
          </Card>
        </div>
      </div>
    </>
  );
}
