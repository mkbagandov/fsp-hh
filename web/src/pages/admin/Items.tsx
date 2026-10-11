import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { api, unwrap, errorMessage } from "@/api/client";
import { ak, useItemStats } from "@/api/admin";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Button, Card, CardBody, Select, Skeleton, Stat, useToast } from "@/components/ui";
import { SPECIALIZATIONS, grade, percent, spec } from "@/lib/format";

/** Банк заданий: экспозиция, доля верных, время, дискриминативность, предупреждения; включение/выключение семейств. */
export function AdminItems() {
  const q = useItemStats();
  const qc = useQueryClient();
  const toast = useToast();
  const [specFilter, setSpec] = useState("");
  const toggle = useMutation({
    mutationFn: async (p: { id: string; enabled: boolean }) => unwrap(await api.POST("/admin/items/{id}/enabled", { params: { path: { id: p.id } }, body: { enabled: p.enabled } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ak.items }),
    onError: (e) => toast.error("Не удалось", errorMessage(e)),
  });
  const recalibrate = useMutation({
    mutationFn: async () => unwrap(await api.POST("/admin/items/recalibrate")),
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: ak.items });
      toast.ok("Калибровка пересчитана", `Сложность обновлена у ${r.updated} из ${r.families} семейств с достаточной статистикой · ответов: ${r.answers}`);
    },
    onError: (e) => toast.error("Не удалось пересчитать", errorMessage(e)),
  });
  if (q.isLoading) return <Skeleton className="h-64" />;
  const all = q.data ?? [];
  const items = all.filter((f) => !specFilter || f.specialization === specFilter);
  const warnings = all.filter((f) => f.warning).length;
  const served = all.reduce((s, f) => s + f.served, 0);
  return (
    <>
      <PageHeader
        eyebrow="администрирование"
        title="Банк заданий"
        description="Параметры IRT, экспозиция и качество по живым ответам. Слабые семейства — первыми; их можно выключить без деплоя."
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Select value={specFilter} onChange={(e) => setSpec(e.target.value)} placeholder="Все специализации" options={Object.entries(SPECIALIZATIONS).map(([value, label]) => ({ value, label }))} className="!w-56" />
            <Button variant="secondary" onClick={() => recalibrate.mutate()} loading={recalibrate.isPending} title="Байесовское уточнение сложности по ответам завершённых сессий (от 30 ответов на семейство); выполняется и автоматически раз в сутки">
              Пересчитать калибровку
            </Button>
          </div>
        }
      />
      <div className="grid gap-3 sm:grid-cols-4">
        <Stat label="семейств" value={all.length} hint={`выключено ${all.filter((f) => !f.enabled).length}`} />
        <Stat label="выдач заданий" value={served} tone="blue" />
        <Stat label="с предупреждениями" value={warnings} tone={warnings ? "red" : "ink"} />
        <Stat label="ср. доля верных" value={percent(all.filter((f) => f.answered > 0).reduce((s, f, _, arr) => s + f.correct_share / arr.length, 0))} hint="по семействам с ответами" />
      </div>
      <Card className="mt-4 overflow-x-auto">
        <CardBody className="!p-0">
          <table className="table min-w-[900px]">
            <thead>
              <tr>
                <th>Семейство</th>
                <th>Грейд</th>
                <th>Формат</th>
                <th>a / b / c</th>
                <th>выдач</th>
                <th>верных</th>
                <th>время</th>
                <th>дискр.</th>
                <th>экспоз.</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((f) => (
                <tr key={f.id} className={!f.enabled ? "opacity-50" : ""}>
                  <td>
                    <div className="mono text-[12.5px]">{f.id}</div>
                    <div className="text-[11.5px] text-muted">
                      {spec(f.specialization)} · {f.topic}
                    </div>
                    {f.warning && (
                      <div className="mt-1 inline-flex items-center gap-1 rounded-sm bg-warn-soft px-1.5 py-0.5 text-[11.5px] text-warn">
                        <AlertTriangle className="size-3" /> {f.warning}
                      </div>
                    )}
                  </td>
                  <td className="mono text-[12.5px]">{grade(f.grade)}</td>
                  <td>
                    <Badge>{f.format}</Badge>
                  </td>
                  <td className="mono text-[12px]" title={`b априорное ${f.b_prior.toFixed(2)}`}>
                    {f.a.toFixed(2)} / {f.b.toFixed(2)} / {f.c.toFixed(2)}
                  </td>
                  <td className="mono">{f.served}</td>
                  <td className="mono">{f.answered ? percent(f.correct_share) : "—"}</td>
                  <td className="mono">{f.answered ? `${Math.round(f.avg_time_sec)} с` : "—"}</td>
                  <td className="mono">{f.discrimination ? f.discrimination.toFixed(2) : "—"}</td>
                  <td className="mono">{percent(f.exposure)}</td>
                  <td>
                    <Button size="sm" variant="ghost" onClick={() => toggle.mutate({ id: f.id, enabled: !f.enabled })}>
                      {f.enabled ? "Выключить" : "Включить"}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardBody>
      </Card>
    </>
  );
}
