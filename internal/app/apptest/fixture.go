// Package apptest — фикстуры для интеграционных тестов сервисов, работающих через пул (не транзакцию):
// пользователи, кандидаты с профилем/согласием/категорией/навыками/контактами, уборка хвостов по префиксу e-mail.
package apptest

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/app/skills"
	"fsp/internal/storage/postgres/pgtest"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Справочник навыков заполняет старт API, а не миграции: на пустой базе (чистая машина, CI) тесты
// сервисов получали «неизвестный навык go». Заполняется один раз на процесс, операция идемпотентна.
var (
	skillsOnce sync.Once
	skillsErr  error
)

// Fixture — общие зависимости теста.
type Fixture struct {
	T      *testing.T
	Ctx    context.Context
	Pool   *pgxpool.Pool
	Q      *sqlcgen.Queries
	Log    *slog.Logger
	prefix string
}

// New подключается к тестовой базе и чистит пользователей с e-mail вида "<prefix>-*@test.local"
// до и после теста (разные пакеты тестов используют разные префиксы — они выполняются параллельно).
func New(t *testing.T, prefix string) *Fixture {
	t.Helper()
	pool := pgtest.Pool(t)
	ctx := context.Background()
	skillsOnce.Do(func() { _, skillsErr = skills.Seed(ctx, pool) })
	if skillsErr != nil {
		t.Fatalf("apptest: справочник навыков: %v", skillsErr)
	}
	f := &Fixture{T: t, Ctx: ctx, Pool: pool, Q: sqlcgen.New(pool), prefix: prefix,
		Log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))}
	f.Cleanup()
	t.Cleanup(f.Cleanup)
	return f
}

// Cleanup удаляет компании тестовых работодателей (каскадом — потребности, вакансии, подборки, приглашения, отклики), затем пользователей.
func (f *Fixture) Cleanup() {
	like := f.prefix + "-%@test.local"
	for _, q := range []string{
		"DELETE FROM companies WHERE id IN (SELECT eu.company_id FROM employer_users eu JOIN users u ON u.id = eu.user_id WHERE u.email LIKE $1)",
		"DELETE FROM users WHERE email LIKE $1",
	} {
		if _, err := f.Pool.Exec(f.Ctx, q, like); err != nil {
			f.T.Fatalf("cleanup: %v", err)
		}
	}
}

// User создаёт активного пользователя роли role.
func (f *Fixture) User(role sqlcgen.UserRole) uuid.UUID {
	f.T.Helper()
	u, err := f.Q.CreateUser(f.Ctx, sqlcgen.CreateUserParams{Email: f.prefix + "-" + uuid.NewString()[:8] + "@test.local", Role: role})
	if err != nil {
		f.T.Fatal(err)
	}
	if err := f.Q.UpdateUserStatus(f.Ctx, sqlcgen.UpdateUserStatusParams{ID: u.ID, Status: sqlcgen.UserStatusActive}); err != nil {
		f.T.Fatal(err)
	}
	return u.ID
}

// CandidateOpts — параметры тестового кандидата.
type CandidateOpts struct {
	Name           string
	City           string
	Visibility     sqlcgen.CandidateVisibility
	Consent        bool
	Salary         int32
	SalaryMin      int32
	Skills         []string
	Specialization sqlcgen.SpecializationCode
	Grade          sqlcgen.GradeCode
	Status         sqlcgen.GradeStatus
	Percentile     float64
	Telegram       string
	NoCategory     bool
}

// Candidate создаёт активного кандидата с профилем, согласием на публикацию, категорией, навыками и контактами.
func (f *Fixture) Candidate(o CandidateOpts) uuid.UUID {
	f.T.Helper()
	id := f.User(sqlcgen.UserRoleCandidate)
	if err := f.Q.EnsureCandidateProfile(f.Ctx, id); err != nil {
		f.T.Fatal(err)
	}
	if o.City == "" {
		o.City = "Москва"
	}
	if o.Visibility == "" {
		o.Visibility = sqlcgen.CandidateVisibilityOpen
	}
	if o.Specialization == "" {
		o.Specialization = sqlcgen.SpecializationCodeBackend
	}
	if o.Grade == "" {
		o.Grade = sqlcgen.GradeCodeMiddle
	}
	if o.Status == "" {
		o.Status = sqlcgen.GradeStatusConfirmed
	}
	var salary, salaryMin *int32
	if o.Salary > 0 {
		salary = &o.Salary
	}
	if o.SalaryMin > 0 {
		salaryMin = &o.SalaryMin
	}
	if _, err := f.Q.UpdateCandidateProfile(f.Ctx, sqlcgen.UpdateCandidateProfileParams{
		UserID: id, FirstName: o.Name, LastName: "Тестовый", City: &o.City, WorkFormats: []sqlcgen.WorkFormat{sqlcgen.WorkFormatRemote},
		SalaryExpected: salary, SalaryMin: salaryMin, Visibility: o.Visibility, Links: []byte(`{}`),
	}); err != nil {
		f.T.Fatal(err)
	}
	if o.Consent {
		if _, err := f.Q.CreateConsent(f.Ctx, sqlcgen.CreateConsentParams{UserID: id, Kind: sqlcgen.ConsentKindPublication, Version: "1"}); err != nil {
			f.T.Fatal(err)
		}
	}
	if !o.NoCategory {
		theta, se, pct := 0.8, 0.3, o.Percentile
		now := time.Now()
		if _, err := f.Q.UpsertCandidateCategory(f.Ctx, sqlcgen.UpsertCandidateCategoryParams{
			UserID: id, Specialization: o.Specialization, Grade: o.Grade, Status: o.Status, IsPrimary: true,
			Theta: &theta, Se: &se, Percentile: &pct, ConfirmedAt: &now,
		}); err != nil {
			f.T.Fatal(err)
		}
	}
	if len(o.Skills) > 0 {
		rows, err := f.Q.GetSkillsByCodes(f.Ctx, o.Skills)
		if err != nil || len(rows) != len(o.Skills) {
			f.T.Fatalf("skills %v: %v (%d)", o.Skills, err, len(rows))
		}
		for _, r := range rows {
			if err := f.Q.UpsertCandidateSkill(f.Ctx, sqlcgen.UpsertCandidateSkillParams{UserID: id, SkillID: r.ID, Source: sqlcgen.SkillSourceDeclared}); err != nil {
				f.T.Fatal(err)
			}
		}
	}
	email := "contact-" + id.String()[:8] + "@test.local"
	if err := f.Q.EnsureCandidateContacts(f.Ctx, sqlcgen.EnsureCandidateContactsParams{UserID: id, Email: &email}); err != nil {
		f.T.Fatal(err)
	}
	if o.Telegram != "" {
		if _, err := f.Q.UpdateCandidateContacts(f.Ctx, sqlcgen.UpdateCandidateContactsParams{UserID: id, Telegram: &o.Telegram, Email: &email, Other: []byte(`{}`)}); err != nil {
			f.T.Fatal(err)
		}
	}
	return id
}
