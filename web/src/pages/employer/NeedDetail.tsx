import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { api, unwrap, errorMessage } from "@/api/client";
import { ek, useEmployerInvitations, useFeedback, useNeed, useNeedMap, useSelection, useVersions, type SelectionFilters, type SelectionItem } from "@/api/employer";
import { useRefdata } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, CardBody, CardHeader, Checkbox, EmptyState, Field, Input, LinkButton, Select, Skeleton, StatusBadge, SalaryRange, Tabs, useToast } from "@/components/ui";
import { CategoryMap } from "@/components/employer/CategoryMap";
import { SelectionTable } from "@/components/employer/SelectionTable";
import { InviteDialog, type InvitePayload } from "@/components/employer/InviteDialog";
import { GRADES, INVITATION_STATUS, NEED_STATUS, WORK_FORMATS, category, dateTime, grade, percent, plural } from "@/lib/format";

type Tab = "map" | "selection" | "versions" | "feedback" | "invitations";

export function NeedDetail() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const tab = (["map", "selection", "versions", "feedback", "invitations"].includes(params.get("tab") ?? "") ? params.get("tab") : "map") as Tab;
  const need = useNeed(id);
  const qc = useQueryClient();
  const toast = useToast();
  const status = useMutation({
    mutationFn: async (s: "draft" | "active" | "archived") => unwrap(await api.POST("/employer/needs/{id}/status", { params: { path: { id } }, body: { status: s } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ek.needs });
      void qc.invalidateQueries({ queryKey: ek.need(id) });
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  if (need.isLoading) return <Skeleton className="h-64" />;
  if (!need.data) return <p className="text-danger">{errorMessage(need.error)}</p>;
  const n = need.data;
  return (
    <>
      <PageHeader
        eyebrow={<Link to="/employer/needs" className="no-underline">потребности →</Link>}
        title={n.title}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <span className="mono">{category(n.specialization, n.grade_min)}{n.grade_min !== n.grade_max ? ` – ${grade(n.grade_max)}` : ""}</span>
            <SalaryRange from={n.salary_from} to={n.salary_to} />
            <StatusBadge status={n.status} label={NEED_STATUS[n.status] ?? n.status} />
            {(n.work_formats ?? []).map((f) => (
              <Badge key={f}>{WORK_FORMATS[f] ?? f}</Badge>
            ))}
            {n.city && <Badge>{n.city}</Badge>}
          </span>
        }
        actions={
          <>
            <LinkButton to={`/employer/needs/${id}/edit`} variant="secondary" size="sm">
              Править
            </LinkButton>
            {n.status === "active" ? (
              <Button variant="ghost" size="sm" onClick={() => status.mutate("archived")} loading={status.isPending}>
                В архив
              </Button>
            ) : (
              <Button variant="ghost" size="sm" onClick={() => status.mutate("active")} loading={status.isPending}>
                Активировать
              </Button>
            )}
          </>
        }
      />
      <Tabs
        value={tab}
        onChange={(t) => setParams({ tab: t })}
        ariaLabel="Разделы потребности"
        items={[
          { value: "map", label: "Карта категорий" },
          { value: "selection", label: "Подборка" },
          { value: "versions", label: "Версии" },
          { value: "invitations", label: "Приглашения" },
          { value: "feedback", label: "Обратная связь" },
        ]}
      />
      <div className="mt-5">
        {tab === "map" && <MapTab id={id} salaryTo={n.salary_to} />}
        {tab === "selection" && <SelectionTab id={id} needTitle={n.title} salary={{ from: n.salary_from, to: n.salary_to }} />}
        {tab === "versions" && <VersionsTab id={id} />}
        {tab === "invitations" && <InvitationsTab id={id} />}
        {tab === "feedback" && <FeedbackTab id={id} />}
      </div>
    </>
  );
}

function MapTab({ id, salaryTo }: { id: string; salaryTo: number }) {
  const map = useNeedMap(id);
  if (map.isLoading) return <Skeleton className="h-48" />;
  if (!map.data) return <p className="text-danger">{errorMessage(map.error)}</p>;
  const total = (map.data.map ?? []).reduce((s, c) => s + c.total, 0);
  return (
    <div className="flex flex-col gap-4">
      <p className="text-[14px] text-muted">
        В специализации сейчас {plural(total, "кандидат", "кандидата", "кандидатов")} с категорией, видимых работодателям. Числа и ожидания по зарплате считаются по живым данным, подсказки — по квартилям ожиданий.
      </p>
      <CategoryMap map={map.data.map ?? []} salaryTo={salaryTo} weights={map.data.weights} />
    </div>
  );
}

function SelectionTab({ id, needTitle, salary }: { id: string; needTitle: string; salary: { from: number; to: number } }) {
  const qc = useQueryClient();
  const toast = useToast();
  const latest = useSelection(id, 0);
  const [filters, setFilters] = useState<SelectionFilters>({ limit: 50 });
  const [skillsText, setSkillsText] = useState("");
  const [invite, setInvite] = useState<SelectionItem | null>(null);
  const build = useMutation({
    mutationFn: async () =>
      unwrap(await api.POST("/employer/needs/{id}/selection", { params: { path: { id } }, body: { ...filters, skills: skillsText.trim() ? skillsText.split(/[\s,]+/).filter(Boolean) : undefined } })),
    onSuccess: (v) => {
      qc.setQueryData(ek.selection(id, 0), v);
      void qc.invalidateQueries({ queryKey: ek.versions(id) });
      toast.ok(`Версия ${v.version_no} построена`, `${plural(v.items_count, "кандидат", "кандидата", "кандидатов")}${v.diff ? `, новых ${v.diff.added?.length ?? 0}, выбыло ${v.diff.removed?.length ?? 0}` : ""}`);
    },
    onError: (e) => toast.error("Не удалось построить", errorMessage(e)),
  });
  const send = useMutation({
    mutationFn: async (p: InvitePayload) => unwrap(await api.POST("/employer/invitations", { body: { candidate_id: invite!.candidate.id, need_id: id, ...p } })),
    onSuccess: () => {
      toast.ok("Приглашение отправлено", "Кандидат получит уведомление и письмо");
      setInvite(null);
      void qc.invalidateQueries({ queryKey: ["employer"] });
      build.mutate();
    },
    onError: (e) => toast.error("Не отправлено", errorMessage(e)),
  });
  const v = latest.data;
  const gradeOpts = Object.entries(GRADES);
  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader title="Фильтры подборки" description="Каждое построение сохраняется как версия: можно сравнить, кто добавился и выбыл" />
        <CardBody className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Field label="Грейды">
            {() => (
              <div className="flex gap-1.5">
                {gradeOpts.map(([code, label]) => {
                  const on = (filters.grades ?? []).includes(code);
                  return (
                    <button key={code} type="button" aria-pressed={on} className={`badge !px-2.5 !py-1 ${on ? "badge-ink" : "badge-neutral"}`} onClick={() => setFilters({ ...filters, grades: on ? (filters.grades ?? []).filter((g) => g !== code) : [...(filters.grades ?? []), code] })}>
                      {label}
                    </button>
                  );
                })}
              </div>
            )}
          </Field>
          <Field label="Статус грейда">
            {(fid) => (
              <Select
                id={fid}
                value={(filters.grade_statuses ?? [])[0] ?? ""}
                onChange={(e) => setFilters({ ...filters, grade_statuses: e.target.value ? [e.target.value] : undefined })}
                placeholder="Любой"
                options={[
                  { value: "confirmed", label: "подтверждён" },
                  { value: "upgrade_suggested", label: "выше заявленного" },
                  { value: "not_confirmed", label: "не подтверждён" },
                ]}
              />
            )}
          </Field>
          <Field label="Навыки (коды через запятую)">{(fid) => <Input id={fid} mono value={skillsText} onChange={(e) => setSkillsText(e.target.value)} placeholder="go, kafka" />}</Field>
          <Field label="Город">{(fid) => <Input id={fid} value={filters.city ?? ""} onChange={(e) => setFilters({ ...filters, city: e.target.value || undefined })} />}</Field>
          <div className="flex flex-wrap items-center gap-4 sm:col-span-2 lg:col-span-3">
            <Checkbox checked={!!filters.has_fsp} onChange={(e) => setFilters({ ...filters, has_fsp: e.target.checked ? true : undefined })} label="только с достижениями ФСП" />
            <Checkbox checked={!!filters.hide_invited} onChange={(e) => setFilters({ ...filters, hide_invited: e.target.checked || undefined })} label="скрыть уже приглашённых" />
            <Field label="Лимит" className="w-24">{(fid) => <Input id={fid} mono inputMode="numeric" value={String(filters.limit ?? 50)} onChange={(e) => setFilters({ ...filters, limit: Number(e.target.value.replace(/\D/g, "")) || 50 })} />}</Field>
          </div>
          <div className="flex items-end justify-end">
            <Button onClick={() => build.mutate()} loading={build.isPending} icon={<RefreshCw className="size-4" />}>
              {v ? "Построить новую версию" : "Построить подборку"}
            </Button>
          </div>
        </CardBody>
      </Card>
      {latest.isLoading && <Skeleton className="h-48" />}
      {!latest.isLoading && !v && <EmptyState title="Подборка ещё не строилась" description="Нажмите «Построить подборку» — ранжирование займёт меньше секунды и сохранится как версия 1." />}
      {v && (
        <>
          <div className="flex flex-wrap items-center gap-3 text-[13px] text-muted">
            <span className="mono text-ink">версия {v.version_no}</span>
            <span>{dateTime(v.created_at)}</span>
            <span>{plural(v.items_count, "кандидат", "кандидата", "кандидатов")}</span>
            {v.diff && (
              <span>
                новых {v.diff.added?.length ?? 0} · выбыло {v.diff.removed?.length ?? 0} · сменили позицию {v.diff.moved}
              </span>
            )}
          </div>
          {(v.items ?? []).length === 0 ? (
            <EmptyState title="Под эти фильтры никто не подошёл" description="Расширьте грейды или снимите фильтр по навыкам. Карта категорий подскажет, где есть люди." />
          ) : (
            <SelectionTable items={v.items ?? []} onInvite={setInvite} />
          )}
        </>
      )}
      {invite && (
        <InviteDialog
          open
          onClose={() => setInvite(null)}
          onSubmit={(p) => send.mutate(p)}
          busy={send.isPending}
          candidateName={invite.candidate.display_name}
          defaults={{ salary_from: salary.from, salary_to: salary.to, message: `Здравствуйте! Приглашаем обсудить роль «${needTitle}».` }}
        />
      )}
    </div>
  );
}

function VersionsTab({ id }: { id: string }) {
  const versions = useVersions(id);
  const [no, setNo] = useState(0);
  const chosen = useSelection(id, no);
  if (versions.isLoading) return <Skeleton className="h-32" />;
  const list = versions.data ?? [];
  if (list.length === 0) return <EmptyState title="Версий пока нет" description="Версии появляются после построения подборки." />;
  return (
    <div className="grid gap-4 lg:grid-cols-[280px_1fr]">
      <Card>
        <CardHeader title="История" />
        <CardBody className="!p-0">
          {list.map((vs) => (
            <button key={vs.version_no} type="button" onClick={() => setNo(vs.version_no)} className={`flex w-full items-center justify-between border-b border-line-soft px-4 py-2.5 text-left last:border-0 hover:bg-paper/60 ${no === vs.version_no ? "bg-brand-blue-soft/40" : ""}`}>
              <span>
                <span className="mono font-semibold">v{vs.version_no}</span>
                <span className="ml-2 text-[12px] text-muted">{dateTime(vs.created_at)}</span>
              </span>
              <span className="mono text-[12px] text-muted">{vs.items_count}</span>
            </button>
          ))}
        </CardBody>
      </Card>
      <div>
        {chosen.isLoading && <Skeleton className="h-40" />}
        {chosen.data && (
          <>
            <div className="mb-3 flex flex-wrap items-center gap-3 text-[13px] text-muted">
              <span className="mono text-ink">версия {chosen.data.version_no}</span>
              <span>фильтры: {describeFilters(chosen.data.filters)}</span>
              {chosen.data.diff && (
                <span>
                  vs предыдущая: +{chosen.data.diff.added?.length ?? 0} / −{chosen.data.diff.removed?.length ?? 0} / ↕{chosen.data.diff.moved}
                </span>
              )}
            </div>
            <SelectionTable items={chosen.data.items ?? []} />
          </>
        )}
      </div>
    </div>
  );
}

function describeFilters(f: SelectionFilters): string {
  const parts: string[] = [];
  if (f.grades?.length) parts.push(`грейды ${f.grades.map((g) => GRADES[g] ?? g).join("/")}`);
  if (f.grade_statuses?.length) parts.push(`статус ${f.grade_statuses.join("/")}`);
  if (f.skills?.length) parts.push(`навыки ${f.skills.join(", ")}`);
  if (f.city) parts.push(`город ${f.city}`);
  if (f.has_fsp) parts.push("с ФСП");
  if (f.hide_invited) parts.push("без приглашённых");
  return parts.length ? parts.join("; ") : "без фильтров";
}

function InvitationsTab({ id }: { id: string }) {
  const inv = useEmployerInvitations(id);
  const qc = useQueryClient();
  const toast = useToast();
  const withdraw = useMutation({
    mutationFn: async (invId: string) => unwrap(await api.POST("/employer/invitations/{id}/withdraw", { params: { path: { id: invId } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["employer", "invitations"] });
      toast.ok("Приглашение отозвано");
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  if (inv.isLoading) return <Skeleton className="h-32" />;
  const items = inv.data ?? [];
  if (items.length === 0) return <EmptyState title="По этой потребности приглашений нет" description="Постройте подборку и пригласите кандидатов — каждое приглашение с вилкой." />;
  return (
    <div className="flex flex-col gap-2">
      {items.map((i) => (
        <Card key={i.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
          <Link to={`/employer/candidates/${i.candidate_id}`} className="font-medium">
            кандидат
          </Link>
          <SalaryRange from={i.salary_from} to={i.salary_to} className="text-[13px]" />
          <span className="text-[12px] text-muted">{dateTime(i.sent_at)}</span>
          {i.below_candidate_min && <Badge tone="warn">ниже минимума кандидата</Badge>}
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
  );
}

function FeedbackTab({ id }: { id: string }) {
  const fb = useFeedback(id);
  const refdata = useRefdata();
  if (fb.isLoading) return <Skeleton className="h-32" />;
  if (!fb.data) return null;
  const d = fb.data;
  const reasons = new Map((refdata.data?.decline_reasons ?? []).map((r) => [r.code, r.name]));
  if (d.total === 0) return <EmptyState title="Обратной связи пока нет" description="Она появится после первых ответов кандидатов на приглашения." />;
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card>
        <CardHeader title="Ответы" />
        <CardBody>
          <div className="num-display text-brand-blue">{percent(d.acceptance_rate)}</div>
          <div className="text-[12px] text-muted">доля принятых среди ответивших</div>
          <dl className="mt-3 grid grid-cols-2 gap-y-1 text-[13px]">
            {Object.entries(d.by_status).map(([k, v]) => (
              <FragmentPair key={k} k={INVITATION_STATUS[k] ?? k} v={v} />
            ))}
          </dl>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Причины отказов" />
        <CardBody>
          {Object.keys(d.decline_reasons).length === 0 && <p className="text-[13px] text-muted">Отказов не было.</p>}
          <dl className="grid grid-cols-[1fr_auto] gap-y-1 text-[13px]">
            {Object.entries(d.decline_reasons).map(([k, v]) => (
              <FragmentPair key={k} k={reasons.get(k) ?? k} v={v} />
            ))}
          </dl>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Что поправить" />
        <CardBody>
          {(d.hints ?? []).length === 0 ? (
            <p className="text-[13px] text-muted">Пока подсказок нет — данных мало.</p>
          ) : (
            <ul className="flex flex-col gap-2 text-[13px]">
              {(d.hints ?? []).map((h) => (
                <li key={h} className="rounded-sm bg-warn-soft px-3 py-2 text-warn">
                  {h}
                </li>
              ))}
            </ul>
          )}
        </CardBody>
      </Card>
    </div>
  );
}

function FragmentPair({ k, v }: { k: string; v: number }) {
  return (
    <>
      <dt className="text-muted">{k}</dt>
      <dd className="mono text-right">{v}</dd>
    </>
  );
}
