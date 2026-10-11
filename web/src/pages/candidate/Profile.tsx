import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Award, Plus, Trash2, Upload } from "lucide-react";
import { api, unwrap, errorMessage, isApiError, type Schemas } from "@/api/client";
import { useSession } from "@/api/session";
import { qk, useAchievements, useContacts, useFspLink, usePrivacy, useProfile, useRefdata, useSkillSearch, useGiveConsent, type Profile, type ResumeUpload } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, CardBody, CardHeader, Checkbox, Field, Input, Select, Skeleton, Tabs, Textarea, useToast } from "@/components/ui";
import { GRADES, SPECIALIZATIONS, WORK_FORMATS, dateShort } from "@/lib/format";
import { downloadProfilePdf } from "./pdf";

type Tab = "profile" | "skills" | "experience" | "contacts" | "resume" | "fsp";
const TABS: { value: Tab; label: string }[] = [
  { value: "profile", label: "Анкета" },
  { value: "skills", label: "Навыки" },
  { value: "experience", label: "Опыт и образование" },
  { value: "contacts", label: "Контакты и приватность" },
  { value: "resume", label: "Резюме PDF" },
  { value: "fsp", label: "ФСП ID" },
];

export function CandidateProfile() {
  const [params, setParams] = useSearchParams();
  const tab = (TABS.some((t) => t.value === params.get("tab")) ? params.get("tab") : "profile") as Tab;
  const profile = useProfile();
  return (
    <>
      <PageHeader
        eyebrow="кандидат"
        title="Профиль"
        description="Анкету видят работодатели в подборках (с учётом приватности); контакты — только после вашего согласия."
        actions={
          <Button variant="secondary" size="sm" onClick={() => void downloadProfilePdf()}>
            PDF-профиль
          </Button>
        }
      />
      <Tabs value={tab} onChange={(t) => setParams({ tab: t })} ariaLabel="Разделы профиля" items={TABS} />
      <div className="mt-5">
        {profile.isLoading && <Skeleton className="h-64" />}
        {profile.data && tab === "profile" && <ProfileForm p={profile.data} />}
        {profile.data && tab === "skills" && <SkillsEditor p={profile.data} />}
        {profile.data && tab === "experience" && <ExperienceEditor p={profile.data} />}
        {tab === "contacts" && <ContactsPrivacy />}
        {profile.data && tab === "resume" && <ResumeTab p={profile.data} />}
        {tab === "fsp" && <FspTab />}
      </div>
    </>
  );
}

// ---------- Анкета ----------
function ProfileForm({ p }: { p: Profile }) {
  const qc = useQueryClient();
  const toast = useToast();
  const refdata = useRefdata();
  const [f, setF] = useState({
    first_name: p.first_name,
    last_name: p.last_name,
    middle_name: p.middle_name ?? "",
    city: p.city ?? "",
    headline: p.headline ?? "",
    about: p.about ?? "",
    experience_years: p.experience_years ? String(p.experience_years) : "",
    primary_specialization: p.primary_specialization ?? "",
    work_formats: p.work_formats ?? [],
    relocation: p.relocation,
    salary_expected: p.salary_expected ? String(p.salary_expected) : "",
    salary_min: p.salary_min ? String(p.salary_min) : "",
    visibility: p.visibility,
    github: p.links?.github ?? "",
    site: p.links?.site ?? "",
  });
  const [fields, setFields] = useState<Record<string, string>>({});
  const save = useMutation({
    mutationFn: async () => {
      const links: Record<string, string> = {};
      if (f.github.trim()) links.github = f.github.trim();
      if (f.site.trim()) links.site = f.site.trim();
      return unwrap(
        await api.PUT("/candidate/profile", {
          body: {
            first_name: f.first_name.trim(),
            last_name: f.last_name.trim(),
            middle_name: f.middle_name.trim() || undefined,
            city: f.city.trim() || undefined,
            headline: f.headline.trim() || undefined,
            about: f.about.trim() || undefined,
            experience_years: f.experience_years ? Number(f.experience_years) : undefined,
            primary_specialization: (f.primary_specialization || undefined) as never,
            work_formats: f.work_formats,
            relocation: f.relocation,
            salary_expected: f.salary_expected ? Number(f.salary_expected) : undefined,
            salary_min: f.salary_min ? Number(f.salary_min) : undefined,
            visibility: f.visibility,
            links,
          },
        }),
      );
    },
    onSuccess: () => {
      setFields({});
      void qc.invalidateQueries({ queryKey: qk.profile });
      toast.ok("Анкета сохранена");
    },
    onError: (e) => {
      if (isApiError(e)) setFields(e.fields);
      toast.error("Не сохранилось", errorMessage(e));
    },
  });
  const specOptions = (refdata.data?.specializations ?? []).filter((s) => s.code !== "common").map((s) => ({ value: s.code, label: s.name }));
  return (
    <Card>
      <CardBody className="grid gap-4 sm:grid-cols-2">
        <Field label="Имя" required error={fields.first_name}>{(id) => <Input id={id} value={f.first_name} onChange={(e) => setF({ ...f, first_name: e.target.value })} />}</Field>
        <Field label="Фамилия" required error={fields.last_name}>{(id) => <Input id={id} value={f.last_name} onChange={(e) => setF({ ...f, last_name: e.target.value })} />}</Field>
        <Field label="Отчество">{(id) => <Input id={id} value={f.middle_name} onChange={(e) => setF({ ...f, middle_name: e.target.value })} />}</Field>
        <Field label="Город" error={fields.city}>{(id) => <Input id={id} value={f.city} onChange={(e) => setF({ ...f, city: e.target.value })} />}</Field>
        <Field label="Заголовок" className="sm:col-span-2" error={fields.headline}>{(id) => <Input id={id} value={f.headline} onChange={(e) => setF({ ...f, headline: e.target.value })} maxLength={160} placeholder="Backend-разработчик на Go, 4 года в финтехе" />}</Field>
        <Field label="О себе" className="sm:col-span-2" error={fields.about}>{(id) => <Textarea id={id} value={f.about} onChange={(e) => setF({ ...f, about: e.target.value })} rows={4} maxLength={4000} />}</Field>
        <Field label="Основная специализация">{(id) => <Select id={id} value={f.primary_specialization} onChange={(e) => setF({ ...f, primary_specialization: e.target.value })} placeholder="Не выбрана" options={specOptions} />}</Field>
        <Field label="Опыт, лет" error={fields.experience_years}>{(id) => <Input id={id} mono inputMode="decimal" value={f.experience_years} onChange={(e) => setF({ ...f, experience_years: e.target.value.replace(",", ".") })} />}</Field>
        <Field label="Формат работы" className="sm:col-span-2">
          {() => (
            <div className="flex flex-wrap gap-2">
              {Object.entries(WORK_FORMATS).map(([code, label]) => {
                const on = f.work_formats.includes(code);
                return (
                  <button key={code} type="button" aria-pressed={on} className={`badge !px-3 !py-1.5 !text-[13px] ${on ? "badge-ink" : "badge-neutral"}`} onClick={() => setF({ ...f, work_formats: on ? f.work_formats.filter((x) => x !== code) : [...f.work_formats, code] })}>
                    {label}
                  </button>
                );
              })}
              <Checkbox className="ml-2" checked={f.relocation} onChange={(e) => setF({ ...f, relocation: e.target.checked })} label="готов к переезду" />
            </div>
          )}
        </Field>
        <Field label="Ожидание по зарплате, ₽" error={fields.salary_expected}>{(id) => <Input id={id} mono inputMode="numeric" value={f.salary_expected} onChange={(e) => setF({ ...f, salary_expected: e.target.value.replace(/\D/g, "") })} />}</Field>
        <Field label="Минимум, ₽" hint="Приглашения ниже этого порога помечаются работодателю" error={fields.salary_min}>{(id) => <Input id={id} mono inputMode="numeric" value={f.salary_min} onChange={(e) => setF({ ...f, salary_min: e.target.value.replace(/\D/g, "") })} />}</Field>
        <Field label="Видимость для работодателей" hint="open — ищу активно; passive — рассматриваю предложения; hidden — не показывать в подборках (откликаться можно)">
          {(id) => (
            <Select
              id={id}
              value={f.visibility}
              onChange={(e) => setF({ ...f, visibility: e.target.value as Profile["visibility"] })}
              options={[
                { value: "open", label: "Открыт к предложениям" },
                { value: "passive", label: "Пассивно" },
                { value: "hidden", label: "Скрыт" },
              ]}
            />
          )}
        </Field>
        <div className="grid gap-4">
          <Field label="GitHub" error={fields["links.github"]}>{(id) => <Input id={id} value={f.github} onChange={(e) => setF({ ...f, github: e.target.value })} placeholder="https://github.com/…" />}</Field>
          <Field label="Сайт / портфолио" error={fields["links.site"]}>{(id) => <Input id={id} value={f.site} onChange={(e) => setF({ ...f, site: e.target.value })} placeholder="https://…" />}</Field>
        </div>
      </CardBody>
      <div className="flex justify-end border-t border-line-soft px-5 py-3">
        <Button onClick={() => save.mutate()} loading={save.isPending}>
          Сохранить
        </Button>
      </div>
    </Card>
  );
}

// ---------- Навыки ----------
function SkillsEditor({ p }: { p: Profile }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [items, setItems] = useState(() => (p.skills ?? []).map((s) => ({ code: s.code, name: s.name, source: s.source })));
  const [q, setQ] = useState("");
  const search = useSkillSearch(q, p.primary_specialization ?? "");
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/candidate/skills", { body: { items: items.map((s) => ({ code: s.code })) } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.profile });
      toast.ok("Навыки сохранены");
    },
    onError: (e) => toast.error("Не сохранилось", errorMessage(e)),
  });
  return (
    <Card>
      <CardHeader title="Навыки" description="Подтверждённые тестом навыки помечены и не удаляются. Остальные — заявленные вами или из резюме." />
      <CardBody>
        <div className="flex flex-wrap gap-1.5">
          {items.length === 0 && <span className="text-[13px] text-muted">Навыков пока нет — добавьте из справочника.</span>}
          {items.map((s) => (
            <span key={s.code} className={`badge !py-1 !text-[12.5px] ${s.source === "test_confirmed" ? "badge-ok" : s.source === "resume" ? "badge-blue" : "badge-neutral"}`}>
              {s.name}
              {s.source === "test_confirmed" ? " ✓" : ""}
              {s.source !== "test_confirmed" && (
                <button type="button" className="ml-1 opacity-70 hover:opacity-100" aria-label={`Убрать ${s.name}`} onClick={() => setItems(items.filter((x) => x.code !== s.code))}>
                  ×
                </button>
              )}
            </span>
          ))}
        </div>
        <Field label="Добавить навык" className="mt-4" hint="Поиск по справочнику из 300+ навыков с синонимами">
          {(id) => <Input id={id} value={q} onChange={(e) => setQ(e.target.value)} placeholder="kubernetes, react, pandas…" />}
        </Field>
        {search.data && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {search.data
              .filter((s) => !items.some((x) => x.code === s.code))
              .slice(0, 12)
              .map((s) => (
                <button key={s.code} type="button" className="badge badge-blue !cursor-pointer" onClick={() => { setItems([...items, { code: s.code, name: s.name, source: "declared" }]); setQ(""); }}>
                  <Plus className="size-3" /> {s.name} <span className="opacity-60">· {s.group}</span>
                </button>
              ))}
            {search.data.length === 0 && <span className="text-[13px] text-muted">Ничего не найдено</span>}
          </div>
        )}
      </CardBody>
      <div className="flex justify-end border-t border-line-soft px-5 py-3">
        <Button onClick={() => save.mutate()} loading={save.isPending}>
          Сохранить
        </Button>
      </div>
    </Card>
  );
}

// ---------- Опыт и образование ----------
type Exp = { company_name: string; position: string; started_on: string; ended_on: string; description: string };
type Edu = { institution: string; degree: string; field: string; graduated_year: string };

function ExperienceEditor({ p }: { p: Profile }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [exps, setExps] = useState<Exp[]>(() => (p.experiences ?? []).map((e) => ({ company_name: e.company_name, position: e.position, started_on: e.started_on, ended_on: e.ended_on ?? "", description: e.description ?? "" })));
  const [edus, setEdus] = useState<Edu[]>(() => (p.education ?? []).map((e) => ({ institution: e.institution, degree: e.degree ?? "", field: e.field ?? "", graduated_year: e.graduated_year ? String(e.graduated_year) : "" })));
  const save = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.PUT("/candidate/experiences", {
          body: { items: exps.map((e) => ({ company_name: e.company_name.trim(), position: e.position.trim(), started_on: e.started_on, ended_on: e.ended_on || undefined, description: e.description.trim() || undefined })) },
        }),
      );
      unwrap(await api.PUT("/candidate/education", { body: { items: edus.map((e) => ({ institution: e.institution.trim(), degree: e.degree.trim() || undefined, field: e.field.trim() || undefined, graduated_year: e.graduated_year ? Number(e.graduated_year) : undefined })) } }));
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.profile });
      toast.ok("Опыт и образование сохранены");
    },
    onError: (e) => toast.error("Не сохранилось", errorMessage(e)),
  });
  const upd = <T,>(arr: T[], i: number, patch: Partial<T>) => arr.map((x, j) => (j === i ? { ...x, ...patch } : x));
  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader title="Опыт работы" action={<Button size="sm" variant="secondary" icon={<Plus className="size-4" />} onClick={() => setExps([...exps, { company_name: "", position: "", started_on: "", ended_on: "", description: "" }])}>Добавить</Button>} />
        <CardBody className="flex flex-col gap-4">
          {exps.length === 0 && <p className="text-[13px] text-muted">Пока пусто. Можно загрузить PDF-резюме — опыт подтянется черновиком.</p>}
          {exps.map((e, i) => (
            <div key={i} className="grid gap-3 rounded-md border border-line-soft p-4 sm:grid-cols-2">
              <Field label="Компания" required>{(id) => <Input id={id} value={e.company_name} onChange={(ev) => setExps(upd(exps, i, { company_name: ev.target.value }))} />}</Field>
              <Field label="Должность" required>{(id) => <Input id={id} value={e.position} onChange={(ev) => setExps(upd(exps, i, { position: ev.target.value }))} />}</Field>
              <Field label="Начало" required>{(id) => <Input id={id} type="date" value={e.started_on} onChange={(ev) => setExps(upd(exps, i, { started_on: ev.target.value }))} />}</Field>
              <Field label="Окончание" hint="Пусто — по настоящее время">{(id) => <Input id={id} type="date" value={e.ended_on} onChange={(ev) => setExps(upd(exps, i, { ended_on: ev.target.value }))} />}</Field>
              <Field label="Что делали" className="sm:col-span-2">{(id) => <Textarea id={id} rows={2} value={e.description} onChange={(ev) => setExps(upd(exps, i, { description: ev.target.value }))} />}</Field>
              <div className="sm:col-span-2 flex justify-end">
                <Button size="sm" variant="ghost" icon={<Trash2 className="size-4" />} onClick={() => setExps(exps.filter((_, j) => j !== i))}>
                  Удалить
                </Button>
              </div>
            </div>
          ))}
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Образование" action={<Button size="sm" variant="secondary" icon={<Plus className="size-4" />} onClick={() => setEdus([...edus, { institution: "", degree: "", field: "", graduated_year: "" }])}>Добавить</Button>} />
        <CardBody className="flex flex-col gap-4">
          {edus.map((e, i) => (
            <div key={i} className="grid gap-3 rounded-md border border-line-soft p-4 sm:grid-cols-2">
              <Field label="Учебное заведение" required className="sm:col-span-2">{(id) => <Input id={id} value={e.institution} onChange={(ev) => setEdus(upd(edus, i, { institution: ev.target.value }))} />}</Field>
              <Field label="Степень">{(id) => <Input id={id} value={e.degree} onChange={(ev) => setEdus(upd(edus, i, { degree: ev.target.value }))} placeholder="бакалавр, магистр…" />}</Field>
              <Field label="Направление">{(id) => <Input id={id} value={e.field} onChange={(ev) => setEdus(upd(edus, i, { field: ev.target.value }))} />}</Field>
              <Field label="Год выпуска">{(id) => <Input id={id} mono inputMode="numeric" value={e.graduated_year} onChange={(ev) => setEdus(upd(edus, i, { graduated_year: ev.target.value.replace(/\D/g, "").slice(0, 4) }))} />}</Field>
              <div className="flex items-end justify-end">
                <Button size="sm" variant="ghost" icon={<Trash2 className="size-4" />} onClick={() => setEdus(edus.filter((_, j) => j !== i))}>
                  Удалить
                </Button>
              </div>
            </div>
          ))}
        </CardBody>
      </Card>
      <div className="flex justify-end">
        <Button onClick={() => save.mutate()} loading={save.isPending}>
          Сохранить
        </Button>
      </div>
    </div>
  );
}

// ---------- Контакты и приватность ----------
function ContactsPrivacy() {
  const qc = useQueryClient();
  const toast = useToast();
  const { consents, reload } = useSession();
  const contacts = useContacts();
  const privacy = usePrivacy();
  const consent = useGiveConsent();
  const [c, setC] = useState({ phone: "", telegram: "", email: "" });
  const [pr, setPr] = useState<Schemas["Privacy"] | null>(null);
  useEffect(() => {
    if (contacts.data) setC({ phone: contacts.data.phone ?? "", telegram: contacts.data.telegram ?? "", email: contacts.data.email ?? "" });
  }, [contacts.data]);
  useEffect(() => {
    if (privacy.data) setPr(privacy.data);
  }, [privacy.data]);
  const saveContacts = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/candidate/contacts", { body: { phone: c.phone.trim() || undefined, telegram: c.telegram.trim() || undefined, email: c.email.trim() || undefined, other: contacts.data?.other ?? {} } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.contacts });
      toast.ok("Контакты сохранены");
    },
    onError: (e) => toast.error("Не сохранилось", errorMessage(e)),
  });
  const savePrivacy = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/candidate/privacy", { body: pr! })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.privacy });
      toast.ok("Приватность обновлена");
    },
    onError: (e) => toast.error("Не сохранилось", errorMessage(e)),
  });
  const withdraw = useMutation({
    mutationFn: async (kind: "publication" | "fsp_data") => unwrap(await api.DELETE("/auth/consents/{kind}", { params: { path: { kind } } })),
    onSuccess: () => {
      void reload();
      toast.ok("Согласие отозвано");
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  const pub = consents.find((x) => x.kind === "publication" && !x.withdrawn_at);
  const privacyLabels: [keyof Schemas["Privacy"], string][] = [
    ["show_full_name", "Фамилия (иначе только имя)"],
    ["show_city", "Город"],
    ["show_salary_expected", "Ожидание по зарплате"],
    ["show_experience", "Опыт работы"],
    ["show_fsp", "Достижения ФСП"],
  ];
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader title="Контакты" description="Хранятся отдельно от анкеты. Работодатель получает их только после принятого приглашения или вашего отклика; каждый просмотр пишется в аудит." />
        <CardBody className="grid gap-4">
          {contacts.isLoading && <Skeleton className="h-24" />}
          <Field label="Телефон">{(id) => <Input id={id} mono value={c.phone} onChange={(e) => setC({ ...c, phone: e.target.value })} placeholder="+7 900 000-00-00" />}</Field>
          <Field label="Telegram">{(id) => <Input id={id} mono value={c.telegram} onChange={(e) => setC({ ...c, telegram: e.target.value })} placeholder="@username" />}</Field>
          <Field label="E-mail для связи">{(id) => <Input id={id} type="email" value={c.email} onChange={(e) => setC({ ...c, email: e.target.value })} />}</Field>
        </CardBody>
        <div className="flex justify-end border-t border-line-soft px-5 py-3">
          <Button onClick={() => saveContacts.mutate()} loading={saveContacts.isPending}>
            Сохранить
          </Button>
        </div>
      </Card>
      <div className="flex flex-col gap-4">
        <Card>
          <CardHeader title="Что видно в карточке" description="До раскрытия контактов работодатель видит анкету с учётом этих настроек" />
          <CardBody className="flex flex-col gap-3">
            {pr &&
              privacyLabels.map(([k, label]) => <Checkbox key={k} checked={Boolean(pr[k])} onChange={(e) => setPr({ ...pr, [k]: e.target.checked })} label={label} />)}
          </CardBody>
          <div className="flex justify-end border-t border-line-soft px-5 py-3">
            <Button onClick={() => savePrivacy.mutate()} loading={savePrivacy.isPending} disabled={!pr}>
              Сохранить
            </Button>
          </div>
        </Card>
        <Card>
          <CardHeader title="Согласия" />
          <CardBody className="flex flex-col gap-3 text-[14px]">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="font-medium">Показ профиля работодателям</div>
                <div className="text-[12px] text-muted">{pub ? `дано ${dateShort(pub.accepted_at)}` : "не дано — вы не попадаете в подборки"}</div>
              </div>
              {pub ? (
                <Button size="sm" variant="ghost" onClick={() => withdraw.mutate("publication")} loading={withdraw.isPending}>
                  Отозвать
                </Button>
              ) : (
                <Button size="sm" onClick={async () => { await consent.mutateAsync("publication"); await reload(); }} loading={consent.isPending}>
                  Дать
                </Button>
              )}
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
}

// ---------- Резюме PDF ----------
function ResumeTab({ p }: { p: Profile }) {
  const qc = useQueryClient();
  const toast = useToast();
  const fileRef = useRef<HTMLInputElement>(null);
  const [upload, setUpload] = useState<ResumeUpload | null>(null);
  const [apply, setApply] = useState({ profile: true, experiences: true, education: true, skills: true, contacts: true, overwrite: false });
  const up = useMutation({
    mutationFn: async (file: File) =>
      unwrap(
        await api.POST("/candidate/resume", {
          body: { file: "" } as never,
          bodySerializer: () => {
            const fd = new FormData();
            fd.append("file", file);
            return fd;
          },
        }),
      ),
    onSuccess: (r) => setUpload(r),
    onError: (e) => toast.error("Не удалось загрузить", errorMessage(e)),
  });
  const applyM = useMutation({
    mutationFn: async () => unwrap(await api.POST("/candidate/resume/{id}/apply", { params: { path: { id: upload!.id } }, body: apply })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["candidate"] });
      toast.ok("Данные из резюме применены", "Проверьте анкету, опыт и навыки");
      setUpload(null);
    },
    onError: (e) => toast.error("Не удалось применить", errorMessage(e)),
  });
  const d = upload?.draft;
  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_1.2fr]">
      <Card>
        <CardHeader title="Загрузить PDF" description={p.resume_uploaded ? "Резюме уже загружалось — можно обновить" : "До 10 МБ. Текст распознаётся и разбирается помощником в черновик"} />
        <CardBody>
          <input ref={fileRef} type="file" accept="application/pdf" className="hidden" onChange={(e) => e.target.files?.[0] && up.mutate(e.target.files[0])} />
          <Button variant="secondary" icon={<Upload className="size-4" />} onClick={() => fileRef.current?.click()} loading={up.isPending}>
            Выбрать файл
          </Button>
          {upload && (
            <p className="mt-3 text-[13px] text-muted">
              {upload.file_name} · {(upload.size_bytes / 1024).toFixed(0)} КБ · {upload.text_chars} символов · разбор: {upload.parser}
            </p>
          )}
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Черновик" description="Ничего не применяется автоматически — вы выбираете блоки" />
        <CardBody>
          {!d && <p className="text-[13px] text-muted">Загрузите PDF, чтобы увидеть, что помощник из него понял.</p>}
          {d && (
            <div className="flex flex-col gap-3 text-[14px]">
              <div>
                <div className="label-mono">анкета</div>
                <div>
                  {d.last_name} {d.first_name} {d.middle_name ?? ""} · {d.city ?? "город не найден"} · {d.headline ?? ""}
                </div>
                {d.primary_specialization && <Badge tone="ink" className="mt-1">{SPECIALIZATIONS[d.primary_specialization] ?? d.primary_specialization}</Badge>}
              </div>
              <div>
                <div className="label-mono">опыт · {(d.experiences ?? []).length}</div>
                <ul className="mt-1 flex flex-col gap-1">
                  {(d.experiences ?? []).slice(0, 5).map((e, i) => (
                    <li key={i}>
                      {e.position} — {e.company} <span className="text-muted">({e.started_on} – {e.ended_on ?? "н. в."})</span>
                    </li>
                  ))}
                </ul>
              </div>
              <div>
                <div className="label-mono">навыки · {(d.skills ?? []).length}</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {(d.skills ?? []).map((s) => (
                    <Badge key={s.code} tone={s.confidence >= 0.8 ? "blue" : "neutral"}>{s.name}</Badge>
                  ))}
                </div>
              </div>
              {(d.warnings ?? []).length > 0 && <p className="rounded-sm bg-warn-soft px-3 py-2 text-[13px] text-warn">{(d.warnings ?? []).join(" · ")}</p>}
              <div className="grid grid-cols-2 gap-2 border-t border-line-soft pt-3">
                <Checkbox checked={apply.profile} onChange={(e) => setApply({ ...apply, profile: e.target.checked })} label="Анкета" />
                <Checkbox checked={apply.experiences} onChange={(e) => setApply({ ...apply, experiences: e.target.checked })} label="Опыт" />
                <Checkbox checked={apply.education} onChange={(e) => setApply({ ...apply, education: e.target.checked })} label="Образование" />
                <Checkbox checked={apply.skills} onChange={(e) => setApply({ ...apply, skills: e.target.checked })} label="Навыки" />
                <Checkbox checked={apply.contacts} onChange={(e) => setApply({ ...apply, contacts: e.target.checked })} label="Контакты" />
                <Checkbox checked={apply.overwrite} onChange={(e) => setApply({ ...apply, overwrite: e.target.checked })} label="Перезаписать заполненное" />
              </div>
              <div className="flex justify-end">
                <Button onClick={() => applyM.mutate()} loading={applyM.isPending}>
                  Применить выбранное
                </Button>
              </div>
            </div>
          )}
        </CardBody>
      </Card>
    </div>
  );
}

// ---------- ФСП ID ----------
function FspTab() {
  const toast = useToast();
  const qc = useQueryClient();
  const link = useFspLink();
  const achievements = useAchievements(!!link.data?.linked);
  const refdata = useRefdata();
  const start = useMutation({
    mutationFn: async () => unwrap(await api.POST("/auth/fsp/start", { body: { mode: "link", consent_processing: true, return_to: "/auth/fsp/done" } })),
    onSuccess: (r) => window.location.assign(r.authorization_url),
    onError: (e) => toast.error(isApiError(e) && e.is("fsp_id_disabled") ? "ФСП ID не настроен на этом стенде" : errorMessage(e)),
  });
  const unlink = useMutation({
    mutationFn: async () => unwrap(await api.DELETE("/auth/fsp/link")),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.fspLink });
      void qc.invalidateQueries({ queryKey: qk.achievements });
      toast.ok("ФСП ID отвязан");
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  const disciplines = new Map((refdata.data?.fsp_disciplines ?? []).map((d) => [d.code, d.name]));
  const levels = new Map((refdata.data?.fsp_levels ?? []).map((d) => [d.code, d.name]));
  const sessionsQ = useQuery({ queryKey: ["noop"], queryFn: async () => null, enabled: false });
  void sessionsQ;
  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_1.2fr]">
      <Card>
        <CardHeader title="ФСП ID" description="Вход и привязка через OpenID Connect Федерации; достижения подтягиваются из реестра" />
        <CardBody className="flex flex-col gap-3 text-[14px]">
          {link.isLoading && <Skeleton className="h-10" />}
          {link.data && !link.data.linked && (
            <>
              <p className="text-muted">Не привязан. Привязка добавит результаты соревнований в профиль и в ранжирование. Их отсутствие ничем не штрафуется.</p>
              <Button onClick={() => start.mutate()} loading={start.isPending} variant="dark">
                Привязать ФСП ID
              </Button>
            </>
          )}
          {link.data?.linked && (
            <>
              <div>
                <span className="mono">{link.data.fsp_id}</span>
                <div className="text-[12px] text-muted">
                  привязан {dateShort(link.data.linked_at)} · синхронизация {link.data.last_sync_at ? dateShort(link.data.last_sync_at) : "—"}
                </div>
                {link.data.sync_error && <div className="mt-1 text-[12px] text-danger">{link.data.sync_error}</div>}
              </div>
              <Button variant="ghost" size="sm" onClick={() => unlink.mutate()} loading={unlink.isPending}>
                Отвязать
              </Button>
            </>
          )}
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Достижения" description={link.data?.linked ? `В реестре: ${achievements.data?.total ?? 0}` : "Появятся после привязки"} />
        <CardBody>
          {link.data?.linked && (achievements.data?.total ?? 0) === 0 && <p className="text-[14px] text-muted">Пока нет записей — ранжирование идёт по тесту и профилю.</p>}
          <ul className="flex flex-col gap-2">
            {(achievements.data?.items ?? []).map((a) => (
              <li key={a.external_id} className="flex items-start gap-3 rounded-md border border-line-soft px-3 py-2">
                <Award className="mt-0.5 size-4 shrink-0 text-brand-red" />
                <div className="min-w-0 flex-1 text-[14px]">
                  <div className="font-medium">{a.event_name}</div>
                  <div className="text-[12px] text-muted">
                    {disciplines.get(a.discipline) ?? a.discipline} · {levels.get(a.level) ?? a.level} · {dateShort(a.event_date)}
                  </div>
                </div>
                <Badge tone="brand">{a.result_kind === "place" && a.place ? `${a.place} место` : a.result_kind === "rank" ? (a.rank_title ?? "разряд") : a.result_kind === "rating" ? `рейтинг ${a.rating ?? ""}` : "участие"}</Badge>
              </li>
            ))}
          </ul>
        </CardBody>
      </Card>
      <p className="text-[12px] text-muted lg:col-span-2">Грейды: {Object.values(GRADES).join(", ")} — определяются тестом, не достижениями.</p>
    </div>
  );
}
