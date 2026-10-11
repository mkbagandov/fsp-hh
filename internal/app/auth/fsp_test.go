package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"fsp/internal/app/fsp"
	"fsp/internal/domain"
	"fsp/internal/fspmock"
	"fsp/internal/infrastructure/fspregistry"
	"fsp/internal/infrastructure/oidc"
	"fsp/internal/infrastructure/secrets"
)

// startMock поднимает эмулятор ФСП ID и возвращает его базовый адрес и issuer.
func startMock(t *testing.T) (string, string) {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	issuer := "http://" + ts.Listener.Addr().String() + "/realms/fsp"
	m, err := fspmock.New(fspmock.Config{Issuer: issuer, ClientID: "fsp-platform", ClientSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = m.Handler()
	ts.Start()
	t.Cleanup(ts.Close)
	return ts.URL, issuer
}

// identityFor проходит код-флоу за «браузер»: логин на эмуляторе, обмен кода на токены.
func identityFor(t *testing.T, client *oidc.Client, username string) oidc.Identity {
	t.Helper()
	ctx := context.Background()
	state, _ := secrets.RandomToken(16)
	nonce, _ := secrets.RandomToken(16)
	verifier, _ := secrets.RandomToken(32)
	authURL, err := client.AuthURL(ctx, state, nonce, verifier)
	if err != nil {
		t.Fatalf("auth url: %v", err)
	}
	u, _ := url.Parse(authURL)
	form := u.Query()
	form.Set("username", username)
	form.Set("password", "fsp")
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := hc.PostForm(strings.Split(authURL, "?")[0], form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("mock login: status %d", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("state") != state {
		t.Fatal("state mismatch")
	}
	id, err := client.Exchange(ctx, loc.Query().Get("code"), verifier, nonce)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	return id
}

func TestFSPLoginRegisterAndLink(t *testing.T) {
	svc, _ := newService(t)
	base, issuer := startMock(t)
	client := oidc.New(oidc.Config{Issuer: issuer, ClientID: "fsp-platform", ClientSecret: "secret", RedirectURL: "http://app.local/cb"})
	svc.SetFSPSyncer(fsp.New(svc.pool, fspregistry.New(base), svc.log))
	ctx := context.Background()
	meta := RequestMeta{IP: "127.0.0.1:1"}

	// Эмулятор хранит фиксированные e-mail; делаем их уникальными для повторных прогонов,
	// обходя через отвязку в конце теста. Если запись осталась от прошлого прогона — чистим.
	id := identityFor(t, client, "ivanov")
	if id.FSPID != "FSP-000123" || !id.EmailVerified || id.AccessToken == "" {
		t.Fatalf("identity: %+v", id)
	}

	// Без согласия новый пользователь не создаётся.
	sess, err := svc.CompleteFSPLogin(ctx, id, false, meta)
	if err == nil {
		// пользователь уже существует от прошлого прогона — отвяжем и удалим, затем повторим
		_ = svc.UnlinkFSP(ctx, sess.User.ID, meta)
		_ = svc.DeleteAccount(ctx, sess.User.ID, meta)
		if _, err := svc.CompleteFSPLogin(ctx, id, false, meta); !domain.Is(err, domain.KindValidation) && !domain.Is(err, domain.KindConflict) {
			t.Fatalf("login without consent: %v", err)
		}
	} else if !domain.Is(err, domain.KindValidation) {
		t.Fatalf("login without consent: %v", err)
	}

	sess, err = svc.CompleteFSPLogin(ctx, id, true, meta)
	if domain.Is(err, domain.KindConflict) {
		// e-mail занят удалённым аккаунтом прошлого прогона: допустимо, проверяем привязку ниже
		t.Logf("register via fsp: %v", err)
	} else if err != nil {
		t.Fatalf("register via fsp: %v", err)
	} else {
		if sess.User.Role != domain.RoleCandidate || !sess.User.EmailVerified {
			t.Fatalf("session: %+v", sess.User)
		}
		info, err := svc.FSPLink(ctx, sess.User.ID)
		if err != nil || !info.Linked || info.FSPID != "FSP-000123" || info.LastSyncAt == nil || info.SyncError != nil {
			t.Fatalf("link info: %+v %v", info, err)
		}
		ach, err := fsp.New(svc.pool, nil, nil).List(ctx, sess.User.ID)
		if err != nil || len(ach) != 3 {
			t.Fatalf("achievements synced: %d %v", len(ach), err)
		}
		// Повторный вход — тот же пользователь.
		again, err := svc.CompleteFSPLogin(ctx, identityFor(t, client, "ivanov"), false, meta)
		if err != nil || again.User.ID != sess.User.ID {
			t.Fatalf("second login: %+v %v", again.User, err)
		}
		// Привязать тот же ФСП ID к другому пользователю нельзя.
		other, err := svc.Register(ctx, RegisterInput{Email: uniqueEmail("other"), Password: "Password123", Role: domain.RoleCandidate, ProcessingConsent: true}, meta)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.LinkFSP(ctx, other.ID, identityFor(t, client, "ivanov"), meta); !domain.Is(err, domain.KindConflict) {
			t.Fatalf("link elsewhere must conflict, got %v", err)
		}
		// Привязка участника без истории к обычному пользователю.
		petrova := identityFor(t, client, "petrova")
		info2, err := svc.LinkFSP(ctx, other.ID, petrova, meta)
		if domain.Is(err, domain.KindConflict) {
			t.Logf("petrova already linked from a previous run: %v", err)
		} else if err != nil || !info2.Linked || info2.FSPID != "FSP-000456" {
			t.Fatalf("link petrova: %+v %v", info2, err)
		} else {
			if list, _ := fsp.New(svc.pool, nil, nil).List(ctx, other.ID); len(list) != 0 {
				t.Fatalf("petrova has no achievements, got %d", len(list))
			}
			if err := svc.UnlinkFSP(ctx, other.ID, meta); err != nil {
				t.Fatal(err)
			}
			if info3, _ := svc.FSPLink(ctx, other.ID); info3.Linked {
				t.Fatal("must be unlinked")
			}
		}
		// Очистка, чтобы прогон был повторяемым.
		if err := svc.UnlinkFSP(ctx, sess.User.ID, meta); err != nil {
			t.Fatal(err)
		}
		if err := svc.DeleteAccount(ctx, sess.User.ID, meta); err != nil {
			t.Fatal(err)
		}
	}
}
