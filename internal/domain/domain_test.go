package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestErrorsKindsAndWrapping(t *testing.T) {
	base := Validation("salary.to", "salary_to_required", "Укажите верхнюю границу")
	if base.Error() != "salary_to_required (salary.to): Укажите верхнюю границу" {
		t.Fatalf("message with field: %q", base.Error())
	}
	if Conflict("x", "y").Error() != "x: y" {
		t.Fatal("message without field")
	}
	cause := errors.New("db down")
	wrapped := NotFound("need").Wrap(cause)
	if !errors.Is(wrapped, cause) || wrapped.Code != "need_not_found" {
		t.Fatalf("wrap must keep code and cause: %+v", wrapped)
	}
	// Ошибка, обёрнутая через fmt.Errorf, сохраняет класс — сервисы так и делают.
	outer := fmt.Errorf("create need: %w", InvalidTransition("need", "archived", "draft"))
	if KindOf(outer) != KindInvalidTransition || !Is(outer, KindInvalidTransition) {
		t.Fatal("kind must survive fmt.Errorf wrapping")
	}
	if KindOf(errors.New("foreign")) != KindUnknown || Is(nil, KindNotFound) {
		t.Fatal("foreign errors are unknown")
	}
	if Forbidden("contacts_not_granted", "закрыто").Kind != KindForbidden || E(KindRateLimited, "q", "m").Kind != KindRateLimited {
		t.Fatal("constructors must set kind")
	}
}

func TestPrincipalAndRoles(t *testing.T) {
	ctx := context.Background()
	if _, err := RequirePrincipal(ctx); !Is(err, KindUnauthorized) {
		t.Fatalf("anonymous must be unauthorized, got %v", err)
	}
	if _, err := RequireRole(ctx, RoleEmployer); !Is(err, KindUnauthorized) {
		t.Fatalf("anonymous role check must be unauthorized, got %v", err)
	}
	p := Principal{UserID: uuid.New(), Role: RoleCandidate, Email: "a@test.local"}
	ctx = WithPrincipal(ctx, p)
	got, err := RequireRole(ctx, RoleCandidate, RoleAdmin)
	if err != nil || got.UserID != p.UserID {
		t.Fatalf("candidate must pass: %v", err)
	}
	if _, err := RequireRole(ctx, RoleEmployer); !Is(err, KindForbidden) {
		t.Fatalf("wrong role must be forbidden, got %v", err)
	}
	if !RoleAdmin.Valid() || Role("root").Valid() {
		t.Fatal("role validity")
	}
}

func TestSalaryRangeValidateAndFit(t *testing.T) {
	cases := []struct {
		r    SalaryRange
		code string
	}{
		{SalaryRange{From: 0, To: 100}, "salary_from_required"},
		{SalaryRange{From: 100, To: 0}, "salary_to_required"},
		{SalaryRange{From: 300, To: 200}, "salary_range_invalid"},
		{SalaryRange{From: 1, To: MaxSalary + 1}, "salary_too_large"},
		{SalaryRange{From: 200000, To: 300000}, ""},
	}
	for _, c := range cases {
		err := c.r.Validate()
		var de *Error
		switch {
		case c.code == "" && err != nil:
			t.Errorf("%+v: unexpected %v", c.r, err)
		case c.code != "" && (!errors.As(err, &de) || de.Code != c.code):
			t.Errorf("%+v: want %s, got %v", c.r, c.code, err)
		}
	}
	r := SalaryRange{From: 200000, To: 300000}
	if !r.Contains(200000) || !r.Contains(300000) || r.Contains(300001) || r.Contains(199999) {
		t.Fatal("contains is inclusive")
	}
	if !r.Covers(150000) || !r.Covers(300000) || r.Covers(300001) || r.Covers(0) {
		t.Fatal("covers: expectation not above the upper bound")
	}
	for _, c := range []struct {
		expected int64
		want     float64
	}{{0, 0.7}, {150000, 1}, {300000, 1}, {340000, 0.5}, {346000, 0}} {
		if got := r.FitScore(c.expected); got != c.want {
			t.Errorf("FitScore(%d) = %v, want %v", c.expected, got, c.want)
		}
	}
}

func TestStatusValidityAndTransitions(t *testing.T) {
	if !VisibilityOpen.Valid() || !VisibilityHidden.Valid() || Visibility("public").Valid() {
		t.Fatal("visibility")
	}
	if !GradeConfirmed.Valid() || GradeStatus("maybe").Valid() {
		t.Fatal("grade status")
	}
	if SkillTestConfirmed.Weight() <= SkillDeclared.Weight() {
		t.Fatal("test-confirmed skill must weigh more than declared")
	}
	needOK := [][2]NeedStatus{{NeedDraft, NeedActive}, {NeedActive, NeedArchived}, {NeedArchived, NeedActive}, {NeedActive, NeedDraft}}
	for _, c := range needOK {
		if !c[0].CanTransitionTo(c[1]) {
			t.Errorf("need %s → %s must be allowed", c[0], c[1])
		}
	}
	if NeedArchived.CanTransitionTo(NeedDraft) {
		t.Error("archived need goes back only to active")
	}
	if !VacancyDraft.CanTransitionTo(VacancyPublished) || VacancyDraft.CanTransitionTo(VacancyClosed) || VacancyClosed.CanTransitionTo(VacancyPublished) {
		t.Error("vacancy transitions")
	}
	if !ApplicationSent.CanTransitionTo(ApplicationViewed) || ApplicationSent.CanTransitionTo(ApplicationInvited) || ApplicationRejected.CanTransitionTo(ApplicationInvited) {
		t.Error("application: invite only after viewing, final states are final")
	}
	if !InvitationSent.IsActive() || InvitationAccepted.IsActive() {
		t.Error("invitation activity")
	}
}
