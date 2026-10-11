package llm

import (
	"context"
	"strings"
	"testing"
)

const sampleResume = `Иванов Иван Иванович
Мужчина, 29 лет, родился 12 мая 1997
+7 (916) 123-45-67
ivanov@example.com
Telegram: @ivanov_dev
Проживает: Москва
Желаемая должность и зарплата
Backend-разработчик (Go)
Специализации: Программист, разработчик
Опыт работы — 5 лет 3 месяца
Июнь 2021 — настоящее время
3 года 4 месяца
Компания «Рога и копыта»
Старший разработчик Go
Разработка микросервисов на Go, PostgreSQL, Kafka, Kubernetes. gRPC, REST, Docker.
Март 2019 — Май 2021
2 года 3 месяца
ООО «Старт»
Python-разработчик
Django, PostgreSQL, Redis, Docker, Git.
Образование
Высшее
2018 МГТУ им. Баумана, Информатика и системы управления
Навыки
Go, Golang, PostgreSQL, Docker, Kubernetes, Git, Linux, CI/CD, SQL
О себе
Люблю чистый код и наставничество. github.com/ivanov-dev
`

func TestMockExtractResume(t *testing.T) {
	m := NewMock()
	d, usage, err := m.ExtractResume(context.Background(), sampleResume)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Provider != "mock" {
		t.Error("usage provider")
	}
	if d.FirstName != "Иван" || d.LastName != "Иванов" || d.MiddleName != "Иванович" {
		t.Errorf("name: %q %q %q", d.FirstName, d.LastName, d.MiddleName)
	}
	if d.Email != "ivanov@example.com" || d.Phone != "+79161234567" || d.Telegram != "ivanov_dev" {
		t.Errorf("contacts: %q %q %q", d.Email, d.Phone, d.Telegram)
	}
	if d.City != "Москва" {
		t.Errorf("city: %q", d.City)
	}
	if !strings.Contains(d.Headline, "Backend") {
		t.Errorf("headline: %q", d.Headline)
	}
	if d.Links["github"] != "https://github.com/ivanov-dev" {
		t.Errorf("links: %v", d.Links)
	}
	if len(d.Experiences) != 2 {
		t.Fatalf("experiences: %+v", d.Experiences)
	}
	e0, e1 := d.Experiences[0], d.Experiences[1]
	if e0.StartedOn != "2021-06-01" || e0.EndedOn != "" || !strings.Contains(e0.Company, "Рога") || !strings.Contains(e0.Position, "Старший") {
		t.Errorf("exp0: %+v", e0)
	}
	if e1.StartedOn != "2019-03-01" || e1.EndedOn != "2021-05-01" || !strings.Contains(e1.Company, "Старт") {
		t.Errorf("exp1: %+v", e1)
	}
	if d.ExperienceYears < 5 {
		t.Errorf("experience years: %v", d.ExperienceYears)
	}
	if len(d.Education) != 1 || d.Education[0].GraduatedYear != 2018 || !strings.Contains(d.Education[0].Institution, "МГТУ") {
		t.Errorf("education: %+v", d.Education)
	}
	codes := map[string]bool{}
	for _, s := range d.Skills {
		codes[s.Code] = true
	}
	if !codes["go"] || len(d.Skills) < 3 {
		t.Errorf("skills: %+v", d.Skills)
	}
	if d.PrimarySpecialization != "backend" {
		t.Errorf("specialization: %q", d.PrimarySpecialization)
	}
}

func TestMockExtractNeed(t *testing.T) {
	m := NewMock()
	text := `Ищем Senior Go-разработчика в команду платёжного шлюза.
Обязательно: Go, PostgreSQL, Kafka, опыт от 5 лет.
Будет плюсом: Kubernetes, Grafana.
Вилка: от 300 до 420 тыс. руб. на руки. Гибрид, офис в Москве, возможна удалёнка.`
	d, _, err := m.ExtractNeed(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if d.GradeMin != "senior" || d.GradeMax != "senior" {
		t.Errorf("grade: %q %q", d.GradeMin, d.GradeMax)
	}
	if d.SalaryFrom != 300000 || d.SalaryTo != 420000 {
		t.Errorf("salary: %d %d", d.SalaryFrom, d.SalaryTo)
	}
	must := strings.Join(d.MustSkills, ",")
	nice := strings.Join(d.NiceSkills, ",")
	if !strings.Contains(must, "go") || strings.Contains(must, "kubernetes") {
		t.Errorf("must: %v", d.MustSkills)
	}
	if !strings.Contains(nice, "kubernetes") {
		t.Errorf("nice: %v", d.NiceSkills)
	}
	if d.City != "Москва" || !d.RemoteOK || len(d.WorkFormats) < 2 {
		t.Errorf("format/city: %+v", d)
	}
	if d.Specialization != "backend" {
		t.Errorf("specialization: %q", d.Specialization)
	}
	short, _, _ := m.ExtractNeed(context.Background(), "Нужен junior frontend разработчик на React, удалённо")
	if short.SalaryFrom != 0 || len(short.Warnings) == 0 || short.GradeMin != "junior" {
		t.Errorf("short need: %+v", short)
	}
}

func TestMockShortResume(t *testing.T) {
	d, _, err := NewMock().ExtractResume(context.Background(), "коротко")
	if err != nil || len(d.Warnings) == 0 {
		t.Fatalf("short resume must warn: %+v %v", d, err)
	}
}
