package domain

import "testing"

func TestInvitationTransitions(t *testing.T) {
	ok := [][2]InvitationStatus{
		{InvitationSent, InvitationViewed},
		{InvitationSent, InvitationWithdrawn},
		{InvitationSent, InvitationExpired},
		{InvitationViewed, InvitationAccepted},
		{InvitationViewed, InvitationDeclined},
	}
	bad := [][2]InvitationStatus{
		{InvitationSent, InvitationAccepted}, // принять нельзя, не посмотрев условия
		{InvitationAccepted, InvitationDeclined},
		{InvitationDeclined, InvitationAccepted},
		{InvitationExpired, InvitationViewed},
		{InvitationWithdrawn, InvitationAccepted},
	}
	for _, c := range ok {
		if !c[0].CanTransitionTo(c[1]) {
			t.Errorf("invitation %s → %s must be allowed", c[0], c[1])
		}
	}
	for _, c := range bad {
		if c[0].CanTransitionTo(c[1]) {
			t.Errorf("invitation %s → %s must be forbidden", c[0], c[1])
		}
	}
	if !InvitationAccepted.IsFinal() || InvitationViewed.IsFinal() {
		t.Error("final statuses")
	}
	if !InvitationSent.IsActive() || InvitationDeclined.IsActive() {
		t.Error("active statuses")
	}
}

func TestApplicationAndVacancyTransitions(t *testing.T) {
	if !ApplicationSent.CanTransitionTo(ApplicationViewed) || !ApplicationViewed.CanTransitionTo(ApplicationInvited) {
		t.Error("application happy path")
	}
	if ApplicationSent.CanTransitionTo(ApplicationInvited) || ApplicationRejected.CanTransitionTo(ApplicationInvited) {
		t.Error("application forbidden transitions")
	}
	if !VacancyDraft.CanTransitionTo(VacancyPublished) || !VacancyPublished.CanTransitionTo(VacancyClosed) {
		t.Error("vacancy happy path")
	}
	if VacancyClosed.CanTransitionTo(VacancyPublished) || VacancyDraft.CanTransitionTo(VacancyClosed) {
		t.Error("vacancy forbidden transitions")
	}
}

func TestTestSessionAndAssignmentTransitions(t *testing.T) {
	if !TestCreated.CanTransitionTo(TestInProgress) || !TestInProgress.CanTransitionTo(TestCompleted) {
		t.Error("session happy path")
	}
	if TestCreated.CanTransitionTo(TestCompleted) || TestCompleted.CanTransitionTo(TestInProgress) {
		t.Error("session forbidden transitions")
	}
	if !TestInProgress.IsOpen() || TestExpired.IsOpen() {
		t.Error("session open")
	}
	if !AssignmentOffered.CanTransitionTo(AssignmentAccepted) || !AssignmentSubmitted.CanTransitionTo(AssignmentReviewed) {
		t.Error("assignment happy path")
	}
	if AssignmentOffered.CanTransitionTo(AssignmentSubmitted) || AssignmentReviewed.CanTransitionTo(AssignmentSubmitted) {
		t.Error("assignment forbidden transitions")
	}
}

func TestSalaryRange(t *testing.T) {
	if err := (SalaryRange{From: 150_000, To: 250_000}).Validate(); err != nil {
		t.Fatalf("valid range rejected: %v", err)
	}
	cases := []SalaryRange{{0, 100}, {100, 0}, {300, 200}, {1, MaxSalary + 1}}
	for _, r := range cases {
		err := r.Validate()
		if err == nil || !Is(err, KindValidation) {
			t.Errorf("range %+v must fail validation, got %v", r, err)
		}
	}
	r := SalaryRange{From: 200_000, To: 300_000}
	if !r.Covers(250_000) || !r.Covers(100_000) || r.Covers(350_000) {
		t.Error("covers")
	}
	if r.FitScore(250_000) != 1.0 || r.FitScore(340_000) != 0.5 || r.FitScore(400_000) != 0 || r.FitScore(0) != 0.7 {
		t.Error("fit score")
	}
}

func TestErrors(t *testing.T) {
	err := NotFound("candidate")
	if !Is(err, KindNotFound) || err.Code != "candidate_not_found" {
		t.Errorf("not found: %v", err)
	}
	tr := InvalidTransition("invitation", "sent", "accepted")
	if !Is(tr, KindInvalidTransition) {
		t.Errorf("transition kind: %v", tr)
	}
	if KindOf(nil) != KindUnknown {
		t.Error("nil error kind")
	}
	if SkillTestConfirmed.Weight() != 1.0 || SkillDeclared.Weight() != 0.5 {
		t.Error("skill weights")
	}
}
