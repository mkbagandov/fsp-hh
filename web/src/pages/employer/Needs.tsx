import { Link } from "react-router";
import { Plus } from "lucide-react";
import { useNeeds } from "@/api/employer";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, EmptyState, LinkButton, Skeleton, StatusBadge, SalaryRange, DataTable } from "@/components/ui";
import { NEED_STATUS, WORK_FORMATS, category, grade, relative } from "@/lib/format";

export function NeedsList() {
  const needs = useNeeds();
  return (
    <>
      <PageHeader eyebrow="работодатель" title="Потребности" description="Каждая потребность — карта категорий, объяснимая подборка и приглашения с вилкой." actions={<LinkButton to="/employer/needs/new" size="sm"><Plus className="size-4" /> Новая</LinkButton>} />
      {needs.isLoading && <Skeleton className="h-40" />}
      {needs.data && (
        <DataTable
          rows={needs.data}
          rowKey={(n) => n.id}
          empty={<EmptyState title="Потребностей пока нет" description="Опишите, кого ищете — помощник разберёт текст в структуру." action={<LinkButton to="/employer/needs/new">Описать потребность</LinkButton>} />}
          columns={[
            {
              key: "title",
              header: "Потребность",
              render: (n) => (
                <div>
                  <Link to={`/employer/needs/${n.id}`} className="font-semibold text-ink no-underline hover:text-brand-blue">
                    {n.title}
                  </Link>
                  <div className="mt-0.5 flex flex-wrap gap-1">
                    {(n.must_skills ?? []).slice(0, 4).map((s) => (
                      <Badge key={s.code}>{s.name}</Badge>
                    ))}
                  </div>
                </div>
              ),
            },
            { key: "cat", header: "Категория", render: (n) => <span className="mono text-[13px]">{category(n.specialization, n.grade_min)}{n.grade_min !== n.grade_max ? ` – ${grade(n.grade_max)}` : ""}</span> },
            { key: "salary", header: "Вилка", render: (n) => <SalaryRange from={n.salary_from} to={n.salary_to} className="text-[13px]" /> },
            { key: "fmt", header: "Формат", render: (n) => <span className="text-[13px] text-muted">{(n.work_formats ?? []).map((f) => WORK_FORMATS[f] ?? f).join(", ") || "—"}</span> },
            { key: "status", header: "Статус", render: (n) => <StatusBadge status={n.status} label={NEED_STATUS[n.status] ?? n.status} /> },
            { key: "upd", header: "Обновлена", render: (n) => <span className="text-[12px] text-muted">{relative(n.updated_at)}</span> },
          ]}
        />
      )}
    </>
  );
}
