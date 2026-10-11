import { useState } from "react";
import { Logo, BrandPattern } from "@/components/brand/Logo";
import { Button, Field, Input, Textarea, Select, Checkbox, SegmentedControl, Card, CardHeader, CardBody, Badge, StatusBadge, Skeleton, EmptyState, Stat, DataTable, Tabs, Dialog, useToast, SalaryRange, SalaryInputs } from "@/components/ui";
import { INVITATION_STATUS } from "@/lib/format";

const swatches: [string, string, string][] = [
  ["--fsp-blue", "#402FFF", "основной интерактивный"],
  ["--fsp-red", "#EC1D35", "акцент ФСП"],
  ["--fsp-ink", "#1B1C21", "текст, тёмные поверхности"],
  ["--fsp-paper", "#EDEDED", "фон"],
  ["--fsp-navy", "#3D4E9C", "графики"],
  ["--fsp-violet", "#624190", "графики"],
  ["--ok", "#0F8A5F", "успех"],
  ["--warn", "#B45309", "предупреждение"],
  ["--danger", "#C8102E", "ошибка"],
];

type Row = { id: string; name: string; category: string; score: number; status: string };
const rows: Row[] = [
  { id: "1", name: "Анна Тестова", category: "backend × middle", score: 0.84, status: "sent" },
  { id: "2", name: "Борис Иванов", category: "backend × senior", score: 0.71, status: "accepted" },
  { id: "3", name: "Вера Смирнова", category: "frontend × middle", score: 0.63, status: "declined" },
];

/** Витрина дизайн-системы (dev): все компоненты в одном месте для проверки стиля и контраста. */
export function UiKit() {
  const toast = useToast();
  const [tab, setTab] = useState<"a" | "b" | "c">("a");
  const [role, setRole] = useState<"candidate" | "employer">("candidate");
  const [open, setOpen] = useState(false);
  const [salary, setSalary] = useState({ from: "200000", to: "300000" });

  return (
    <div className="mx-auto max-w-[1100px] px-4 py-8 sm:px-6">
      <div className="label-mono">dev · дизайн-система</div>
      <h1 className="mt-1">Компоненты ФСП</h1>
      <p className="mt-1 text-muted">JetBrains Mono — заголовки и цифры, Inter — текст, TDAText — акценты. Радиусы 6/10 px, сетка 8 px.</p>

      <Section title="Бренд">
        <div className="flex flex-wrap items-center gap-8">
          <Logo variant="short" height={40} />
          <Logo variant="full" height={40} />
          <Logo variant="mark" height={40} />
          <div className="relative h-14 w-40 rounded-md bg-ink">
            <Logo tone="dark" height={28} className="absolute left-3 top-3" />
          </div>
        </div>
        <div className="relative mt-4 h-24 overflow-hidden rounded-md border border-line-soft bg-surface">
          <BrandPattern variant="banner" opacity={0.12} />
        </div>
        <div className="mt-4 grid grid-cols-3 gap-2 sm:grid-cols-5 lg:grid-cols-9">
          {swatches.map(([name, hex, role]) => (
            <div key={name} className="card overflow-hidden">
              <div className="h-12" style={{ background: hex }} />
              <div className="px-2 py-1.5">
                <div className="mono text-[11px]">{hex}</div>
                <div className="text-[11px] text-muted">{role}</div>
              </div>
            </div>
          ))}
        </div>
      </Section>

      <Section title="Типографика">
        <h1>Заголовок первого уровня</h1>
        <h2 className="mt-2">Заголовок второго уровня</h2>
        <h3 className="mt-2">Заголовок третьего уровня</h3>
        <p className="mt-3 max-w-2xl">Основной текст набирается Inter 15 px с межстрочным 1,5. Длинные описания, письма и формы — только им. Числа и коды — моноширинным: <code>backend × middle</code>, <span className="mono">θ = 0.84</span>.</p>
        <div className="mt-3 flex items-end gap-6">
          <span className="num-display text-brand-blue">84 %</span>
          <span className="num-display text-brand-red">12</span>
          <span className="label-mono">подпись в капсе</span>
        </div>
      </Section>

      <Section title="Кнопки">
        <div className="flex flex-wrap items-center gap-3">
          <Button>Основная</Button>
          <Button variant="secondary">Вторичная</Button>
          <Button variant="ghost">Призрачная</Button>
          <Button variant="dark">Тёмная</Button>
          <Button variant="danger">Опасная</Button>
          <Button loading>Загрузка</Button>
          <Button disabled>Недоступна</Button>
          <Button size="sm">Малая</Button>
          <Button size="lg">Большая</Button>
        </div>
      </Section>

      <Section title="Поля">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="E-mail" hint="Подсказка под полем" required>
            {(id, by) => <Input id={id} aria-describedby={by} placeholder="name@example.com" />}
          </Field>
          <Field label="С ошибкой" error="Укажите значение">
            {(id, by) => <Input id={id} aria-describedby={by} invalid defaultValue="неверно" />}
          </Field>
          <Field label="Специализация">
            {(id) => (
              <Select
                id={id}
                placeholder="Выберите"
                options={[
                  { value: "backend", label: "Backend" },
                  { value: "frontend", label: "Frontend" },
                ]}
              />
            )}
          </Field>
          <Field label="Переключатель">
            {() => (
              <SegmentedControl
                value={role}
                onChange={setRole}
                ariaLabel="Роль"
                options={[
                  { value: "candidate", label: "Кандидат" },
                  { value: "employer", label: "Работодатель" },
                ]}
              />
            )}
          </Field>
          <Field label="Описание" className="sm:col-span-2">
            {(id) => <Textarea id={id} placeholder="Расскажите о задаче" />}
          </Field>
          <div className="sm:col-span-2">
            <SalaryInputs from={salary.from} to={salary.to} onChange={setSalary} />
          </div>
          <Checkbox label="Согласие на показ профиля работодателям" description="Можно отозвать в любой момент" defaultChecked />
        </div>
      </Section>

      <Section title="Бейджи и статусы">
        <div className="flex flex-wrap gap-2">
          <Badge>neutral</Badge>
          <Badge tone="blue">blue</Badge>
          <Badge tone="red">red</Badge>
          <Badge tone="brand">ФСП</Badge>
          <Badge tone="ok">ok</Badge>
          <Badge tone="warn">warn</Badge>
          <Badge tone="ink">ink</Badge>
          {Object.entries(INVITATION_STATUS).map(([k, v]) => (
            <StatusBadge key={k} status={k} label={v} />
          ))}
        </div>
      </Section>

      <Section title="Карточки и метрики">
        <div className="grid gap-4 sm:grid-cols-3">
          <Stat label="кандидатов в категории" value="48" hint="подтверждённых — 31" />
          <Stat label="покрытие вилкой" value="62 %" tone="blue" />
          <Stat label="достижения ФСП" value="7" tone="red" hint="среди подтверждённых" />
        </div>
        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <Card>
            <CardHeader title="Карточка" description="Заголовок, описание, действие" action={<Button size="sm" variant="secondary">Действие</Button>} />
            <CardBody>
              Тело карточки. Вилка: <SalaryRange from={200000} to={300000} />
            </CardBody>
          </Card>
          <div className="card-dark p-5">
            <div className="label-mono !text-paper/60">тёмная карточка</div>
            <div className="num-display mt-2 text-white">backend × senior</div>
          </div>
        </div>
      </Section>

      <Section title="Таблица">
        <DataTable
          rows={rows}
          rowKey={(r) => r.id}
          columns={[
            { key: "name", header: "Кандидат", render: (r) => <span className="font-medium">{r.name}</span> },
            { key: "cat", header: "Категория", render: (r) => <span className="mono">{r.category}</span> },
            { key: "score", header: "Score", render: (r) => <span className="mono">{r.score.toFixed(2)}</span>, width: "90px" },
            { key: "status", header: "Статус", render: (r) => <StatusBadge status={r.status} label={INVITATION_STATUS[r.status]} /> },
          ]}
        />
      </Section>

      <Section title="Табы, диалог, тосты, скелетоны, пустое состояние">
        <Tabs
          value={tab}
          onChange={setTab}
          ariaLabel="Пример"
          items={[
            { value: "a", label: "Подборка", count: 12 },
            { value: "b", label: "Приглашения", count: 3 },
            { value: "c", label: "Архив" },
          ]}
        />
        <div className="mt-4 flex flex-wrap gap-3">
          <Button variant="secondary" onClick={() => setOpen(true)}>
            Открыть диалог
          </Button>
          <Button variant="ghost" onClick={() => toast.ok("Приглашение отправлено", "Кандидат получит уведомление")}>
            Тост: успех
          </Button>
          <Button variant="ghost" onClick={() => toast.error("Не удалось сохранить", "Вилка ниже минимума")}>
            Тост: ошибка
          </Button>
        </div>
        <div className="mt-4 grid gap-3 sm:grid-cols-3">
          <Skeleton className="h-20" />
          <Skeleton className="h-20" />
          <Skeleton className="h-20" />
        </div>
        <div className="mt-4">
          <EmptyState title="Подборка пока пуста" description="Опишите потребность — помощник разберёт её в структуру, а карта категорий подскажет вилку." action={<Button>Создать потребность</Button>} />
        </div>
        <Dialog open={open} onClose={() => setOpen(false)} title="Пригласить кандидата" footer={<><Button variant="ghost" onClick={() => setOpen(false)}>Отмена</Button><Button onClick={() => setOpen(false)}>Отправить</Button></>}>
          <p className="text-[14px] text-muted">Вилка обязательна, способ связи раскрывается кандидату после принятия.</p>
        </Dialog>
      </Section>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mt-10">
      <h2 className="mb-4 border-b border-line pb-2 text-[18px]">{title}</h2>
      {children}
    </section>
  );
}
