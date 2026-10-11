package authtoken

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"fsp/internal/domain"
)

func TestIssueAndParse(t *testing.T) {
	iss, err := New("0123456789abcdef0123456789abcdef", 15*time.Minute, "fsp")
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{UserID: uuid.New(), Role: domain.RoleCandidate, Email: "a@b.ru"}
	tok, exp, err := iss.Issue(p)
	if err != nil || tok == "" || exp.Before(time.Now()) {
		t.Fatalf("issue: %v", err)
	}
	got, err := iss.Parse(tok)
	if err != nil || got != p {
		t.Fatalf("parse: %+v %v", got, err)
	}
	other, _ := New("fedcba9876543210fedcba9876543210", 15*time.Minute, "fsp")
	if _, err := other.Parse(tok); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
	if _, err := New("short", time.Minute, "fsp"); err == nil {
		t.Fatal("short secret must be rejected")
	}
}

func TestExpired(t *testing.T) {
	iss, _ := New("0123456789abcdef0123456789abcdef", time.Minute, "fsp")
	iss.now = func() time.Time { return time.Now().Add(-10 * time.Minute) }
	tok, _, _ := iss.Issue(domain.Principal{UserID: uuid.New(), Role: domain.RoleEmployer})
	iss.now = time.Now
	if _, err := iss.Parse(tok); err == nil {
		t.Fatal("expired token must be rejected")
	}
}
