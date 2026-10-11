import { useState } from "react";
import { useAudit } from "@/api/admin";
import { PageHeader } from "@/components/layout/AppShell";
import { Button, Card, CardBody, EmptyState, Field, Input, Select, Skeleton } from "@/components/ui";
import { dateTime } from "@/lib/format";

const PREFIXES = [
  { value: "", label: "Все действия" },
  { value: "invitation.", label: "Приглашения" },
  { value: "application.", label: "Отклики" },
  { value: "contacts.", label: "Просмотры контактов" },
  { value: "company.", label: "Компании" },
  { value: "admin.", label: "Действия админов" },
  { value: "user.", label: "Регистрация и вход" },
  { value: "consent.", label: "Согласия" },
  { value: "privacy.", label: "Приватность" },
];

export function AdminAudit() {
  const [actor, setActor] = useState("");
  const [p, setP] = useState({ action: "", actor: "", offset: 0 });
  const q = useAudit(p);
  return (
    <>
      <PageHeader eyebrow="администрирование" title="Аудит" description="Кто, что и когда: раскрытие контактов, приглашения, отклики, согласия, действия администраторов." />
      <Card>
        <CardBody className="grid gap-3 sm:grid-cols-[220px_1fr_auto]">
          <Field label="Действие">{(id) => <Select id={id} value={p.action} onChange={(e) => setP({ ...p, action: e.target.value, offset: 0 })} options={PREFIXES} />}</Field>
          <Field label="E-mail актора содержит">{(id) => <Input id={id} value={actor} onChange={(e) => setActor(e.target.value)} onKeyDown={(e) => e.key === "Enter" && setP({ ...p, actor: actor.trim(), offset: 0 })} />}</Field>
          <div className="flex items-end">
            <Button variant="secondary" onClick={() => setP({ ...p, actor: actor.trim(), offset: 0 })}>
              Показать
            </Button>
          </div>
        </CardBody>
      </Card>
      <div className="mt-4">
        {q.isLoading && <Skeleton className="h-48" />}
        {q.data && (q.data.items ?? []).length === 0 && <EmptyState title="Записей нет" pattern={false} />}
        {q.data && (q.data.items ?? []).length > 0 && (
          <Card className="overflow-x-auto">
            <table className="table min-w-[760px]">
              <thead>
                <tr>
                  <th style={{ width: 150 }}>Когда</th>
                  <th>Кто</th>
                  <th>Действие</th>
                  <th>Объект</th>
                  <th>Детали</th>
                </tr>
              </thead>
              <tbody>
                {(q.data.items ?? []).map((a) => (
                  <tr key={a.id}>
                    <td className="mono text-[12px] text-muted">{dateTime(a.created_at)}</td>
                    <td className="mono text-[12.5px]">{a.actor_email ?? (a.actor_id ? a.actor_id.slice(0, 8) : "система")}</td>
                    <td className="mono text-[12.5px] font-semibold">{a.action}</td>
                    <td className="mono text-[12px] text-muted">
                      {a.object_type}
                      {a.object_id ? ` ${a.object_id.slice(0, 8)}` : ""}
                    </td>
                    <td className="mono max-w-[320px] truncate text-[11.5px] text-muted" title={JSON.stringify(a.details)}>
                      {JSON.stringify(a.details)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>
        )}
        {q.data && q.data.total > 50 && (
          <div className="mt-3 flex gap-2">
            <Button size="sm" variant="ghost" disabled={p.offset === 0} onClick={() => setP({ ...p, offset: Math.max(0, p.offset - 50) })}>
              ← Назад
            </Button>
            <Button size="sm" variant="ghost" disabled={p.offset + 50 >= q.data.total} onClick={() => setP({ ...p, offset: p.offset + 50 })}>
              Дальше →
            </Button>
          </div>
        )}
      </div>
    </>
  );
}
