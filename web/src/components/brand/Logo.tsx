// Логотипы из assets/brand/logos (карта — скилл fsp-brand, references/logos.md). Не перекрашивать и не искажать.
const files = {
  short: { light: "/brand/fsp-short.svg", dark: "/brand/fsp-short-dark.svg", ratio: 281.63 / 94.28 },
  full: { light: "/brand/fsp-full.svg", dark: "/brand/fsp-short-dark.svg", ratio: 1239.07 / 155.12 },
  mark: { light: "/brand/fsp-zvezda.svg", dark: "/brand/fsp-zvezda-dark.svg", ratio: 194.12 / 184.7 },
} as const;

export function Logo({ variant = "short", tone = "light", height = 28, className = "" }: { variant?: keyof typeof files; tone?: "light" | "dark"; height?: number; className?: string }) {
  const f = files[variant];
  return <img src={f[tone]} alt="ФСП — Федерация спортивного программирования" height={height} width={Math.round(height * f.ratio)} className={`block select-none ${className}`} draggable={false} />;
}

/**
 * Фирменный паттерн «кодовые глифы» — только как фон, с низкой непрозрачностью на светлом.
 * Баннерный файл брендбука содержит логотипы партнёров, поэтому в интерфейсе используется
 * только «ковровый» паттерн (глифы без логотипов) и узкая полоска.
 */
export function BrandPattern({ tone = "light", variant = "banner", className = "", opacity }: { tone?: "light" | "dark"; variant?: "banner" | "dense" | "strip"; className?: string; opacity?: number }) {
  const strip = variant === "strip";
  const src = strip ? "/brand/pattern-strip.svg" : "/brand/pattern-dense.svg";
  const op = opacity ?? (tone === "dark" ? 1 : 0.1);
  return (
    <div
      aria-hidden
      className={`pointer-events-none absolute inset-0 ${className}`}
      style={{
        backgroundImage: `url(${src})`,
        backgroundSize: strip ? "auto 8px" : variant === "banner" ? "min(100%, 980px)" : "420px",
        backgroundPosition: variant === "banner" ? "right center" : "center",
        backgroundRepeat: "repeat",
        opacity: op,
      }}
    />
  );
}
