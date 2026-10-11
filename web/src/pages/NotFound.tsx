import { Link } from "react-router";
import { BrandPattern } from "@/components/brand/Logo";

export function NotFound() {
  return (
    <div className="relative flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
      <BrandPattern variant="dense" opacity={0.07} />
      <div className="relative">
        <div className="num-display text-brand-red !text-[72px]">404</div>
        <h1 className="mt-3">Такой страницы нет</h1>
        <p className="mt-2 text-muted">Проверьте адрес или вернитесь на главную.</p>
        <Link to="/" className="btn btn-primary mt-6 no-underline">
          На главную
        </Link>
      </div>
    </div>
  );
}
