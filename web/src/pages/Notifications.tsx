import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { api, unwrap } from "@/api/client";
import { PageHeader } from "@/components/layout/AppShell";
import { Button, Card, EmptyState, Skeleton } from "@/components/ui";
import { relative } from "@/lib/format";

export function Notifications() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["notifications", "list"], queryFn: async () => unwrap(await api.GET("/notifications", { params: { query: { limit: 100 } } })) });
  const invalidate = () => qc.invalidateQueries({ queryKey: ["notifications"] });
  const readOne = useMutation({ mutationFn: async (id: string) => unwrap(await api.POST("/notifications/{id}/read", { params: { path: { id } } })), onSuccess: invalidate });
  const readAll = useMutation({ mutationFn: async () => unwrap(await api.POST("/notifications/read-all")), onSuccess: invalidate });

  return (
    <>
      <PageHeader
        title="Уведомления"
        description="Приглашения, отклики и результаты теста. То же самое приходит письмом."
        actions={
          <Button variant="secondary" size="sm" onClick={() => readAll.mutate()} loading={readAll.isPending} disabled={!q.data?.unread_count}>
            Прочитать все
          </Button>
        }
      />
      {q.isLoading && (
        <div className="flex flex-col gap-2">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-16" />
          ))}
        </div>
      )}
      {q.data && (q.data.items ?? []).length === 0 && <EmptyState title="Пока тихо" description="Когда появится приглашение или ответ на отклик, вы увидите это здесь." />}
      <div className="flex flex-col gap-2">
        {(q.data?.items ?? []).map((n) => (
          <Card key={n.id} className={`px-4 py-3 ${n.read_at ? "opacity-75" : "border-l-4 border-l-brand-blue"}`}>
            <div className="flex items-start gap-3">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span className="font-semibold">{n.title}</span>
                  <span className="mono text-[12px] text-muted">{relative(n.created_at)}</span>
                </div>
                {n.body && <p className="mt-0.5 text-[14px] text-muted">{n.body}</p>}
                {n.link && (
                  <Link to={n.link} className="mt-1 inline-block text-[13px]" onClick={() => !n.read_at && readOne.mutate(n.id)}>
                    Открыть →
                  </Link>
                )}
              </div>
              {!n.read_at && (
                <Button size="sm" variant="ghost" onClick={() => readOne.mutate(n.id)}>
                  Прочитано
                </Button>
              )}
            </div>
          </Card>
        ))}
      </div>
    </>
  );
}
