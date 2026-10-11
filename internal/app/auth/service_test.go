package auth

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"testing"
	"time"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/authtoken"
	"fsp/internal/infrastructure/mail"
	"fsp/internal/storage/postgres/pgtest"
)

var codeRe = regexp.MustCompile(`\b(\d{6})\b`)

func newService(t *testing.T) (*Service, *mail.Console) {
	t.Helper()
	pool := pgtest.Pool(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tokens, err := authtoken.New("test-secret-test-secret-test-secret-1234", 15*time.Minute, "fsp-test")
	if err != nil {
		t.Fatal(err)
	}
	mailer := mail.NewConsole(nil, 50)
	return New(Config{}, pool, tokens, mailer, log), mailer
}

func uniqueEmail(prefix string) string {
	return prefix + "+" + time.Now().Format("150405.000000") + "@test.local"
}

func TestRegisterVerifyLoginRefreshLogout(t *testing.T) {
	svc, mailer := newService(t)
	ctx := context.Background()
	email := uniqueEmail("cand")
	meta := RequestMeta{IP: "127.0.0.1:5000", UserAgent: "go-test"}

	u, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleCandidate, ProcessingConsent: true}, meta)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.EmailVerified || u.Role != domain.RoleCandidate {
		t.Fatalf("unexpected user: %+v", u)
	}

	// Повторная регистрация — конфликт.
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleCandidate, ProcessingConsent: true}, meta); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("duplicate register must conflict, got %v", err)
	}
	// Вход до подтверждения запрещён.
	if _, err := svc.Login(ctx, email, "Password123", meta); !domain.Is(err, domain.KindForbidden) {
		t.Fatalf("login before verify must be forbidden, got %v", err)
	}

	msg, ok := mailer.Last(email)
	if !ok {
		t.Fatal("verification mail not sent")
	}
	code := codeRe.FindString(msg.Text)
	if code == "" {
		t.Fatalf("no code in mail: %q", msg.Text)
	}
	// Неверный код.
	if _, err := svc.VerifyEmail(ctx, email, "000000", meta); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("wrong code must fail validation, got %v", err)
	}
	sess, err := svc.VerifyEmail(ctx, email, code, meta)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !sess.User.EmailVerified || sess.AccessToken == "" || sess.RefreshToken == "" {
		t.Fatalf("session after verify: %+v", sess)
	}

	// Вход с неверным паролем.
	if _, err := svc.Login(ctx, email, "wrong", meta); !domain.Is(err, domain.KindUnauthorized) {
		t.Fatalf("wrong password: %v", err)
	}
	sess2, err := svc.Login(ctx, email, "Password123", meta)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Refresh ротирует токен; старый больше не работает, повторное использование закрывает все сессии.
	sess3, err := svc.Refresh(ctx, sess2.RefreshToken, meta)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if sess3.RefreshToken == sess2.RefreshToken {
		t.Fatal("refresh token must rotate")
	}
	if _, err := svc.Refresh(ctx, sess2.RefreshToken, meta); !domain.Is(err, domain.KindUnauthorized) {
		t.Fatalf("reused refresh must be rejected, got %v", err)
	}
	if _, err := svc.Refresh(ctx, sess3.RefreshToken, meta); !domain.Is(err, domain.KindUnauthorized) {
		t.Fatalf("after reuse detection all sessions must be revoked, got %v", err)
	}

	sess4, err := svc.Login(ctx, email, "Password123", meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(ctx, sess4.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, sess4.RefreshToken, meta); !domain.Is(err, domain.KindUnauthorized) {
		t.Fatalf("refresh after logout must fail, got %v", err)
	}

	// Me и согласия.
	info, consents, err := svc.Me(ctx, sess4.User.ID)
	if err != nil || info.Email != email || len(consents) != 1 || consents[0].Kind != domain.ConsentProcessing {
		t.Fatalf("me: %+v %+v %v", info, consents, err)
	}
	if _, err := svc.GiveConsent(ctx, info.ID, domain.ConsentPublication, meta); err != nil {
		t.Fatal(err)
	}
	if err := svc.WithdrawConsent(ctx, info.ID, domain.ConsentProcessing, meta); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("processing consent must not be withdrawable, got %v", err)
	}
	if err := svc.WithdrawConsent(ctx, info.ID, domain.ConsentPublication, meta); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordResetAndValidation(t *testing.T) {
	svc, mailer := newService(t)
	ctx := context.Background()
	email := uniqueEmail("reset")
	meta := RequestMeta{IP: "10.0.0.1:1", UserAgent: "t"}

	if _, err := svc.Register(ctx, RegisterInput{Email: "bad-email", Password: "Password123", Role: domain.RoleEmployer, ProcessingConsent: true}, meta); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("bad email: %v", err)
	}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "short", Role: domain.RoleEmployer, ProcessingConsent: true}, meta); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("short password: %v", err)
	}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleAdmin, ProcessingConsent: true}, meta); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("admin self-register: %v", err)
	}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleEmployer}, meta); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("missing consent: %v", err)
	}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleEmployer, ProcessingConsent: true}, meta); err != nil {
		t.Fatal(err)
	}

	// Сброс пароля также подтверждает e-mail (владение почтой доказано).
	if err := svc.ForgotPassword(ctx, email); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, "nobody@test.local"); err != nil {
		t.Fatal("unknown email must not error")
	}
	msg, _ := mailer.Last(email)
	code := codeRe.FindString(msg.Text)
	if err := svc.ResetPassword(ctx, email, code, "NewPassword456", meta); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := svc.Login(ctx, email, "Password123", meta); err == nil {
		t.Fatal("old password must not work")
	}
	sess, err := svc.Login(ctx, email, "NewPassword456", meta)
	if err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	if err := svc.ChangePassword(ctx, sess.User.ID, "wrong", "Another789x", meta); !domain.Is(err, domain.KindValidation) {
		t.Fatalf("change with wrong current: %v", err)
	}
	if err := svc.ChangePassword(ctx, sess.User.ID, "NewPassword456", "Another789x", meta); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, sess.RefreshToken, meta); !domain.Is(err, domain.KindUnauthorized) {
		t.Fatal("sessions must be revoked after password change")
	}
	if err := svc.DeleteAccount(ctx, sess.User.ID, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, email, "Another789x", meta); err == nil {
		t.Fatal("deleted account must not log in")
	}
}

func TestCodeAttemptsLimit(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	email := uniqueEmail("attempts")
	meta := RequestMeta{}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleCandidate, ProcessingConsent: true}, meta); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.VerifyEmail(ctx, email, "111111", meta); !domain.Is(err, domain.KindValidation) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := svc.VerifyEmail(ctx, email, "111111", meta); !domain.Is(err, domain.KindValidation) && !domain.Is(err, domain.KindRateLimited) {
		t.Fatalf("after limit: %v", err)
	}
}
