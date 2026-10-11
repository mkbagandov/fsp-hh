import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, errorMessage, isApiError } from "@/api/client";
import { ek, useCompany } from "@/api/employer";
import { PageHeader } from "@/components/layout/AppShell";
import { Button, Card, CardBody, Field, Input, Skeleton, Textarea, useToast } from "@/components/ui";

/** Компания: создаётся при первом сохранении, пользователь становится владельцем. */
export function CompanyPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const company = useCompany();
  const setup = params.get("setup") === "1";
  const [f, setF] = useState({ name: "", inn: "", industry: "", city: "", website: "", description: "", site: "", email: "", telegram: "", position: "" });
  const [fields, setFields] = useState<Record<string, string>>({});
  useEffect(() => {
    const c = company.data;
    if (!c) return;
    setF((p) => ({ ...p, name: c.name, inn: c.inn ?? "", industry: c.industry ?? "", city: c.city ?? "", website: c.website ?? "", description: c.description ?? "", site: c.public_contacts?.site ?? "", email: c.public_contacts?.email ?? "", telegram: c.public_contacts?.telegram ?? "" }));
  }, [company.data]);
  const save = useMutation({
    mutationFn: async () => {
      const contacts: Record<string, string> = {};
      if (f.site.trim()) contacts.site = f.site.trim();
      if (f.email.trim()) contacts.email = f.email.trim();
      if (f.telegram.trim()) contacts.telegram = f.telegram.trim();
      return unwrap(
        await api.PUT("/employer/company", {
          body: { name: f.name.trim(), inn: f.inn.trim() || undefined, industry: f.industry.trim() || undefined, city: f.city.trim() || undefined, website: f.website.trim() || undefined, description: f.description.trim() || undefined, public_contacts: contacts, position: f.position.trim() || undefined },
        }),
      );
    },
    onSuccess: (created) => {
      setFields({});
      qc.setQueryData(ek.company, created);
      toast.ok(setup ? "Компания создана" : "Сохранено");
      if (setup) navigate("/employer", { replace: true });
    },
    onError: (e) => {
      if (isApiError(e)) setFields(e.fields);
      toast.error("Не сохранилось", errorMessage(e));
    },
  });
  if (company.isLoading) return <Skeleton className="h-64" />;
  return (
    <div className="mx-auto max-w-[820px]">
      <PageHeader eyebrow="работодатель" title={company.data ? "Компания" : "Расскажите о компании"} description={company.data ? "Название и публичные контакты видят кандидаты, принявшие приглашение." : "Это займёт минуту. Публичные контакты кандидат увидит только после того, как примет приглашение."} />
      <Card>
        <CardBody className="grid gap-4 sm:grid-cols-2">
          <Field label="Название" required error={fields.name} className="sm:col-span-2">{(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} maxLength={200} />}</Field>
          <Field label="ИНН" hint="10 или 12 цифр" error={fields.inn}>{(id) => <Input id={id} mono value={f.inn} onChange={(e) => setF({ ...f, inn: e.target.value.replace(/\D/g, "").slice(0, 12) })} />}</Field>
          <Field label="Отрасль" error={fields.industry}>{(id) => <Input id={id} value={f.industry} onChange={(e) => setF({ ...f, industry: e.target.value })} placeholder="финтех, ритейл, госуслуги…" />}</Field>
          <Field label="Город" error={fields.city}>{(id) => <Input id={id} value={f.city} onChange={(e) => setF({ ...f, city: e.target.value })} />}</Field>
          <Field label="Сайт" error={fields.website}>{(id) => <Input id={id} value={f.website} onChange={(e) => setF({ ...f, website: e.target.value })} placeholder="https://…" />}</Field>
          <Field label="О компании" className="sm:col-span-2" error={fields.description}>{(id) => <Textarea id={id} rows={3} maxLength={4000} value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />}</Field>
          <div className="sm:col-span-2 label-mono">публичные контакты (кандидату после принятия приглашения)</div>
          <Field label="Страница вакансий">{(id) => <Input id={id} value={f.site} onChange={(e) => setF({ ...f, site: e.target.value })} placeholder="https://…/jobs" />}</Field>
          <Field label="E-mail HR">{(id) => <Input id={id} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />}</Field>
          <Field label="Telegram">{(id) => <Input id={id} mono value={f.telegram} onChange={(e) => setF({ ...f, telegram: e.target.value })} placeholder="@hr" />}</Field>
          {!company.data && <Field label="Ваша должность">{(id) => <Input id={id} value={f.position} onChange={(e) => setF({ ...f, position: e.target.value })} placeholder="HR-менеджер, тимлид…" />}</Field>}
        </CardBody>
        <div className="flex justify-end border-t border-line-soft px-5 py-3">
          <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!f.name.trim()}>
            {company.data ? "Сохранить" : "Создать компанию"}
          </Button>
        </div>
      </Card>
    </div>
  );
}
