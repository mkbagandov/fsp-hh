import type { HTMLAttributes, ReactNode } from "react";

export function Card({ className = "", children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={`card ${className}`} {...rest}>
      {children}
    </div>
  );
}

export function CardHeader({ title, description, action }: { title: ReactNode; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-line-soft px-5 py-4">
      <div>
        <h3 className="text-[16px]">{title}</h3>
        {description && <p className="mt-0.5 text-[13px] text-muted">{description}</p>}
      </div>
      {action}
    </div>
  );
}

export function CardBody({ className = "", children }: { className?: string; children: ReactNode }) {
  return <div className={`px-5 py-4 ${className}`}>{children}</div>;
}

export type BadgeTone = "neutral" | "blue" | "red" | "brand" | "ok" | "warn" | "ink";

export function Badge({ tone = "neutral", className = "", children }: { tone?: BadgeTone; className?: string; children: ReactNode }) {
  return <span className={`badge badge-${tone} ${className}`}>{children}</span>;
}

/** Бейдж статуса: единые тона для приглашений, откликов, вакансий, грейдов. */
const statusTone: Record<string, BadgeTone> = {
  sent: "blue",
  viewed: "neutral",
  accepted: "ok",
  invited: "ok",
  declined: "red",
  rejected: "red",
  withdrawn: "neutral",
  expired: "neutral",
  draft: "neutral",
  active: "ok",
  published: "ok",
  archived: "neutral",
  closed: "neutral",
  confirmed: "ok",
  upgrade_suggested: "brand",
  not_confirmed: "warn",
  claimed: "neutral",
  completed: "ok",
  in_progress: "blue",
};
export function StatusBadge({ status, label }: { status: string; label: string }) {
  return <Badge tone={statusTone[status] ?? "neutral"}>{label}</Badge>;
}

export function Skeleton({ className = "" }: { className?: string }) {
  return <div className={`skeleton ${className}`} aria-hidden />;
}

export function EmptyState({ title, description, action, pattern = true }: { title: ReactNode; description?: ReactNode; action?: ReactNode; pattern?: boolean }) {
  return (
    <div className="relative overflow-hidden rounded-md border border-dashed border-line bg-surface px-6 py-10 text-center">
      {pattern && (
        <div
          aria-hidden
          className="pointer-events-none absolute inset-0 opacity-[0.07]"
          style={{ backgroundImage: "url(/brand/pattern-dense.svg)", backgroundSize: "420px" }}
        />
      )}
      <div className="relative">
        <h3 className="text-[16px]">{title}</h3>
        {description && <p className="mx-auto mt-1 max-w-md text-[14px] text-muted">{description}</p>}
        {action && <div className="mt-4 flex justify-center">{action}</div>}
      </div>
    </div>
  );
}

/** Метрика: подпись моноширинным капсом, значение акцентным шрифтом. */
export function Stat({ label, value, hint, tone = "ink" }: { label: ReactNode; value: ReactNode; hint?: ReactNode; tone?: "ink" | "blue" | "red" }) {
  const color = tone === "blue" ? "text-brand-blue" : tone === "red" ? "text-brand-red" : "text-ink";
  return (
    <div className="card px-5 py-4">
      <div className="label-mono">{label}</div>
      <div className={`num-display mt-2 ${color}`}>{value}</div>
      {hint && <div className="mt-1 text-[13px] text-muted">{hint}</div>}
    </div>
  );
}

export function Spinner({ label = "Загрузка" }: { label?: string }) {
  return (
    <div role="status" className="flex items-center gap-2 text-[13px] text-muted">
      <span className="size-3.5 animate-spin rounded-full border-2 border-line border-t-brand-blue" aria-hidden />
      {label}
    </div>
  );
}
