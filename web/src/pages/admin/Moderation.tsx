import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, errorMessage } from "@/api/client";
import { ak, useAdminVacancies, useEmployers, useFlaggedSessions } from "@/api/admin";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, DataTable, Dialog, EmptyState, Field, Skeleton, StatusBadge, SalaryRange, Textarea, useToast } from "@/components/ui";
import { GRADE_STATUS, VACANCY_STATUS, category, dateTime, grade, percent, relative } from "@/lib/format";

const REP: Record<string, { label: string; tone: "neutral" | "ok" | "warn" | "red" }> = {
  new: { label: "новая", tone: "neutral" },
  good: { label: "хорошая", tone: "ok" },
  normal: { label: "средняя", tone: "warn" },
  low: { label: "низкая", tone: "red" },
};

export function AdminEmployers() {
  const q = useEmployers();
  return (
    <>
      <PageHeader eyebrow="администрирование" title="Работодатели" description="Репутация — доля принятых приглашений среди ответивших. Компании с низкой долей стоит проверить на вилки и описания." />
      {q.isLoading && <Skeleton className="h-48" />}
      {q.data && (
        <DataTable
          rows={q.data}
          rowKey={(e) => e.id}
          empty={<EmptyState title="Компаний пока нет" pattern={false} />}
          columns={[
            {
              key: "name",
              header: "Компания",
              render: (e) => (
                <div>
                  <div className="font-semibold">{e.name}</div>
                  <div className="text-[12px] text-muted">{[e.industry, e.city].filter(Boolean).join(" · ") || "—"}</div>
                </div>
              ),
            },
            { key: "users", header: "Пользователей", render: (e) => <span className="mono">{e.users}</span> },
            { key: "needs", header: "Потребностей", render: (e) => <span className="mono">{e.needs}</span> },
            { key: "vac", header: "Вакансий", render: (e) => <span className="mono">{e.vacancies}</span> },
            { key: "inv", header: "Приглашений", render: (e) => <span className="mono">{e.invitations} <span className="text-muted">(+{e.accepted} / −{e.declined} / ↩{e.withdrawn})</span></span> },
            { key: "rep", header: "Репутация", render: (e) => <Badge tone={REP[e.reputation]?.tone ?? "neutral"}>{REP[e.reputation]?.label ?? e.reputation}{e.invitations ? ` · ${percent(e.acceptance_rate)}` : ""}</Badge> },
            { key: "created", header: "Создана", render: (e) => <span className="text-[12px] text-muted">{relative(e.created_at)}</span> },
          ]}
        />
      )}
    </>
  );
}

export function AdminVacancies() {
  const q = useAdminVacancies();
  const qc = useQueryClient();
  const toast = useToast();
  const [hide, setHide] = useState<{ id: string; title: string } | null>(null);
  const [reason, setReason] = useState("");
  const doHide = useMutation({
    mutationFn: async () => unwrap(await api.POST("/admin/vacancies/{id}/hide", { params: { path: { id: hide!.id } }, body: { reason } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ak.vacancies });
      toast.ok("Вакансия снята с публикации");
      setHide(null);
      setReason("");
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  return (
    <>
      <PageHeader eyebrow="администрирование" title="Модерация вакансий" description="Снятие с публикации переводит вакансию в «закрыта» и пишет причину в аудит; работодатель может опубликовать её снова после правок." />
      {q.isLoading && <Skeleton className="h-48" />}
      {q.data && (
        <DataTable
          rows={q.data}
          rowKey={(v) => v.id}
          empty={<EmptyState title="Вакансий нет" pattern={false} />}
          columns={[
            {
              key: "title",
              header: "Вакансия",
              render: (v) => (
                <div>
                  {v.status === "published" ? (
                    <Link to={`/vacancies/${v.id}`} className="font-semibold text-ink no-underline hover:text-brand-blue">
                      {v.title}
                    </Link>
                  ) : (
                    <span className="font-semibold">{v.title}</span>
                  )}
                  <div className="text-[12px] text-muted">{v.company_name}</div>
                </div>
              ),
            },
            { key: "cat", header: "Категория", render: (v) => <span className="mono text-[12.5px]">{category(v.specialization, v.grade_min)}{v.grade_min !== v.grade_max ? ` – ${grade(v.grade_max)}` : ""}</span> },
            { key: "salary", header: "Вилка", render: (v) => <SalaryRange from={v.salary_from} to={v.salary_to} className="text-[12.5px]" /> },
            { key: "apps", header: "Откликов", render: (v) => <span className="mono">{v.applications}</span> },
            { key: "status", header: "Статус", render: (v) => <StatusBadge status={v.status} label={VACANCY_STATUS[v.status] ?? v.status} /> },
            { key: "date", header: "Опубликована", render: (v) => <span className="text-[12px] text-muted">{v.published_at ? dateTime(v.published_at) : "—"}</span> },
            { key: "act", header: "", render: (v) => v.status === "published" ? <Button size="sm" variant="ghost" onClick={() => setHide({ id: v.id, title: v.title })}>Снять</Button> : null },
          ]}
        />
      )}
      <Dialog
        open={!!hide}
        onClose={() => setHide(null)}
        title={`Снять с публикации: ${hide?.title ?? ""}`}
        footer={
          <>
            <Button variant="ghost" onClick={() => setHide(null)}>
              Отмена
            </Button>
            <Button variant="danger" onClick={() => doHide.mutate()} loading={doHide.isPending} disabled={!reason.trim()}>
              Снять
            </Button>
          </>
        }
      >
        <Field label="Причина" required hint="Попадёт в аудит">
          {(id) => <Textarea id={id} rows={3} maxLength={1000} value={reason} onChange={(e) => setReason(e.target.value)} />}
        </Field>
      </Dialog>
    </>
  );
}

export function AdminSessions() {
  const q = useFlaggedSessions();
  return (
    <>
      <PageHeader eyebrow="администрирование" title="Сессии с флагами" description="Телеметрия теста: потери фокуса, скрытие вкладки, вставка. Подозрительные — много событий при высокой доле верных. Санкций автоматически нет — это повод посмотреть внимательнее." />
      {q.isLoading && <Skeleton className="h-48" />}
      {q.data && q.data.length === 0 && <EmptyState title="Флагов нет" description="Ни одна сессия не зафиксировала событий телеметрии." pattern={false} />}
      {q.data && q.data.length > 0 && (
        <div className="flex flex-col gap-2">
          {q.data.map((s) => (
            <Card key={s.id} className={`flex flex-wrap items-center gap-3 px-4 py-3 ${s.suspicious ? "border-l-4 border-l-danger" : ""}`}>
              <span className="mono text-[13px]">{s.email}</span>
              <span className="mono text-[12.5px] text-muted">{category(s.specialization, s.target_grade)}</span>
              <Badge>{s.status}</Badge>
              {s.decision && <StatusBadge status={s.decision} label={GRADE_STATUS[s.decision] ?? s.decision} />}
              <span className="mono text-[12.5px]">
                верных {s.correct_count}/{s.items_count}
              </span>
              <span className="flex flex-wrap gap-1">
                {Object.entries(s.flags).map(([k, v]) => (
                  <Badge key={k} tone={k === "paste" || k === "devtools" ? "red" : "warn"}>
                    {k} × {v}
                  </Badge>
                ))}
              </span>
              {s.suspicious && <Badge tone="brand">подозрительно</Badge>}
              <span className="ml-auto text-[12px] text-muted">{s.started_at ? relative(s.started_at) : ""}</span>
            </Card>
          ))}
        </div>
      )}
    </>
  );
}
