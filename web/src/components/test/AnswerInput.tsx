import { ArrowDown, ArrowUp } from "lucide-react";
import type { ItemView, Submission } from "@/api/candidate";
import { Input, Textarea } from "@/components/ui";

/** Ввод ответа по формату задания; значение — SubmissionStruct, которую принимает API. */
export function AnswerInput({ item, value, onChange, disabled }: { item: ItemView; value: Submission; onChange: (v: Submission) => void; disabled?: boolean }) {
  const options = item.options ?? [];
  switch (item.format) {
    case "single_choice":
      return (
        <div role="radiogroup" aria-label="Варианты ответа" className="flex flex-col gap-2">
          {options.map((o) => {
            const checked = (value.keys ?? [])[0] === o.key;
            return (
              <label key={o.key} className={`flex cursor-pointer items-start gap-3 rounded-md border px-4 py-3 transition-colors ${checked ? "border-brand-blue bg-brand-blue-soft/40" : "border-line-soft bg-surface hover:border-line"}`}>
                <input type="radio" name="answer" className="mt-1 accent-brand-blue" checked={checked} disabled={disabled} onChange={() => onChange({ keys: [o.key] })} />
                <span className="mono mr-1 text-[12px] text-muted">{o.key}</span>
                <span className="text-[15px]">{o.text}</span>
              </label>
            );
          })}
        </div>
      );
    case "multi_choice":
      return (
        <div className="flex flex-col gap-2">
          <p className="text-[13px] text-muted">Выберите все подходящие варианты.</p>
          {options.map((o) => {
            const keys = value.keys ?? [];
            const checked = keys.includes(o.key);
            return (
              <label key={o.key} className={`flex cursor-pointer items-start gap-3 rounded-md border px-4 py-3 transition-colors ${checked ? "border-brand-blue bg-brand-blue-soft/40" : "border-line-soft bg-surface hover:border-line"}`}>
                <input
                  type="checkbox"
                  className="mt-1 accent-brand-blue"
                  checked={checked}
                  disabled={disabled}
                  onChange={(e) => onChange({ keys: e.target.checked ? [...keys, o.key] : keys.filter((k) => k !== o.key) })}
                />
                <span className="mono mr-1 text-[12px] text-muted">{o.key}</span>
                <span className="text-[15px]">{o.text}</span>
              </label>
            );
          })}
        </div>
      );
    case "numeric":
      return (
        <div className="max-w-xs">
          <Input
            mono
            inputMode="decimal"
            placeholder="число"
            disabled={disabled}
            value={value.text ?? ""}
            onChange={(e) => {
              const t = e.target.value.replace(",", ".");
              const n = Number(t);
              onChange({ text: t, number: t.trim() === "" || Number.isNaN(n) ? undefined : n });
            }}
          />
          <p className="mt-1 text-[12px] text-muted">Десятичный разделитель — точка или запятая.</p>
        </div>
      );
    case "short_text":
    case "code_output":
      return (
        <div>
          <Textarea
            className="input-mono !min-h-[72px]"
            placeholder={item.format === "code_output" ? "Что выведет программа — построчно, как в консоли" : "Короткий ответ"}
            disabled={disabled}
            value={value.text ?? ""}
            onChange={(e) => onChange({ text: e.target.value })}
            rows={item.format === "code_output" ? 4 : 2}
          />
          <p className="mt-1 text-[12px] text-muted">Лишние пробелы и пустые строки в конце не учитываются; регистр — учитывается.</p>
        </div>
      );
    case "ordering":
      return <Ordering options={options} value={value.order ?? options.map((o) => o.key)} onChange={(order) => onChange({ order })} disabled={disabled} />;
    case "sql_result":
      return (
        <div>
          <Textarea
            className="input-mono"
            placeholder={"строка 1: значение1, значение2\nстрока 2: ..."}
            disabled={disabled}
            value={value.text ?? ""}
            rows={5}
            onChange={(e) => {
              const text = e.target.value;
              const rows = text
                .split("\n")
                .map((l) => l.trim())
                .filter(Boolean)
                .map((l) => l.split(/\s*[,|;]\s*/).map((c) => c.trim()));
              onChange({ text, rows });
            }}
          />
          <p className="mt-1 text-[12px] text-muted">Каждая строка результата — с новой строки, значения столбцов через запятую. Порядок строк — как вернёт запрос.</p>
        </div>
      );
    default:
      return <p className="text-danger">Неизвестный формат задания</p>;
  }
}

function Ordering({ options, value, onChange, disabled }: { options: { key: string; text: string }[]; value: string[]; onChange: (v: string[]) => void; disabled?: boolean }) {
  const byKey = new Map(options.map((o) => [o.key, o.text]));
  const order = value.filter((k) => byKey.has(k));
  for (const o of options) if (!order.includes(o.key)) order.push(o.key);
  const move = (i: number, d: -1 | 1) => {
    const j = i + d;
    if (j < 0 || j >= order.length) return;
    const next = [...order];
    [next[i], next[j]] = [next[j], next[i]];
    onChange(next);
  };
  return (
    <div className="flex flex-col gap-2">
      <p className="text-[13px] text-muted">Расставьте шаги в правильном порядке — стрелками или клавишами.</p>
      <ol className="flex flex-col gap-1.5">
        {order.map((k, i) => (
          <li key={k} className="flex items-center gap-3 rounded-md border border-line-soft bg-surface px-3 py-2">
            <span className="num-display w-7 !text-[22px] text-brand-blue">{i + 1}</span>
            <span className="flex-1 text-[14px]">{byKey.get(k)}</span>
            <span className="flex gap-1">
              <button type="button" className="btn btn-ghost btn-sm !px-2" aria-label="Выше" disabled={disabled || i === 0} onClick={() => move(i, -1)}>
                <ArrowUp className="size-4" />
              </button>
              <button type="button" className="btn btn-ghost btn-sm !px-2" aria-label="Ниже" disabled={disabled || i === order.length - 1} onClick={() => move(i, 1)}>
                <ArrowDown className="size-4" />
              </button>
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}

/** Начальное значение ответа: для «порядка» показанный порядок уже является ответом. */
export function initialSubmission(item: ItemView): Submission {
  if (item.format === "ordering") return { order: (item.options ?? []).map((o) => o.key) };
  return {};
}

export function isAnswered(item: ItemView, v: Submission): boolean {
  switch (item.format) {
    case "single_choice":
      return (v.keys?.length ?? 0) === 1;
    case "multi_choice":
      return (v.keys?.length ?? 0) >= 1;
    case "numeric":
      return typeof v.number === "number";
    case "short_text":
    case "code_output":
      return (v.text ?? "").trim() !== "";
    case "ordering":
      return (v.order?.length ?? 0) === (item.options?.length ?? 0);
    case "sql_result":
      return (v.rows?.length ?? 0) >= 1;
    default:
      return false;
  }
}
