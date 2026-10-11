import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, MapPin } from "lucide-react";
import { api, unwrap, errorMessage, isApiError } from "@/api/client";
import { useSession } from "@/api/session";
import { Badge, Button, Card, CardBody, Dialog, LinkButton, Skeleton, SalaryRange, Textarea, useToast } from "@/components/ui";
import { WORK_FORMATS, grade, spec, dateShort } from "@/lib/format";

export function VacancyDetail() {
  const { id = "" } = useParams();
  const { user } = useSession();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [cover, setCover] = useState("");
  const v = useQuery({ queryKey: ["vacancies", id], queryFn: async () => unwrap(await api.GET("/vacancies/{id}", { params: { path: { id } } })) });
  const apply = useMutation({
    mutationFn: async () => unwrap(await api.POST("/vacancies/{id}/apply", { params: { path: { id } }, body: { cover_letter: cover.trim() || undefined } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["candidate", "applications"] });
      setOpen(false);
      toast.ok("Отклик отправлен", "Компания получила ваши контакты и анкету");
      navigate("/candidate/applications");
    },
    onError: (e) => {
      if (isApiError(e) && e.is("profile_required")) {
        toast.push({ tone: "warn", title: "Сначала заполните анкету", description: "Работодатель увидит её вместе с откликом" });
        navigate("/candidate/onboarding");
        return;
      }
      toast.error("Не удалось откликнуться", errorMessage(e));
    },
  });

  if (v.isLoading) return <div className="mx-auto max-w-[900px] px-4 py-8"><Skeleton className="h-64" /></div>;
  if (v.error || !v.data)
    return (
      <div className="mx-auto max-w-[900px] px-4 py-8">
        <p className="text-danger">{errorMessage(v.error)}</p>
        <Link to="/vacancies" className="mt-3 inline-block">
          ← К списку вакансий
        </Link>
      </div>
    );
  const d = v.data;
  return (
    <div className="mx-auto max-w-[1000px] px-4 py-8 sm:px-6">
      <Link to="/vacancies" className="text-[13px] text-muted no-underline hover:text-ink">
        ← Все вакансии
      </Link>
      <div className="mt-3 grid gap-6 lg:grid-cols-[1fr_320px]">
        <div>
          <div className="label-mono">{spec(d.specialization)} · {grade(d.grade_min)}{d.grade_min !== d.grade_max ? ` – ${grade(d.grade_max)}` : ""}</div>
          <h1 className="mt-1">{d.title}</h1>
          <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[14px] text-muted">
            <span className="inline-flex items-center gap-1">
              <Building2 className="size-4" /> {d.company.name}
            </span>
            {(d.city || d.remote_ok) && (
              <span className="inline-flex items-center gap-1">
                <MapPin className="size-4" /> {[d.city, d.remote_ok ? "удалённо" : ""].filter(Boolean).join(" · ")}
              </span>
            )}
            {d.published_at && <span>опубликована {dateShort(d.published_at)}</span>}
          </div>
          <div className="mt-3 flex flex-wrap gap-1.5">
            {(d.work_formats ?? []).map((f) => (
              <Badge key={f}>{WORK_FORMATS[f] ?? f}</Badge>
            ))}
          </div>
          <Card className="mt-5">
            <CardBody>
              <p className="whitespace-pre-wrap text-[15px] leading-7">{d.description}</p>
            </CardBody>
          </Card>
          {((d.must_skills?.length ?? 0) > 0 || (d.nice_skills?.length ?? 0) > 0) && (
            <Card className="mt-4">
              <CardBody className="grid gap-4 sm:grid-cols-2">
                <div>
                  <div className="label-mono">обязательные навыки</div>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {(d.must_skills ?? []).map((s) => (
                      <Badge key={s.code} tone="ink">{s.name}</Badge>
                    ))}
                  </div>
                </div>
                <div>
                  <div className="label-mono">будет плюсом</div>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {(d.nice_skills ?? []).map((s) => (
                      <Badge key={s.code}>{s.name}</Badge>
                    ))}
                    {(d.nice_skills?.length ?? 0) === 0 && <span className="text-[13px] text-muted">—</span>}
                  </div>
                </div>
              </CardBody>
            </Card>
          )}
        </div>
        <aside className="flex flex-col gap-4">
          <Card>
            <CardBody>
              <div className="label-mono">вилка, ₽ в месяц</div>
              <SalaryRange from={d.salary_from} to={d.salary_to} className="mt-1 block text-[24px]" />
              <p className="mt-2 text-[12px] text-muted">Вилка — обязательное условие публикации на платформе.</p>
              <div className="mt-4">
                {!user && (
                  <LinkButton to={`/login?next=/vacancies/${id}`} className="w-full">
                    Войти, чтобы откликнуться
                  </LinkButton>
                )}
                {user?.role === "candidate" && (
                  <Button className="w-full" onClick={() => setOpen(true)}>
                    Откликнуться
                  </Button>
                )}
                {user?.role === "employer" && <p className="text-[13px] text-muted">Вы вошли как работодатель.</p>}
              </div>
            </CardBody>
          </Card>
          <Card>
            <CardBody>
              <div className="label-mono">компания</div>
              <div className="mt-1 font-semibold">{d.company.name}</div>
              {d.company.industry && <div className="text-[13px] text-muted">{d.company.industry}</div>}
              {d.company.city && <div className="text-[13px] text-muted">{d.company.city}</div>}
              {d.company.description && <p className="mt-2 text-[13px]">{d.company.description}</p>}
              {d.company.website && (
                <a href={d.company.website} target="_blank" rel="noreferrer" className="mt-2 inline-block text-[13px]">
                  {d.company.website}
                </a>
              )}
            </CardBody>
          </Card>
        </aside>
      </div>
      <Dialog
        open={open}
        onClose={() => setOpen(false)}
        title="Откликнуться на вакансию"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button onClick={() => apply.mutate()} loading={apply.isPending}>
              Отправить отклик
            </Button>
          </>
        }
      >
        <p className="text-[14px] text-muted">Откликаясь, вы открываете компании «{d.company.name}» свои контакты и анкету (правило платформы). Сопроводительное письмо необязательно.</p>
        <Textarea className="mt-3" rows={5} maxLength={4000} value={cover} onChange={(e) => setCover(e.target.value)} placeholder="Почему вам интересна эта роль" />
      </Dialog>
    </div>
  );
}
