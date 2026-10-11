import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router";
import { MapPin, Building2 } from "lucide-react";
import { api, unwrap } from "@/api/client";
import { Badge, Card, EmptyState, Field, Input, Select, Skeleton, SalaryRange } from "@/components/ui";
import { GRADES, SPECIALIZATIONS, WORK_FORMATS, grade, spec, relative, plural } from "@/lib/format";

/** Публичный список вакансий: вилка видна всегда — это обязательное условие платформы. */
export function PublicVacancies() {
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState(params.get("q") ?? "");
  const filters = {
    specialization: params.get("specialization") ?? "",
    grade: params.get("grade") ?? "",
    work_format: params.get("work_format") ?? "",
    q: params.get("q") ?? "",
  };
  const set = (k: string, v: string) => {
    const next = new URLSearchParams(params);
    if (v) next.set(k, v);
    else next.delete(k);
    setParams(next, { replace: true });
  };
  const list = useQuery({
    queryKey: ["vacancies", filters],
    queryFn: async () =>
      unwrap(
        await api.GET("/vacancies", {
          params: {
            query: {
              specialization: (filters.specialization || undefined) as never,
              grade: (filters.grade || undefined) as never,
              work_format: (filters.work_format || undefined) as never,
              q: filters.q || undefined,
              limit: 50,
            },
          },
        }),
      ),
  });

  return (
    <div className="mx-auto max-w-[1200px] px-4 py-8 sm:px-6">
      <div className="label-mono">открытые позиции</div>
      <h1 className="mt-1">Вакансии</h1>
      <p className="mt-1 max-w-2xl text-muted">Каждая вакансия на платформе публикуется с зарплатной вилкой. Откликаясь, вы открываете компании свои контакты.</p>

      <form
        className="card mt-6 grid gap-3 p-4 sm:grid-cols-[1fr_180px_160px_160px]"
        onSubmit={(e) => {
          e.preventDefault();
          set("q", q.trim());
        }}
      >
        <Field label="Поиск">{(id) => <Input id={id} value={q} onChange={(e) => setQ(e.target.value)} placeholder="Название или компания" />}</Field>
        <Field label="Специализация">
          {(id) => <Select id={id} value={filters.specialization} onChange={(e) => set("specialization", e.target.value)} placeholder="Любая" options={Object.entries(SPECIALIZATIONS).filter(([k]) => k !== "common").map(([value, label]) => ({ value, label }))} />}
        </Field>
        <Field label="Грейд">{(id) => <Select id={id} value={filters.grade} onChange={(e) => set("grade", e.target.value)} placeholder="Любой" options={Object.entries(GRADES).map(([value, label]) => ({ value, label }))} />}</Field>
        <Field label="Формат">{(id) => <Select id={id} value={filters.work_format} onChange={(e) => set("work_format", e.target.value)} placeholder="Любой" options={Object.entries(WORK_FORMATS).map(([value, label]) => ({ value, label }))} />}</Field>
      </form>

      <div className="mt-4 text-[13px] text-muted">{list.data ? plural(list.data.total, "вакансия", "вакансии", "вакансий") : ""}</div>
      <div className="mt-3 grid gap-3">
        {list.isLoading && [1, 2, 3].map((i) => <Skeleton key={i} className="h-28" />)}
        {list.data && (list.data.items ?? []).length === 0 && <EmptyState title="Ничего не нашлось" description="Попробуйте снять часть фильтров." />}
        {(list.data?.items ?? []).map((v) => (
          <Link key={v.id} to={`/vacancies/${v.id}`} className="no-underline">
            <Card className="p-5 transition-colors hover:border-brand-blue">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <h3 className="text-[17px] text-ink">{v.title}</h3>
                  <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-[13px] text-muted">
                    <span className="inline-flex items-center gap-1">
                      <Building2 className="size-3.5" /> {v.company.name}
                    </span>
                    {(v.city || v.remote_ok) && (
                      <span className="inline-flex items-center gap-1">
                        <MapPin className="size-3.5" /> {[v.city, v.remote_ok ? "удалённо" : ""].filter(Boolean).join(" · ")}
                      </span>
                    )}
                    <span>{relative(v.published_at)}</span>
                  </div>
                </div>
                <SalaryRange from={v.salary_from} to={v.salary_to} className="text-[18px] text-ink" />
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5">
                <Badge tone="ink">{spec(v.specialization)}</Badge>
                <Badge tone="blue">
                  {grade(v.grade_min)}
                  {v.grade_min !== v.grade_max ? ` – ${grade(v.grade_max)}` : ""}
                </Badge>
                {(v.work_formats ?? []).map((f) => (
                  <Badge key={f}>{WORK_FORMATS[f] ?? f}</Badge>
                ))}
                {(v.must_skills ?? []).slice(0, 5).map((s) => (
                  <Badge key={s.code}>{s.name}</Badge>
                ))}
              </div>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}
