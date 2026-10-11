import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { api, unwrap, errorMessage, isApiError } from "@/api/client";
import { ek, useNeeds, useVacancies, useVacancy, useVacancyApplications, type SelectionItem, type Application } from "@/api/employer";
import { useRefdata, useSkillSearch } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, CardBody, Checkbox, DataTable, Dialog, EmptyState, Field, Input, LinkButton, Select, Skeleton, StatusBadge, SalaryRange, SalaryInputs, Tabs, Textarea, useToast } from "@/components/ui";
import { SkillPicker } from "./NeedForm";
import { SelectionTable } from "@/components/employer/SelectionTable";
import { InviteDialog, type InvitePayload } from "@/components/employer/InviteDialog";
import { APPLICATION_STATUS, GRADES, GRADE_STATUS, VACANCY_STATUS, WORK_FORMATS, category, grade, money, percent, relative } from "@/lib/format";

export function VacanciesList() {
  const list = useVacancies();
  return (
    <>
      <PageHeader eyebrow="работодатель" title="Вакансии" description="Классический путь: вакансия с обязательной вилкой, публичная страница и отклики." actions={<LinkButton to="/employer/vacancies/new" size="sm"><Plus className="size-4" /> Новая</LinkButton>} />
      {list.isLoading && <Skeleton className="h-40" />}
      {list.data && (
        <DataTable
          rows={list.data}
          rowKey={(v) => v.id}
          empty={<EmptyState title="Вакансий пока нет" description="Создайте вакансию — после публикации она появится в общем списке." action={<LinkButton to="/employer/vacancies/new">Создать</LinkButton>} />}
          columns={[
            {
              key: "title",
              header: "Вакансия",
              render: (v) => (
                <Link to={`/employer/vacancies/${v.id}`} className="font-semibold text-ink no-underline hover:text-brand-blue">
                  {v.title}
                </Link>
              ),
            },
            { key: "cat", header: "Категория", render: (v) => <span className="mono text-[13px]">{category(v.specialization, v.grade_min)}{v.grade_min !== v.grade_max ? ` – ${grade(v.grade_max)}` : ""}</span> },
            { key: "salary", header: "Вилка", render: (v) => <SalaryRange from={v.salary_from} to={v.salary_to} className="text-[13px]" /> },
            {
              key: "apps",
              header: "Отклики",
              render: (v) => (
                <span className="mono text-[13px]">
                  {v.applications?.total ?? 0}
                  {v.applications?.new ? <span className="text-brand-red"> · новых {v.applications.new}</span> : null}
                </span>
              ),
            },
            { key: "status", header: "Статус", render: (v) => <StatusBadge status={v.status} label={VACANCY_STATUS[v.status] ?? v.status} /> },
            { key: "upd", header: "Обновлена", render: (v) => <span className="text-[12px] text-muted">{relative(v.updated_at)}</span> },
          ]}
        />
      )}
    </>
  );
}

type Form = { title: string; description: string; specialization: string; grade_min: string; grade_max: string; must: { code: string; name: string }[]; nice: { code: string; name: string }[]; salary: { from: string; to: string }; work_formats: string[]; city: string; remote_ok: boolean; need_id: string };
const empty: Form = { title: "", description: "", specialization: "backend", grade_min: "middle", grade_max: "senior", must: [], nice: [], salary: { from: "", to: "" }, work_formats: ["remote"], city: "", remote_ok: true, need_id: "" };

export function VacancyForm() {
  const { id } = useParams();
  const editing = !!id;
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const refdata = useRefdata();
  const needs = useNeeds();
  const existing = useVacancy(id ?? "");
  const [f, setF] = useState<Form>(empty);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [mustQ, setMustQ] = useState("");
  const [niceQ, setNiceQ] = useState("");
  const mustSearch = useSkillSearch(mustQ, f.specialization);
  const niceSearch = useSkillSearch(niceQ, f.specialization);
  useEffect(() => {
    const v = existing.data;
    if (!v) return;
    setF({
      title: v.title, description: v.description, specialization: v.specialization, grade_min: v.grade_min, grade_max: v.grade_max,
      must: (v.must_skills ?? []).map((s) => ({ code: s.code, name: s.name })), nice: (v.nice_skills ?? []).map((s) => ({ code: s.code, name: s.name })),
      salary: { from: String(v.salary_from), to: String(v.salary_to) }, work_formats: v.work_formats ?? [], city: v.city ?? "", remote_ok: v.remote_ok, need_id: v.need_id ?? "",
    });
  }, [existing.data]);
  const save = useMutation({
    mutationFn: async () => {
      const body = {
        title: f.title.trim(), description: f.description.trim(), specialization: f.specialization as never, grade_min: f.grade_min as never, grade_max: f.grade_max as never,
        must_skills: f.must.map((s) => s.code), nice_skills: f.nice.map((s) => s.code), salary_from: Number(f.salary.from), salary_to: Number(f.salary.to),
        work_formats: f.work_formats, city: f.city.trim() || undefined, remote_ok: f.remote_ok, need_id: f.need_id || undefined,
      };
      if (editing) return unwrap(await api.PUT("/employer/vacancies/{id}", { params: { path: { id: id! } }, body }));
      return unwrap(await api.POST("/employer/vacancies", { body }));
    },
    onSuccess: (v) => {
      void qc.invalidateQueries({ queryKey: ek.vacancies });
      void qc.invalidateQueries({ queryKey: ek.vacancy(v.id) });
      toast.ok(editing ? "Вакансия обновлена" : "Черновик создан", editing ? undefined : "Опубликуйте, когда будете готовы");
      navigate(`/employer/vacancies/${v.id}`);
    },
    onError: (e) => {
      if (isApiError(e)) setFields(e.fields);
      toast.error("Не сохранилось", errorMessage(e));
    },
  });
  if (editing && existing.isLoading) return <Skeleton className="h-64" />;
  const specOptions = (refdata.data?.specializations ?? []).filter((s) => s.code !== "common").map((s) => ({ value: s.code, label: s.name }));
  const gradeOptions = Object.entries(GRADES).map(([value, label]) => ({ value, label }));
  const valid = f.title.trim() && f.description.trim().length >= 20 && Number(f.salary.from) > 0 && Number(f.salary.to) >= Number(f.salary.from);
  return (
    <div className="mx-auto max-w-[900px]">
      <PageHeader eyebrow="работодатель" title={editing ? "Вакансия" : "Новая вакансия"} description="Вилка обязательна — без неё вакансия не публикуется." />
      <Card>
        <CardBody className="grid gap-4 sm:grid-cols-2">
          <Field label="Название" required error={fields.title} className="sm:col-span-2">{(fid) => <Input id={fid} value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} maxLength={160} />}</Field>
          <Field label="Описание" required hint="От 20 символов: задачи, команда, условия" error={fields.description} className="sm:col-span-2">{(fid) => <Textarea id={fid} rows={6} value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} maxLength={20000} />}</Field>
          <Field label="Специализация" required>{(fid) => <Select id={fid} value={f.specialization} onChange={(e) => setF({ ...f, specialization: e.target.value, must: [], nice: [] })} options={specOptions} />}</Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Грейд от" required>{(fid) => <Select id={fid} value={f.grade_min} onChange={(e) => setF({ ...f, grade_min: e.target.value })} options={gradeOptions} />}</Field>
            <Field label="до" required>{(fid) => <Select id={fid} value={f.grade_max} onChange={(e) => setF({ ...f, grade_max: e.target.value })} options={gradeOptions} />}</Field>
          </div>
          <div className="sm:col-span-2">
            <SalaryInputs from={f.salary.from} to={f.salary.to} onChange={(salary) => setF({ ...f, salary })} errorFrom={fields.salary_from} errorTo={fields.salary_to} />
          </div>
          <SkillPicker label="Обязательные навыки" items={f.must} onChange={(must) => setF({ ...f, must })} q={mustQ} setQ={setMustQ} results={mustSearch.data ?? []} />
          <SkillPicker label="Будет плюсом" items={f.nice} onChange={(nice) => setF({ ...f, nice })} q={niceQ} setQ={setNiceQ} results={niceSearch.data ?? []} />
          <Field label="Формат" className="sm:col-span-2">
            {() => (
              <div className="flex flex-wrap items-center gap-2">
                {Object.entries(WORK_FORMATS).map(([code, label]) => {
                  const on = f.work_formats.includes(code);
                  return (
                    <button key={code} type="button" aria-pressed={on} className={`badge !px-3 !py-1.5 !text-[13px] ${on ? "badge-ink" : "badge-neutral"}`} onClick={() => setF({ ...f, work_formats: on ? f.work_formats.filter((x) => x !== code) : [...f.work_formats, code] })}>
                      {label}
                    </button>
                  );
                })}
                <Checkbox className="ml-2" checked={f.remote_ok} onChange={(e) => setF({ ...f, remote_ok: e.target.checked })} label="можно полностью удалённо" />
              </div>
            )}
          </Field>
          <Field label="Город">{(fid) => <Input id={fid} value={f.city} onChange={(e) => setF({ ...f, city: e.target.value })} />}</Field>
          <Field label="Связанная потребность" hint="Для приглашений из откликов и подборки">{(fid) => <Select id={fid} value={f.need_id} onChange={(e) => setF({ ...f, need_id: e.target.value })} placeholder="Нет" options={(needs.data ?? []).map((n) => ({ value: n.id, label: n.title }))} />}</Field>
        </CardBody>
        <div className="flex justify-between border-t border-line-soft px-5 py-3">
          <Button variant="ghost" onClick={() => navigate(-1)}>
            Отмена
          </Button>
          <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!valid}>
            {editing ? "Сохранить" : "Создать черновик"}
          </Button>
        </div>
      </Card>
    </div>
  );
}

export function VacancyDetailEmployer() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const tab = (params.get("tab") === "candidates" ? "candidates" : "applications") as "applications" | "candidates";
  const v = useVacancy(id);
  const qc = useQueryClient();
  const toast = useToast();
  const status = useMutation({
    mutationFn: async (s: "draft" | "published" | "closed") => unwrap(await api.POST("/employer/vacancies/{id}/status", { params: { path: { id } }, body: { status: s } })),
    onSuccess: (vv) => {
      void qc.invalidateQueries({ queryKey: ek.vacancies });
      void qc.invalidateQueries({ queryKey: ek.vacancy(id) });
      toast.ok(VACANCY_STATUS[vv.status] ?? vv.status);
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  if (v.isLoading) return <Skeleton className="h-64" />;
  if (!v.data) return <p className="text-danger">{errorMessage(v.error)}</p>;
  const d = v.data;
  return (
    <>
      <PageHeader
        eyebrow={<Link to="/employer/vacancies" className="no-underline">вакансии →</Link>}
        title={d.title}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <span className="mono">{category(d.specialization, d.grade_min)}{d.grade_min !== d.grade_max ? ` – ${grade(d.grade_max)}` : ""}</span>
            <SalaryRange from={d.salary_from} to={d.salary_to} />
            <StatusBadge status={d.status} label={VACANCY_STATUS[d.status] ?? d.status} />
            {d.status === "published" && (
              <Link to={`/vacancies/${d.id}`} className="text-[13px]">
                публичная страница →
              </Link>
            )}
          </span>
        }
        actions={
          <>
            <LinkButton to={`/employer/vacancies/${id}/edit`} variant="secondary" size="sm">
              Править
            </LinkButton>
            {d.status === "draft" && (
              <Button size="sm" onClick={() => status.mutate("published")} loading={status.isPending}>
                Опубликовать
              </Button>
            )}
            {d.status === "published" && (
              <Button size="sm" variant="ghost" onClick={() => status.mutate("closed")} loading={status.isPending}>
                Закрыть
              </Button>
            )}
          </>
        }
      />
      <Tabs value={tab} onChange={(t) => setParams({ tab: t })} ariaLabel="Разделы вакансии" items={[{ value: "applications", label: "Отклики", count: d.applications?.total }, { value: "candidates", label: "Подходящие из банка" }]} />
      <div className="mt-5">{tab === "applications" ? <ApplicationsTab vacancyId={id} salary={{ from: d.salary_from, to: d.salary_to }} /> : <CandidatesTab vacancyId={id} needId={d.need_id} salary={{ from: d.salary_from, to: d.salary_to }} title={d.title} />}</div>
    </>
  );
}

function ApplicationsTab({ vacancyId, salary }: { vacancyId: string; salary: { from: number; to: number } }) {
  const apps = useVacancyApplications(vacancyId);
  const qc = useQueryClient();
  const toast = useToast();
  const [reject, setReject] = useState<Application | null>(null);
  const [reason, setReason] = useState("");
  const [invite, setInvite] = useState<Application | null>(null);
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ek.applications(vacancyId) });
    void qc.invalidateQueries({ queryKey: ek.vacancy(vacancyId) });
    void qc.invalidateQueries({ queryKey: ek.vacancies });
  };
  const view = useMutation({ mutationFn: async (id: string) => unwrap(await api.POST("/employer/applications/{id}/view", { params: { path: { id } } })), onSuccess: refresh });
  const doReject = useMutation({
    mutationFn: async () => unwrap(await api.POST("/employer/applications/{id}/reject", { params: { path: { id: reject!.id } }, body: { reason } })),
    onSuccess: () => {
      refresh();
      setReject(null);
      setReason("");
      toast.ok("Отказ отправлен", "Кандидат увидит причину");
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  const doInvite = useMutation({
    mutationFn: async (p: InvitePayload) => unwrap(await api.POST("/employer/applications/{id}/invite", { params: { path: { id: invite!.id } }, body: p })),
    onSuccess: () => {
      refresh();
      setInvite(null);
      toast.ok("Приглашение отправлено");
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  if (apps.isLoading) return <Skeleton className="h-40" />;
  const items = apps.data ?? [];
  if (items.length === 0) return <EmptyState title="Откликов пока нет" description="После публикации вакансия видна всем кандидатам; отклик сразу открывает вам контакты." />;
  return (
    <div className="flex flex-col gap-3">
      {items.map((a) => (
        <Card key={a.id} className={`p-4 ${a.status === "sent" ? "border-l-4 border-l-brand-red" : ""}`}>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <Link to={`/employer/candidates/${a.candidate_id}`} className="font-semibold text-ink no-underline hover:text-brand-blue" onClick={() => a.status === "sent" && view.mutate(a.id)}>
                  {a.applicant?.display_name ?? "Кандидат"}
                </Link>
                {a.applicant?.grade && <span className="mono text-[12px]">{grade(a.applicant.grade)}</span>}
                {a.applicant?.grade_status && <StatusBadge status={a.applicant.grade_status} label={GRADE_STATUS[a.applicant.grade_status] ?? a.applicant.grade_status} />}
                {a.applicant?.percentile !== undefined && <Badge tone="blue">{percent(a.applicant.percentile)}</Badge>}
              </div>
              <div className="mt-0.5 text-[12.5px] text-muted">
                {[a.applicant?.headline, a.applicant?.city, a.applicant?.salary_expected ? money(a.applicant.salary_expected) : null, a.applicant?.experience_years !== undefined ? `${a.applicant.experience_years} лет` : null].filter(Boolean).join(" · ")} · {relative(a.sent_at)}
              </div>
              {a.cover_letter && <p className="mt-2 whitespace-pre-wrap text-[14px] text-ink-2">{a.cover_letter}</p>}
              {a.reject_reason && <p className="mt-2 text-[13px] text-danger">Отказ: {a.reject_reason}</p>}
            </div>
            <div className="flex flex-col items-end gap-2">
              <StatusBadge status={a.status} label={APPLICATION_STATUS[a.status] ?? a.status} />
              {(a.status === "sent" || a.status === "viewed") && (
                <div className="flex gap-1">
                  {a.status === "sent" && (
                    <Button size="sm" variant="ghost" onClick={() => view.mutate(a.id)}>
                      Просмотрено
                    </Button>
                  )}
                  <Button size="sm" variant="ghost" onClick={() => setReject(a)}>
                    Отказать
                  </Button>
                  <Button size="sm" onClick={() => setInvite(a)}>
                    Пригласить
                  </Button>
                </div>
              )}
            </div>
          </div>
        </Card>
      ))}
      <Dialog
        open={!!reject}
        onClose={() => setReject(null)}
        title="Отказ по отклику"
        footer={
          <>
            <Button variant="ghost" onClick={() => setReject(null)}>
              Отмена
            </Button>
            <Button variant="danger" onClick={() => doReject.mutate()} loading={doReject.isPending} disabled={!reason.trim()}>
              Отказать
            </Button>
          </>
        }
      >
        <Field label="Причина" required hint="Кандидат увидит её — это уважение к его времени">
          {(fid) => <Textarea id={fid} rows={3} maxLength={1000} value={reason} onChange={(e) => setReason(e.target.value)} />}
        </Field>
      </Dialog>
      {invite && <InviteDialog open onClose={() => setInvite(null)} onSubmit={(p) => doInvite.mutate(p)} busy={doInvite.isPending} candidateName={invite.applicant?.display_name} title="Пригласить из отклика" defaults={{ salary_from: salary.from, salary_to: salary.to }} />}
    </div>
  );
}

function CandidatesTab({ vacancyId, needId, salary, title }: { vacancyId: string; needId?: string; salary: { from: number; to: number }; title: string }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [items, setItems] = useState<SelectionItem[] | null>(null);
  const [invite, setInvite] = useState<SelectionItem | null>(null);
  const preview = useMutation({
    mutationFn: async () => unwrap(await api.POST("/employer/vacancies/{id}/candidates", { params: { path: { id: vacancyId } }, body: { limit: 30 } })).items ?? [],
    onSuccess: setItems,
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  const send = useMutation({
    mutationFn: async (p: InvitePayload) => unwrap(await api.POST("/employer/invitations", { body: { candidate_id: invite!.candidate.id, need_id: needId, ...p } })),
    onSuccess: () => {
      toast.ok("Приглашение отправлено");
      setInvite(null);
      void qc.invalidateQueries({ queryKey: ["employer"] });
      preview.mutate();
    },
    onError: (e) => toast.error("Не отправлено", errorMessage(e)),
  });
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-[14px] text-muted">Та же объяснимая модель, что и для потребности: кандидаты категорий вакансии, без сохранения версии.</p>
        <Button onClick={() => preview.mutate()} loading={preview.isPending}>
          {items ? "Обновить" : "Показать подходящих"}
        </Button>
      </div>
      {items && items.length === 0 && <EmptyState title="Подходящих пока нет" description="В категориях вакансии нет видимых кандидатов с согласием на публикацию." />}
      {items && items.length > 0 && <SelectionTable items={items} onInvite={setInvite} />}
      {invite && <InviteDialog open onClose={() => setInvite(null)} onSubmit={(p) => send.mutate(p)} busy={send.isPending} candidateName={invite.candidate.display_name} defaults={{ salary_from: salary.from, salary_to: salary.to, message: `Здравствуйте! Приглашаем обсудить вакансию «${title}».` }} />}
    </div>
  );
}
