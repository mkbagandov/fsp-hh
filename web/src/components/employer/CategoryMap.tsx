import { Lightbulb } from "lucide-react";
import type { CategoryStat, Weights } from "@/api/employer";
import { grade, money, percent, plural } from "@/lib/format";

/** Карта категорий: по грейдам специализации — объём, подтверждённые, ФСП, ожидания и покрытие вилкой. */
export function CategoryMap({ map, salaryTo, weights }: { map: CategoryStat[]; salaryTo: number; weights?: Weights }) {
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 md:grid-cols-3">
        {map.map((c) => (
          <div key={c.grade} className={`card p-4 ${c.in_range ? "border-brand-blue" : "opacity-80"}`}>
            <div className="flex items-start justify-between">
              <div>
                <div className="label-mono">{c.in_range ? "в диапазоне потребности" : "вне диапазона"}</div>
                <div className="num-display mt-1 !text-[28px]">{grade(c.grade)}</div>
              </div>
              <div className="text-right">
                <div className="num-display text-brand-blue !text-[30px]">{c.total}</div>
                <div className="text-[11px] text-muted">{plural(c.total, "кандидат", "кандидата", "кандидатов")}</div>
              </div>
            </div>
            <dl className="mt-3 grid grid-cols-2 gap-x-3 gap-y-1 text-[13px]">
              <dt className="text-muted">подтверждённых</dt>
              <dd className="mono text-right">
                {c.confirmed} <span className="text-muted">({percent(c.confirmed_share)})</span>
              </dd>
              <dt className="text-muted">с достижениями ФСП</dt>
              <dd className="mono text-right">
                {c.with_fsp} <span className="text-muted">({percent(c.fsp_share)})</span>
              </dd>
              <dt className="text-muted">ожидания p25 / p50 / p75</dt>
              <dd className="mono text-right">{c.salary_p50 ? `${short(c.salary_p25)} / ${short(c.salary_p50)} / ${short(c.salary_p75)}` : "—"}</dd>
              <dt className="text-muted">покрытие вилкой до {short(salaryTo)}</dt>
              <dd className={`mono text-right font-semibold ${c.coverage >= 0.6 ? "text-ok" : c.coverage >= 0.3 ? "text-warn" : "text-danger"}`}>{percent(c.coverage)}</dd>
            </dl>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-line-soft">
              <div className={`h-full ${c.coverage >= 0.6 ? "bg-ok" : c.coverage >= 0.3 ? "bg-warn" : "bg-danger"}`} style={{ width: `${Math.round(c.coverage * 100)}%` }} />
            </div>
            {(c.hints ?? []).length > 0 && (
              <ul className="mt-3 flex flex-col gap-1">
                {(c.hints ?? []).map((h) => (
                  <li key={h} className="flex items-start gap-1.5 text-[12.5px] text-ink-2">
                    <Lightbulb className="mt-0.5 size-3.5 shrink-0 text-warn" /> {h}
                  </li>
                ))}
              </ul>
            )}
          </div>
        ))}
      </div>
      {weights && (
        <p className="text-[12px] text-muted">
          Релевантность = {Object.entries(weights).map(([k, v]) => `${k} × ${v}`).join(" + ")}; итог = релевантность × (0,6 + 0,4 × вероятность отклика). Подтверждённые тестом всегда выше неподтверждённых.
        </p>
      )}
    </div>
  );
}

function short(v: number): string {
  if (!v) return "—";
  return v >= 1000 ? `${Math.round(v / 1000)}к` : money(v);
}
