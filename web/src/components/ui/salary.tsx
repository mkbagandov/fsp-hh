import { Field, Input } from "./field";
import { salaryRange } from "@/lib/format";

/** Вилка — обязательное условие платформы: показывается крупно и моноширинно. */
export function SalaryRange({ from, to, className = "" }: { from?: number | null; to?: number | null; className?: string }) {
  return (
    <span className={`mono font-semibold tabular-nums ${className}`} title="Вилка зарплаты, рублей в месяц">
      {salaryRange(from, to)}
    </span>
  );
}

export function SalaryInputs({
  from,
  to,
  onChange,
  errorFrom,
  errorTo,
  required = true,
}: {
  from: string;
  to: string;
  onChange: (v: { from: string; to: string }) => void;
  errorFrom?: string;
  errorTo?: string;
  required?: boolean;
}) {
  return (
    <div className="grid grid-cols-2 gap-3">
      <Field label="Вилка от, ₽" required={required} error={errorFrom}>
        {(id, by) => <Input id={id} aria-describedby={by} inputMode="numeric" mono value={from} onChange={(e) => onChange({ from: e.target.value.replace(/\D/g, ""), to })} placeholder="200000" invalid={!!errorFrom} />}
      </Field>
      <Field label="до, ₽" required={required} error={errorTo}>
        {(id, by) => <Input id={id} aria-describedby={by} inputMode="numeric" mono value={to} onChange={(e) => onChange({ from, to: e.target.value.replace(/\D/g, "") })} placeholder="300000" invalid={!!errorTo} />}
      </Field>
    </div>
  );
}
