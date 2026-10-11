import { useState } from "react";
import { Link } from "react-router";
import { ChevronDown, ChevronUp, Award } from "lucide-react";
import type { SelectionItem } from "@/api/employer";
import { COMPONENT_LABELS, FACT_KIND_LABELS } from "@/api/employer";
import { Badge, Button, StatusBadge } from "@/components/ui";
import { GRADE_STATUS, WORK_FORMATS, grade, money, percent } from "@/lib/format";

/** Таблица подборки: баллы, компоненты и раскрываемые факты-объяснения; приглашение в один клик. */
export function SelectionTable({ items, onInvite }: { items: SelectionItem[]; onInvite?: (item: SelectionItem) => void }) {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <div className="card overflow-x-auto">
      <table className="table min-w-[760px]">
        <thead>
          <tr>
            <th style={{ width: 44 }}>#</th>
            <th>Кандидат</th>
            <th style={{ width: 120 }}>Грейд</th>
            <th style={{ width: 110 }}>Score</th>
            <th style={{ width: 250 }}>Компоненты</th>
            <th style={{ width: 150 }} />
          </tr>
        </thead>
        <tbody>
          {items.map((it) => {
            const c = it.candidate;
            const expanded = open === c.id;
            return (
              <FragmentRow key={c.id}>
                <tr className={expanded ? "!bg-brand-blue-soft/30" : ""}>
                  <td className="mono text-muted">{it.rank}</td>
                  <td>
                    <div className="flex items-center gap-2">
                      <Link to={`/employer/candidates/${c.id}`} className="font-semibold text-ink no-underline hover:text-brand-blue">
                        {c.display_name}
                      </Link>
                      {c.has_fsp && <Award className="size-4 text-brand-red" aria-label="Есть достижения ФСП" />}
                      {c.invited && <Badge tone="blue">приглашён</Badge>}
                    </div>
                    <div className="mt-0.5 text-[12.5px] text-muted">
                      {[c.headline, c.city, c.salary_expected ? money(c.salary_expected) : null, (c.work_formats ?? []).map((f) => WORK_FORMATS[f] ?? f).join("/")].filter(Boolean).join(" · ")}
                    </div>
                  </td>
                  <td>
                    <div className="mono text-[13px]">{grade(c.grade)}</div>
                    <StatusBadge status={c.grade_status} label={GRADE_STATUS[c.grade_status] ?? c.grade_status} />
                  </td>
                  <td>
                    <div className="mono text-[16px] font-semibold">{it.score.toFixed(2)}</div>
                    <div className="text-[11px] text-muted">
                      rel {it.relevance.toFixed(2)} · откл {percent(it.p_accept)}
                    </div>
                  </td>
                  <td>
                    <Bars components={it.components} />
                  </td>
                  <td className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button size="sm" variant="ghost" onClick={() => setOpen(expanded ? null : c.id)} aria-expanded={expanded}>
                        {expanded ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />} почему
                      </Button>
                      {onInvite && !c.invited && (
                        <Button size="sm" onClick={() => onInvite(it)}>
                          Пригласить
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
                {expanded && (
                  <tr>
                    <td colSpan={6} className="!bg-paper/60">
                      <Facts item={it} />
                    </td>
                  </tr>
                )}
              </FragmentRow>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function FragmentRow({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}

function Bars({ components }: { components: Record<string, number> }) {
  const keys = ["s_test", "s_skills", "s_fsp", "s_fresh", "s_fit"];
  return (
    <div className="flex flex-col gap-0.5">
      {keys.map((k) => {
        const v = components[k] ?? 0;
        return (
          <div key={k} className="flex items-center gap-2 text-[11px]">
            <span className="mono w-14 text-muted">{COMPONENT_LABELS[k]}</span>
            <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-line-soft">
              <span className="block h-full bg-brand-blue" style={{ width: `${Math.round(Math.min(1, v) * 100)}%` }} />
            </span>
            <span className="mono w-8 text-right text-muted">{v.toFixed(2)}</span>
          </div>
        );
      })}
    </div>
  );
}

export function Facts({ item }: { item: SelectionItem }) {
  const facts = item.facts ?? [];
  return (
    <ul className="grid gap-1.5 py-1 sm:grid-cols-2">
      {facts.map((f, i) => (
        <li key={i} className="flex items-start gap-2 text-[13px]">
          <span className={`mono mt-0.5 inline-flex size-5 shrink-0 items-center justify-center rounded-sm text-[12px] font-bold ${f.polarity === "+" ? "bg-ok-soft text-ok" : f.polarity === "-" ? "bg-danger-soft text-danger" : "bg-line-soft text-muted"}`}>{f.polarity}</span>
          <span>
            <span className="label-mono mr-1 !text-[10px]">{FACT_KIND_LABELS[f.kind] ?? f.kind}</span>
            {f.text}
            {f.contrib !== undefined && f.contrib > 0 && <span className="mono ml-1 text-[11px] text-muted">+{f.contrib.toFixed(2)}</span>}
          </span>
        </li>
      ))}
    </ul>
  );
}
