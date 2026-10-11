/**
 * Безопасная цель перехода после входа: только внутренний путь приложения.
 * «//host» и «/\host» браузер трактует как внешний адрес, поэтому они отбрасываются.
 */
export function safeNext(next: string | null | undefined): string | null {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return null;
  if (/[\u0000-\u001f]/.test(next)) return null;
  return next;
}
