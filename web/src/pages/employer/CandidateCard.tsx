import { useState } from "react";
import { Link, useParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Award, Lock, Unlock } from "lucide-react";
import { api, unwrap, errorMessage } from "@/api/client";
import { ek, useCandidateCard, useNeeds } from "@/api/employer";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, CardBody, CardHeader, Select, Skeleton, StatusBadge, useToast } from "@/components/ui";
import { InviteDialog, type InvitePayload } from "@/components/employer/InviteDialog";
import { GRADE_STATUS, INVITATION_STATUS, WORK_FORMATS, category, dateShort, money, percent } from "@/lib/format";

/** Карточка кандидата для работодателя: анкета по приватности, контакты только при гранте. */
export function EmployerCandidateCard() {
  const { id = "" } = useParams();
  const card = useCandidateCard(id);
  const needs = useNeeds();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [needId, setNeedId] = useState("");
  const send = useMutation({
    mutationFn: async (p: InvitePayload) => unwrap(await api.POST("/employer/invitations", { body: { candidate_id: id, need_id: needId || undefined, ...p } })),
    onSuccess: () => {
      toast.ok("Приглашение отправлено");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ek.card(id) });
      void qc.invalidateQueries({ queryKey: ["employer", "invitations"] });
    },
    onError: (e) => toast.error("Не отправлено", errorMessage(e)),
  });
  if (card.isLoading) return <Skeleton className="h-64" />;
  if (!card.data) return <p className="text-danger">{errorMessage(card.error)}</p>;
  const c = card.data;
  const canInvite = !c.invitation_status || ["declined", "withdrawn", "expired"].includes(c.invitation_status);
  const need = (needs.data ?? []).find((n) => n.id === needId);
  return (
    <>
      <PageHeader
        eyebrow={<Link to="/employer/candidates" className="no-underline">банк кандидатов →</Link>}
        title={c.display_name}
        description={
          <span className="flex flex-wrap items-center gap-2">
            {c.headline && <span>{c.headline}</span>}
            {c.specialization && c.grade && <span className="mono">{category(c.specialization, c.grade)}</span>}
            {c.grade_status && <StatusBadge status={c.grade_status} label={GRADE_STATUS[c.grade_status] ?? c.grade_status} />}
            {c.percentile !== undefined && <Badge tone="blue">{percent(c.percentile)} процентиль</Badge>}
            {c.invitation_status && <StatusBadge status={c.invitation_status} label={`приглашение: ${INVITATION_STATUS[c.invitation_status] ?? c.invitation_status}`} />}
          </span>
        }
        actions={
          canInvite && (
            <Button onClick={() => setOpen(true)} size="sm">
              Пригласить
            </Button>
          )
        }
      />
      <div className="grid gap-4 lg:grid-cols-[1fr_320px]">
        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader title="Анкета" description="С учётом настроек приватности кандидата" />
            <CardBody className="grid gap-3 text-[14px] sm:grid-cols-2">
              <Info label="Город" value={c.city ?? "скрыт"} />
              <Info label="Формат" value={(c.work_formats ?? []).map((f) => WORK_FORMATS[f] ?? f).join(", ") || "—"} />
              <Info label="Ожидание" value={c.salary_expected ? money(c.salary_expected) : "скрыто"} />
              <Info label="Опыт" value={c.experience_years !== undefined ? `${c.experience_years} лет` : "скрыт"} />
              <Info label="Переезд" value={c.relocation ? "готов" : "нет"} />
              <Info label="Грейд подтверждён" value={c.confirmed_at ? dateShort(c.confirmed_at) : "—"} />
              {c.about && <p className="sm:col-span-2 whitespace-pre-wrap text-ink-2">{c.about}</p>}
            </CardBody>
          </Card>
          <Card>
            <CardHeader title="Навыки" description="✓ — подтверждены тестом" />
            <CardBody className="flex flex-wrap gap-1.5">
              {(c.skills ?? []).length === 0 && <span className="text-[13px] text-muted">Навыки не указаны</span>}
              {(c.skills ?? []).map((s) => (
                <Badge key={s.code} tone={s.source === "test_confirmed" ? "ok" : "neutral"}>
                  {s.name}
                  {s.source === "test_confirmed" ? " ✓" : ""}
                </Badge>
              ))}
            </CardBody>
          </Card>
          {(c.experiences ?? []).length > 0 && (
            <Card>
              <CardHeader title="Опыт" />
              <CardBody className="flex flex-col gap-3">
                {(c.experiences ?? []).map((e, i) => (
                  <div key={i} className="text-[14px]">
                    <div className="font-medium">
                      {e.position} — {e.company_name}
                    </div>
                    <div className="text-[12px] text-muted">
                      {dateShort(e.started_on)} – {e.ended_on ? dateShort(e.ended_on) : "н. в."}
                    </div>
                    {e.description && <p className="mt-1 text-ink-2">{e.description}</p>}
                  </div>
                ))}
              </CardBody>
            </Card>
          )}
        </div>
        <div className="flex flex-col gap-4">
          <Card className={c.contacts_granted ? "border-ok" : ""}>
            <CardHeader title={<span className="inline-flex items-center gap-2">{c.contacts_granted ? <Unlock className="size-4 text-ok" /> : <Lock className="size-4 text-muted" />} Контакты</span>} />
            <CardBody className="text-[14px]">
              {c.contacts ? (
                <dl className="flex flex-col gap-1.5">
                  {c.contacts.phone && <Info label="Телефон" value={c.contacts.phone} mono />}
                  {c.contacts.telegram && <Info label="Telegram" value={c.contacts.telegram.startsWith("@") ? c.contacts.telegram : `@${c.contacts.telegram}`} mono />}
                  {c.contacts.email && <Info label="E-mail" value={c.contacts.email} mono />}
                  {Object.entries(c.contacts.other ?? {}).map(([k, v]) => (
                    <Info key={k} label={k} value={v} mono />
                  ))}
                </dl>
              ) : (
                <p className="text-muted">Откроются, когда кандидат примет ваше приглашение или откликнется на вакансию. Каждый просмотр пишется в аудит.</p>
              )}
            </CardBody>
          </Card>
          <Card>
            <CardHeader title="Достижения ФСП" />
            <CardBody className="text-[14px]">
              {c.fsp_state === "none" && <p className="text-muted">Нет данных ФСП — ранжирование по тесту и профилю.</p>}
              {c.fsp_state === "linked_empty" && <p className="text-muted">ФСП ID привязан, достижений в реестре пока нет.</p>}
              {c.fsp_state === "hidden" && <p className="text-muted">Кандидат скрыл достижения.</p>}
              <ul className="flex flex-col gap-2">
                {(c.achievements ?? []).map((a, i) => (
                  <li key={i} className="flex items-start gap-2">
                    <Award className="mt-0.5 size-4 shrink-0 text-brand-red" />
                    <span>
                      <span className="font-medium">{a.event_name}</span> <span className="text-muted">· {a.year} · {a.level}</span>
                      <Badge tone="brand" className="ml-1">{a.result}</Badge>
                    </span>
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>
        </div>
      </div>
      {open && (
        <div className="hidden">
          <Select value={needId} onChange={(e) => setNeedId(e.target.value)} options={(needs.data ?? []).map((n) => ({ value: n.id, label: n.title }))} placeholder="Без потребности" />
        </div>
      )}
      {open && (
        <InviteDialog
          open
          onClose={() => setOpen(false)}
          onSubmit={(p) => send.mutate(p)}
          busy={send.isPending}
          candidateName={c.display_name}
          defaults={need ? { salary_from: need.salary_from, salary_to: need.salary_to, message: `Здравствуйте! Приглашаем обсудить роль «${need.title}».` } : undefined}
        />
      )}
    </>
  );
}

function Info({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <div className="label-mono">{label}</div>
      <div className={mono ? "mono" : ""}>{value}</div>
    </div>
  );
}
