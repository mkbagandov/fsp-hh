package matching_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"fsp/internal/app/apptest"
	"fsp/internal/app/employers"
	"fsp/internal/app/matching"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/storage/postgres/sqlcgen"
)

func find(items []matching.SelectionItem, id uuid.UUID) (matching.SelectionItem, bool) {
	for _, it := range items {
		if it.Candidate.ID == id {
			return it, true
		}
	}
	return matching.SelectionItem{}, false
}

func factsText(it matching.SelectionItem) string {
	var b strings.Builder
	for _, f := range it.Facts {
		b.WriteString(f.Kind + ": " + f.Text + "\n")
	}
	return b.String()
}

// Скрытые кандидатом поля не утекают работодателю ни через объяснения, ни через фильтр города,
// ни через сохранённые версии подборки; приватный минимум зарплаты не называется никогда.
func TestHiddenFieldsDoNotLeak(t *testing.T) {
	f := apptest.New(t, "mpriv")
	ctx := f.Ctx
	svc := matching.New(f.Pool, f.Log)

	// Уникальный город изолирует сценарий от кандидатов других пакетов в общей тестовой базе.
	city := "Приватск-" + uuid.NewString()[:6]
	hidden := f.Candidate(apptest.CandidateOpts{Name: "Скрытный", City: city, Consent: true, Salary: 333000, SalaryMin: 444000, Skills: []string{"go"}})
	open := f.Candidate(apptest.CandidateOpts{Name: "Открытый", City: city, Consent: true, Salary: 222000, SalaryMin: 250000, Skills: []string{"go"}})
	if _, err := f.Pool.Exec(ctx, `UPDATE candidate_profiles SET experience_years = 7.5 WHERE user_id = ANY($1)`, []uuid.UUID{hidden, open}); err != nil {
		t.Fatal(err)
	}
	setPrivacy := func(id uuid.UUID, show bool) {
		t.Helper()
		if _, err := f.Pool.Exec(ctx, `INSERT INTO privacy_settings (user_id, show_salary_expected, show_city, show_experience) VALUES ($1, $2, $2, $2)
			ON CONFLICT (user_id) DO UPDATE SET show_salary_expected = $2, show_city = $2, show_experience = $2`, id, show); err != nil {
			t.Fatal(err)
		}
	}
	setPrivacy(hidden, false)

	need := matching.Need{Specialization: "backend", GradeMin: "junior", GradeMax: "senior", Salary: domain.SalaryRange{From: 100000, To: 200000}, City: city}
	items, err := svc.Preview(ctx, need, matching.SelectionFilters{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := find(items, hidden)
	if !ok {
		t.Fatal("hidden-fields candidate must still be ranked")
	}
	text := factsText(h)
	if h.Candidate.City != "" || h.Candidate.SalaryExpected != nil || h.Candidate.ExperienceYears != nil {
		t.Fatalf("summary leaks hidden fields: %+v", h.Candidate)
	}
	for _, leak := range []string{"333", "444", city, "локация подходит", "локация не совпадает"} {
		if strings.Contains(text, leak) {
			t.Fatalf("facts leak %q:\n%s", leak, text)
		}
	}
	if !strings.Contains(text, "скрыто кандидатом") || !strings.Contains(text, "минимальных ожиданий") {
		t.Fatalf("facts must say the salary is hidden and the range is below the private minimum:\n%s", text)
	}
	o, ok := find(items, open)
	if !ok || o.Candidate.City != city || o.Candidate.SalaryExpected == nil || !strings.Contains(factsText(o), "222 000 ₽") || strings.Contains(factsText(o), "250 000") {
		t.Fatalf("open candidate shows visible fields but never the private minimum: %+v\n%s", o.Candidate, factsText(o))
	}

	// Фильтр по городу не отвечает на вопрос «живёт ли скрывший город кандидат в Казани».
	byCity, err := svc.Preview(ctx, need, matching.SelectionFilters{City: city, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := find(byCity, hidden); ok {
		t.Fatal("city filter must not match a hidden city")
	}
	if _, ok := find(byCity, open); !ok {
		t.Fatal("city filter must match a visible city")
	}

	// Версия подборки, построенная, пока поля были открыты, после скрытия не показывает сумму ожидания,
	// а после скрытия профиля — вообще никаких сохранённых объяснений.
	setPrivacy(hidden, true)
	owner := f.User(sqlcgen.UserRoleEmployer)
	emp := employers.New(f.Pool, llm.NewMock(), f.Log)
	if _, err := emp.UpsertCompany(ctx, owner, employers.CompanyInput{Name: "Проверка приватности"}); err != nil {
		t.Fatal(err)
	}
	n, err := emp.CreateNeed(ctx, owner, employers.NeedInput{Title: "Go", Specialization: "backend", GradeMin: "junior", GradeMax: "senior", MustSkills: []string{"go"}, SalaryFrom: 100000, SalaryTo: 200000, City: city})
	if err != nil {
		t.Fatal(err)
	}
	row, err := emp.OwnNeed(ctx, owner, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.BuildVersion(ctx, row, owner, matching.SelectionFilters{City: city, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	// Повторная сборка без изменений не создаёт новую версию.
	if again, err := svc.BuildVersion(ctx, row, owner, matching.SelectionFilters{City: city, Limit: 200}); err != nil || again.VersionNo != first.VersionNo {
		t.Fatalf("identical rebuild must reuse version %d, got %d (%v)", first.VersionNo, again.VersionNo, err)
	}
	setPrivacy(hidden, false)
	v, err := svc.GetVersion(ctx, row, 0)
	if err != nil {
		t.Fatal(err)
	}
	h, ok = find(v.Items, hidden)
	if !ok || strings.Contains(factsText(h), "333") || strings.Contains(factsText(h), "444") {
		t.Fatalf("stored facts must be redacted after hiding the salary:\n%s", factsText(h))
	}
	if _, err := f.Pool.Exec(ctx, `UPDATE candidate_profiles SET visibility = 'hidden' WHERE user_id = $1`, hidden); err != nil {
		t.Fatal(err)
	}
	v, err = svc.GetVersion(ctx, row, 0)
	if err != nil {
		t.Fatal(err)
	}
	if h, ok = find(v.Items, hidden); !ok || len(h.Facts) != 0 || h.Candidate.Headline != "профиль скрыт или удалён" {
		t.Fatalf("hidden profile must keep only a placeholder: %+v %v", h.Candidate, h.Facts)
	}
}
