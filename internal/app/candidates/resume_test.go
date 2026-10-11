package candidates

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/llm"
)

// Круговой сценарий: заполненный профиль → PDF-профиль (с контактами и без) → загрузка этого PDF как резюме
// другим кандидатом → черновик → применение блоков. Проверяет и защиту загрузки (сигнатура, размер, имя файла, чужой черновик).
func TestProfilePDFAndResumeRoundTrip(t *testing.T) {
	src, uid := newCandidate(t)
	ctx := context.Background()
	src.SetPDFDeps(PDFDeps{VerifyBase: "https://fsp.example/verify/", FSPLinked: func(context.Context, uuid.UUID) (bool, error) { return false, nil }})

	if _, err := src.Update(ctx, uid, UpdateInput{
		FirstName: "Анна", LastName: "Резюмева", City: ptr("Казань"), WorkFormats: []string{"remote", "hybrid"}, Visibility: "open",
		Headline: ptr("Backend-разработчик на Go"), About: ptr("Пишу сервисы на Go и PostgreSQL, люблю наблюдаемость."),
		ExperienceYears: ptr(4.5), SalaryExpected: ptr(int64(260000)), PrimarySpecialization: ptr("backend"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.ReplaceExperiences(ctx, uid, []ExperienceInput{
		{CompanyName: "Орбита Пэй", Position: "Backend-разработчик", StartedOn: "2021-02-01", Description: ptr("Платёжный шлюз на Go"), Stack: []string{"go", "postgresql"}},
		{CompanyName: "Квант Логистика", Position: "Python-разработчик", StartedOn: "2019-01-01", EndedOn: ptr("2021-01-01")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.ReplaceEducation(ctx, uid, []EducationInput{{Institution: "КФУ", Degree: ptr("бакалавр"), Field: ptr("Прикладная информатика"), GraduatedYear: ptr(int32(2019))}}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.SetSkills(ctx, uid, []SkillInput{{Code: "go"}, {Code: "postgresql"}, {Code: "kafka"}}, domain.SkillDeclared); err != nil {
		t.Fatal(err)
	}
	if _, err := src.UpdateContacts(ctx, uid, Contacts{Phone: ptr("+7 900 123-45-67"), Telegram: ptr("@anna_go"), Email: ptr("anna@example.com"), Other: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	contacts, err := src.GetContacts(ctx, uid)
	if err != nil || contacts.Telegram == nil || *contacts.Telegram != "anna_go" {
		t.Fatalf("telegram is stored without @: %+v %v", contacts, err)
	}
	if pr, err := src.GetPrivacy(ctx, uid); err != nil || !pr.ShowFullName || !pr.ShowFSP {
		t.Fatalf("privacy defaults are open: %+v %v", pr, err)
	}

	full, err := src.ProfilePDF(ctx, uid, true)
	if err != nil || !bytes.HasPrefix(full, []byte("%PDF")) {
		t.Fatalf("full pdf: %v", err)
	}
	public, err := src.ProfilePDF(ctx, uid, false)
	if err != nil || !bytes.HasPrefix(public, []byte("%PDF")) {
		t.Fatalf("public pdf: %v", err)
	}
	if len(public) >= len(full) {
		t.Fatalf("public profile must not contain contacts block: %d >= %d bytes", len(public), len(full))
	}

	dst, uid2 := newCandidate(t)
	dst.SetResumeDeps(ResumeDeps{LLM: llm.NewMock(), UploadDir: t.TempDir()})
	if _, err := dst.UploadResume(ctx, uid2, "cv.pdf", []byte("not a pdf at all")); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("non-pdf must be rejected, got %v", err)
	}
	if _, err := dst.UploadResume(ctx, uid2, "cv.pdf", nil); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("empty file must be rejected, got %v", err)
	}
	up, err := dst.UploadResume(ctx, uid2, `..\..\etc/passwd.pdf`, full)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	t.Logf("chars=%d warnings=%v first=%q last=%q headline=%q", up.TextChars, up.Draft.Warnings, up.Draft.FirstName, up.Draft.LastName, up.Draft.Headline)
	if up.FileName != "passwd.pdf" || up.Parser != "mock" || up.SizeBytes != len(full) {
		t.Fatalf("upload meta: %+v", up)
	}
	draft, err := dst.GetResumeDraft(ctx, uid2, up.ID)
	if err != nil || draft.ID != up.ID {
		t.Fatalf("draft: %+v %v", draft, err)
	}
	if _, err := src.GetResumeDraft(ctx, uid, up.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("someone else's draft must be invisible, got %v", err)
	}
	prof, err := dst.ApplyResumeDraft(ctx, uid2, up.ID, ApplyInput{Profile: true, Contacts: true, Experiences: true, Education: true, Skills: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !prof.ResumeUploaded {
		t.Fatal("profile must remember the uploaded resume")
	}
	if _, err := dst.ApplyResumeDraft(ctx, uid2, uuid.New(), ApplyInput{Profile: true}); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("unknown upload must be not found, got %v", err)
	}
	// Без настроенного разбора загрузка выключена явно.
	plain, uid3 := newCandidate(t)
	if _, err := plain.UploadResume(ctx, uid3, "cv.pdf", full); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("resume disabled must conflict, got %v", err)
	}
}
