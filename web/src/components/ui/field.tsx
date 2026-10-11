import { forwardRef, useId, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";

type FieldProps = {
  label?: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  required?: boolean;
  className?: string;
  children: (id: string, describedBy: string | undefined) => ReactNode;
};

/** Обёртка поля: подпись в JetBrains Mono, подсказка и ошибка со связью через aria. */
export function Field({ label, hint, error, required, className = "", children }: FieldProps) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errId = error ? `${id}-err` : undefined;
  const describedBy = [hintId, errId].filter(Boolean).join(" ") || undefined;
  return (
    <div className={`flex flex-col gap-1.5 ${className}`}>
      {label && (
        <label htmlFor={id} className="label-mono !text-ink-2 !normal-case !tracking-normal !text-[13px] font-medium">
          {label}
          {required && <span className="text-brand-red"> *</span>}
        </label>
      )}
      {children(id, describedBy)}
      {hint && !error && (
        <p id={hintId} className="text-[13px] text-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errId} role="alert" className="text-[13px] text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

type InputProps = InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean; mono?: boolean };
export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ invalid, mono, className = "", ...rest }, ref) {
  return <input ref={ref} className={`input ${mono ? "input-mono" : ""} ${className}`} aria-invalid={invalid || undefined} {...rest} />;
});

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean };
export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea({ invalid, className = "", ...rest }, ref) {
  return <textarea ref={ref} className={`input ${className}`} aria-invalid={invalid || undefined} {...rest} />;
});

type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean; options: { value: string; label: string }[]; placeholder?: string };
export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select({ invalid, options, placeholder, className = "", ...rest }, ref) {
  return (
    <select ref={ref} className={`input ${className}`} aria-invalid={invalid || undefined} {...rest}>
      {placeholder !== undefined && <option value="">{placeholder}</option>}
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
});

type CheckboxProps = Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & { label: ReactNode; description?: ReactNode };
export function Checkbox({ label, description, className = "", id, ...rest }: CheckboxProps) {
  const auto = useId();
  const cid = id ?? auto;
  return (
    <label htmlFor={cid} className={`flex cursor-pointer items-start gap-3 ${className}`}>
      <input id={cid} type="checkbox" className="mt-1 size-4 shrink-0 accent-brand-blue" {...rest} />
      <span className="text-[14px] leading-5">
        <span className="text-ink">{label}</span>
        {description && <span className="block text-[13px] text-muted">{description}</span>}
      </span>
    </label>
  );
}

/** Группа переключателей (роль, формат): одна выбранная кнопка. */
export function SegmentedControl<T extends string>({
  value,
  onChange,
  options,
  ariaLabel,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: ReactNode }[];
  ariaLabel: string;
}) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} className="inline-flex rounded-md border border-line bg-surface p-1">
      {options.map((o) => {
        const active = o.value === value;
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(o.value)}
            className={`mono rounded-[4px] px-3 py-1.5 text-[13px] font-semibold transition-colors ${
              active ? "bg-ink text-white" : "text-ink-2 hover:bg-paper"
            }`}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
