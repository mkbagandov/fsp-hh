import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { CheckCircle2, AlertTriangle, Info, XCircle, X } from "lucide-react";

type Tone = "ok" | "warn" | "danger" | "info";
type Toast = { id: number; tone: Tone; title: string; description?: string };
type Api = { push: (t: Omit<Toast, "id">) => void; ok: (title: string, description?: string) => void; error: (title: string, description?: string) => void };

const Ctx = createContext<Api | null>(null);

const icons: Record<Tone, ReactNode> = {
  ok: <CheckCircle2 className="size-4 text-ok" />,
  warn: <AlertTriangle className="size-4 text-warn" />,
  danger: <XCircle className="size-4 text-danger" />,
  info: <Info className="size-4 text-brand-blue" />,
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const seq = useRef(0);
  const remove = useCallback((id: number) => setItems((xs) => xs.filter((x) => x.id !== id)), []);
  const push = useCallback(
    (t: Omit<Toast, "id">) => {
      const id = ++seq.current;
      setItems((xs) => [...xs.slice(-3), { ...t, id }]);
      window.setTimeout(() => remove(id), t.tone === "danger" ? 7000 : 4500);
    },
    [remove],
  );
  const api = useMemo<Api>(
    () => ({ push, ok: (title, description) => push({ tone: "ok", title, description }), error: (title, description) => push({ tone: "danger", title, description }) }),
    [push],
  );
  return (
    <Ctx.Provider value={api}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-4 z-50 flex flex-col items-center gap-2 px-4 sm:items-end sm:pr-6" aria-live="polite">
        {items.map((t) => (
          <div key={t.id} className="pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-md border border-line bg-surface px-4 py-3 shadow-md animate-rise">
            <span className="mt-0.5 shrink-0">{icons[t.tone]}</span>
            <div className="min-w-0 flex-1">
              <div className="text-[14px] font-semibold">{t.title}</div>
              {t.description && <div className="mt-0.5 text-[13px] text-muted">{t.description}</div>}
            </div>
            <button type="button" onClick={() => remove(t.id)} className="rounded-sm p-0.5 text-muted hover:text-ink" aria-label="Скрыть">
              <X className="size-4" />
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}

export function useToast(): Api {
  const v = useContext(Ctx);
  if (!v) throw new Error("useToast вне ToastProvider");
  return v;
}
