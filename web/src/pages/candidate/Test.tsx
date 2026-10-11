import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, XCircle } from "lucide-react";
import { api, unwrap, errorMessage, isApiError } from "@/api/client";
import { qk, type ItemView, type SessionView, type Submission, type ItemReview } from "@/api/candidate";
import { Stem, CodeView } from "@/components/test/Stem";
import { AnswerInput, initialSubmission, isAnswered } from "@/components/test/AnswerInput";
import { Badge, Button, Card, CardBody, Skeleton, Stat, StatusBadge, useToast } from "@/components/ui";
import { grade, spec, percent, GRADE_STATUS } from "@/lib/format";
import { Logo } from "@/components/brand/Logo";

const FORMAT_LABEL: Record<string, string> = {
  single_choice: "один вариант",
  multi_choice: "несколько вариантов",
  numeric: "число",
  short_text: "короткий ответ",
  code_output: "вывод программы",
  ordering: "порядок",
  sql_result: "результат запроса",
};

/** Экран адаптивного теста: одно задание за раз, таймер по дедлайну, телеметрия, итог с разбором. */
export function TestPage() {
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();
  const [item, setItem] = useState<ItemView | null>(null);
  const [value, setValue] = useState<Submission>({});
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  const session = useQuery({ queryKey: qk.session(id), queryFn: async () => unwrap(await api.GET("/candidate/tests/{id}", { params: { path: { id } } })) });

  const loadItem = useCallback(async () => {
    try {
      const r = unwrap(await api.GET("/candidate/tests/{id}/item", { params: { path: { id } } }));
      if (r.finished || !r.item) {
        setItem(null);
        await qc.invalidateQueries({ queryKey: qk.session(id) });
        await qc.invalidateQueries({ queryKey: qk.categories });
        return;
      }
      setItem(r.item);
      setValue(initialSubmission(r.item));
    } catch (e) {
      setLoadError(errorMessage(e));
    }
  }, [id, qc]);

  useEffect(() => {
    if (session.data?.status === "in_progress" || session.data?.status === "created") void loadItem();
  }, [session.data?.status, loadItem]);

  // Телеметрия: потеря фокуса, скрытие вкладки, вставка. Не влияет на результат автоматически.
  useEffect(() => {
    if (!item) return;
    const send = (kind: "blur" | "paste" | "copy" | "hidden" | "resize" | "devtools") => {
      void api.POST("/candidate/tests/{id}/events", { params: { path: { id } }, body: { kind } });
    };
    const onBlur = () => send("blur");
    const onVis = () => document.visibilityState === "hidden" && send("hidden");
    const onPaste = () => send("paste");
    window.addEventListener("blur", onBlur);
    document.addEventListener("visibilitychange", onVis);
    document.addEventListener("paste", onPaste);
    return () => {
      window.removeEventListener("blur", onBlur);
      document.removeEventListener("visibilitychange", onVis);
      document.removeEventListener("paste", onPaste);
    };
  }, [item, id]);

  const submit = useCallback(
    async (auto = false) => {
      if (!item || busy) return;
      setBusy(true);
      try {
        const r = unwrap(await api.POST("/candidate/tests/{id}/answer", { params: { path: { id } }, body: { item_id: item.id, submission: value } }));
        if (r.late && !auto) toast.push({ tone: "warn", title: "Время вышло", description: "Ответ принят, но засчитан как неверный" });
        if (r.finished) {
          setItem(null);
          await qc.invalidateQueries({ queryKey: qk.session(id) });
          await qc.invalidateQueries({ queryKey: qk.categories });
          await qc.invalidateQueries({ queryKey: qk.sessions });
        } else {
          await loadItem();
        }
      } catch (e) {
        if (isApiError(e) && (e.is("item_already_answered") || e.is("session_closed") || e.is("session_expired"))) {
          await loadItem();
          await qc.invalidateQueries({ queryKey: qk.session(id) });
        } else {
          toast.error("Не удалось отправить ответ", errorMessage(e));
        }
      } finally {
        setBusy(false);
      }
    },
    [item, busy, id, value, qc, loadItem, toast],
  );

  if (session.isLoading) return <Skeleton className="h-64" />;
  if (session.error || !session.data) return <p className="text-danger">{errorMessage(session.error)}</p>;
  const s = session.data;

  if (s.status === "completed" || s.status === "abandoned" || s.status === "expired") {
    return <ResultScreen session={s} />;
  }

  return (
    <div className="mx-auto max-w-[860px]">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Logo variant="mark" height={26} />
          <div>
            <div className="label-mono">адаптивный тест</div>
            <div className="mono text-[14px] font-semibold">
              {spec(s.specialization)} × {grade(s.target_grade)}
            </div>
          </div>
        </div>
        <Progress answered={item?.items_answered ?? s.items_answered} max={s.max_items} min={s.min_items} />
      </div>
      {loadError && <p className="mb-3 rounded-sm bg-danger-soft px-3 py-2 text-[13px] text-danger">{loadError}</p>}
      {!item && !loadError && <Skeleton className="h-48" />}
      {item && (
        <Card className="animate-fade">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line-soft px-5 py-3">
            <div className="flex items-center gap-2">
              <span className="mono text-[13px] text-muted">#{item.position}</span>
              <Badge tone="ink">{item.topic}</Badge>
              <Badge>{FORMAT_LABEL[item.format] ?? item.format}</Badge>
            </div>
            <Countdown key={item.id} deadline={item.deadline_at} total={item.time_limit_sec} onExpire={() => void submit(true)} />
          </div>
          <CardBody className="!py-5">
            <Stem text={item.stem} />
            {item.code && <CodeView source={item.code.source} lang={item.code.lang} />}
            <div className="mt-5">
              <AnswerInput item={item} value={value} onChange={setValue} disabled={busy} />
            </div>
          </CardBody>
          <div className="flex items-center justify-between gap-3 border-t border-line-soft px-5 py-3">
            <span className="text-[12px] text-muted">Правильность не показывается до конца теста. Назад вернуться нельзя.</span>
            <Button onClick={() => void submit()} loading={busy} disabled={!isAnswered(item, value)}>
              Ответить
            </Button>
          </div>
        </Card>
      )}
      <div className="mt-4 text-right">
        <button
          type="button"
          className="text-[12px] text-muted underline-offset-2 hover:underline"
          onClick={async () => {
            if (!window.confirm("Прервать тест? Результат не будет засчитан, повторная попытка — после cooldown.")) return;
            await api.POST("/candidate/tests/{id}/abandon", { params: { path: { id } } });
            await qc.invalidateQueries({ queryKey: ["candidate"] });
            navigate("/candidate");
          }}
        >
          Прервать тест
        </button>
      </div>
    </div>
  );
}

function Progress({ answered, max, min }: { answered: number; max: number; min: number }) {
  const pct = Math.min(100, Math.round((answered / max) * 100));
  return (
    <div className="min-w-[200px]">
      <div className="mono flex justify-between text-[12px] text-muted">
        <span>
          задание {Math.min(answered + 1, max)} из ≤ {max}
        </span>
        <span>мин. {min}</span>
      </div>
      <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-line-soft">
        <div className="h-full bg-brand-blue transition-[width] duration-500" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

function Countdown({ deadline, total, onExpire }: { deadline: string; total: number; onExpire: () => void }) {
  const [left, setLeft] = useState(() => Math.max(0, Math.round((Date.parse(deadline) - Date.now()) / 1000)));
  const fired = useRef(false);
  useEffect(() => {
    const t = window.setInterval(() => {
      const l = Math.max(0, Math.round((Date.parse(deadline) - Date.now()) / 1000));
      setLeft(l);
      if (l === 0 && !fired.current) {
        fired.current = true;
        onExpire();
      }
    }, 500);
    return () => window.clearInterval(t);
  }, [deadline, onExpire]);
  const m = Math.floor(left / 60);
  const sec = left % 60;
  const danger = left <= 15;
  return (
    <div className={`mono flex items-center gap-2 text-[14px] font-semibold tabular-nums ${danger ? "text-danger" : "text-ink"}`} aria-live={danger ? "polite" : "off"}>
      <span className="relative inline-block size-4">
        <svg viewBox="0 0 36 36" className="size-4 -rotate-90">
          <circle cx="18" cy="18" r="15" fill="none" stroke="#e2e3e5" strokeWidth="4" />
          <circle cx="18" cy="18" r="15" fill="none" stroke={danger ? "#c8102e" : "#402fff"} strokeWidth="4" strokeDasharray={`${(left / total) * 94.2} 94.2`} />
        </svg>
      </span>
      {m}:{sec.toString().padStart(2, "0")}
    </div>
  );
}

/** Итог: решение по грейду, θ и процентиль, разбор заданий с пояснениями. */
function ResultScreen({ session }: { session: SessionView }) {
  const r = session.result;
  const review = session.review ?? [];
  return (
    <div className="mx-auto max-w-[900px]">
      <div className="label-mono">результат теста</div>
      <h1 className="mt-1">
        {spec(session.specialization)} × {grade(session.target_grade)}
      </h1>
      {session.status !== "completed" && (
        <p className="mt-2 rounded-sm bg-warn-soft px-3 py-2 text-[13px] text-warn">Сессия {session.status === "expired" ? "истекла" : "прервана"} — результат не засчитан.</p>
      )}
      {r && (
        <>
          <div className="mt-5 grid gap-3 sm:grid-cols-4">
            <Stat label="решение" value={<span className="block !text-[18px] leading-tight">{GRADE_STATUS[r.decision] ?? r.decision}</span>} tone={r.decision === "not_confirmed" ? "red" : "blue"} hint={r.result_grade ? `грейд: ${grade(r.result_grade)}` : undefined} />
            <Stat label="процентиль" value={percent(r.percentile)} hint="среди кандидатов категории" />
            <Stat label="θ ± SE" value={<span className="!text-[26px]">{r.theta.toFixed(2)} ± {r.se.toFixed(2)}</span>} />
            <Stat label="верных" value={`${r.correct_count}/${r.items_count}`} hint={`P(подтверждён) ${percent(r.p_confirm)}`} />
          </div>
          <p className="mt-4 text-[15px]">{r.message}</p>
          {r.next_test_allowed_at && <p className="mt-1 text-[13px] text-muted">Следующая попытка в этой специализации — не раньше {new Date(r.next_test_allowed_at).toLocaleDateString("ru-RU")}.</p>}
        </>
      )}
      <div className="mt-6 flex gap-2">
        <Link to="/candidate" className="btn btn-primary no-underline">
          В кабинет
        </Link>
        <Link to="/candidate/invitations" className="btn btn-secondary no-underline">
          Приглашения
        </Link>
      </div>
      {review.length > 0 && (
        <section className="mt-10">
          <h2>Разбор заданий</h2>
          <p className="mt-1 text-[13px] text-muted">Пояснения помогают понять, где именно не хватило знаний. Сами задания в следующих попытках будут другими.</p>
          <div className="mt-4 flex flex-col gap-3">
            {review.map((it) => (
              <ReviewCard key={it.position} item={it} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

function ReviewCard({ item }: { item: ItemReview }) {
  const ok = item.is_correct;
  return (
    <Card>
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line-soft px-5 py-3">
        <div className="flex items-center gap-2">
          <span className="mono text-[13px] text-muted">#{item.position}</span>
          <Badge tone="ink">{item.topic}</Badge>
          <Badge>{FORMAT_LABEL[item.format] ?? item.format}</Badge>
        </div>
        {ok === undefined ? (
          <StatusBadge status="viewed" label="без ответа" />
        ) : ok ? (
          <span className="inline-flex items-center gap-1 text-[13px] text-ok">
            <CheckCircle2 className="size-4" /> верно
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 text-[13px] text-danger">
            <XCircle className="size-4" /> неверно
          </span>
        )}
      </div>
      <CardBody>
        <Stem text={item.stem} className="!text-[14px] !leading-6" />
        {item.code && <CodeView source={item.code.source} lang={item.code.lang} />}
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          <AnswerBox title="Ваш ответ" value={describe(item, item.submission)} tone={ok === false ? "bad" : "neutral"} />
          <AnswerBox title="Верный ответ" value={describe(item, item.answer)} tone="good" />
        </div>
        <div className="mt-3 flex gap-2 rounded-md bg-brand-blue-soft/50 px-4 py-3 text-[14px]">
          <AlertTriangle className="mt-0.5 size-4 shrink-0 text-brand-blue" />
          <span>{item.explanation}</span>
        </div>
      </CardBody>
    </Card>
  );
}

function AnswerBox({ title, value, tone }: { title: string; value: string; tone: "good" | "bad" | "neutral" }) {
  const cls = tone === "good" ? "border-ok/40 bg-ok-soft/50" : tone === "bad" ? "border-danger/40 bg-danger-soft/50" : "border-line-soft bg-paper";
  return (
    <div className={`rounded-md border px-3 py-2 ${cls}`}>
      <div className="label-mono">{title}</div>
      <div className="mono mt-1 whitespace-pre-wrap text-[13px]">{value || "—"}</div>
    </div>
  );
}

function describe(item: ItemReview, v: { keys?: string[] | null; number?: number; text?: string; order?: string[] | null; rows?: (string[] | null)[] | null } | undefined): string {
  if (!v) return "";
  const opt = new Map((item.options ?? []).map((o) => [o.key, o.text]));
  if (v.keys?.length) return v.keys.map((k) => `${k}) ${opt.get(k) ?? ""}`).join("\n");
  if (v.order?.length) return v.order.map((k, i) => `${i + 1}. ${opt.get(k) ?? k}`).join("\n");
  if (v.rows?.length) return v.rows.map((r) => (r ?? []).join(", ")).join("\n");
  if (typeof v.number === "number") return String(v.number);
  return v.text ?? "";
}
