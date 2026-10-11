package testing

import (
	"context"
	"errors"
	"slices"
	"strings"
	stdtesting "testing"

	"fsp/internal/domain"
)

// Тест доступен только по специализациям с собственным банком заданий: без него категория «QA middle»
// подтверждалась бы одними общими заданиями.
func TestOnlySpecializationsWithOwnBankAreTestable(t *stdtesting.T) {
	got := TestableSpecializations(DefaultMinSpecFamilies)
	if !slices.Equal(got, []string{"backend", "frontend", "data_ml", "devops"}) {
		t.Fatalf("testable specializations: %v", got)
	}
	svc, uid := newService(t)
	ctx := context.Background()
	if _, err := svc.SubmitSurvey(ctx, uid, SurveyInput{Specialization: "qa", ClaimedGrade: "middle"}); err != nil {
		t.Fatalf("survey for qa is still allowed: %v", err)
	}
	_, _, err := svc.StartTest(ctx, uid, StartInput{Specialization: "qa", TargetGrade: "middle"})
	if !domain.Is(err, domain.KindConflict) {
		t.Fatalf("qa test must be refused, got %v", err)
	}
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "bank_too_small" || !strings.Contains(de.Message, "Backend") {
		t.Fatalf("refusal must explain where the test is available: %+v", de)
	}
}
