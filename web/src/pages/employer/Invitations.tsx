import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, errorMessage } from "@/api/client";
import { useEmployerInvitations, useNeeds } from "@/api/employer";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, EmptyState, Skeleton, StatusBadge, SalaryRange, Tabs, useToast } from "@/components/ui";
import { INVITATION_STATUS, dateShort, relative } from "@/lib/format";

const GROUPS: { key: string; label: string; statuses: string[] }[] = [
  { key: "active", label: "Ждут ответа", statuses: ["sent", "viewed"] },
  { key: "accepted", label: "Приняты", statuses: ["accepted"] },
  { key: "declined", label: "Отклонены", statuses: ["declined"] },
  { key: "closed", label: "Отозваны и истекшие", statuses: ["withdrawn", "expired"] },
];

/** Трекер приглашений: группировка по статусам, вилка, потребность, отзыв до ответа. */
export function EmployerInvitations() {
  const inv = useEmployerInvitations();
  const needs = useNeeds();
  const qc = useQueryClient();
  const toast = useToast();
  const [group, setGroup] = useState("active");
  const withdraw = useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/employer/invitations/{id}/withdraw", { params: { path: { id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["employer", "invitations"] });
      toast.ok("Приглашение отозвано");
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  const all = inv.data ?? [];
  const needTitle = new Map((needs.data ?? []).map((n) => [n.id, n.title]));
  const g = GROUPS.find((x) => x.key === group)!;
  const items = all.filter((i) => g.statuses.includes(i.status));
  return (
    <>
      <PageHeader eyebrow="работодатель" title="Приглашения" description="Квоты: 20 активных на потребность, 50 отправок в сутки. Отозвать можно до ответа кандидата." />
      <Tabs value={group} onChange={setGroup} ariaLabel="Группы приглашений" items={GROUPS.map((x) => ({ value: x.key, label: x.label, count: all.filter((i) => x.statuses.includes(i.status)).length }))} />
      <div className="mt-4 flex flex-col gap-2">
        {inv.isLoading && <Skeleton className="h-32" />}
        {inv.data && items.length === 0 && <EmptyState title="Пусто" description="В этой группе приглашений нет." pattern={false} />}
        {items.map((i) => (
          <Card key={i.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
            <Link to={`/employer/candidates/${i.candidate_id}`} className="font-medium">
              Кандидат
            </Link>
            {i.need_id && <Badge>{needTitle.get(i.need_id) ?? "потребность"}</Badge>}
            {i.vacancy_id && <Badge>из отклика</Badge>}
            <SalaryRange from={i.salary_from} to={i.salary_to} className="text-[13px]" />
            <span className="text-[12px] text-muted" title={dateShort(i.sent_at)}>
              {relative(i.sent_at)}
            </span>
            {i.below_candidate_min && <Badge tone="warn">ниже минимума кандидата</Badge>}
            {i.decline_reason && <Badge tone="red">{i.decline_reason}</Badge>}
            <span className="ml-auto flex items-center gap-2">
              <StatusBadge status={i.status} label={INVITATION_STATUS[i.status] ?? i.status} />
              {(i.status === "sent" || i.status === "viewed") && (
                <Button size="sm" variant="ghost" onClick={() => withdraw.mutate(i.id)} loading={withdraw.isPending}>
                  Отозвать
                </Button>
              )}
            </span>
          </Card>
        ))}
      </div>
    </>
  );
}
