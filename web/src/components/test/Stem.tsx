import type { ReactNode } from "react";

/**
 * Рендер условия задания: абзацы, блоки ```код```, таблицы в «палочках», `код` в строке, **жирный**.
 * Семейства банка генерируют именно такой подмножество Markdown; полноценный парсер не нужен.
 */
export function Stem({ text, className = "" }: { text: string; className?: string }) {
  const blocks = splitBlocks(text);
  return (
    <div className={`prose-fsp text-[15px] leading-7 ${className}`}>
      {blocks.map((b, i) => {
        if (b.kind === "code") {
          return <CodeView key={i} source={b.text} lang={b.lang} />;
        }
        if (b.kind === "table") {
          return <PipeTable key={i} rows={b.rows} />;
        }
        return (
          <p key={i} className="whitespace-pre-wrap">
            {inline(b.text)}
          </p>
        );
      })}
    </div>
  );
}

export function CodeView({ source, lang }: { source: string; lang?: string }) {
  return (
    <div className="my-3 overflow-hidden rounded-md border border-line-soft bg-ink text-paper">
      {lang && <div className="label-mono border-b border-white/10 px-3 py-1.5 !text-paper/50">{lang}</div>}
      <pre className="overflow-x-auto px-4 py-3 text-[13px] leading-6">
        <code>{source}</code>
      </pre>
    </div>
  );
}

type Block = { kind: "p"; text: string } | { kind: "code"; text: string; lang?: string } | { kind: "table"; rows: string[][] };

function splitBlocks(text: string): Block[] {
  const out: Block[] = [];
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  let i = 0;
  let para: string[] = [];
  const flush = () => {
    const t = para.join("\n").trim();
    if (t) out.push({ kind: "p", text: t });
    para = [];
  };
  while (i < lines.length) {
    const line = lines[i];
    if (line.startsWith("```")) {
      flush();
      const lang = line.slice(3).trim() || undefined;
      const buf: string[] = [];
      i++;
      while (i < lines.length && !lines[i].startsWith("```")) buf.push(lines[i++]);
      i++;
      out.push({ kind: "code", text: buf.join("\n"), lang });
      continue;
    }
    if (isTableLine(line) && i + 1 < lines.length && /^\s*\|?\s*:?-{2,}/.test(lines[i + 1])) {
      flush();
      const rows: string[][] = [cells(line)];
      i += 2;
      while (i < lines.length && isTableLine(lines[i])) rows.push(cells(lines[i++]));
      out.push({ kind: "table", rows });
      continue;
    }
    if (line.trim() === "") {
      flush();
      i++;
      continue;
    }
    para.push(line);
    i++;
  }
  flush();
  return out;
}

function isTableLine(l: string) {
  return l.includes("|") && l.trim().startsWith("|");
}
function cells(l: string) {
  return l
    .trim()
    .replace(/^\||\|$/g, "")
    .split("|")
    .map((c) => c.trim());
}

function PipeTable({ rows }: { rows: string[][] }) {
  const [head, ...body] = rows;
  return (
    <div className="my-3 overflow-x-auto">
      <table className="table !w-auto min-w-[260px] text-[13px]">
        <thead>
          <tr>
            {head.map((h, i) => (
              <th key={i} className="!normal-case !tracking-normal mono !text-[12px]">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {body.map((r, i) => (
            <tr key={i}>
              {r.map((c, j) => (
                <td key={j} className="mono !py-1.5">
                  {c}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function inline(text: string): ReactNode[] {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g);
  return parts.map((p, i) => {
    if (p.startsWith("`") && p.endsWith("`")) return <code key={i} className="rounded-sm bg-paper px-1 py-0.5 text-[13px] text-ink">{p.slice(1, -1)}</code>;
    if (p.startsWith("**") && p.endsWith("**")) return <strong key={i}>{p.slice(2, -2)}</strong>;
    return <span key={i}>{p}</span>;
  });
}
