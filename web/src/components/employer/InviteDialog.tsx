import { useState } from "react";
import { Button, Dialog, Field, Input, Select, Textarea, SalaryInputs } from "@/components/ui";
import type { ContactMethod } from "@/api/employer";

export type InvitePayload = { message: string; salary_from: number; salary_to: number; contact_method: ContactMethod };

/** Диалог приглашения: вилка и способ связи обязательны — это правило платформы, а не формы. */
export function InviteDialog({
  open,
  onClose,
  onSubmit,
  busy,
  title = "Пригласить кандидата",
  candidateName,
  defaults,
}: {
  open: boolean;
  onClose: () => void;
  onSubmit: (p: InvitePayload) => void;
  busy?: boolean;
  title?: string;
  candidateName?: string;
  defaults?: { salary_from?: number; salary_to?: number; message?: string };
}) {
  const [message, setMessage] = useState(defaults?.message ?? "");
  const [salary, setSalary] = useState({ from: defaults?.salary_from ? String(defaults.salary_from) : "", to: defaults?.salary_to ? String(defaults.salary_to) : "" });
  const [kind, setKind] = useState<ContactMethod["kind"]>("telegram");
  const [value, setValue] = useState("");
  const from = Number(salary.from);
  const to = Number(salary.to);
  const valid = message.trim().length > 0 && from > 0 && to >= from && value.trim().length > 0;
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={candidateName ? `${title}: ${candidateName}` : title}
      width="max-w-xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Отмена
          </Button>
          <Button onClick={() => onSubmit({ message: message.trim(), salary_from: from, salary_to: to, contact_method: { kind, value: value.trim() } })} loading={busy} disabled={!valid}>
            Отправить приглашение
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <p className="text-[13px] text-muted">Кандидат увидит вилку и текст сразу, способ связи — после принятия. Контакты кандидата откроются вам после его «да».</p>
        <SalaryInputs from={salary.from} to={salary.to} onChange={setSalary} errorTo={to > 0 && to < from ? "«до» не может быть меньше «от»" : undefined} />
        <Field label="Сообщение" required hint="Что за команда и задачи, почему именно этот кандидат">
          {(id) => <Textarea id={id} rows={4} maxLength={2000} value={message} onChange={(e) => setMessage(e.target.value)} />}
        </Field>
        <div className="grid grid-cols-[150px_1fr] gap-3">
          <Field label="Способ связи" required>
            {(id) => (
              <Select
                id={id}
                value={kind}
                onChange={(e) => setKind(e.target.value as ContactMethod["kind"])}
                options={[
                  { value: "telegram", label: "Telegram" },
                  { value: "email", label: "E-mail" },
                  { value: "phone", label: "Телефон" },
                  { value: "link", label: "Ссылка" },
                ]}
              />
            )}
          </Field>
          <Field label="Контакт" required>
            {(id) => <Input id={id} mono value={value} onChange={(e) => setValue(e.target.value)} placeholder={kind === "telegram" ? "@hr_company" : kind === "email" ? "hr@company.ru" : kind === "phone" ? "+7 …" : "https://…"} />}
          </Field>
        </div>
      </div>
    </Dialog>
  );
}
