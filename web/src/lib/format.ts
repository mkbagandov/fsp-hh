// Форматирование: деньги, даты, названия грейдов и специализаций (термины — скилл fsp-domain).
const rub = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 });

export function money(v: number | null | undefined): string {
  if (v === null || v === undefined) return "—";
  return `${rub.format(v)} ₽`;
}

export function salaryRange(from: number | null | undefined, to: number | null | undefined): string {
  if (!from && !to) return "вилка не указана";
  if (from && to) return `${rub.format(from)} – ${rub.format(to)} ₽`;
  return money(from ?? to);
}

export function dateShort(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleDateString("ru-RU", { day: "numeric", month: "short", year: "numeric" });
}

export function dateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("ru-RU", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

export function relative(iso: string | null | undefined): string {
  if (!iso) return "—";
  const diff = Date.now() - Date.parse(iso);
  const m = Math.round(diff / 60000);
  if (m < 1) return "только что";
  if (m < 60) return `${m} мин назад`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} ч назад`;
  const d = Math.round(h / 24);
  if (d < 30) return `${d} дн назад`;
  return dateShort(iso);
}

export const GRADES: Record<string, string> = { junior: "Junior", middle: "Middle", senior: "Senior" };
export const SPECIALIZATIONS: Record<string, string> = {
  backend: "Backend",
  frontend: "Frontend",
  data_ml: "Data / ML",
  devops: "DevOps",
  qa: "QA",
  security: "Безопасность",
  common: "Общее",
};
export const WORK_FORMATS: Record<string, string> = { office: "Офис", hybrid: "Гибрид", remote: "Удалённо" };
export const GRADE_STATUS: Record<string, string> = {
  claimed: "заявлен",
  confirmed: "подтверждён",
  not_confirmed: "не подтверждён",
  upgrade_suggested: "выше заявленного",
};
export const INVITATION_STATUS: Record<string, string> = {
  sent: "отправлено",
  viewed: "просмотрено",
  accepted: "принято",
  declined: "отклонено",
  withdrawn: "отозвано",
  expired: "истекло",
};
export const APPLICATION_STATUS: Record<string, string> = {
  sent: "отправлен",
  viewed: "просмотрен",
  invited: "приглашение",
  rejected: "отказ",
  withdrawn: "отозван",
};
export const VACANCY_STATUS: Record<string, string> = { draft: "черновик", published: "опубликована", closed: "закрыта" };
export const NEED_STATUS: Record<string, string> = { draft: "черновик", active: "активна", archived: "в архиве" };

export function grade(code: string | null | undefined): string {
  return code ? (GRADES[code] ?? code) : "—";
}
export function spec(code: string | null | undefined): string {
  return code ? (SPECIALIZATIONS[code] ?? code) : "—";
}
export function category(specCode: string | null | undefined, gradeCode: string | null | undefined): string {
  return `${spec(specCode)} × ${grade(gradeCode)}`;
}
export function percent(v: number | null | undefined, digits = 0): string {
  if (v === null || v === undefined) return "—";
  return `${(v * 100).toFixed(digits)} %`;
}
export function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return `${n} ${one}`;
  if (m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20)) return `${n} ${few}`;
  return `${n} ${many}`;
}
