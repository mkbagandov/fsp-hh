package auth

import (
	"context"
	"sync"
	"testing"

	"fsp/internal/domain"
)

// Регрессия: /auth/verify для уже подтверждённого адреса открывал сессию без проверки кода —
// вход в любой аккаунт (в том числе администратора) по одному e-mail.
func TestVerifyNeverLogsIntoVerifiedAccount(t *testing.T) {
	svc, mailer := newService(t)
	ctx := context.Background()
	email := uniqueEmail("verified")
	meta := RequestMeta{IP: "127.0.0.1:5000"}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleCandidate, ProcessingConsent: true}, meta); err != nil {
		t.Fatal(err)
	}
	msg, _ := mailer.Last(email)
	code := codeRe.FindString(msg.Text)
	if _, err := svc.VerifyEmail(ctx, email, code, meta); err != nil {
		t.Fatalf("first verification: %v", err)
	}
	for _, c := range []string{"000000", code, ""} {
		sess, err := svc.VerifyEmail(ctx, email, c, meta)
		if !domain.Is(err, domain.KindValidation) || sess.AccessToken != "" {
			t.Fatalf("verify(%q) on verified account must fail without a session, got %v / %q", c, err, sess.AccessToken)
		}
	}
	// Заблокированный пользователь не получает сессию ни по паролю, ни по коду.
	u, err := svc.q.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.pool.Exec(ctx, `UPDATE users SET status = 'blocked' WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, email, "Password123", meta); !domain.Is(err, domain.KindForbidden) {
		t.Fatalf("blocked login must be forbidden, got %v", err)
	}
	if u, err = svc.q.GetUserByID(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.openSession(ctx, u, meta); err == nil {
		t.Fatal("openSession must refuse a non-active user")
	}
}

// Верный код, отправленный параллельно, открывает ровно одну сессию; после исчерпания попыток
// даже верный код не принимается.
func TestCodeIsAtomic(t *testing.T) {
	svc, mailer := newService(t)
	ctx := context.Background()
	email := uniqueEmail("atomic")
	meta := RequestMeta{}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleCandidate, ProcessingConsent: true}, meta); err != nil {
		t.Fatal(err)
	}
	msg, _ := mailer.Last(email)
	code := codeRe.FindString(msg.Text)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.VerifyEmail(ctx, email, code, meta); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("exactly one parallel verification must succeed, got %d", ok)
	}

	// Сброс пароля: 5 неверных попыток сжигают код, верный код после этого не работает.
	if err := svc.ForgotPassword(ctx, email); err != nil {
		t.Fatal(err)
	}
	msg, _ = mailer.Last(email)
	reset := codeRe.FindString(msg.Text)
	for i := 0; i < 5; i++ {
		if err := svc.ResetPassword(ctx, email, "999999", "NewPassword123", meta); err == nil {
			t.Fatal("wrong code accepted")
		}
	}
	if err := svc.ResetPassword(ctx, email, reset, "NewPassword123", meta); err == nil {
		t.Fatal("code must be burnt after the attempt limit")
	}
}

// Параллельный обмен одного refresh-токена создаёт не больше одной новой сессии.
func TestRefreshRotationIsAtomic(t *testing.T) {
	svc, mailer := newService(t)
	ctx := context.Background()
	email := uniqueEmail("rotate")
	meta := RequestMeta{}
	if _, err := svc.Register(ctx, RegisterInput{Email: email, Password: "Password123", Role: domain.RoleCandidate, ProcessingConsent: true}, meta); err != nil {
		t.Fatal(err)
	}
	msg, _ := mailer.Last(email)
	sess, err := svc.VerifyEmail(ctx, email, codeRe.FindString(msg.Text), meta)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Refresh(ctx, sess.RefreshToken, meta); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("exactly one parallel refresh must succeed, got %d", ok)
	}
}
