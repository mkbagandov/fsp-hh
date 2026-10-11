import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { Card, CardBody, EmptyState, Skeleton } from "@/components/ui";
import { dateTime } from "@/lib/format";

type Message = { to: string; subject: string; text: string; sent_at: string };

/** Письма консольного мейлера (только dev): коды подтверждения и уведомления. */
export function Outbox() {
  // Операция только для dev и не входит в экспорт OpenAPI, поэтому запрос без типизированного клиента.
  const q = useQuery({
    queryKey: ["dev", "outbox"],
    queryFn: async (): Promise<Message[]> => {
      const res = await fetch("/api/v1/dev/outbox", { headers: { Accept: "application/json" } });
      if (!res.ok) throw new Error(res.status === 404 ? "Страница доступна только на dev-стенде" : `HTTP ${res.status}`);
      return ((await res.json()) as { messages: Message[] | null }).messages ?? [];
    },
    refetchInterval: 3000,
  });
  return (
    <div className="mx-auto max-w-[900px] px-4 py-8 sm:px-6">
      <div className="label-mono">dev</div>
      <h1 className="mt-1">Исходящие письма</h1>
      <p className="mt-1 text-[14px] text-muted">
        Стенд не отправляет настоящую почту: всё, что ушло бы пользователю, появляется здесь. Коды подтверждения можно вставить в <Link to="/verify">форму</Link>.
      </p>
      <div className="mt-6 flex flex-col gap-3">
        {q.isLoading && [1, 2].map((i) => <Skeleton key={i} className="h-24" />)}
        {q.error && <EmptyState title="Не удалось загрузить" description={String(q.error)} pattern={false} />}
        {q.data?.length === 0 && <EmptyState title="Писем пока нет" description="Зарегистрируйтесь или запросите код — письмо появится здесь через пару секунд." />}
        {q.data?.map((m, i) => (
          <Card key={`${m.sent_at}-${i}`}>
            <CardBody>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="mono text-[13px]">
                  <span className="text-muted">кому </span>
                  {m.to}
                </div>
                <div className="mono text-[12px] text-muted">{dateTime(m.sent_at)}</div>
              </div>
              <div className="mt-1 font-semibold">{m.subject}</div>
              <pre className="mt-2 whitespace-pre-wrap rounded-sm bg-paper p-3 text-[13px]">{highlightCodes(m.text)}</pre>
            </CardBody>
          </Card>
        ))}
      </div>
    </div>
  );
}

function highlightCodes(text: string) {
  const parts = text.split(/(\b\d{6}\b)/g);
  return parts.map((p, i) => (/^\d{6}$/.test(p) ? <mark key={i} className="mono rounded-sm bg-brand-blue-soft px-1 text-brand-blue">{p}</mark> : <span key={i}>{p}</span>));
}
