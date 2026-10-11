// Простые доступные графики на SVG в палитре ФСП: горизонтальные полоски и столбцы по дням.
// Без библиотек: данных мало, важнее единый стиль и читаемые подписи.

export const CHART_COLORS = ["#402fff", "#ec1d35", "#3d4e9c", "#624190", "#0f8a5f", "#b45309"];

export function HBars({ items, color = CHART_COLORS[0], max }: { items: { label: string; value: number; color?: string }[]; color?: string; max?: number }) {
  const m = max ?? Math.max(1, ...items.map((i) => i.value));
  return (
    <ul className="flex flex-col gap-1.5" aria-label="Диаграмма">
      {items.map((it) => (
        <li key={it.label} className="grid grid-cols-[120px_1fr_48px] items-center gap-2 text-[12.5px]">
          <span className="truncate text-muted" title={it.label}>
            {it.label}
          </span>
          <span className="h-3 overflow-hidden rounded-sm bg-line-soft">
            <span className="block h-full rounded-sm" style={{ width: `${Math.round((it.value / m) * 100)}%`, background: it.color ?? color }} />
          </span>
          <span className="mono text-right">{it.value}</span>
        </li>
      ))}
    </ul>
  );
}

export function DailyBars({ days, series }: { days: string[]; series: { name: string; values: number[]; color: string }[] }) {
  const w = 640;
  const h = 160;
  const pad = { l: 28, r: 8, t: 8, b: 22 };
  const max = Math.max(1, ...series.flatMap((s) => s.values));
  const n = days.length;
  const group = (w - pad.l - pad.r) / Math.max(1, n);
  const bw = Math.max(2, (group - 4) / series.length);
  const y = (v: number) => pad.t + (h - pad.t - pad.b) * (1 - v / max);
  return (
    <figure>
      <svg viewBox={`0 0 ${w} ${h}`} className="w-full" role="img" aria-label={`Активность по дням: ${series.map((s) => s.name).join(", ")}`}>
        {[0, 0.5, 1].map((t) => (
          <g key={t}>
            <line x1={pad.l} x2={w - pad.r} y1={y(max * t)} y2={y(max * t)} stroke="#e2e3e5" strokeWidth="1" />
            <text x={pad.l - 4} y={y(max * t) + 4} textAnchor="end" fontSize="10" fontFamily="JetBrains Mono, monospace" fill="#6b6e78">
              {Math.round(max * t)}
            </text>
          </g>
        ))}
        {days.map((d, i) => (
          <g key={d}>
            {series.map((s, k) => {
              const v = s.values[i] ?? 0;
              const x = pad.l + i * group + 2 + k * bw;
              return <rect key={s.name} x={x} y={y(v)} width={bw - 1} height={Math.max(0, h - pad.b - y(v))} fill={s.color} rx="1">
                <title>{`${d}: ${s.name} ${v}`}</title>
              </rect>;
            })}
            {(n <= 14 || i % Math.ceil(n / 14) === 0) && (
              <text x={pad.l + i * group + group / 2} y={h - 6} textAnchor="middle" fontSize="10" fontFamily="JetBrains Mono, monospace" fill="#6b6e78">
                {d.slice(5)}
              </text>
            )}
          </g>
        ))}
      </svg>
      <figcaption className="mt-1 flex flex-wrap gap-3 text-[12px] text-muted">
        {series.map((s) => (
          <span key={s.name} className="inline-flex items-center gap-1">
            <span className="inline-block size-2.5 rounded-sm" style={{ background: s.color }} /> {s.name}
          </span>
        ))}
      </figcaption>
    </figure>
  );
}

export function Funnel({ steps }: { steps: { label: string; value: number }[] }) {
  const max = Math.max(1, steps[0]?.value ?? 1);
  return (
    <ol className="flex flex-col gap-1.5">
      {steps.map((s, i) => {
        const prev = i > 0 ? steps[i - 1].value : null;
        const conv = prev ? Math.round((s.value / Math.max(1, prev)) * 100) : null;
        return (
          <li key={s.label} className="grid grid-cols-[150px_1fr_60px_50px] items-center gap-2 text-[12.5px]">
            <span className="text-muted">{s.label}</span>
            <span className="h-4 overflow-hidden rounded-sm bg-line-soft">
              <span className="block h-full rounded-sm bg-brand-blue" style={{ width: `${Math.round((s.value / max) * 100)}%`, opacity: 1 - i * 0.12 }} />
            </span>
            <span className="mono text-right">{s.value}</span>
            <span className="mono text-right text-muted">{conv !== null ? `${conv} %` : ""}</span>
          </li>
        );
      })}
    </ol>
  );
}
