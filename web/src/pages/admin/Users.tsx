import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, errorMessage } from "@/api/client";
import { useAdminUsers } from "@/api/admin";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, CardBody, DataTable, Field, Input, Select, Skeleton, StatusBadge, useToast } from "@/components/ui";
import { dateTime, relative } from "@/lib/format";

const ROLE: Record<string, string> = { candidate: "кандидат", employer: "работодатель", admin: "админ" };
const STATUS: Record<string, string> = { active: "активен", blocked: "заблокирован", deleted: "удалён" };

export function AdminUsers() {
  const [q, setQ] = useState("");
  const [applied, setApplied] = useState({ q: "", role: "", status: "", offset: 0 });
  const list = useAdminUsers(applied);
  const qc = useQueryClient();
  const toast = useToast();
  const setStatus = useMutation({
    mutationFn: async (p: { id: string; status: "active" | "blocked" }) => unwrap(await api.POST("/admin/users/{id}/status", { params: { path: { id: p.id } }, body: { status: p.status } })),
    onSuccess: (u) => {
      void qc.invalidateQueries({ queryKey: ["admin", "users"] });
      toast.ok(u.status === "blocked" ? "Пользователь заблокирован" : "Пользователь разблокирован", u.email);
    },
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  return (
    <>
      <PageHeader eyebrow="администрирование" title="Пользователи" description="Блокировка закрывает вход и все операции; данные сохраняются. Администраторов меняют только в базе." />
      <Card>
        <CardBody className="grid gap-3 sm:grid-cols-[1fr_160px_160px_auto]">
          <Field label="E-mail содержит">{(id) => <Input id={id} value={q} onChange={(e) => setQ(e.target.value)} placeholder="@test.local" onKeyDown={(e) => e.key === "Enter" && setApplied({ ...applied, q: q.trim(), offset: 0 })} />}</Field>
          <Field label="Роль">{(id) => <Select id={id} value={applied.role} onChange={(e) => setApplied({ ...applied, role: e.target.value, offset: 0 })} placeholder="Любая" options={Object.entries(ROLE).map(([value, label]) => ({ value, label }))} />}</Field>
          <Field label="Статус">{(id) => <Select id={id} value={applied.status} onChange={(e) => setApplied({ ...applied, status: e.target.value, offset: 0 })} placeholder="Любой" options={Object.entries(STATUS).map(([value, label]) => ({ value, label }))} />}</Field>
          <div className="flex items-end">
            <Button variant="secondary" onClick={() => setApplied({ ...applied, q: q.trim(), offset: 0 })}>
              Найти
            </Button>
          </div>
        </CardBody>
      </Card>
      <div className="mt-4">
        {list.isLoading && <Skeleton className="h-48" />}
        {list.data && (
          <>
            <div className="mb-2 text-[13px] text-muted">всего: {list.data.total}</div>
            <DataTable
              rows={list.data.items ?? []}
              rowKey={(u) => u.id}
              columns={[
                { key: "email", header: "E-mail", render: (u) => <span className="mono text-[13px]">{u.email}</span> },
                { key: "role", header: "Роль", render: (u) => <Badge tone={u.role === "admin" ? "brand" : u.role === "employer" ? "blue" : "neutral"}>{ROLE[u.role] ?? u.role}</Badge> },
                { key: "status", header: "Статус", render: (u) => <StatusBadge status={u.status === "blocked" ? "declined" : u.status === "deleted" ? "withdrawn" : "accepted"} label={STATUS[u.status] ?? u.status} /> },
                { key: "verified", header: "E-mail", render: (u) => <span className="text-[12px] text-muted">{u.email_verified ? "подтверждён" : "не подтверждён"}</span> },
                { key: "created", header: "Создан", render: (u) => <span className="text-[12px] text-muted" title={dateTime(u.created_at)}>{relative(u.created_at)}</span> },
                { key: "login", header: "Вход", render: (u) => <span className="text-[12px] text-muted">{u.last_login_at ? relative(u.last_login_at) : "—"}</span> },
                {
                  key: "actions",
                  header: "",
                  render: (u) =>
                    u.role !== "admin" && u.status !== "deleted" ? (
                      <Button size="sm" variant={u.status === "blocked" ? "secondary" : "ghost"} onClick={() => setStatus.mutate({ id: u.id, status: u.status === "blocked" ? "active" : "blocked" })} loading={setStatus.isPending}>
                        {u.status === "blocked" ? "Разблокировать" : "Заблокировать"}
                      </Button>
                    ) : null,
                },
              ]}
            />
            {list.data.total > 50 && (
              <div className="mt-3 flex gap-2">
                <Button size="sm" variant="ghost" disabled={applied.offset === 0} onClick={() => setApplied({ ...applied, offset: Math.max(0, applied.offset - 50) })}>
                  ← Назад
                </Button>
                <Button size="sm" variant="ghost" disabled={applied.offset + 50 >= list.data.total} onClick={() => setApplied({ ...applied, offset: applied.offset + 50 })}>
                  Дальше →
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </>
  );
}
