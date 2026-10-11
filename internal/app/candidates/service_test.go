package candidates

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"fsp/internal/app/skills"
	"fsp/internal/domain"
	"fsp/internal/storage/postgres/pgtest"
	"fsp/internal/storage/postgres/sqlcgen"
)

func newCandidate(t *testing.T) (*Service, uuid.UUID) {
	t.Helper()
	pool := pgtest.Pool(t)
	ctx := context.Background()
	if _, err := skills.Seed(ctx, pool); err != nil {
		t.Fatalf("seed skills: %v", err)
	}
	email := "cand-" + time.Now().Format("150405.000000") + "@test.local"
	u, err := sqlcgen.New(pool).CreateUser(ctx, sqlcgen.CreateUserParams{Email: email, Role: sqlcgen.UserRoleCandidate})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(pool, log), u.ID
}

func ptr[T any](v T) *T { return &v }

func TestProfileLifecycle(t *testing.T) {
	svc, uid := newCandidate(t)
	ctx := context.Background()

	// Пустой профиль создаётся лениво.
	p, err := svc.Get(ctx, uid)
	if err != nil || p.Completeness != 0 || len(p.Skills) != 0 {
		t.Fatalf("empty profile: %+v %v", p, err)
	}

	// Валидация анкеты.
	if _, err := svc.Update(ctx, uid, UpdateInput{FirstName: "", LastName: "Иванов"}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("empty first name: %v", err)
	}
	if _, err := svc.Update(ctx, uid, UpdateInput{FirstName: "Иван", LastName: "Иванов", SalaryMin: ptr[int64](300000), SalaryExpected: ptr[int64](200000)}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("salary min above expected: %v", err)
	}
	if _, err := svc.Update(ctx, uid, UpdateInput{FirstName: "Иван", LastName: "Иванов", WorkFormats: []string{"moon"}}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("bad work format: %v", err)
	}
	if _, err := svc.Update(ctx, uid, UpdateInput{FirstName: "Иван", LastName: "Иванов", Links: map[string]string{"github": "ftp://x"}}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("bad link: %v", err)
	}

	p, err = svc.Update(ctx, uid, UpdateInput{
		FirstName: "Иван", LastName: "Иванов", City: ptr("Москва"), WorkFormats: []string{"remote", "hybrid", "remote"},
		SalaryMin: ptr[int64](200000), SalaryExpected: ptr[int64](260000), Visibility: "open",
		Headline: ptr("Backend-разработчик (Go)"), About: ptr("Пишу сервисы на Go уже пять лет, люблю наблюдаемость и тесты."),
		ExperienceYears: ptr(5.5), PrimarySpecialization: ptr("backend"), Links: map[string]string{"github": "https://github.com/ivanov"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(p.WorkFormats) != 2 || *p.SalaryExpected != 260000 || *p.PrimarySpecialization != "backend" || p.Links["github"] == "" {
		t.Fatalf("profile after update: %+v", p)
	}

	// Опыт и образование.
	exps, err := svc.ReplaceExperiences(ctx, uid, []ExperienceInput{
		{CompanyName: "Рога и копыта", Position: "Старший разработчик", StartedOn: "2021-06-01", Stack: []string{"go", "postgresql"}},
		{CompanyName: "Старт", Position: "Разработчик", StartedOn: "2019-03-01", EndedOn: ptr("2021-05-01"), Specialization: ptr("backend")},
	})
	if err != nil || len(exps) != 2 || exps[1].EndedOn == nil {
		t.Fatalf("experiences: %+v %v", exps, err)
	}
	if _, err := svc.ReplaceExperiences(ctx, uid, []ExperienceInput{{CompanyName: "X", Position: "Y", StartedOn: "2022-01-01", EndedOn: ptr("2021-01-01")}}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("ended before started: %v", err)
	}
	edu, err := svc.ReplaceEducation(ctx, uid, []EducationInput{{Institution: "МГТУ им. Баумана", GraduatedYear: ptr[int32](2018)}})
	if err != nil || len(edu) != 1 {
		t.Fatalf("education: %+v %v", edu, err)
	}

	// Навыки: неизвестный код — ошибка; затем замена набора.
	if _, err := svc.SetSkills(ctx, uid, []SkillInput{{Code: "go"}, {Code: "nonexistent_skill_xyz"}}, domain.SkillDeclared); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("unknown skill: %v", err)
	}
	sk, err := svc.SetSkills(ctx, uid, []SkillInput{{Code: "go", Level: ptr[int16](5)}, {Code: "postgresql", Level: ptr[int16](4)}, {Code: "docker"}}, domain.SkillDeclared)
	if err != nil || len(sk) != 3 {
		t.Fatalf("set skills: %+v %v", sk, err)
	}
	sk, err = svc.SetSkills(ctx, uid, []SkillInput{{Code: "go"}}, domain.SkillDeclared)
	if err != nil || len(sk) != 1 || sk[0].Code != "go" {
		t.Fatalf("replace skills: %+v %v", sk, err)
	}

	// Контакты и приватность.
	c, err := svc.UpdateContacts(ctx, uid, Contacts{Phone: ptr("+7 916 123-45-67"), Telegram: ptr("@ivanov_dev"), Email: ptr("Ivanov@Example.com")})
	if err != nil || *c.Telegram != "ivanov_dev" || *c.Email != "ivanov@example.com" {
		t.Fatalf("contacts: %+v %v", c, err)
	}
	if _, err := svc.UpdateContacts(ctx, uid, Contacts{Phone: ptr("abc")}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("bad phone: %v", err)
	}
	pr, err := svc.UpdatePrivacy(ctx, uid, Privacy{ShowFullName: true, ShowExperience: false, ShowFSP: true, ShowSalaryExpected: false, ShowCity: true})
	if err != nil || pr.ShowExperience || !pr.ShowFullName {
		t.Fatalf("privacy: %+v %v", pr, err)
	}

	full, err := svc.Get(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if full.Completeness < 80 || len(full.Experiences) != 2 || len(full.Education) != 1 || len(full.Skills) != 1 {
		t.Fatalf("full profile: completeness=%d exps=%d edu=%d skills=%d", full.Completeness, len(full.Experiences), len(full.Education), len(full.Skills))
	}
}
