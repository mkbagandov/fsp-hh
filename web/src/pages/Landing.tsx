import { Link } from "react-router";
import { ArrowRight, BadgeCheck, Scale, ShieldCheck, Sparkles } from "lucide-react";
import { Logo, BrandPattern } from "@/components/brand/Logo";
import { LinkButton } from "@/components/ui";

const categories = [
  { spec: "backend", grades: [12, 48, 21] },
  { spec: "frontend", grades: [18, 37, 14] },
  { spec: "data_ml", grades: [9, 22, 8] },
  { spec: "devops", grades: [6, 19, 11] },
];
const gradeNames = ["Junior", "Middle", "Senior"];

/** Лендинг: объясняет обратную механику за один экран — категория вместо вакансии, вилка вместо «обсудим». */
export function Landing() {
  return (
    <>
      <section className="relative overflow-hidden bg-ink text-paper">
        <BrandPattern tone="dark" opacity={0.9} className="[mask-image:linear-gradient(90deg,transparent_35%,black_100%)]" />
        <div className="relative mx-auto grid max-w-[1200px] gap-10 px-4 py-16 sm:px-6 lg:grid-cols-[1.1fr_0.9fr] lg:py-24">
          <div className="animate-rise">
            <div className="label-mono !text-paper/60">платформа найма ит-специалистов</div>
            <h1 className="mt-4 text-[clamp(30px,4.6vw,54px)] leading-[1.08]">
              Не вакансии ищут людей.
              <br />
              <span className="text-brand-red">Работодатель</span> приглашает <span className="text-[#8f86ff]">подтверждённого</span> кандидата.
            </h1>
            <p className="mt-5 max-w-xl text-[16px] leading-7 text-paper/80">
              Кандидат проходит короткий адаптивный тест и попадает в категорию «специализация × грейд». Работодатель описывает потребность, видит карту
              категорий с реальными зарплатными ожиданиями и приглашает конкретных людей — всегда с вилкой.
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <LinkButton to="/register?role=candidate" size="lg">
                Я кандидат <ArrowRight className="size-4" />
              </LinkButton>
              <LinkButton to="/register?role=employer" size="lg" variant="secondary" className="!border-paper/40 !bg-transparent !text-paper hover:!bg-white/10">
                Я работодатель
              </LinkButton>
            </div>
            <div className="mono mt-10 flex flex-wrap gap-x-8 gap-y-2 text-[12px] text-paper/60">
              <span>тест ≈ 20 минут</span>
              <span>вилка обязательна</span>
              <span>контакты — только после ответа</span>
              <span>достижения ФСП — в плюс</span>
            </div>
          </div>
          <div className="animate-rise [animation-delay:120ms]">
            <CategoryMapPreview />
          </div>
        </div>
      </section>

      <section id="how" className="mx-auto max-w-[1200px] px-4 py-16 sm:px-6">
        <div className="label-mono">как это работает</div>
        <h2 className="mt-2">Две стороны, один механизм</h2>
        <div className="mt-8 grid gap-6 lg:grid-cols-2">
          <Steps
            title="Кандидату"
            accent="blue"
            steps={[
              ["Опрос и заявленный грейд", "Три минуты: специализация, стек, опыт, ожидание по зарплате."],
              ["Адаптивный тест", "12–20 заданий, сложность подстраивается под ответы. Результат — категория и процентиль."],
              ["Приглашения с вилкой", "Работодатель обязан указать деньги. Контакты открываются только после вашего «да»."],
            ]}
          />
          <Steps
            title="Работодателю"
            accent="red"
            steps={[
              ["Потребность вместо вакансии", "Опишите задачу текстом — помощник разберёт её в структуру: навыки, грейд, вилка."],
              ["Карта категорий", "Сколько подтверждённых людей в категории, их ожидания и покрытие вашей вилкой."],
              ["Объяснимая подборка", "Каждая строка — с фактами: тест, навыки, достижения ФСП, свежесть, условия."],
            ]}
          />
        </div>
      </section>

      <section className="border-y border-line-soft bg-surface">
        <div className="mx-auto grid max-w-[1200px] gap-8 px-4 py-14 sm:grid-cols-2 sm:px-6 lg:grid-cols-4">
          <Principle icon={<Scale className="size-5" />} title="Вилка обязательна" text="Нет приглашения без денег. Карта категорий показывает, какая вилка реально покрывает кандидатов." />
          <Principle icon={<ShieldCheck className="size-5" />} title="Контакты под контролем" text="Телефон и мессенджеры видит только компания, которой вы ответили «да» или сами откликнулись." />
          <Principle icon={<BadgeCheck className="size-5" />} title="Грейд подтверждён тестом" text="Категория — не самооценка, а результат адаптивного теста с вероятностью и процентилем." />
          <Principle icon={<Sparkles className="size-5" />} title="Достижения ФСП" text="Результаты соревнований из реестра Федерации учитываются в ранжировании и видны в профиле." />
        </div>
      </section>

      <section className="mx-auto max-w-[1200px] px-4 py-16 sm:px-6">
        <div className="card relative overflow-hidden px-6 py-10 text-center sm:px-10">
          <BrandPattern variant="dense" opacity={0.06} />
          <div className="relative">
            <Logo variant="full" height={44} className="mx-auto" />
            <h2 className="mt-6">Готовы попробовать?</h2>
            <p className="mx-auto mt-2 max-w-xl text-muted">Регистрация занимает минуту. Кандидаты сразу идут к тесту, работодатели — к карте категорий.</p>
            <div className="mt-6 flex flex-wrap justify-center gap-3">
              <LinkButton to="/register" size="lg">
                Создать аккаунт
              </LinkButton>
              <Link to="/vacancies" className="btn btn-ghost btn-lg no-underline">
                Смотреть вакансии
              </Link>
            </div>
          </div>
        </div>
      </section>
    </>
  );
}

function CategoryMapPreview() {
  return (
    <div className="rounded-md border border-white/15 bg-ink/85 p-4 shadow-md backdrop-blur-md">
      <div className="flex items-center justify-between">
        <span className="label-mono !text-paper/60">карта категорий · пример</span>
        <span className="badge badge-brand">live</span>
      </div>
      <div className="mt-3 grid grid-cols-[88px_repeat(3,1fr)] gap-1.5 text-[12px]">
        <div />
        {gradeNames.map((g) => (
          <div key={g} className="mono text-center text-paper/60">
            {g}
          </div>
        ))}
        {categories.map((c) => (
          <CategoryRow key={c.spec} spec={c.spec} grades={c.grades} />
        ))}
      </div>
      <p className="mt-3 text-[12px] text-paper/60">Числа — подтверждённые тестом кандидаты. В кабинете работодателя к ним добавляются ожидания по зарплате и покрытие вашей вилкой.</p>
    </div>
  );
}

function CategoryRow({ spec, grades }: { spec: string; grades: number[] }) {
  const max = Math.max(...grades);
  const label: Record<string, string> = { backend: "backend", frontend: "frontend", data_ml: "data / ml", devops: "devops" };
  return (
    <>
      <div className="mono self-center text-paper/80">{label[spec]}</div>
      {grades.map((n, i) => (
        <div key={i} className="mono rounded-[4px] border border-white/10 px-2 py-2 text-center" style={{ background: `rgb(64 47 255 / ${0.15 + (n / max) * 0.6})` }}>
          {n}
        </div>
      ))}
    </>
  );
}

function Steps({ title, steps, accent }: { title: string; steps: [string, string][]; accent: "blue" | "red" }) {
  const color = accent === "blue" ? "text-brand-blue" : "text-brand-red";
  return (
    <div className="card p-6">
      <h3 className="text-[18px]">{title}</h3>
      <ol className="mt-5 flex flex-col gap-5">
        {steps.map(([t, d], i) => (
          <li key={t} className="flex gap-4">
            <span className={`num-display ${color} w-10 shrink-0 !text-[34px]`}>{i + 1}</span>
            <div>
              <div className="mono text-[14px] font-semibold">{t}</div>
              <div className="mt-0.5 text-[14px] text-muted">{d}</div>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

function Principle({ icon, title, text }: { icon: React.ReactNode; title: string; text: string }) {
  return (
    <div>
      <div className="flex size-9 items-center justify-center rounded-sm bg-brand-blue-soft text-brand-blue">{icon}</div>
      <h3 className="mt-3 text-[15px]">{title}</h3>
      <p className="mt-1 text-[13.5px] text-muted">{text}</p>
    </div>
  );
}
