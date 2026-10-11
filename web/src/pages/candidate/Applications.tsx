import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, errorMessage } from "@/api/client";
import { useApplications } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Button, Card, EmptyState, LinkButton, Skeleton, StatusBadge, SalaryRange, useToast } from "@/components/ui";
import { APPLICATION_STATUS, relative } from "@/lib/format";

export function CandidateApplications() {
  const list = useApplications();
  const qc = useQueryClient();
  const toast = useToast();
  const withdraw = useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/candidate/applications/{id}/withdraw", { params: { path: { id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["candidate", "applications"] });
      toast.ok("Отклик отозван");
    },
    onError: (e) => toast.error("Не удалось отозвать", errorMessage(e)),
  });
  return (
    <>
      <PageHeader eyebrow="кандидат" title="Мои отклики" description="Отклик открывает компании ваши контакты. Статус меняется, когда работодатель просматривает, отказывает или приглашает." actions={<LinkButton to="/vacancies" variant="secondary" size="sm">Вакансии</LinkButton>} />
      <div className="flex flex-col gap-3">
        {list.isLoading && [1, 2].map((i) => <Skeleton key={i} className="h-24" />)}
        {list.data && list.data.length === 0 && <EmptyState title="Откликов пока нет" description="Классический путь тоже работает: выберите вакансию с подходящей вилкой и откликнитесь." action={<LinkButton to="/vacancies">Смотреть вакансии</LinkButton>} />}
        {(list.data ?? []).map((a) => (
          <Card key={a.id} className="p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <Link to={`/vacancies/${a.vacancy_id}`} className="text-[15px] font-semibold text-ink no-underline hover:text-brand-blue">
                  {a.vacancy?.title ?? "Вакансия"}
                </Link>
                <div className="mt-0.5 text-[13px] text-muted">
                  {a.vacancy?.company_name} · отправлен {relative(a.sent_at)}
                </div>
                {a.reject_reason && <p className="mt-2 rounded-sm bg-danger-soft px-3 py-1.5 text-[13px] text-danger">Причина отказа: {a.reject_reason}</p>}
                {a.status === "invited" && a.invitation_id && (
                  <Link to={`/candidate/invitations?open=${a.invitation_id}`} className="mt-2 inline-block text-[13px]">
                    Открыть приглашение →
                  </Link>
                )}
              </div>
              <div className="flex flex-col items-end gap-2">
                {a.vacancy && <SalaryRange from={a.vacancy.salary_from} to={a.vacancy.salary_to} className="text-[15px]" />}
                <StatusBadge status={a.status} label={APPLICATION_STATUS[a.status] ?? a.status} />
                {(a.status === "sent" || a.status === "viewed") && (
                  <Button size="sm" variant="ghost" onClick={() => withdraw.mutate(a.id)} loading={withdraw.isPending}>
                    Отозвать
                  </Button>
                )}
              </div>
            </div>
          </Card>
        ))}
      </div>
    </>
  );
}
