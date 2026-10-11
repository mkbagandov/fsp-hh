package admin

import (
	"strings"
	"testing"

	"fsp/internal/app/apptest"
	"fsp/internal/app/employers"
	apptesting "fsp/internal/app/testing"
	"fsp/internal/app/vacancies"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/storage/postgres/sqlcgen"
	_ "fsp/pkg/itembank/families/all"
)

func TestAdminOperations(t *testing.T) {
	f := apptest.New(t, "adm")
	ctx := f.Ctx
	svc := New(f.Pool, f.Log)
	if _, err := apptesting.SyncFamilies(ctx, f.Pool); err != nil {
		t.Fatal(err)
	}

	adminID := f.User(sqlcgen.UserRoleEmployer)
	adminUser, err := f.Q.GetUserByID(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.PromoteAdmins(ctx, []string{"  " + strings.ToUpper(adminUser.Email) + " ", ""})
	if err != nil || n != 1 {
		t.Fatalf("promote: %d %v", n, err)
	}
	if n, _ := svc.PromoteAdmins(ctx, []string{adminUser.Email}); n != 0 {
		t.Fatal("promotion must be idempotent")
	}

	cand := f.Candidate(apptest.CandidateOpts{Name: "Админка", Consent: true, Salary: 200000, Skills: []string{"go"}, Percentile: 0.5})
	other := f.User(sqlcgen.UserRoleEmployer)

	// Пользователи: поиск и блокировка с защитой себя и других админов.
	list, err := svc.Users(ctx, UserFilter{Query: "adm-", Role: "candidate", Status: "active", Limit: 500})
	if err != nil || list.Total < 1 || len(list.Items) < 1 {
		t.Fatalf("users: %+v %v", list, err)
	}
	if _, err := svc.SetUserStatus(ctx, adminID, adminID, "blocked"); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("self block must conflict, got %v", err)
	}
	if _, err := svc.SetUserStatus(ctx, other, adminID, "blocked"); !domain.Is(err, domain.KindForbidden) {
		t.Fatalf("admins are protected, got %v", err)
	}
	if _, err := svc.SetUserStatus(ctx, adminID, cand, "deleted"); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("only active/blocked allowed, got %v", err)
	}
	u, err := svc.SetUserStatus(ctx, adminID, cand, "blocked")
	if err != nil || u.Status != "blocked" {
		t.Fatalf("block: %+v %v", u, err)
	}
	if u, err = svc.SetUserStatus(ctx, adminID, cand, "active"); err != nil || u.Status != "active" {
		t.Fatalf("unblock: %+v %v", u, err)
	}

	// Сводка, банк заданий, флаги, работодатели.
	ov, err := svc.Overview(ctx, 7)
	if err != nil || len(ov.Daily) != 7 || ov.Families == 0 || ov.Funnel.Candidates < 1 {
		t.Fatalf("overview: %+v %v", ov.Funnel, err)
	}
	items, err := svc.ItemStats(ctx)
	if err != nil || len(items) == 0 {
		t.Fatalf("items: %d %v", len(items), err)
	}
	if err := svc.SetFamilyEnabled(ctx, adminID, items[0].ID, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.SetFamilyEnabled(ctx, adminID, items[0].ID, true) })
	if _, err := svc.FlaggedSessions(ctx, 10); err != nil {
		t.Fatal(err)
	}

	// Модерация: снять можно только опубликованную вакансию; причина — в аудите.
	emp := employers.New(f.Pool, llm.NewMock(), f.Log)
	company, err := emp.UpsertCompany(ctx, other, employers.CompanyInput{Name: "Модерируемая"})
	if err != nil {
		t.Fatal(err)
	}
	vac := vacancies.New(f.Pool, f.Log)
	v, err := vac.Create(ctx, other, company.ID, vacancies.Input{Title: "Подозрительная вакансия", Description: "Описание достаточной длины для проверки модерации.", Specialization: "backend", GradeMin: "junior", GradeMax: "middle", SalaryFrom: 100000, SalaryTo: 150000})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.HideVacancy(ctx, adminID, v.ID, "черновик"); !domain.Is(err, domain.KindInvalidTransition) {
		t.Fatalf("draft cannot be hidden, got %v", err)
	}
	if _, err := vac.SetStatus(ctx, company.ID, v.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if err := svc.HideVacancy(ctx, adminID, v.ID, "Вилка не соответствует описанию"); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.Vacancies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == v.ID && r.Status != "closed" {
			t.Fatalf("hidden vacancy must be closed: %+v", r)
		}
	}
	emps, err := svc.Employers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range emps {
		if e.ID == company.ID {
			found = true
			if e.Reputation != "new" || e.Vacancies != 1 {
				t.Fatalf("employer row: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("company must be listed")
	}
	audit, err := svc.Audit(ctx, "admin.", adminUser.Email[:8], 50, 0)
	if err != nil || audit.Total < 3 {
		t.Fatalf("audit of admin actions: %+v %v", audit.Total, err)
	}
	hasHide := false
	for _, a := range audit.Items {
		if a.Action == "admin.vacancy.hide" && strings.Contains(string(a.Details), "Вилка не соответствует") {
			hasHide = true
		}
	}
	if !hasHide {
		t.Fatal("hide reason must be in audit")
	}
	metrics, err := svc.Metrics(ctx)
	if err != nil || !strings.Contains(metrics, "fsp_users{") || !strings.Contains(metrics, "# TYPE fsp_invitations gauge") {
		t.Fatalf("metrics: %v\n%s", err, metrics)
	}
}
