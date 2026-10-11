package auth_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"fsp/internal/app/apptest"
	"fsp/internal/app/auth"
	"fsp/internal/app/employers"
	"fsp/internal/infrastructure/authtoken"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/infrastructure/mail"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Удаление аккаунта стирает персональные данные кандидата и отзывает доступ к контактам (152-ФЗ).
func TestDeleteAccountWipesCandidateData(t *testing.T) {
	f := apptest.New(t, "del")
	ctx := f.Ctx
	tokens, err := authtoken.New("test-secret-test-secret-test-secret-1234", 15*time.Minute, "fsp-test")
	if err != nil {
		t.Fatal(err)
	}
	svc := auth.New(auth.Config{}, f.Pool, tokens, mail.NewConsole(nil, 10), f.Log)

	cand := f.Candidate(apptest.CandidateOpts{Name: "Удаляемый", Consent: true, Salary: 210000, Skills: []string{"go"}, Telegram: "wipe_me"})
	t.Cleanup(func() { _, _ = f.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, cand) })
	owner := f.User(sqlcgen.UserRoleEmployer)
	company, err := employers.New(f.Pool, llm.NewMock(), f.Log).UpsertCompany(ctx, owner, employers.CompanyInput{Name: "Получатель контактов"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pool.Exec(ctx, `INSERT INTO contact_grants (candidate_user_id, company_id, source, source_id) VALUES ($1, $2, 'application_sent', gen_random_uuid())`, cand, company.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Q.ContactsForCompany(ctx, sqlcgen.ContactsForCompanyParams{UserID: cand, CompanyID: company.ID}); err != nil {
		t.Fatalf("contacts must be visible before deletion: %v", err)
	}
	file := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(file, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Q.CreateResumeUpload(ctx, sqlcgen.CreateResumeUploadParams{UserID: cand, FileName: "resume.pdf", Mime: "application/pdf", SizeBytes: 8, StoragePath: file}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteAccount(ctx, cand, auth.RequestMeta{}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Q.ContactsForCompany(ctx, sqlcgen.ContactsForCompanyParams{UserID: cand, CompanyID: company.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("contacts must be gone after deletion, got %v", err)
	}
	var email, first, visibility string
	var contacts, grants, skills, uploads int
	if err := f.Pool.QueryRow(ctx, `SELECT u.email, p.first_name, p.visibility::text,
		(SELECT count(*) FROM candidate_contacts WHERE user_id = u.id),
		(SELECT count(*) FROM contact_grants WHERE candidate_user_id = u.id AND revoked_at IS NULL),
		(SELECT count(*) FROM candidate_skills WHERE user_id = u.id),
		(SELECT count(*) FROM resume_uploads WHERE user_id = u.id)
		FROM users u JOIN candidate_profiles p ON p.user_id = u.id WHERE u.id = $1`, cand).
		Scan(&email, &first, &visibility, &contacts, &grants, &skills, &uploads); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(email, "deleted+") || first != "" || visibility != "hidden" || contacts+grants+skills+uploads != 0 {
		t.Fatalf("data not wiped: email=%s first=%q vis=%s contacts=%d grants=%d skills=%d uploads=%d", email, first, visibility, contacts, grants, skills, uploads)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resume file must be removed, stat: %v", err)
	}
}
