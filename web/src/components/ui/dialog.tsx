import { useEffect, useRef, type ReactNode } from "react";
import { X } from "lucide-react";

/** Модальное окно на нативном <dialog>: фокус-ловушка и Esc бесплатно от браузера. */
export function Dialog({ open, onClose, title, children, footer, width = "max-w-lg" }: { open: boolean; onClose: () => void; title: ReactNode; children: ReactNode; footer?: ReactNode; width?: string }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === ref.current) onClose();
      }}
      className={`m-auto w-[calc(100%-32px)] ${width} rounded-md border border-line bg-surface p-0 text-ink shadow-md backdrop:bg-ink/50 open:animate-fade`}
    >
      <div className="flex items-center justify-between border-b border-line-soft px-5 py-3.5">
        <h3 className="text-[16px]">{title}</h3>
        <button type="button" onClick={onClose} className="rounded-sm p-1 text-muted hover:bg-paper hover:text-ink" aria-label="Закрыть">
          <X className="size-4" />
        </button>
      </div>
      <div className="px-5 py-4">{children}</div>
      {footer && <div className="flex justify-end gap-2 border-t border-line-soft px-5 py-3">{footer}</div>}
    </dialog>
  );
}
