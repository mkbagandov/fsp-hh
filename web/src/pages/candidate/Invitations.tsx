import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, Clock } from "lucide-react";
import { api, unwrap, errorMessage } from "@/api/client";
import { qk, useInvitations, useRefdata, type Invitation } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Button, Card, Dialog, EmptyState, Field, Select, Skeleton, StatusBadge, SalaryRange, Tabs, Textarea, useToast } from "@/components/ui";
import { INVITATION_STATUS, dateShort, relative } from "@/lib/format";

type Filter = "active" | "all";

/** Входящие приглашения: вилка видна сразу, способ связи — после принятия; отказ — с причиной. */
export function CandidateInvitations() {
  const [params, setParams] = useSearchParams();
  const [filter, setFilter] = useState<Filter>("active");
  const list = useInvitations();
  const openId = params.get("open");
  const items = (list.data ?? []).filter((i) => filter === "all" || i.status === "sent" || i.status === "viewed" || i.status === "accepted");

  return (
    <>
      <PageHeader eyebrow="кандидат" title="Приглашения" description="Каждое приглашение содержит зарплатную вилку. Приняв его, вы откроете компании контакты и получите её способ связи." />
      <Tabs
        value={filter}
        onChange={setFilter}
        ariaLabel="Фильтр приглашений"
        items={[
          { value: "active", label: "Актуальные", count: (list.data ?? []).filter((i) => i.status === "sent" || i.status === "viewed" || i.status === "accepted").length },
          { value: "all", label: "Все", count: list.data?.length },
        ]}
      />
      <div className="mt-4 flex flex-col gap-3">
        {list.isLoading && [1, 2].map((i) => <Skeleton key={i} className="h-24" />)}
        {list.data && items.length === 0 && <EmptyState title="Приглашений нет" description="Они появятся, когда работодатель выберет вас из карты категорий или ответит на отклик." />}
        {items.map((inv) => (
          <button key={inv.id} type="button" className="text-left" onClick={() => setParams({ open: inv.id })}>
            <Card className={`p-5 transition-colors hover:border-brand-blue ${inv.status === "sent" ? "border-l-4 border-l-brand-blue" : ""}`}>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-[15px] font-semibold">
                    <Building2 className="size-4 text-muted" /> {inv.company_name ?? "Компания"}
                  </div>
                  <p className="mt-1 line-clamp-2 text-[14px] text-ink-2">{inv.message}</p>
                  <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[12px] text-muted">
                    <span>{relative(inv.sent_at)}</span>
                    {(inv.status === "sent" || inv.status === "viewed") && (
                      <span className="inline-flex items-center gap-1">
                        <Clock className="size-3.5" /> до {dateShort(inv.expires_at)}
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex flex-col items-end gap-2">
                  <SalaryRange from={inv.salary_from} to={inv.salary_to} className="text-[17px]" />
                  <StatusBadge status={inv.status} label={INVITATION_STATUS[inv.status] ?? inv.status} />
                </div>
              </div>
            </Card>
          </button>
        ))}
      </div>
      {openId && <InvitationDialog id={openId} onClose={() => setParams({})} />}
    </>
  );
}

function InvitationDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const refdata = useRefdata();
  const [mode, setMode] = useState<"view" | "decline">("view");
  const [reason, setReason] = useState("");
  const [comment, setComment] = useState("");
  const inv = useQuery({ queryKey: qk.invitation(id), queryFn: async () => unwrap(await api.GET("/candidate/invitations/{id}", { params: { path: { id } } })) });
  const [accepted, setAccepted] = useState<{ contact: Invitation["contact_method"]; company: Record<string, string> } | null>(null);
  useEffect(() => {
    void qc.invalidateQueries({ queryKey: qk.invitations });
  }, [inv.data?.status, qc]);

  const accept = useMutation({
    mutationFn: async () => unwrap(await api.POST("/candidate/invitations/{id}/accept", { params: { path: { id } } })),
    onSuccess: (r) => {
      setAccepted({ contact: r.contact_method, company: r.company_contacts });
      void qc.invalidateQueries({ queryKey: ["candidate"] });
      toast.ok("Приглашение принято", "Компания получила ваши контакты");
    },
    onError: (e) => toast.error("Не удалось принять", errorMessage(e)),
  });
  const decline = useMutation({
    mutationFn: async () => unwrap(await api.POST("/candidate/invitations/{id}/decline", { params: { path: { id } }, body: { reason, comment: comment.trim() || undefined } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["candidate"] });
      toast.ok("Приглашение отклонено", "Причина передана работодателю как обратная связь");
      onClose();
    },
    onError: (e) => toast.error("Не удалось отклонить", errorMessage(e)),
  });

  const d = inv.data;
  const active = d && (d.status === "sent" || d.status === "viewed");
  return (
    <Dialog open onClose={onClose} title={d?.company_name ?? "Приглашение"} width="max-w-xl">
      {inv.isLoading && <Skeleton className="h-32" />}
      {d && (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <StatusBadge status={d.status} label={INVITATION_STATUS[d.status] ?? d.status} />
            <SalaryRange from={d.salary_from} to={d.salary_to} className="text-[20px]" />
          </div>
          <p className="whitespace-pre-wrap text-[15px]">{d.message}</p>
          <div className="text-[12px] text-muted">
            Отправлено {dateShort(d.sent_at)} · действует до {dateShort(d.expires_at)}
          </div>
          {(d.status === "accepted" || accepted) && (
            <div className="rounded-md border border-ok/40 bg-ok-soft/50 px-4 py-3 text-[14px]">
              <div className="label-mono !text-ok">способ связи компании</div>
              {(accepted?.contact ?? d.contact_method) ? (
                <div className="mono mt-1 text-[15px]">
                  {(accepted?.contact ?? d.contact_method)!.kind}: {(accepted?.contact ?? d.contact_method)!.value}
                </div>
              ) : (
                <div className="mt-1 text-muted">Способ связи уже был показан при принятии.</div>
              )}
              {accepted && Object.keys(accepted.company).length > 0 && (
                <div className="mt-2 text-[13px] text-muted">
                  {Object.entries(accepted.company).map(([k, v]) => (
                    <div key={k}>
                      {k}: {v}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
          {d.status === "declined" && d.decline_reason && (
            <p className="text-[13px] text-muted">
              Вы отклонили: {(refdata.data?.decline_reasons ?? []).find((r) => r.code === d.decline_reason)?.name ?? d.decline_reason}
              {d.decline_comment ? ` — ${d.decline_comment}` : ""}
            </p>
          )}
          {active && !accepted && mode === "view" && (
            <div className="flex flex-wrap justify-end gap-2">
              <Button variant="ghost" onClick={() => setMode("decline")}>
                Отклонить
              </Button>
              <Button onClick={() => accept.mutate()} loading={accept.isPending}>
                Принять и открыть контакты
              </Button>
            </div>
          )}
          {active && !accepted && mode === "decline" && (
            <div className="flex flex-col gap-3 rounded-md border border-line-soft bg-paper p-4">
              <Field label="Причина" required hint="Работодатель увидит обобщённую статистику причин, это помогает ему исправить условия">
                {(id2) => <Select id={id2} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Выберите причину" options={(refdata.data?.decline_reasons ?? []).map((r) => ({ value: r.code, label: r.name }))} />}
              </Field>
              <Field label="Комментарий" hint="Причину и комментарий увидит работодатель">{(id2) => <Textarea id={id2} value={comment} onChange={(e) => setComment(e.target.value)} rows={2} maxLength={1000} placeholder="Необязательно" />}</Field>
              <div className="flex justify-end gap-2">
                <Button variant="ghost" onClick={() => setMode("view")}>
                  Назад
                </Button>
                <Button variant="danger" onClick={() => decline.mutate()} loading={decline.isPending} disabled={!reason}>
                  Отклонить
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
    </Dialog>
  );
}
