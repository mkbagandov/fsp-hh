import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { api, unwrap, errorMessage } from "@/api/client";
import { useCandidateSearch, type SearchParams, type SelectionItem } from "@/api/employer";
import { useRefdata } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Button, Card, CardBody, Checkbox, EmptyState, Field, Input, Select, Skeleton, useToast } from "@/components/ui";
import { SelectionTable } from "@/components/employer/SelectionTable";
import { InviteDialog, type InvitePayload } from "@/components/employer/InviteDialog";
import { GRADES } from "@/lib/format";

/** Поиск по банку: ранжирование без потребности (условия нейтральны), приглашение в один клик. */
export function CandidateSearch() {
  const refdata = useRefdata();
  const qc = useQueryClient();
  const toast = useToast();
  const [form, setForm] = useState<SearchParams>({ specialization: "backend", grades: [], skills: [], city: "", has_fsp: "", grade_statuses: [], limit: 50 });
  const [skillsText, setSkillsText] = useState("");
  const [applied, setApplied] = useState<SearchParams | null>(null);
  const [invite, setInvite] = useState<SelectionItem | null>(null);
  const results = useCandidateSearch(applied ?? form, !!applied);
  const send = useMutation({
    mutationFn: async (p: InvitePayload) => unwrap(await api.POST("/employer/invitations", { body: { candidate_id: invite!.candidate.id, ...p } })),
    onSuccess: () => {
      toast.ok("Приглашение отправлено");
      setInvite(null);
      void qc.invalidateQueries({ queryKey: ["employer"] });
    },
    onError: (e) => toast.error("Не отправлено", errorMessage(e)),
  });
  const specOptions = (refdata.data?.specializations ?? []).filter((s) => s.code !== "common").map((s) => ({ value: s.code, label: s.name }));
  return (
    <>
      <PageHeader eyebrow="работодатель" title="Банк кандидатов" description="Только видимые кандидаты с согласием на публикацию. Ранжирование по тесту, навыкам, достижениям ФСП и свежести; условия (вилка) здесь не учитываются." />
      <Card>
        <CardBody className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Field label="Специализация" required>{(id) => <Select id={id} value={form.specialization} onChange={(e) => setForm({ ...form, specialization: e.target.value })} options={specOptions} />}</Field>
          <Field label="Грейды">
            {() => (
              <div className="flex gap-1.5">
                {Object.entries(GRADES).map(([code, label]) => {
                  const on = (form.grades ?? []).includes(code);
                  return (
                    <button key={code} type="button" aria-pressed={on} className={`badge !px-2.5 !py-1 ${on ? "badge-ink" : "badge-neutral"}`} onClick={() => setForm({ ...form, grades: on ? (form.grades ?? []).filter((g) => g !== code) : [...(form.grades ?? []), code] })}>
                      {label}
                    </button>
                  );
                })}
              </div>
            )}
          </Field>
          <Field label="Навыки (коды через запятую)">{(id) => <Input id={id} mono value={skillsText} onChange={(e) => setSkillsText(e.target.value)} placeholder="go, postgresql" />}</Field>
          <Field label="Город">{(id) => <Input id={id} value={form.city ?? ""} onChange={(e) => setForm({ ...form, city: e.target.value })} />}</Field>
          <div className="flex flex-wrap items-center gap-4 sm:col-span-2 lg:col-span-3">
            <Checkbox checked={form.has_fsp === "true"} onChange={(e) => setForm({ ...form, has_fsp: e.target.checked ? "true" : "" })} label="только с достижениями ФСП" />
            <Checkbox checked={(form.grade_statuses ?? []).includes("confirmed")} onChange={(e) => setForm({ ...form, grade_statuses: e.target.checked ? ["confirmed"] : [] })} label="только подтверждённые" />
          </div>
          <div className="flex items-end justify-end">
            <Button icon={<Search className="size-4" />} onClick={() => setApplied({ ...form, skills: skillsText.split(/[\s,]+/).filter(Boolean) })}>
              Найти
            </Button>
          </div>
        </CardBody>
      </Card>
      <div className="mt-4">
        {results.isLoading && <Skeleton className="h-48" />}
        {results.data && results.data.length === 0 && <EmptyState title="Никого не нашли" description="Снимите часть фильтров или посмотрите другую специализацию." />}
        {results.data && results.data.length > 0 && <SelectionTable items={results.data} onInvite={setInvite} />}
      </div>
      {invite && <InviteDialog open onClose={() => setInvite(null)} onSubmit={(p) => send.mutate(p)} busy={send.isPending} candidateName={invite.candidate.display_name} />}
    </>
  );
}
