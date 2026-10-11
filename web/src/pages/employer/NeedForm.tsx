import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles, Wand2 } from "lucide-react";
import { api, unwrap, errorMessage, isApiError } from "@/api/client";
import { ek, useNeed, type Extraction } from "@/api/employer";
import { useRefdata, useSkillSearch } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, CardBody, CardHeader, Checkbox, Field, Input, Select, Skeleton, Textarea, SalaryInputs, useToast } from "@/components/ui";
import { GRADES, WORK_FORMATS } from "@/lib/format";

type Form = { title: string; description: string; team_description: string; specialization: string; grade_min: string; grade_max: string; must: { code: string; name: string }[]; nice: { code: string; name: string }[]; salary: { from: string; to: string }; work_formats: string[]; city: string; remote_ok: boolean };
const empty: Form = { title: "", description: "", team_description: "", specialization: "backend", grade_min: "middle", grade_max: "senior", must: [], nice: [], salary: { from: "", to: "" }, work_formats: ["remote"], city: "", remote_ok: true };

/** Мастер потребности: свободный текст → разбор помощником → подтверждение структуры. Ничего не сохраняется до «Сохранить». */
export function NeedForm() {
  const { id } = useParams();
  const editing = !!id;
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const refdata = useRefdata();
  const existing = useNeed(id ?? "");
  const [text, setText] = useState("");
  const [extraction, setExtraction] = useState<Extraction | null>(null);
  const [f, setF] = useState<Form>(empty);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [mustQ, setMustQ] = useState("");
  const [niceQ, setNiceQ] = useState("");
  const mustSearch = useSkillSearch(mustQ, f.specialization);
  const niceSearch = useSkillSearch(niceQ, f.specialization);

  useEffect(() => {
    const n = existing.data;
    if (!n) return;
    setF({
      title: n.title, description: n.description ?? "", team_description: n.team_description ?? "", specialization: n.specialization, grade_min: n.grade_min, grade_max: n.grade_max,
      must: (n.must_skills ?? []).map((s) => ({ code: s.code, name: s.name })), nice: (n.nice_skills ?? []).map((s) => ({ code: s.code, name: s.name })),
      salary: { from: String(n.salary_from), to: String(n.salary_to) }, work_formats: n.work_formats ?? [], city: n.city ?? "", remote_ok: n.remote_ok,
    });
  }, [existing.data]);

  const extract = useMutation({
    mutationFn: async () => unwrap(await api.POST("/employer/needs/extract", { body: { text } })),
    onSuccess: (ex) => {
      setExtraction(ex);
      const d = ex.draft;
      setF((p) => ({
        ...p,
        title: d.title || p.title,
        description: text,
        specialization: d.specialization || p.specialization,
        grade_min: d.grade_min || p.grade_min,
        grade_max: d.grade_max || p.grade_max,
        must: (ex.must_skills ?? []).map((s) => ({ code: s.code, name: s.name })),
        nice: (ex.nice_skills ?? []).map((s) => ({ code: s.code, name: s.name })),
        salary: { from: d.salary_from ? String(d.salary_from) : p.salary.from, to: d.salary_to ? String(d.salary_to) : p.salary.to },
        work_formats: d.work_formats?.length ? d.work_formats : p.work_formats,
        city: d.city ?? p.city,
        remote_ok: d.remote_ok,
      }));
    },
    onError: (e) => toast.error("Не удалось разобрать", errorMessage(e)),
  });

  const save = useMutation({
    mutationFn: async () => {
      const body = {
        title: f.title.trim(),
        description: f.description.trim() || undefined,
        team_description: f.team_description.trim() || undefined,
        specialization: f.specialization as never,
        grade_min: f.grade_min as never,
        grade_max: f.grade_max as never,
        must_skills: f.must.map((s) => s.code),
        nice_skills: f.nice.map((s) => s.code),
        salary_from: Number(f.salary.from),
        salary_to: Number(f.salary.to),
        work_formats: f.work_formats,
        city: f.city.trim() || undefined,
        remote_ok: f.remote_ok,
        extraction: extraction ? (extraction.draft as unknown as Record<string, unknown>) : undefined,
      };
      if (editing) return unwrap(await api.PUT("/employer/needs/{id}", { params: { path: { id: id! } }, body }));
      return unwrap(await api.POST("/employer/needs", { body }));
    },
    onSuccess: (n) => {
      void qc.invalidateQueries({ queryKey: ek.needs });
      void qc.invalidateQueries({ queryKey: ek.need(n.id) });
      toast.ok(editing ? "Потребность обновлена" : "Потребность создана", "Теперь посмотрите карту категорий");
      navigate(`/employer/needs/${n.id}`);
    },
    onError: (e) => {
      if (isApiError(e)) setFields(e.fields);
      toast.error("Не сохранилось", errorMessage(e));
    },
  });

  if (editing && existing.isLoading) return <Skeleton className="h-64" />;
  const specOptions = (refdata.data?.specializations ?? []).filter((s) => s.code !== "common").map((s) => ({ value: s.code, label: s.name }));
  const gradeOptions = Object.entries(GRADES).map(([value, label]) => ({ value, label }));
  const valid = f.title.trim() && Number(f.salary.from) > 0 && Number(f.salary.to) >= Number(f.salary.from);

  return (
    <div className="mx-auto max-w-[900px]">
      <PageHeader eyebrow="работодатель" title={editing ? "Потребность" : "Новая потребность"} description="Потребность — не вакансия: это описание того, кто нужен. Из неё строятся карта категорий и подборка." />
      {!editing && (
        <Card className="mb-4 border-brand-blue/40">
          <CardHeader title={<span className="inline-flex items-center gap-2"><Wand2 className="size-4 text-brand-blue" /> Опишите словами</span>} description="Помощник предложит структуру — вы её подтвердите. В MVP работает эмулятор на правилах; подключение модели — одной переменной окружения." />
          <CardBody>
            <Textarea rows={5} value={text} onChange={(e) => setText(e.target.value)} placeholder="Ищем senior Go-разработчика в платёжную команду: PostgreSQL, Kafka, опыт с высоконагруженными сервисами. Удалённо, вилка 300–400 тысяч." maxLength={8000} />
            <div className="mt-3 flex items-center justify-between gap-3">
              <span className="text-[12px] text-muted">{text.trim().length < 20 ? "Хотя бы одно-два предложения" : `${text.trim().length} символов`}</span>
              <Button variant="secondary" onClick={() => extract.mutate()} loading={extract.isPending} disabled={text.trim().length < 20} icon={<Sparkles className="size-4" />}>
                Разобрать в структуру
              </Button>
            </div>
            {extraction && (
              <div className="mt-3 rounded-md bg-brand-blue-soft/40 px-4 py-3 text-[13px]">
                <span className="label-mono">разбор · {extraction.provider}</span>
                <div className="mt-1">Поля ниже предзаполнены. {(extraction.draft.warnings ?? []).length > 0 && <span className="text-warn">{(extraction.draft.warnings ?? []).join(" · ")}</span>}</div>
              </div>
            )}
          </CardBody>
        </Card>
      )}
      <Card>
        <CardBody className="grid gap-4 sm:grid-cols-2">
          <Field label="Название" required error={fields.title} className="sm:col-span-2">{(id2) => <Input id={id2} value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} maxLength={160} placeholder="Go-разработчик в платёжную команду" />}</Field>
          <Field label="Специализация" required>{(id2) => <Select id={id2} value={f.specialization} onChange={(e) => setF({ ...f, specialization: e.target.value, must: [], nice: [] })} options={specOptions} />}</Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Грейд от" required>{(id2) => <Select id={id2} value={f.grade_min} onChange={(e) => setF({ ...f, grade_min: e.target.value })} options={gradeOptions} />}</Field>
            <Field label="до" required>{(id2) => <Select id={id2} value={f.grade_max} onChange={(e) => setF({ ...f, grade_max: e.target.value })} options={gradeOptions} />}</Field>
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
          <Field label="Город" error={fields.city}>{(id2) => <Input id={id2} value={f.city} onChange={(e) => setF({ ...f, city: e.target.value })} placeholder="если важно" />}</Field>
          <Field label="О команде" className="sm:col-span-2" hint="Кандидат увидит это в приглашении — расскажите, с кем предстоит работать">{(id2) => <Textarea id={id2} rows={2} value={f.team_description} onChange={(e) => setF({ ...f, team_description: e.target.value })} maxLength={4000} />}</Field>
          {editing && <Field label="Описание" className="sm:col-span-2">{(id2) => <Textarea id={id2} rows={3} value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} maxLength={8000} />}</Field>}
        </CardBody>
        <div className="flex justify-between border-t border-line-soft px-5 py-3">
          <Button variant="ghost" onClick={() => navigate(-1)}>
            Отмена
          </Button>
          <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!valid}>
            {editing ? "Сохранить" : "Создать и открыть карту"}
          </Button>
        </div>
      </Card>
    </div>
  );
}

export function SkillPicker({ label, items, onChange, q, setQ, results }: { label: string; items: { code: string; name: string }[]; onChange: (v: { code: string; name: string }[]) => void; q: string; setQ: (v: string) => void; results: { code: string; name: string }[] }) {
  return (
    <Field label={label}>
      {(id) => (
        <div>
          <div className="mb-1.5 flex flex-wrap gap-1.5">
            {items.map((s) => (
              <button key={s.code} type="button" className="badge badge-ink !cursor-pointer" title="Убрать" onClick={() => onChange(items.filter((x) => x.code !== s.code))}>
                {s.name} ×
              </button>
            ))}
          </div>
          <Input id={id} value={q} onChange={(e) => setQ(e.target.value)} placeholder="поиск по справочнику…" />
          {q.trim().length >= 2 && (
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {results
                .filter((s) => !items.some((x) => x.code === s.code))
                .slice(0, 8)
                .map((s) => (
                  <button key={s.code} type="button" className="badge badge-blue !cursor-pointer" onClick={() => { onChange([...items, { code: s.code, name: s.name }]); setQ(""); }}>
                    + {s.name}
                  </button>
                ))}
              {results.length === 0 && <Badge>ничего не найдено</Badge>}
            </div>
          )}
        </div>
      )}
    </Field>
  );
}
