package candidates

import (
	"testing"

	"github.com/google/uuid"

	"fsp/internal/app/apptest"
	"fsp/internal/domain"
	"fsp/internal/storage/postgres/sqlcgen"
)

func TestVerifyShowsCategoriesOnlyForVisibleConsentingCandidate(t *testing.T) {
	f := apptest.New(t, "verify")
	svc := New(f.Pool, f.Log)

	ok := f.Candidate(apptest.CandidateOpts{Name: "Публичный", Consent: true, Percentile: 0.7})
	v, err := svc.Verify(f.Ctx, ok)
	if err != nil || len(v.Categories) != 1 {
		t.Fatalf("verify: %+v %v", v, err)
	}
	if c := v.Categories[0]; c.Specialization != "backend" || c.Grade != "middle" || c.Status != "confirmed" || c.Percentile == nil || *c.Percentile != 0.7 {
		t.Fatalf("category: %+v", c)
	}

	for name, id := range map[string]uuid.UUID{
		"без согласия на публикацию": f.Candidate(apptest.CandidateOpts{Name: "Без согласия", Consent: false}),
		"скрытый профиль":            f.Candidate(apptest.CandidateOpts{Name: "Скрытый", Consent: true, Visibility: sqlcgen.CandidateVisibilityHidden}),
		"без категории":              f.Candidate(apptest.CandidateOpts{Name: "Новый", Consent: true, NoCategory: true}),
		"неизвестный идентификатор":  uuid.New(),
	} {
		if _, err := svc.Verify(f.Ctx, id); !domain.Is(err, domain.KindNotFound) {
			t.Errorf("%s: ожидали «не найдено», получили %v", name, err)
		}
	}
}
