import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { BadgeCheck, ShieldCheck } from "lucide-react";
import { api, unwrap, isApiError } from "@/api/client";
import { BrandPattern } from "@/components/brand/Logo";
import { Badge, Card, CardBody, Skeleton, StatusBadge } from "@/components/ui";
import { GRADE_STATUS, category, dateShort, percent } from "@/lib/format";

/** Публичная страница подтверждения категории — сюда ведёт QR-код из PDF-профиля. Персональных данных нет. */
export function VerifyCategory() {
  const { id = "" } = useParams();
  const q = useQuery({
    queryKey: ["public-verify", id],
    queryFn: async () => unwrap(await api.GET("/public/verify/{id}", { params: { path: { id } } })),
    retry: false,
  });
  const notFound = q.isError && isApiError(q.error) && q.error.status === 404;

  return (
    <div className="relative mx-auto max-w-[760px] px-4 py-10 sm:px-6">
      <BrandPattern variant="dense" opacity={0.06} />
      <div className="relative">
        <div className="label-mono">проверка подлинности</div>
        <h1 className="mt-1">Подтверждение категории</h1>
        <p className="mt-2 max-w-2xl text-muted">
          Страница открывается по QR-коду из PDF-профиля кандидата платформы «ФСП». Здесь нет персональных данных — только категории, подтверждённые адаптивным тестом.
        </p>

        {q.isLoading && <Skeleton className="mt-6 h-40" />}
        {notFound && (
          <Card className="mt-6">
            <CardBody>
              <h2>Подтверждение не найдено</h2>
              <p className="mt-2 text-muted">Кандидат скрыл профиль, отозвал согласие на публикацию, ещё не проходил тест, либо ссылка устарела.</p>
            </CardBody>
          </Card>
        )}
        {q.isError && !notFound && (
          <p role="alert" className="mt-6 text-danger">
            Не удалось загрузить подтверждение. Попробуйте позже.
          </p>
        )}
        {q.data && (
          <>
            <div className="mt-6 flex items-center gap-3 rounded-md bg-ok-soft px-4 py-3 text-ok">
              <ShieldCheck className="size-6 shrink-0" aria-hidden />
              <div>
                <div className="font-semibold">Подтверждено платформой «ФСП»</div>
                <div className="text-[13px]">
                  Проверено {dateShort(q.data.generated_at)} · {q.data.fsp_linked ? "аккаунт связан с ФСП ID" : "без привязки к ФСП ID"}
                </div>
              </div>
            </div>
            <div className="mt-4 grid gap-3">
              {(q.data.categories ?? []).map((c) => (
                <Card key={`${c.specialization}-${c.grade}`} className={c.is_primary ? "border-brand-blue" : ""}>
                  <CardBody className="flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <div className="label-mono">{c.is_primary ? "основная категория" : "категория"}</div>
                      <div className="num-display mt-1 !text-[30px]">{category(c.specialization, c.grade)}</div>
                      <div className="mt-2 flex flex-wrap items-center gap-2">
                        <StatusBadge status={c.status} label={GRADE_STATUS[c.status] ?? c.status} />
                        {c.percentile !== undefined && c.status === "confirmed" && <Badge tone="blue">{percent(c.percentile)} процентиль</Badge>}
                        {c.confirmed_at && <span className="text-[12px] text-muted">подтверждено {dateShort(c.confirmed_at)}</span>}
                      </div>
                    </div>
                    {c.status === "confirmed" && <BadgeCheck className="size-8 text-ok" aria-hidden />}
                  </CardBody>
                </Card>
              ))}
            </div>
            <Card className="mt-6">
              <CardBody className="text-[14px] text-muted">
                <h3>Что это значит</h3>
                <p className="mt-2">
                  Статус «подтверждён» означает, что кандидат прошёл адаптивный тест из 12–20 параметрических заданий и вероятность того, что его уровень не ниже границы грейда, составила не меньше 70 %. Процентиль — место среди кандидатов той же специализации. Задания генерируются заново для каждого прохождения, поэтому результат нельзя получить по чужим ответам.
                </p>
              </CardBody>
            </Card>
          </>
        )}
        <p className="mt-6 text-[13px] text-muted">
          Платформа найма «ФСП» — <Link to="/">как это работает</Link>
        </p>
      </div>
    </div>
  );
}
