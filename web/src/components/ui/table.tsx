import type { ReactNode } from "react";

export type Column<T> = {
  key: string;
  header: ReactNode;
  render: (row: T) => ReactNode;
  className?: string;
  width?: string;
};

/** Таблица с заголовками в моноширинном капсе; на узких экранах — горизонтальный скролл. */
export function DataTable<T>({ rows, columns, rowKey, empty, onRowClick }: { rows: T[]; columns: Column<T>[]; rowKey: (row: T) => string; empty?: ReactNode; onRowClick?: (row: T) => void }) {
  if (rows.length === 0 && empty) return <>{empty}</>;
  return (
    <div className="card overflow-x-auto">
      <table className="table min-w-[560px]">
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.key} style={{ width: c.width }} className={c.className}>
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={rowKey(r)} onClick={onRowClick ? () => onRowClick(r) : undefined} className={onRowClick ? "cursor-pointer" : undefined}>
              {columns.map((c) => (
                <td key={c.key} className={c.className}>
                  {c.render(r)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
