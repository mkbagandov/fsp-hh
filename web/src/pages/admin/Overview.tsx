import { useState } from "react";
import { useOverview } from "@/api/admin";
import { PageHeader } from "@/components/layout/AppShell";
import { Card, CardBody, CardHeader, SegmentedControl, Skeleton, Stat } from "@/components/ui";
import { CHART_COLORS, DailyBars, Funnel, HBars } from "@/components/charts/Bars";
import { APPLICATION_STATUS, GRADE_STATUS, INVITATION_STATUS, VACANCY_STATUS, category, dateTime } from "@/lib/format";

/** Сводка администратора: воронка, активность по дням, распределения по статусам и категориям. */
export function AdminOverview() {
  const [days, setDays] = useState<"7" | "14" | "30">("14");
  const q = useOverview(Number(days));
  if (q.isLoading) return <Skeleton className="h-64" />;
  if (!q.data) return <p className="text-danger">Не удалось загрузить сводку</p>;
  const d = q.data;
  const f = d.funnel;
  const users = d.users ?? [];
  const byRole = (role: string) => users.filter((u) => u.key.startsWith(role + ":")).reduce((s, u) => s + u.total, 0);
  const blocked = users.filter((u) => u.key.endsWith(":blocked")).reduce((s, u) => s + u.total, 0);
  const sessions = d.sessions ?? [];
  const completed = sessions.find((s) => s.status === "completed");
  const flagged = sessions.reduce((s, x) => s + x.flagged, 0);
  const cats = d.categories ?? [];
  const catTotals = new Map<string, number>();
  for (const c of cats) catTotals.set(category(c.specialization, c.grade), (catTotals.get(category(c.specialization, c.grade)) ?? 0) + c.total);
  const catConfirmed = cats.filter((c) => c.status === "confirmed" || c.status === "upgrade_suggested").reduce((s, c) => s + c.total, 0);
  const daily = d.daily ?? [];
  return (
    <>
      <PageHeader
        eyebrow="администрирование"
        title="Сводка платформы"
        description={`Обновлено ${dateTime(d.generated_at)}. Метрики в формате Prometheus — /metrics.`}
        actions={
          <SegmentedControl
            value={days}
            onChange={setDays}
            ariaLabel="Период"
            options={[
              { value: "7", label: "7 дн" },
              { value: "14", label: "14 дн" },
              { value: "30", label: "30 дн" },
            ]}
          />
        }
      />
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <Stat label="кандидатов" value={byRole("candidate")} hint={`+${f.new_users_7d} за 7 дней (все роли)`} />
        <Stat label="работодателей" value={byRole("employer")} hint={`компаний: ${f.companies}`} />
        <Stat label="тест-сессий" value={sessions.reduce((s, x) => s + x.total, 0)} tone="blue" hint={`завершено ${completed?.total ?? 0}, с флагами ${flagged}`} />
        <Stat label="в категориях" value={cats.reduce((s, c) => s + c.total, 0)} hint={`подтверждённых ${catConfirmed}`} />
        <Stat label="заблокировано" value={blocked} tone={blocked ? "red" : "ink"} hint={`семейств в банке: ${d.families}`} />
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader title="Воронка кандидата" description="Регистрация → опрос → тест → подтверждённая категория → приглашение → принятие" />
          <CardBody>
            <Funnel
              steps={[
                { label: "кандидаты", value: f.candidates },
                { label: "прошли опрос", value: f.surveyed },
                { label: "завершили тест", value: f.tested },
                { label: "подтверждённые", value: f.confirmed },
                { label: "получили приглашение", value: f.invited },
                { label: "приняли", value: f.accepted },
              ]}
            />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Активность по дням" description="Регистрации, тест-сессии, приглашения, отклики" />
          <CardBody>
            <DailyBars
              days={daily.map((x) => x.day)}
              series={[
                { name: "регистрации", values: daily.map((x) => x.registrations), color: CHART_COLORS[0] },
                { name: "тесты", values: daily.map((x) => x.sessions), color: CHART_COLORS[2] },
                { name: "приглашения", values: daily.map((x) => x.invitations), color: CHART_COLORS[1] },
                { name: "отклики", values: daily.map((x) => x.applications), color: CHART_COLORS[4] },
              ]}
            />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Категории" description="Кандидаты по специализации × грейд" />
          <CardBody>
            <HBars items={[...catTotals.entries()].sort((a, b) => b[1] - a[1]).map(([label, value]) => ({ label, value }))} />
            <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-muted">
              {Object.entries(GRADE_STATUS).map(([k, label]) => (
                <span key={k}>
                  {label}: <span className="mono text-ink">{cats.filter((c) => c.status === k).reduce((s, c) => s + c.total, 0)}</span>
                </span>
              ))}
            </div>
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Приглашения и отклики" description={`Приглашения: принято ${(d.invitations ?? []).find((x) => x.key === "accepted")?.total ?? 0} из ${(d.invitations ?? []).reduce((s, x) => s + x.total, 0)}`} />
          <CardBody className="grid gap-4 sm:grid-cols-2">
            <div>
              <div className="label-mono mb-2">приглашения</div>
              <HBars items={(d.invitations ?? []).map((x) => ({ label: INVITATION_STATUS[x.key] ?? x.key, value: x.total }))} color={CHART_COLORS[0]} />
            </div>
            <div>
              <div className="label-mono mb-2">отклики</div>
              <HBars items={(d.applications ?? []).map((x) => ({ label: APPLICATION_STATUS[x.key] ?? x.key, value: x.total }))} color={CHART_COLORS[4]} />
            </div>
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Тест-сессии" />
          <CardBody>
            <table className="table">
              <thead>
                <tr>
                  <th>статус</th>
                  <th>всего</th>
                  <th>ср. заданий</th>
                  <th>ср. время</th>
                  <th>с флагами</th>
                </tr>
              </thead>
              <tbody>
                {sessions.map((s) => (
                  <tr key={s.status}>
                    <td className="mono">{s.status}</td>
                    <td className="mono">{s.total}</td>
                    <td className="mono">{s.avg_items.toFixed(1)}</td>
                    <td className="mono">{s.avg_seconds ? `${Math.round(s.avg_seconds / 60)} мин` : "—"}</td>
                    <td className="mono">{s.flagged}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Вакансии и потребности" />
          <CardBody className="grid gap-4 sm:grid-cols-2">
            <div>
              <div className="label-mono mb-2">вакансии</div>
              <HBars items={(d.vacancies ?? []).map((x) => ({ label: VACANCY_STATUS[x.key] ?? x.key, value: x.total }))} color={CHART_COLORS[2]} />
            </div>
            <div>
              <div className="label-mono mb-2">потребности</div>
              <HBars items={(d.needs ?? []).map((x) => ({ label: x.key, value: x.total }))} color={CHART_COLORS[3]} />
            </div>
          </CardBody>
        </Card>
      </div>
    </>
  );
}
