package notifications

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"fsp/internal/app/apptest"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/mail"
	"fsp/internal/storage/postgres/sqlcgen"
)

func TestNotifyFeedAndMail(t *testing.T) {
	f := apptest.New(t, "ntf")
	ctx := f.Ctx
	mailer := mail.NewConsole(f.Log, 20)
	svc := New(f.Pool, mailer, "http://front.example/", f.Log)
	u := f.User(sqlcgen.UserRoleCandidate)
	user, err := f.Q.GetUserByID(ctx, u)
	if err != nil {
		t.Fatal(err)
	}

	svc.Notify(ctx, u, Event{Kind: KindInvitationReceived, Title: "Новое приглашение", Body: "Вилка 200–300 тыс.", Link: "/candidate/invitations/1", Payload: map[string]any{"invitation_id": 1}, Email: true})
	svc.Notify(ctx, u, Event{Kind: KindSystem, Title: "Без письма"})
	msg, ok := mailer.Last(user.Email)
	if !ok || msg.Subject != "ФСП: Новое приглашение" || !strings.Contains(msg.Text, "http://front.example/candidate/invitations/1") {
		t.Fatalf("mail: %+v %v", msg, ok)
	}
	if n := len(mailer.Recent(20)); n != 1 {
		t.Fatalf("only events with Email=true are mailed, got %d", n)
	}
	items, err := svc.List(ctx, u, false, 0)
	if err != nil || len(items) != 2 || items[0].Title != "Без письма" || string(items[1].Payload) == "" {
		t.Fatalf("feed: %+v %v", items, err)
	}
	if n, _ := svc.UnreadCount(ctx, u); n != 2 {
		t.Fatalf("unread: %d", n)
	}
	if err := svc.MarkRead(ctx, u, items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkRead(ctx, uuid.New(), items[1].ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("foreign notification must be not found, got %v", err)
	}
	if unread, _ := svc.List(ctx, u, true, 10); len(unread) != 1 {
		t.Fatalf("unread filter: %d", len(unread))
	}
	if n, err := svc.MarkAllRead(ctx, u); err != nil || n != 1 {
		t.Fatalf("mark all: %d %v", n, err)
	}
	// Заблокированному пользователю письма не уходят (уведомление в ленте остаётся).
	if err := f.Q.UpdateUserStatus(ctx, sqlcgen.UpdateUserStatusParams{ID: u, Status: sqlcgen.UserStatusBlocked}); err != nil {
		t.Fatal(err)
	}
	svc.Notify(ctx, u, Event{Kind: KindSystem, Title: "После блокировки", Email: true})
	if last, _ := mailer.Last(user.Email); last.Subject != "ФСП: Новое приглашение" {
		t.Fatalf("blocked user must not be mailed: %+v", last)
	}
}
