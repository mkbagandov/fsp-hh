package applications

import (
	"testing"

	"github.com/google/uuid"

	"fsp/internal/app/apptest"
	"fsp/internal/app/employers"
	"fsp/internal/app/invitations"
	"fsp/internal/app/matching"
	"fsp/internal/app/notifications"
	"fsp/internal/app/vacancies"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/storage/postgres/sqlcgen"
)

func TestVacanciesApplicationsNotifications(t *testing.T) {
	f := apptest.New(t, "e2e9")
	ctx := f.Ctx
	llmClient, err := llm.New(llm.Config{Provider: "mock"}, f.Log)
	if err != nil {
		t.Fatal(err)
	}
	empSvc := employers.New(f.Pool, llmClient, f.Log)
	matchSvc := matching.New(f.Pool, f.Log)
	notifySvc := notifications.New(f.Pool, nil, "http://localhost:5173", f.Log)
	invSvc := invitations.New(invitations.Config{}, f.Pool, f.Log)
	invSvc.SetNotifier(notifySvc)
	vacSvc := vacancies.New(f.Pool, f.Log)
	appSvc := New(f.Pool, invSvc, f.Log)
	appSvc.SetNotifier(notifySvc)

	employerA := f.User(sqlcgen.UserRoleEmployer)
	companyA, err := empSvc.UpsertCompany(ctx, employerA, employers.CompanyInput{Name: "Альфа", City: "Москва"})
	if err != nil {
		t.Fatal(err)
	}
	employerB := f.User(sqlcgen.UserRoleEmployer)
	companyB, err := empSvc.UpsertCompany(ctx, employerB, employers.CompanyInput{Name: "Бета"})
	if err != nil {
		t.Fatal(err)
	}

	// Вакансия: черновик не виден публично, публикация делает её видимой; вилка обязательна.
	in := vacancies.Input{Title: "Go-разработчик (платежи)", Description: "Разработка платёжного шлюза на Go и PostgreSQL, команда из 6 человек.",
		Specialization: "backend", GradeMin: "middle", GradeMax: "senior", MustSkills: []string{"go", "postgresql"}, SalaryFrom: 220000, SalaryTo: 320000, WorkFormats: []string{"remote"}, RemoteOK: true}
	if _, err := vacSvc.Create(ctx, employerA, companyA.ID, vacancies.Input{Title: "x", Description: "короткое", Specialization: "backend", GradeMin: "middle", GradeMax: "senior", SalaryFrom: 1, SalaryTo: 2}); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("short description must fail validation, got %v", err)
	}
	v, err := vacSvc.Create(ctx, employerA, companyA.ID, in)
	if err != nil || v.Status != "draft" || len(v.MustSkills) != 2 {
		t.Fatalf("create: %+v %v", v, err)
	}
	if _, err := vacSvc.GetPublic(ctx, v.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("draft must not be public, got %v", err)
	}
	if _, err := vacSvc.SetStatus(ctx, companyA.ID, v.ID, "closed"); !domain.Is(err, domain.KindInvalidTransition) {
		t.Fatalf("draft→closed must be invalid, got %v", err)
	}
	if _, err := vacSvc.SetStatus(ctx, companyB.ID, v.ID, "published"); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("another company must not manage the vacancy, got %v", err)
	}
	if v, err = vacSvc.SetStatus(ctx, companyA.ID, v.ID, "published"); err != nil || v.Status != "published" || v.PublishedAt == nil {
		t.Fatalf("publish: %+v %v", v, err)
	}
	pub, err := vacSvc.GetPublic(ctx, v.ID)
	if err != nil || pub.Company.Name != "Альфа" || pub.SalaryFrom != 220000 {
		t.Fatalf("public: %+v %v", pub, err)
	}
	list, err := vacSvc.ListPublic(ctx, vacancies.PublicFilter{Specialization: "backend", Grade: "middle", SalaryMin: 300000, Query: "платежи"})
	if err != nil || list.Total < 1 {
		t.Fatalf("public list: %+v %v", list, err)
	}
	found := false
	for _, it := range list.Items {
		if it.ID == v.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("published vacancy must be in the filtered public list")
	}
	if l, err := vacSvc.ListPublic(ctx, vacancies.PublicFilter{Grade: "junior"}); err != nil {
		t.Fatal(err)
	} else {
		for _, it := range l.Items {
			if it.ID == v.ID {
				t.Fatal("junior filter must exclude a middle–senior vacancy")
			}
		}
	}

	// Кандидаты: пассивный с согласием (виден в подборке) и скрытый без согласия (виден компании только после отклика).
	candP := f.Candidate(apptest.CandidateOpts{Name: "Полина", Visibility: sqlcgen.CandidateVisibilityPassive, Consent: true, Salary: 260000, Skills: []string{"go", "postgresql"}, Percentile: 0.8, Telegram: "@polina"})
	candH := f.Candidate(apptest.CandidateOpts{Name: "Хасан", Visibility: sqlcgen.CandidateVisibilityHidden, Consent: false, Salary: 300000, Skills: []string{"go"}, Percentile: 0.7, Telegram: "@hasan"})
	noProfile := f.User(sqlcgen.UserRoleCandidate)
	if err := f.Q.EnsureCandidateProfile(ctx, noProfile); err != nil {
		t.Fatal(err)
	}

	preview, err := matchSvc.Preview(ctx, vacancies.NeedFor(mustOwn(t, vacSvc, companyA.ID, v.ID)), matching.SelectionFilters{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uuid.UUID]bool{}
	for _, it := range preview {
		seen[it.Candidate.ID] = true
	}
	if !seen[candP] || seen[candH] {
		t.Fatalf("preview must include the consenting candidate and exclude the hidden one: %v", seen)
	}

	// Отклики.
	if _, err := appSvc.Apply(ctx, noProfile, v.ID, ""); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("empty profile must be rejected with profile_required, got %v", err)
	}
	appH, err := appSvc.Apply(ctx, candH, v.ID, "Готов обсудить")
	if err != nil || appH.Status != "sent" {
		t.Fatalf("apply H: %+v %v", appH, err)
	}
	if _, err := appSvc.Apply(ctx, candH, v.ID, ""); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("second application must conflict, got %v", err)
	}
	appP, err := appSvc.Apply(ctx, candP, v.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	// Отклик даёт компании контакты и карточку даже скрытого кандидата; другой компании — нет.
	if c, err := matchSvc.Contacts(ctx, companyA.ID, candH); err != nil || c.Telegram == nil || *c.Telegram != "@hasan" {
		t.Fatalf("contacts after application: %+v %v", c, err)
	}
	if card, err := matchSvc.CandidateCard(ctx, companyA.ID, candH); err != nil || !card.ContactsGranted || card.DisplayName == "" {
		t.Fatalf("card of hidden applicant for the company: %+v %v", card, err)
	}
	if _, err := matchSvc.CandidateCard(ctx, companyB.ID, candH); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("hidden candidate must stay invisible to another company, got %v", err)
	}
	if _, err := matchSvc.Contacts(ctx, companyB.ID, candH); !domain.Is(err, domain.KindForbidden) {
		t.Fatalf("other company must not get contacts, got %v", err)
	}
	// Уведомление компании об отклике.
	if items, err := notifySvc.List(ctx, employerA, true, 10); err != nil || len(items) < 2 || items[0].Kind != string(notifications.KindApplicationReceived) {
		t.Fatalf("employer notifications: %+v %v", items, err)
	}

	// Списки и счётчики.
	mine, err := appSvc.ListForCandidate(ctx, candH)
	if err != nil || len(mine) != 1 || mine[0].Vacancy == nil || mine[0].Vacancy.CompanyName != "Альфа" {
		t.Fatalf("candidate applications: %+v %v", mine, err)
	}
	forVac, err := appSvc.ListForVacancy(ctx, companyA.ID, v.ID)
	if err != nil || len(forVac) != 2 || forVac[0].Applicant == nil || forVac[0].Applicant.DisplayName == "" {
		t.Fatalf("vacancy applications: %+v %v", forVac, err)
	}
	if _, err := appSvc.ListForVacancy(ctx, companyB.ID, v.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("other company must not list applications, got %v", err)
	}
	got, err := vacSvc.Get(ctx, companyA.ID, v.ID)
	if err != nil || got.Applications == nil || got.Applications.Total != 2 || got.Applications.New != 2 {
		t.Fatalf("counts: %+v %v", got.Applications, err)
	}

	// Просмотр, отказ с причиной (кандидат получает уведомление), приглашение из отклика.
	viewed, err := appSvc.View(ctx, employerA, companyA.ID, appH.ID)
	if err != nil || viewed.Status != "viewed" || viewed.ViewedAt == nil {
		t.Fatalf("view: %+v %v", viewed, err)
	}
	if _, err := appSvc.Reject(ctx, employerA, companyA.ID, appP.ID, ""); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("reject without reason must fail, got %v", err)
	}
	rejected, err := appSvc.Reject(ctx, employerA, companyA.ID, appP.ID, "Нужен опыт с Kafka")
	if err != nil || rejected.Status != "rejected" || rejected.RejectReason == "" || rejected.ViewedAt == nil {
		t.Fatalf("reject: %+v %v", rejected, err)
	}
	if n, err := notifySvc.List(ctx, candP, true, 10); err != nil || len(n) != 1 || n[0].Kind != string(notifications.KindApplicationStatus) {
		t.Fatalf("candidate P notifications: %+v %v", n, err)
	}
	// Скрытого кандидата без согласия на публикацию можно пригласить только потому, что он сам откликнулся.
	cm := invitations.ContactMethod{Kind: "email", Value: "hr@alpha.test"}
	if _, err := invSvc.Create(ctx, employerB, companyB.ID, invitations.CreateInput{CandidateID: candH, Message: "Привет", SalaryFrom: 200000, SalaryTo: 300000, ContactMethod: cm}); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("another company must not invite the hidden candidate, got %v", err)
	}
	res, err := appSvc.Invite(ctx, employerA, companyA.ID, appH.ID, InviteInput{Message: "Приглашаем на интервью", SalaryFrom: 250000, SalaryTo: 320000, ContactMethod: cm})
	if err != nil || res.Application.Status != "invited" || res.Invitation.VacancyID == nil || *res.Invitation.VacancyID != v.ID || res.Application.InvitationID == nil {
		t.Fatalf("invite: %+v %v", res, err)
	}
	if _, err := appSvc.Invite(ctx, employerA, companyA.ID, appH.ID, InviteInput{Message: "Ещё", SalaryFrom: 250000, SalaryTo: 320000, ContactMethod: cm}); !domain.Is(err, domain.KindInvalidTransition) {
		t.Fatalf("second invite must be an invalid transition, got %v", err)
	}
	if n, err := notifySvc.List(ctx, candH, true, 10); err != nil || len(n) != 1 || n[0].Kind != string(notifications.KindInvitationReceived) {
		t.Fatalf("candidate H notifications: %+v %v", n, err)
	}
	unread, _ := notifySvc.UnreadCount(ctx, candH)
	if unread != 1 {
		t.Fatalf("unread count: %d", unread)
	}
	if _, err := invSvc.Accept(ctx, candH, res.Invitation.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	n, err := notifySvc.List(ctx, employerA, false, 10)
	if err != nil || n[0].Kind != string(notifications.KindInvitationStatus) {
		t.Fatalf("employer must be notified about acceptance: %+v %v", n, err)
	}
	if err := notifySvc.MarkRead(ctx, candH, n[0].ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("marking someone else's notification must be not found, got %v", err)
	}
	if err := notifySvc.MarkRead(ctx, employerA, n[0].ID); err != nil {
		t.Fatal(err)
	}
	if marked, err := notifySvc.MarkAllRead(ctx, employerA); err != nil || marked < 1 {
		t.Fatalf("mark all: %d %v", marked, err)
	}
	if unread, _ := notifySvc.UnreadCount(ctx, employerA); unread != 0 {
		t.Fatalf("unread after mark all: %d", unread)
	}

	// Отзыв отклика и закрытие вакансии.
	v2, err := vacSvc.Create(ctx, employerA, companyA.ID, vacancies.Input{Title: "Второй Go-разработчик", Description: "Поддержка сервисов биллинга, дежурства по графику.",
		Specialization: "backend", GradeMin: "junior", GradeMax: "middle", SalaryFrom: 150000, SalaryTo: 200000, NeedID: nil})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = vacSvc.SetStatus(ctx, companyA.ID, v2.ID, "published"); err != nil {
		t.Fatal(err)
	}
	a2, err := appSvc.Apply(ctx, candP, v2.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appSvc.Withdraw(ctx, candH, a2.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("another candidate must not withdraw, got %v", err)
	}
	if w, err := appSvc.Withdraw(ctx, candP, a2.ID); err != nil || w.Status != "withdrawn" {
		t.Fatalf("withdraw: %+v %v", w, err)
	}
	if _, err := vacSvc.SetStatus(ctx, companyA.ID, v2.ID, "closed"); err != nil {
		t.Fatal(err)
	}
	if _, err := appSvc.Apply(ctx, candH, v2.ID, ""); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("closed vacancy must not accept applications, got %v", err)
	}
	all, err := vacSvc.ListForCompany(ctx, companyA.ID)
	if err != nil || len(all) != 2 {
		t.Fatalf("company vacancies: %d %v", len(all), err)
	}
}

func mustOwn(t *testing.T, svc *vacancies.Service, companyID, id uuid.UUID) *sqlcgen.Vacancy {
	t.Helper()
	row, err := svc.Own(t.Context(), companyID, id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}
