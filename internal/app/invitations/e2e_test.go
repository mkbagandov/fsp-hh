package invitations

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/app/employers"
	"fsp/internal/app/matching"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/storage/postgres/pgtest"
	"fsp/internal/storage/postgres/sqlcgen"
)

type fixture struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
	log  *slog.Logger
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := pgtest.Pool(t)
	ctx := context.Background()
	// Сервисы работают через пул, а не транзакцию: убираем хвосты прошлых (упавших) запусков и чистим за собой.
	cleanup := func() {
		// Сначала компании тестовых работодателей (каскадом уходят потребности, подборки и приглашения), затем пользователи.
		for _, q := range []string{
			"DELETE FROM companies WHERE id IN (SELECT eu.company_id FROM employer_users eu JOIN users u ON u.id = eu.user_id WHERE u.email LIKE 'e2e-%@test.local')",
			"DELETE FROM users WHERE email LIKE 'e2e-%@test.local'",
		} {
			if _, err := pool.Exec(ctx, q); err != nil {
				t.Fatalf("cleanup: %v", err)
			}
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	return &fixture{t: t, ctx: ctx, pool: pool, q: sqlcgen.New(pool),
		log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))}
}

func (f *fixture) user(role sqlcgen.UserRole) uuid.UUID {
	f.t.Helper()
	u, err := f.q.CreateUser(f.ctx, sqlcgen.CreateUserParams{Email: "e2e-" + uuid.NewString()[:8] + "@test.local", Role: role})
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.q.UpdateUserStatus(f.ctx, sqlcgen.UpdateUserStatusParams{ID: u.ID, Status: sqlcgen.UserStatusActive}); err != nil {
		f.t.Fatal(err)
	}
	return u.ID
}

type candOpts struct {
	name       string
	visibility sqlcgen.CandidateVisibility
	consent    bool
	salary     int32
	salaryMin  int32
	skills     []string
	grade      sqlcgen.GradeCode
	status     sqlcgen.GradeStatus
	percentile float64
	telegram   string
}

// candidate создаёт активного кандидата с профилем, согласием, категорией, навыками и контактами.
func (f *fixture) candidate(o candOpts) uuid.UUID {
	f.t.Helper()
	id := f.user(sqlcgen.UserRoleCandidate)
	if err := f.q.EnsureCandidateProfile(f.ctx, id); err != nil {
		f.t.Fatal(err)
	}
	city := "Москва"
	var salary, salaryMin *int32
	if o.salary > 0 {
		salary = &o.salary
	}
	if o.salaryMin > 0 {
		salaryMin = &o.salaryMin
	}
	if _, err := f.q.UpdateCandidateProfile(f.ctx, sqlcgen.UpdateCandidateProfileParams{
		UserID: id, FirstName: o.name, LastName: "Тестовый", City: &city, WorkFormats: []sqlcgen.WorkFormat{sqlcgen.WorkFormatRemote},
		SalaryExpected: salary, SalaryMin: salaryMin, Visibility: o.visibility, Links: []byte(`{}`),
	}); err != nil {
		f.t.Fatal(err)
	}
	if o.consent {
		if _, err := f.q.CreateConsent(f.ctx, sqlcgen.CreateConsentParams{UserID: id, Kind: sqlcgen.ConsentKindPublication, Version: "1"}); err != nil {
			f.t.Fatal(err)
		}
	}
	theta, se, pct := 0.8, 0.3, o.percentile
	now := time.Now()
	if _, err := f.q.UpsertCandidateCategory(f.ctx, sqlcgen.UpsertCandidateCategoryParams{
		UserID: id, Specialization: sqlcgen.SpecializationCodeBackend, Grade: o.grade, Status: o.status, IsPrimary: true,
		Theta: &theta, Se: &se, Percentile: &pct, ConfirmedAt: &now,
	}); err != nil {
		f.t.Fatal(err)
	}
	if len(o.skills) > 0 {
		rows, err := f.q.GetSkillsByCodes(f.ctx, o.skills)
		if err != nil || len(rows) != len(o.skills) {
			f.t.Fatalf("skills %v: %v (%d)", o.skills, err, len(rows))
		}
		for _, r := range rows {
			if err := f.q.UpsertCandidateSkill(f.ctx, sqlcgen.UpsertCandidateSkillParams{UserID: id, SkillID: r.ID, Source: sqlcgen.SkillSourceDeclared}); err != nil {
				f.t.Fatal(err)
			}
		}
	}
	email := "contact-" + id.String()[:8] + "@test.local"
	if err := f.q.EnsureCandidateContacts(f.ctx, sqlcgen.EnsureCandidateContactsParams{UserID: id, Email: &email}); err != nil {
		f.t.Fatal(err)
	}
	if o.telegram != "" {
		if _, err := f.q.UpdateCandidateContacts(f.ctx, sqlcgen.UpdateCandidateContactsParams{UserID: id, Telegram: &o.telegram, Email: &email, Other: []byte(`{}`)}); err != nil {
			f.t.Fatal(err)
		}
	}
	return id
}

func TestEmployerEndToEnd(t *testing.T) {
	f := newFixture(t)
	ctx := f.ctx
	llmClient, err := llm.New(llm.Config{Provider: "mock"}, f.log)
	if err != nil {
		t.Fatal(err)
	}
	empSvc := employers.New(f.pool, llmClient, f.log)
	matchSvc := matching.New(f.pool, f.log)
	invSvc := New(Config{}, f.pool, f.log)

	// Кандидаты: видимый с согласием, скрытый, без согласия.
	candA := f.candidate(candOpts{name: "Анна", visibility: sqlcgen.CandidateVisibilityOpen, consent: true, salary: 250000, skills: []string{"go", "postgresql"}, grade: sqlcgen.GradeCodeMiddle, status: sqlcgen.GradeStatusConfirmed, percentile: 0.9, telegram: "@anna"})
	candHidden := f.candidate(candOpts{name: "Скрытый", visibility: sqlcgen.CandidateVisibilityHidden, consent: true, salary: 250000, skills: []string{"go"}, grade: sqlcgen.GradeCodeMiddle, status: sqlcgen.GradeStatusConfirmed, percentile: 0.95})
	candNoConsent := f.candidate(candOpts{name: "БезСогласия", visibility: sqlcgen.CandidateVisibilityOpen, consent: false, salary: 250000, skills: []string{"go"}, grade: sqlcgen.GradeCodeMiddle, status: sqlcgen.GradeStatusConfirmed, percentile: 0.95})
	candB := f.candidate(candOpts{name: "Борис", visibility: sqlcgen.CandidateVisibilityPassive, consent: true, salary: 350000, salaryMin: 320000, skills: []string{"go"}, grade: sqlcgen.GradeCodeSenior, status: sqlcgen.GradeStatusNotConfirmed, percentile: 0.4, telegram: "@boris"})

	// Работодатель без компании не может создать потребность.
	employer := f.user(sqlcgen.UserRoleEmployer)
	needIn := employers.NeedInput{Title: "Go-разработчик в платёжную команду", Specialization: "backend", GradeMin: "middle", GradeMax: "senior",
		MustSkills: []string{"go", "postgresql"}, SalaryFrom: 200000, SalaryTo: 300000, WorkFormats: []string{"remote", "hybrid"}, City: "Москва", RemoteOK: true}
	if _, err := empSvc.CreateNeed(ctx, employer, needIn); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("need without company must conflict, got %v", err)
	}
	company, err := empSvc.UpsertCompany(ctx, employer, employers.CompanyInput{Name: "ООО Тест", INN: "7707083893", Website: "https://test.local", PublicContacts: map[string]string{"site": "https://test.local/jobs"}})
	if err != nil || company.Role != "owner" {
		t.Fatalf("company: %+v %v", company, err)
	}
	if again, err := empSvc.UpsertCompany(ctx, employer, employers.CompanyInput{Name: "ООО Тест 2", PublicContacts: map[string]string{"site": "https://test.local/jobs"}}); err != nil || again.ID != company.ID || again.Name != "ООО Тест 2" {
		t.Fatalf("second upsert must update the same company: %+v %v", again, err)
	}
	if _, err := empSvc.CreateNeed(ctx, employer, employers.NeedInput{Title: "x", Specialization: "backend", GradeMin: "middle", GradeMax: "senior", MustSkills: []string{"no_such_skill"}, SalaryFrom: 1, SalaryTo: 2}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("unknown skill must fail validation, got %v", err)
	}
	need, err := empSvc.CreateNeed(ctx, employer, needIn)
	if err != nil || need.Status != "active" || len(need.MustSkills) != 2 {
		t.Fatalf("need: %+v %v", need, err)
	}
	needRow, err := empSvc.OwnNeed(ctx, employer, need.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Разбор текста потребности через эмулятор LLM.
	ex, err := empSvc.ExtractNeed(ctx, "Ищем senior Go-разработчика с опытом PostgreSQL и Kafka, удалённо, вилка 250 000 – 350 000 рублей")
	if err != nil || ex.Provider == "" {
		t.Fatalf("extract: %+v %v", ex, err)
	}

	// Карта категорий и подборка.
	cmap, err := matchSvc.CategoryMapFor(ctx, matching.NeedFromRow(needRow))
	if err != nil || len(cmap) != 3 {
		t.Fatalf("map: %v %v", cmap, err)
	}
	for _, s := range cmap {
		if s.Grade == "middle" && (s.Total < 1 || !s.InRange) {
			t.Fatalf("middle stat: %+v", s)
		}
	}
	v1, err := matchSvc.BuildVersion(ctx, needRow, employer, matching.SelectionFilters{})
	if err != nil || v1.VersionNo != 1 {
		t.Fatalf("v1: %+v %v", v1, err)
	}
	var itemA *matching.SelectionItem
	for i := range v1.Items {
		id := v1.Items[i].Candidate.ID
		if id == candHidden || id == candNoConsent {
			t.Fatalf("hidden or non-consenting candidate leaked into selection: %s", id)
		}
		if id == candA {
			itemA = &v1.Items[i]
		}
	}
	if itemA == nil {
		t.Fatal("visible consenting candidate must be in the selection")
	}
	if itemA.Candidate.SkillsMatched != 2 || !itemA.Confirmed || len(itemA.Facts) < 5 || itemA.Score <= 0 {
		t.Fatalf("item A: %+v", itemA)
	}
	if itemA.Candidate.DisplayName != "Тестовый Анна" {
		t.Fatalf("display name: %q", itemA.Candidate.DisplayName)
	}
	if v1.Items[0].Candidate.ID != candA {
		t.Fatalf("confirmed candidate with full skill match must rank first, got %+v", v1.Items[0].Candidate)
	}
	v2, err := matchSvc.BuildVersion(ctx, needRow, employer, matching.SelectionFilters{Grades: []string{"senior"}, GradeStatuses: []string{"confirmed"}})
	if err != nil || v2.VersionNo != 2 || v2.Diff == nil {
		t.Fatalf("v2: %+v %v", v2, err)
	}
	for _, it := range v2.Items {
		if it.Candidate.ID == candA {
			t.Fatal("grade filter must exclude middle candidate")
		}
	}
	removed := false
	for _, id := range v2.Diff.Removed {
		if id == candA {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("diff must report candidate A removed: %+v", v2.Diff)
	}
	got1, err := matchSvc.GetVersion(ctx, needRow, 1)
	if err != nil || got1.VersionNo != 1 || got1.ItemsCount != v1.ItemsCount || len(got1.Items) != len(v1.Items) {
		t.Fatalf("get v1: %+v %v", got1, err)
	}
	if got1.Items[0].Facts == nil || got1.Items[0].Components["s_test"] == 0 {
		t.Fatalf("stored snapshot must keep facts and components: %+v", got1.Items[0])
	}
	if versions, err := matchSvc.ListVersions(ctx, need.ID); err != nil || len(versions) != 2 {
		t.Fatalf("versions: %v %v", versions, err)
	}

	// До приглашения контактов нет.
	card, err := matchSvc.CandidateCard(ctx, company.ID, candA)
	if err != nil || card.ContactsGranted || card.Contacts != nil || card.DisplayName != "Тестовый Анна" {
		t.Fatalf("card before grant: %+v %v", card, err)
	}
	if _, err := matchSvc.Contacts(ctx, company.ID, candA); !domain.Is(err, domain.KindForbidden) {
		t.Fatalf("contacts before grant must be forbidden, got %v", err)
	}

	// Приглашение: вилка и способ связи обязательны; скрытого кандидата пригласить нельзя.
	cm := ContactMethod{Kind: "telegram", Value: "@hr_test"}
	if _, err := invSvc.Create(ctx, employer, company.ID, CreateInput{CandidateID: candA, NeedID: &need.ID, Message: "Привет", SalaryFrom: 300000, SalaryTo: 200000, ContactMethod: cm}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("inverted salary range must fail, got %v", err)
	}
	if _, err := invSvc.Create(ctx, employer, company.ID, CreateInput{CandidateID: candHidden, NeedID: &need.ID, Message: "Привет", SalaryFrom: 200000, SalaryTo: 300000, ContactMethod: cm}); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("hidden candidate must not be invitable, got %v", err)
	}
	inv, err := invSvc.Create(ctx, employer, company.ID, CreateInput{CandidateID: candA, NeedID: &need.ID, Message: "Приглашаем в платёжную команду", SalaryFrom: 220000, SalaryTo: 280000, ContactMethod: cm})
	if err != nil || inv.Status != "sent" || inv.BelowCandidateMin {
		t.Fatalf("invitation: %+v %v", inv, err)
	}
	if _, err := invSvc.Create(ctx, employer, company.ID, CreateInput{CandidateID: candA, NeedID: &need.ID, Message: "Ещё раз", SalaryFrom: 220000, SalaryTo: 280000, ContactMethod: cm}); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("second active invitation must conflict, got %v", err)
	}

	// Кандидат: вилку видит сразу, способ связи — только после принятия.
	mine, err := invSvc.ListForCandidate(ctx, candA)
	if err != nil || len(mine) != 1 || mine[0].CompanyName == "" || mine[0].ContactMethod != nil || mine[0].SalaryFrom != 220000 {
		t.Fatalf("candidate list: %+v %v", mine, err)
	}
	viewed, err := invSvc.GetForCandidate(ctx, candA, inv.ID)
	if err != nil || viewed.Status != "viewed" || viewed.ViewedAt == nil || viewed.ContactMethod != nil {
		t.Fatalf("viewed: %+v %v", viewed, err)
	}
	if _, err := invSvc.GetForCandidate(ctx, candB, inv.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("another candidate must not see the invitation, got %v", err)
	}
	acc, err := invSvc.Accept(ctx, candA, inv.ID)
	if err != nil || acc.Invitation.Status != "accepted" || acc.ContactMethod.Value != "@hr_test" || acc.CompanyContacts["site"] == "" {
		t.Fatalf("accept: %+v %v", acc, err)
	}
	if _, err := invSvc.Accept(ctx, candA, inv.ID); !domain.Is(err, domain.KindInvalidTransition) {
		t.Fatalf("accepting twice must be an invalid transition, got %v", err)
	}

	// После принятия компания видит контакты, другая компания — нет.
	contacts, err := matchSvc.Contacts(ctx, company.ID, candA)
	if err != nil || contacts.Telegram == nil || *contacts.Telegram != "@anna" {
		t.Fatalf("contacts after grant: %+v %v", contacts, err)
	}
	card, err = matchSvc.CandidateCard(ctx, company.ID, candA)
	if err != nil || !card.ContactsGranted || card.Contacts == nil || card.InvitationStatus != "accepted" {
		t.Fatalf("card after grant: %+v %v", card, err)
	}
	otherEmployer := f.user(sqlcgen.UserRoleEmployer)
	other, err := empSvc.UpsertCompany(ctx, otherEmployer, employers.CompanyInput{Name: "Другая"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := matchSvc.Contacts(ctx, other.ID, candA); !domain.Is(err, domain.KindForbidden) {
		t.Fatalf("grant must be company-scoped, got %v", err)
	}
	if _, err := empSvc.OwnNeed(ctx, otherEmployer, need.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("need must be invisible to another company, got %v", err)
	}

	// Отказ с причиной → обратная связь; вилка ниже минимума помечается работодателю.
	invB, err := invSvc.Create(ctx, employer, company.ID, CreateInput{CandidateID: candB, NeedID: &need.ID, Message: "Приглашаем", SalaryFrom: 200000, SalaryTo: 300000, ContactMethod: cm})
	if err != nil || !invB.BelowCandidateMin {
		t.Fatalf("invitation B must be flagged below candidate minimum: %+v %v", invB, err)
	}
	if _, err := invSvc.Decline(ctx, candB, invB.ID, "bogus", ""); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("unknown decline reason must fail, got %v", err)
	}
	declined, err := invSvc.Decline(ctx, candB, invB.ID, "salary_below", "Ожидаю от 320 000")
	if err != nil || declined.Status != "declined" || declined.DeclineReason == nil || *declined.DeclineReason != "salary_below" {
		t.Fatalf("decline: %+v %v", declined, err)
	}
	if _, err := matchSvc.Contacts(ctx, company.ID, candB); !domain.Is(err, domain.KindForbidden) {
		t.Fatalf("declined invitation must not grant contacts, got %v", err)
	}
	fb, err := invSvc.FeedbackForNeed(ctx, company.ID, need.ID)
	if err != nil || fb.Total != 2 || fb.ByStatus["accepted"] != 1 || fb.DeclineReasons["salary_below"] != 1 || fb.AcceptanceRate != 0.5 || len(fb.Hints) == 0 {
		t.Fatalf("feedback: %+v %v", fb, err)
	}

	// Отзыв приглашения и квота на потребность.
	candC := f.candidate(candOpts{name: "Вера", visibility: sqlcgen.CandidateVisibilityOpen, consent: true, salary: 240000, skills: []string{"go"}, grade: sqlcgen.GradeCodeMiddle, status: sqlcgen.GradeStatusConfirmed, percentile: 0.6})
	invC, err := invSvc.Create(ctx, employer, company.ID, CreateInput{CandidateID: candC, NeedID: &need.ID, Message: "Приглашаем", SalaryFrom: 200000, SalaryTo: 300000, ContactMethod: cm})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invSvc.Withdraw(ctx, otherEmployer, other.ID, invC.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("another company must not withdraw, got %v", err)
	}
	withdrawn, err := invSvc.Withdraw(ctx, employer, company.ID, invC.ID)
	if err != nil || withdrawn.Status != "withdrawn" {
		t.Fatalf("withdraw: %+v %v", withdrawn, err)
	}
	if _, err := invSvc.Withdraw(ctx, employer, company.ID, invC.ID); !domain.Is(err, domain.KindInvalidTransition) {
		t.Fatalf("double withdraw must fail, got %v", err)
	}
	limited := New(Config{MaxActivePerNeed: 1}, f.pool, f.log)
	if _, err := limited.Create(ctx, employer, company.ID, CreateInput{CandidateID: candC, NeedID: &need.ID, Message: "Снова", SalaryFrom: 200000, SalaryTo: 300000, ContactMethod: cm}); err != nil {
		t.Fatalf("one active invitation must fit the quota: %v", err)
	}
	candD := f.candidate(candOpts{name: "Дмитрий", visibility: sqlcgen.CandidateVisibilityOpen, consent: true, salary: 240000, skills: []string{"go"}, grade: sqlcgen.GradeCodeMiddle, status: sqlcgen.GradeStatusConfirmed, percentile: 0.6})
	if _, err := limited.Create(ctx, employer, company.ID, CreateInput{CandidateID: candD, NeedID: &need.ID, Message: "Снова", SalaryFrom: 200000, SalaryTo: 300000, ContactMethod: cm}); !domain.Is(err, domain.KindRateLimited) {
		t.Fatalf("second active invitation must hit the need quota, got %v", err)
	}
	list, err := invSvc.ListForCompany(ctx, company.ID, &need.ID)
	if err != nil || len(list) != 4 {
		t.Fatalf("company list: %d %v", len(list), err)
	}

	// Поиск по банку: приглашённые помечены, скрытые отсутствуют.
	items, err := matchSvc.Search(ctx, company.ID, matching.SearchInput{Specialization: "backend", Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uuid.UUID]matching.SelectionItem{}
	for _, it := range items {
		seen[it.Candidate.ID] = it
	}
	if _, ok := seen[candHidden]; ok {
		t.Fatal("search leaked a hidden candidate")
	}
	if a, ok := seen[candA]; !ok || !a.Candidate.Invited {
		t.Fatalf("search must mark invited candidate: %+v", a)
	}
	if d, ok := seen[candD]; !ok || d.Candidate.Invited {
		t.Fatalf("not invited candidate must not be marked: %+v", d)
	}
	if _, err := matchSvc.Search(ctx, company.ID, matching.SearchInput{Specialization: "nope"}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("unknown specialization must fail, got %v", err)
	}
}
