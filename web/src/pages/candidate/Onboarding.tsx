import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Check } from "lucide-react";
import { api, unwrap, errorMessage, isApiError } from "@/api/client";
import { useSession } from "@/api/session";
import { qk, useProfile, useRefdata, useSurvey, useSkillSearch, useGiveConsent } from "@/api/candidate";
import { Button, Card, CardBody, Checkbox, Field, Input, Select, SegmentedControl, Skeleton, useToast } from "@/components/ui";
import { PageHeader } from "@/components/layout/AppShell";
import { GRADES, SPECIALIZATIONS, WORK_FORMATS } from "@/lib/format";

type Step = 1 | 2 | 3 | 4;
const steps: [Step, string][] = [
  [1, "Анкета"],
  [2, "Согласие"],
  [3, "Опрос"],
  [4, "Тест"],
];

/** Онбординг: анкета → согласие на публикацию → опрос с заявленным грейдом → старт адаптивного теста. */
export function Onboarding() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const { consents, reload } = useSession();
  const profile = useProfile();
  const refdata = useRefdata();
  const consent = useGiveConsent();
  const [step, setStep] = useState<Step>(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fields, setFields] = useState<Record<string, string>>({});

  const [form, setForm] = useState({ first_name: "", last_name: "", city: "", work_formats: ["remote"] as string[], salary_expected: "", salary_min: "", primary_specialization: "", headline: "" });
  const [agree, setAgree] = useState(false);
  const [sv, setSv] = useState({ specialization: "backend", claimed_grade: "middle", experience_years: "", industry: "", primary_stack: [] as string[] });
  const survey = useSurvey(sv.specialization);
  const [stackQuery, setStackQuery] = useState("");
  const stack = useSkillSearch(stackQuery, sv.specialization);

  useEffect(() => {
    const p = profile.data;
    if (!p) return;
    setForm((f) => ({
      ...f,
      first_name: p.first_name || f.first_name,
      last_name: p.last_name || f.last_name,
      city: p.city ?? f.city,
      work_formats: p.work_formats?.length ? p.work_formats : f.work_formats,
      salary_expected: p.salary_expected ? String(p.salary_expected) : f.salary_expected,
      salary_min: p.salary_min ? String(p.salary_min) : f.salary_min,
      primary_specialization: p.primary_specialization ?? f.primary_specialization,
      headline: p.headline ?? f.headline,
    }));
    if (p.primary_specialization) setSv((s) => ({ ...s, specialization: p.primary_specialization! }));
  }, [profile.data]);
  useEffect(() => {
    const s = survey.data;
    if (!s) return;
    setSv((v) => ({ ...v, specialization: s.specialization, claimed_grade: s.claimed_grade, experience_years: s.experience_years ? String(s.experience_years) : "", industry: s.industry ?? "", primary_stack: s.primary_stack ?? [] }));
  }, [survey.data]);

  const hasPublication = consents.some((c) => c.kind === "publication" && !c.withdrawn_at);

  async function saveProfile() {
    setBusy(true);
    setError(null);
    setFields({});
    try {
      unwrap(
        await api.PUT("/candidate/profile", {
          body: {
            first_name: form.first_name.trim(),
            last_name: form.last_name.trim(),
            city: form.city.trim() || undefined,
            work_formats: form.work_formats,
            salary_expected: form.salary_expected ? Number(form.salary_expected) : undefined,
            salary_min: form.salary_min ? Number(form.salary_min) : undefined,
            primary_specialization: (form.primary_specialization || undefined) as never,
            headline: form.headline.trim() || undefined,
            visibility: "open",
            relocation: profile.data?.relocation ?? false,
            about: profile.data?.about,
            experience_years: profile.data?.experience_years,
            links: profile.data?.links ?? {},
          },
        }),
      );
      await qc.invalidateQueries({ queryKey: qk.profile });
      setStep(hasPublication ? 3 : 2);
    } catch (e) {
      if (isApiError(e)) setFields(e.fields);
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function giveConsent() {
    setBusy(true);
    setError(null);
    try {
      await consent.mutateAsync("publication");
      await reload();
      setStep(3);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function saveSurvey() {
    setBusy(true);
    setError(null);
    setFields({});
    try {
      unwrap(
        await api.POST("/candidate/survey", {
          body: {
            specialization: sv.specialization as never,
            claimed_grade: sv.claimed_grade as never,
            experience_years: sv.experience_years ? Number(sv.experience_years) : undefined,
            industry: sv.industry.trim() || undefined,
            primary_stack: sv.primary_stack,
          },
        }),
      );
      await qc.invalidateQueries({ queryKey: qk.survey });
      setStep(4);
    } catch (e) {
      if (isApiError(e)) setFields(e.fields);
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function startTest() {
    setBusy(true);
    setError(null);
    try {
      const r = unwrap(await api.POST("/candidate/tests", { body: { specialization: sv.specialization as never, target_grade: sv.claimed_grade as never } }));
      await qc.invalidateQueries({ queryKey: qk.sessions });
      navigate(`/candidate/tests/${r.session.id}`);
    } catch (e) {
      if (isApiError(e) && e.status === 423) toast.push({ tone: "warn", title: "Пока нельзя", description: e.detail });
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  // Тест — только по специализации с собственным банком заданий: если в профиле указана другая,
  // предлагаем первую доступную (в профиле специализация остаётся как есть).
  useEffect(() => {
    const list = refdata.data?.testable_specializations ?? [];
    if (list.length > 0 && !list.includes(sv.specialization)) setSv((s) => ({ ...s, specialization: list[0], primary_stack: [] }));
  }, [refdata.data, sv.specialization]);

  if (profile.isLoading || survey.isLoading) return <Skeleton className="h-64" />;

  const specOptions = (refdata.data?.specializations ?? []).filter((s) => s.code !== "common").map((s) => ({ value: s.code, label: s.name }));
  // Тест — только по специализациям с собственным банком заданий; остальные можно указать в профиле.
  const testable = new Set(refdata.data?.testable_specializations ?? []);
  const testOptions = specOptions.filter((o) => testable.has(o.value));
  const comingSoon = specOptions.filter((o) => !testable.has(o.value)).map((o) => o.label);
  const gradeOptions = (refdata.data?.grades ?? []).map((g) => ({ value: g.code, label: GRADES[g.code] ?? g.name }));

  return (
    <div className="mx-auto max-w-[760px]">
      <PageHeader eyebrow="онбординг" title="Четыре шага до категории" description="Анкета и опрос занимают три минуты, тест — около двадцати. Вернуться к любому шагу можно позже из профиля." />
      <ol className="mb-6 grid grid-cols-4 gap-2">
        {steps.map(([n, label]) => {
          const done = n < step;
          const active = n === step;
          return (
            <li key={n} className={`flex items-center gap-2 rounded-md border px-3 py-2 ${active ? "border-brand-blue bg-brand-blue-soft/40" : done ? "border-ok/40 bg-ok-soft/40" : "border-line-soft bg-surface"}`}>
              <span className={`mono flex size-6 items-center justify-center rounded-full text-[12px] font-bold ${active ? "bg-brand-blue text-white" : done ? "bg-ok text-white" : "bg-paper text-muted"}`}>{done ? <Check className="size-3.5" /> : n}</span>
              <span className="text-[13px] font-medium">{label}</span>
            </li>
          );
        })}
      </ol>

      {error && (
        <p role="alert" className="mb-4 rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">
          {error}
        </p>
      )}

      {step === 1 && (
        <Card>
          <CardBody className="grid gap-4 sm:grid-cols-2">
            <Field label="Имя" required error={fields.first_name}>
              {(id) => <Input id={id} value={form.first_name} onChange={(e) => setForm({ ...form, first_name: e.target.value })} invalid={!!fields.first_name} />}
            </Field>
            <Field label="Фамилия" required error={fields.last_name}>
              {(id) => <Input id={id} value={form.last_name} onChange={(e) => setForm({ ...form, last_name: e.target.value })} invalid={!!fields.last_name} />}
            </Field>
            <Field label="Город" error={fields.city}>
              {(id) => <Input id={id} value={form.city} onChange={(e) => setForm({ ...form, city: e.target.value })} placeholder="Москва" />}
            </Field>
            <Field label="Основная специализация" error={fields.primary_specialization}>
              {(id) => <Select id={id} value={form.primary_specialization} onChange={(e) => setForm({ ...form, primary_specialization: e.target.value })} placeholder="Выберите" options={specOptions} />}
            </Field>
            <Field label="Заголовок профиля" hint="Одной строкой: кто вы и что умеете" className="sm:col-span-2">
              {(id) => <Input id={id} value={form.headline} onChange={(e) => setForm({ ...form, headline: e.target.value })} placeholder="Backend-разработчик на Go, 4 года в финтехе" maxLength={160} />}
            </Field>
            <Field label="Формат работы" className="sm:col-span-2">
              {() => (
                <div className="flex flex-wrap gap-2">
                  {Object.entries(WORK_FORMATS).map(([code, label]) => {
                    const on = form.work_formats.includes(code);
                    return (
                      <button key={code} type="button" aria-pressed={on} className={`badge !px-3 !py-1.5 !text-[13px] ${on ? "badge-ink" : "badge-neutral"}`} onClick={() => setForm({ ...form, work_formats: on ? form.work_formats.filter((f) => f !== code) : [...form.work_formats, code] })}>
                        {label}
                      </button>
                    );
                  })}
                </div>
              )}
            </Field>
            <Field label="Ожидание по зарплате, ₽" hint="Работодатели видят его при вашем разрешении" error={fields.salary_expected}>
              {(id) => <Input id={id} mono inputMode="numeric" value={form.salary_expected} onChange={(e) => setForm({ ...form, salary_expected: e.target.value.replace(/\D/g, "") })} placeholder="250000" />}
            </Field>
            <Field label="Минимум, ₽" hint="Приглашения с вилкой ниже помечаются работодателю" error={fields.salary_min}>
              {(id) => <Input id={id} mono inputMode="numeric" value={form.salary_min} onChange={(e) => setForm({ ...form, salary_min: e.target.value.replace(/\D/g, "") })} placeholder="200000" />}
            </Field>
          </CardBody>
          <div className="flex justify-end border-t border-line-soft px-5 py-3">
            <Button onClick={saveProfile} loading={busy} disabled={!form.first_name.trim() || !form.last_name.trim()}>
              Дальше
            </Button>
          </div>
        </Card>
      )}

      {step === 2 && (
        <Card>
          <CardBody>
            <h3>Показывать профиль работодателям</h3>
            <p className="mt-2 text-[14px] text-muted">
              Без этого согласия вы не попадёте в подборки: работодатели не увидят ни категорию, ни анкету. Контакты в любом случае остаются скрытыми, пока вы не примете приглашение или не откликнетесь сами. Согласие можно отозвать в настройках.
            </p>
            <div className="mt-4">
              <Checkbox checked={agree} onChange={(e) => setAgree(e.target.checked)} label="Даю согласие на показ профиля работодателям (публикацию)" />
            </div>
          </CardBody>
          <div className="flex justify-between border-t border-line-soft px-5 py-3">
            <Button variant="ghost" onClick={() => setStep(1)}>
              Назад
            </Button>
            <Button onClick={giveConsent} loading={busy} disabled={!agree}>
              Дальше
            </Button>
          </div>
        </Card>
      )}

      {step === 3 && (
        <Card>
          <CardBody className="grid gap-4">
            <Field label="Специализация теста" required hint={comingSoon.length > 0 ? `${comingSoon.join(", ")}: банки заданий в разработке, тест по ним появится позже` : undefined}>
              {() => (
                <SegmentedControl
                  value={sv.specialization}
                  onChange={(v) => setSv({ ...sv, specialization: v, primary_stack: [] })}
                  ariaLabel="Специализация"
                  options={testOptions.map((o) => ({ value: o.value, label: o.label }))}
                />
              )}
            </Field>
            <Field label="Заявленный грейд" required hint="Тест начнётся с заданий этого уровня и подстроится под ответы. Грейд не понижается, а подтверждённый выше — предложение повысить.">
              {() => <SegmentedControl value={sv.claimed_grade} onChange={(v) => setSv({ ...sv, claimed_grade: v })} ariaLabel="Грейд" options={gradeOptions.map((o) => ({ value: o.value, label: o.label }))} />}
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Опыт, лет" error={fields.experience_years}>
                {(id) => <Input id={id} mono inputMode="decimal" value={sv.experience_years} onChange={(e) => setSv({ ...sv, experience_years: e.target.value.replace(",", ".") })} placeholder="3" />}
              </Field>
              <Field label="Отрасль" error={fields.industry}>
                {(id) => <Input id={id} value={sv.industry} onChange={(e) => setSv({ ...sv, industry: e.target.value })} placeholder="финтех, e-commerce, госуслуги…" />}
              </Field>
            </div>
            <Field label="Основной стек" hint="Начните вводить название технологии и выберите из справочника">
              {(id) => (
                <div>
                  <Input id={id} value={stackQuery} onChange={(e) => setStackQuery(e.target.value)} placeholder="go, postgresql, kafka…" />
                  {stack.data && stack.data.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {stack.data
                        .filter((s) => !sv.primary_stack.includes(s.code))
                        .slice(0, 10)
                        .map((s) => (
                          <button key={s.code} type="button" className="badge badge-blue !cursor-pointer" onClick={() => { setSv({ ...sv, primary_stack: [...sv.primary_stack, s.code] }); setStackQuery(""); }}>
                            + {s.name}
                          </button>
                        ))}
                    </div>
                  )}
                  {sv.primary_stack.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {sv.primary_stack.map((c) => (
                        <button key={c} type="button" className="badge badge-ink !cursor-pointer" onClick={() => setSv({ ...sv, primary_stack: sv.primary_stack.filter((x) => x !== c) })} title="Убрать">
                          {c} ×
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </Field>
          </CardBody>
          <div className="flex justify-between border-t border-line-soft px-5 py-3">
            <Button variant="ghost" onClick={() => setStep(hasPublication ? 1 : 2)}>
              Назад
            </Button>
            <Button onClick={saveSurvey} loading={busy}>
              Дальше
            </Button>
          </div>
        </Card>
      )}

      {step === 4 && (
        <Card>
          <CardBody>
            <h3>
              Тест: {SPECIALIZATIONS[sv.specialization]} × {GRADES[sv.claimed_grade]}
            </h3>
            <ul className="mt-3 flex flex-col gap-1.5 text-[14px] text-ink-2">
              <li>• 12–20 заданий, сложность подстраивается под ваши ответы; обычно 15–25 минут.</li>
              <li>• На каждое задание — свой таймер (45–180 секунд). Назад вернуться нельзя.</li>
              <li>• Правильность ответов не показывается до конца; разбор с пояснениями — после.</li>
              <li>• Потеря фокуса вкладки и вставка из буфера фиксируются как телеметрия.</li>
              <li>• Повторная попытка в этой специализации — через 90 дней.</li>
            </ul>
          </CardBody>
          <div className="flex justify-between border-t border-line-soft px-5 py-3">
            <Button variant="ghost" onClick={() => setStep(3)}>
              Назад
            </Button>
            <Button onClick={startTest} loading={busy} size="lg">
              Начать тест
            </Button>
          </div>
        </Card>
      )}
    </div>
  );
}
