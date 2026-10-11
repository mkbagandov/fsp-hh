import { getSession, refreshSession } from "@/api/client";

/** Скачивает PDF-профиль: ссылка требует bearer, поэтому через fetch и blob. */
export async function downloadProfilePdf(): Promise<void> {
  if (!getSession()) await refreshSession();
  const res = await fetch("/api/v1/candidate/profile.pdf", { headers: { Authorization: `Bearer ${getSession()?.accessToken ?? ""}` } });
  if (!res.ok) throw new Error(`Не удалось получить PDF (${res.status})`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "fsp-profile.pdf";
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 10_000);
}
