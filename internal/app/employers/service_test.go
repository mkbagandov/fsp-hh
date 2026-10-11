package employers

import (
	"testing"

	"fsp/internal/app/apptest"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/storage/postgres/sqlcgen"
)

func TestCompanyAndNeedLifecycle(t *testing.T) {
	f := apptest.New(t, "emp")
	ctx := f.Ctx
	svc := New(f.Pool, llm.NewMock(), f.Log)
	owner := f.User(sqlcgen.UserRoleEmployer)

	if _, err := svc.MyCompany(ctx, owner); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("no company yet, got %v", err)
	}
	for _, bad := range []CompanyInput{
		{Name: ""},
		{Name: "ООО", INN: "12345"},
		{Name: "ООО", INN: "12345678ab"},
		{Name: "ООО", Website: "ftp://site"},
	} {
		if _, err := svc.UpsertCompany(ctx, owner, bad); !domain.Is(err, domain.KindValidation) {
			t.Fatalf("%+v must fail validation, got %v", bad, err)
		}
	}
	c, err := svc.UpsertCompany(ctx, owner, CompanyInput{Name: "  ООО «Проверка»  ", INN: "7707083893", Website: "https://check.example", PublicContacts: map[string]string{"email": "hr@check.example"}})
	if err != nil || c.Name != "ООО «Проверка»" || c.Role != "owner" {
		t.Fatalf("create: %+v %v", c, err)
	}
	mine, err := svc.MyCompany(ctx, owner)
	if err != nil || mine.ID != c.ID || mine.PublicContacts["email"] != "hr@check.example" {
		t.Fatalf("my company: %+v %v", mine, err)
	}

	in := NeedInput{Title: "Go-разработчик", Specialization: "backend", GradeMin: "senior", GradeMax: "middle", MustSkills: []string{"go", "go", "postgresql"}, NiceSkills: []string{"kafka"}, SalaryFrom: 200000, SalaryTo: 300000, WorkFormats: []string{"remote"}}
	n, err := svc.CreateNeed(ctx, owner, in)
	if err != nil {
		t.Fatal(err)
	}
	if n.GradeMin != "middle" || n.GradeMax != "senior" {
		t.Fatalf("swapped grade range must be normalized: %s–%s", n.GradeMin, n.GradeMax)
	}
	if len(n.MustSkills) != 2 || len(n.NiceSkills) != 1 {
		t.Fatalf("skills deduplicated: %+v", n.MustSkills)
	}
	for _, bad := range []NeedInput{
		{Title: "", Specialization: "backend", GradeMin: "middle", GradeMax: "senior", SalaryFrom: 1, SalaryTo: 2},
		{Title: "x", Specialization: "common", GradeMin: "middle", GradeMax: "senior", SalaryFrom: 1, SalaryTo: 2},
		{Title: "x", Specialization: "backend", GradeMin: "lead", GradeMax: "senior", SalaryFrom: 1, SalaryTo: 2},
		{Title: "x", Specialization: "backend", GradeMin: "middle", GradeMax: "senior", SalaryFrom: 300, SalaryTo: 200},
		{Title: "x", Specialization: "backend", GradeMin: "middle", GradeMax: "senior", SalaryFrom: 1, SalaryTo: 2, WorkFormats: []string{"moon"}},
		{Title: "x", Specialization: "backend", GradeMin: "middle", GradeMax: "senior", SalaryFrom: 1, SalaryTo: 2, NiceSkills: []string{"no_such_skill"}},
	} {
		if _, err := svc.CreateNeed(ctx, owner, bad); !domain.Is(err, domain.KindValidation) {
			t.Fatalf("%+v must fail validation, got %v", bad, err)
		}
	}

	in.Title = "Senior Go-разработчик"
	in.City = "Москва"
	upd, err := svc.UpdateNeed(ctx, owner, n.ID, in)
	if err != nil || upd.Title != "Senior Go-разработчик" || upd.City != "Москва" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if got, err := svc.GetNeed(ctx, owner, n.ID); err != nil || got.ID != n.ID {
		t.Fatalf("get: %v", err)
	}
	if list, err := svc.ListNeeds(ctx, owner); err != nil || len(list) != 1 {
		t.Fatalf("list: %d %v", len(list), err)
	}
	if st, err := svc.SetStatus(ctx, owner, n.ID, "archived"); err != nil || st.Status != "archived" {
		t.Fatalf("archive: %+v %v", st, err)
	}
	if _, err := svc.SetStatus(ctx, owner, n.ID, "draft"); !domain.Is(err, domain.KindInvalidTransition) {
		t.Fatalf("archived → draft must be invalid, got %v", err)
	}

	// Чужая компания не видит и не меняет потребность.
	stranger := f.User(sqlcgen.UserRoleEmployer)
	if _, err := svc.UpsertCompany(ctx, stranger, CompanyInput{Name: "Чужая"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetNeed(ctx, stranger, n.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("foreign need must be invisible, got %v", err)
	}
	if _, err := svc.UpdateNeed(ctx, stranger, n.ID, in); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("foreign need must not be editable, got %v", err)
	}

	// Разбор текста — черновик с навыками из справочника.
	if _, err := svc.ExtractNeed(ctx, "коротко"); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("short text must fail, got %v", err)
	}
	ex, err := svc.ExtractNeed(ctx, "Ищем middle Go-разработчика: PostgreSQL, Kafka, удалённо, вилка 250 000 – 350 000 рублей")
	if err != nil || ex.Provider != "mock" || len(ex.MustSkills) == 0 || ex.Draft.SalaryTo != 350000 {
		t.Fatalf("extract: %+v %v", ex, err)
	}
}
