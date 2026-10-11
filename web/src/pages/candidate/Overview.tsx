import { Link } from "react-router";
import { ArrowRight, Award, FileText, Mail } from "lucide-react";
import { useSession } from "@/api/session";
import { useAchievements, useCategories, useFspLink, useInvitations, useProfile, useSessions, useSurvey } from "@/api/candidate";
import { PageHeader } from "@/components/layout/AppShell";
import { Badge, Card, CardBody, CardHeader, EmptyState, LinkButton, Skeleton, StatusBadge, SalaryRange } from "@/components/ui";
import { GRADE_STATUS, INVITATION_STATUS, category, dateShort, percent, relative } from "@/lib/format";
import { downloadProfilePdf } from "./pdf";

/** Обзор кандидата: категория, заполненность, незавершённый тест, приглашения, достижения ФСП. */
export function CandidateOverview() {
  const { user } = useSession();
  const profile = useProfile();
  const survey = useSurvey(profile.data?.primary_specialization ?? "backend");
  const cats = useCategories();
  const sessions = useSessions();
  const invitations = useInvitations();
  const link = useFspLink();
  const achievements = useAchievements(!!link.data?.linked);

  const inProgress = sessions.data?.find((s) => s.status === "in_progress" || s.status === "created");
  const categories = cats.data?.categories ?? [];
  const primary = categories.find((c) => c.is_primary) ?? categories[0];
  const recentInv = (invitations.data ?? []).slice(0, 3);
  const name = profile.data?.first_name ? `${profile.data.first_name}` : user?.email;

  return (
    <>
      <PageHeader
        eyebrow="кабинет кандидата"
        title={`Здравствуйте, ${name}`}
        description={
          primary
            ? primary.status === "confirmed" || primary.status === "upgrade_suggested"
              ? "Категория подтверждена — работодатели видят вас в подборках. Следите за приглашениями."
              : "Грейд пока не подтверждён: вы в подборках ниже подтверждённых кандидатов. Можно пройти тест на уровень ниже или повторить позже."
            : "Пройдите онбординг: анкета, опрос и адаптивный тест определят вашу категорию."
        }
        actions={
          profile.data && (
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => void downloadProfilePdf()}>
              <FileText className="size-4" /> PDF-профиль
            </button>
          )
        }
      />

      {inProgress && (
        <Card className="mb-4 border-l-4 border-l-brand-red">
          <CardBody className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div className="font-semibold">Незавершённый тест: {category(inProgress.specialization, inProgress.target_grade)}</div>
              <div className="text-[13px] text-muted">
                Отвечено {inProgress.items_answered} из ≤ {inProgress.max_items}. Сессия истекает {relative(inProgress.expires_at)}.
              </div>
            </div>
            <LinkButton to={`/candidate/tests/${inProgress.id}`}>
              Продолжить <ArrowRight className="size-4" />
            </LinkButton>
          </CardBody>
        </Card>
      )}

      <div className="grid gap-4 lg:grid-cols-[1.3fr_1fr]">
        <div className="flex flex-col gap-4">
          {cats.isLoading ? (
            <Skeleton className="h-40" />
          ) : categories.length === 0 ? (
            <EmptyState
              title={survey.data ? "Категория ещё не определена" : "Начните с онбординга"}
              description={survey.data ? "Опрос пройден — осталось пройти тест." : "Три минуты на анкету и опрос, около двадцати — на адаптивный тест. После него вы попадёте в карту категорий работодателей."}
              action={<LinkButton to="/candidate/onboarding">{survey.data ? "Пройти тест" : "Начать"}</LinkButton>}
            />
          ) : (
            categories.map((c) => (
              <Card key={`${c.specialization}-${c.grade}`} className={c.is_primary ? "border-brand-blue" : ""}>
                <CardBody className="flex flex-wrap items-start justify-between gap-4">
                  <div>
                    <div className="label-mono">{c.is_primary ? "основная категория" : "категория"}</div>
                    <div className="num-display mt-1 !text-[30px]">{category(c.specialization, c.grade)}</div>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                      <StatusBadge status={c.status} label={GRADE_STATUS[c.status] ?? c.status} />
                      {c.percentile !== undefined && <Badge tone="blue">{percent(c.percentile)} процентиль</Badge>}
                      {c.confirmed_at && <span className="text-[12px] text-muted">подтверждено {dateShort(c.confirmed_at)}</span>}
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-2 text-right">
                    {c.can_test_higher ? (
                      <LinkButton to="/candidate/onboarding" variant="secondary" size="sm">
                        Тест на грейд выше
                      </LinkButton>
                    ) : c.next_test_allowed_at ? (
                      <span className="text-[12px] text-muted">следующий тест — с {dateShort(c.next_test_allowed_at)}</span>
                    ) : null}
                  </div>
                </CardBody>
              </Card>
            ))
          )}

          <Card>
            <CardHeader title="Приглашения" action={<Link to="/candidate/invitations" className="text-[13px]">все →</Link>} />
            <CardBody className="!p-0">
              {invitations.isLoading && <Skeleton className="m-4 h-16" />}
              {invitations.data && recentInv.length === 0 && (
                <p className="px-5 py-4 text-[14px] text-muted">
                  Пока нет приглашений. Они приходят от работодателей, которые нашли вашу категорию в карте; каждое — с зарплатной вилкой.
                </p>
              )}
              {recentInv.map((inv) => (
                <Link key={inv.id} to={`/candidate/invitations?open=${inv.id}`} className="flex items-center gap-3 border-b border-line-soft px-5 py-3 no-underline last:border-0 hover:bg-paper/60">
                  <Mail className="size-4 shrink-0 text-brand-blue" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[14px] font-medium text-ink">{inv.company_name ?? "Компания"}</div>
                    <div className="text-[12px] text-muted">{relative(inv.sent_at)}</div>
                  </div>
                  <SalaryRange from={inv.salary_from} to={inv.salary_to} className="text-[13px] text-ink" />
                  <StatusBadge status={inv.status} label={INVITATION_STATUS[inv.status] ?? inv.status} />
                </Link>
              ))}
            </CardBody>
          </Card>
        </div>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader title="Заполненность профиля" description="Чем полнее анкета, тем точнее подбор" action={<Link to="/candidate/profile" className="text-[13px]">править</Link>} />
            <CardBody>
              {profile.isLoading ? (
                <Skeleton className="h-6" />
              ) : (
                <>
                  <div className="flex items-end justify-between">
                    <span className="num-display text-brand-blue">{Math.round(profile.data?.completeness ?? 0)} %</span>
                    <span className="text-[12px] text-muted">{profile.data?.resume_uploaded ? "резюме загружено" : "резюме не загружено"}</span>
                  </div>
                  <div className="mt-2 h-2 overflow-hidden rounded-full bg-line-soft">
                    <div className="h-full bg-brand-blue" style={{ width: `${Math.min(100, Math.round(profile.data?.completeness ?? 0))}%` }} />
                  </div>
                  <ul className="mt-3 flex flex-col gap-1 text-[13px] text-muted">
                    <li>{(profile.data?.skills?.length ?? 0) > 0 ? "✓" : "○"} навыки: {profile.data?.skills?.length ?? 0}</li>
                    <li>{(profile.data?.experiences?.length ?? 0) > 0 ? "✓" : "○"} опыт: {profile.data?.experiences?.length ?? 0}</li>
                    <li>{profile.data?.salary_expected ? "✓" : "○"} ожидание по зарплате</li>
                  </ul>
                </>
              )}
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Достижения ФСП" description="Из реестра Федерации, с плюсом в ранжировании" />
            <CardBody>
              {link.isLoading ? (
                <Skeleton className="h-10" />
              ) : !link.data?.linked ? (
                <div className="flex flex-col gap-2">
                  <p className="text-[14px] text-muted">ФСП ID не привязан. Привязка подтянет ваши соревновательные результаты; их отсутствие ни на что не влияет.</p>
                  <Link to="/candidate/profile?tab=fsp" className="text-[13px]">
                    Привязать ФСП ID →
                  </Link>
                </div>
              ) : (achievements.data?.total ?? 0) === 0 ? (
                <p className="text-[14px] text-muted">ФСП ID привязан, достижений в реестре пока нет — это нормально: ранжирование идёт по тесту и профилю.</p>
              ) : (
                <ul className="flex flex-col gap-2">
                  {(achievements.data?.items ?? []).slice(0, 3).map((a) => (
                    <li key={a.external_id} className="flex items-start gap-2 text-[13px]">
                      <Award className="mt-0.5 size-4 shrink-0 text-brand-red" />
                      <span>
                        <span className="font-medium">{a.event_name}</span> <span className="text-muted">· {a.event_date.slice(0, 4)}</span>
                      </span>
                    </li>
                  ))}
                  {(achievements.data?.total ?? 0) > 3 && (
                    <li>
                      <Link to="/candidate/profile?tab=fsp" className="text-[13px]">
                        все {achievements.data?.total} →
                      </Link>
                    </li>
                  )}
                </ul>
              )}
            </CardBody>
          </Card>
        </div>
      </div>
    </>
  );
}
