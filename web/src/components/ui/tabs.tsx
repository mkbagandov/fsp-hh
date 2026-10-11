import type { ReactNode } from "react";

export function Tabs<T extends string>({ value, onChange, items, ariaLabel }: { value: T; onChange: (v: T) => void; items: { value: T; label: ReactNode; count?: number }[]; ariaLabel: string }) {
  return (
    <div role="tablist" aria-label={ariaLabel} className="tabs overflow-x-auto">
      {items.map((it) => (
        <button key={it.value} role="tab" type="button" aria-selected={it.value === value} className="tab" onClick={() => onChange(it.value)}>
          {it.label}
          {it.count !== undefined && <span className="ml-1.5 rounded-sm bg-paper px-1.5 text-[11px] text-muted">{it.count}</span>}
        </button>
      ))}
    </div>
  );
}
