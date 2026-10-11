import type { ReactNode } from "react";
import { Link } from "react-router";
import { Logo, BrandPattern } from "@/components/brand/Logo";

/** Общая рамка страниц входа: узкая карточка на фоне паттерна, логотип сверху. */
export function AuthLayout({ title, subtitle, children, footer }: { title: string; subtitle?: ReactNode; children: ReactNode; footer?: ReactNode }) {
  return (
    <div className="relative flex min-h-dvh flex-col items-center justify-center px-4 py-10">
      <BrandPattern variant="banner" opacity={0.08} />
      <Link to="/" className="relative mb-6">
        <Logo height={34} />
      </Link>
      <div className="card relative w-full max-w-[440px] animate-rise p-6 sm:p-8">
        <h1 className="text-[24px]">{title}</h1>
        {subtitle && <p className="mt-1 text-[14px] text-muted">{subtitle}</p>}
        <div className="mt-6">{children}</div>
      </div>
      {footer && <div className="relative mt-4 text-center text-[14px] text-muted">{footer}</div>}
    </div>
  );
}
