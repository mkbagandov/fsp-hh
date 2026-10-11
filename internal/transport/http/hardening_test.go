package httptransport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/authtoken"
)

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRealIPTrustsOnlyConfiguredProxies(t *testing.T) {
	var seen string
	h := realIP([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.RemoteAddr }))
	cases := []struct{ peer, xff, want string }{
		{"10.0.0.5:1234", "1.2.3.4", "1.2.3.4:0"},
		{"10.0.0.5:1234", "6.6.6.6, 1.2.3.4, 10.0.0.7", "1.2.3.4:0"}, // левые адреса клиент может подделать
		{"8.8.8.8:1234", "1.2.3.4", "8.8.8.8:1234"},                  // не прокси — заголовок игнорируется
		{"10.0.0.5:1234", "not-an-ip", "10.0.0.5:1234"},
		{"10.0.0.5:1234", "", "10.0.0.5:1234"},
		{"[::ffff:10.0.0.5]:1234", "2001:db8::1", "[2001:db8::1]:0"},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		serve(h, r)
		if seen != c.want {
			t.Errorf("peer %s xff %q: got %s, want %s", c.peer, c.xff, seen, c.want)
		}
	}
	// Без доверенных прокси заголовок не влияет ни на что.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:1"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	serve(realIP(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.RemoteAddr })), r)
	if seen != "10.0.0.5:1" {
		t.Fatalf("no proxies configured: %s", seen)
	}
}

func TestRateLimitPathsAffectsOnlyListedRoutes(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := rateLimitPaths([]string{"/auth/login"}, 2)(ok)
	codes := []int{}
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest(http.MethodPost, APIPrefix+"/auth/login", nil)
		r.RemoteAddr = "192.0.2.1:5000"
		codes = append(codes, serve(h, r).Code)
	}
	if codes[0] != http.StatusNoContent || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("login limit: %v", codes)
	}
	r := httptest.NewRequest(http.MethodPost, APIPrefix+"/auth/refresh", nil)
	r.RemoteAddr = "192.0.2.1:5000"
	if serve(h, r).Code != http.StatusNoContent {
		t.Fatal("other routes are not limited by this limiter")
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	api := serve(h, httptest.NewRequest(http.MethodGet, APIPrefix+"/refdata/skills", nil)).Header()
	if api.Get("Strict-Transport-Security") == "" || !strings.Contains(api.Get("Content-Security-Policy"), "default-src 'none'") ||
		api.Get("X-Frame-Options") != "DENY" || api.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("api headers: %v", api)
	}
	if docs := serve(h, httptest.NewRequest(http.MethodGet, APIPrefix+"/docs", nil)).Header(); docs.Get("Content-Security-Policy") != "" {
		t.Fatal("docs page loads its UI from a CDN and must not get the strict API policy")
	}
	plain := securityHeaders(false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if serve(plain, httptest.NewRequest(http.MethodGet, "/", nil)).Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS only behind HTTPS")
	}
}

func TestMetricsGuard(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("fsp_users 1")) })
	if serve(metricsGuard("", inner), httptest.NewRequest(http.MethodGet, "/metrics", nil)).Code != http.StatusOK {
		t.Fatal("dev without token is open")
	}
	g := metricsGuard("s3cret-token", inner)
	for _, auth := range []string{"", "Bearer wrong", "Basic s3cret-token", "Bearer s3cret-tokenX"} {
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		if serve(g, r).Code != http.StatusUnauthorized {
			t.Fatalf("%q must be rejected", auth)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.Header.Set("Authorization", "bearer s3cret-token")
	if w := serve(g, r); w.Code != http.StatusOK || w.Body.String() != "fsp_users 1" {
		t.Fatalf("valid token: %d", w.Code)
	}
}

func TestAuthenticateRespectsUserStatus(t *testing.T) {
	tokens, err := authtoken.New("test-secret-test-secret-test-secret-1234", time.Minute, "fsp-test")
	if err != nil {
		t.Fatal(err)
	}
	blocked, active := uuid.New(), uuid.New()
	calls := 0
	cache := newStatusCache(func(_ context.Context, id uuid.UUID) (bool, error) {
		calls++
		if id == uuid.Nil {
			return false, errors.New("db down")
		}
		return id == active, nil
	})
	var got *domain.Principal
	h := authenticate(tokens, cache)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = nil
		if p, err := domain.RequirePrincipal(r.Context()); err == nil {
			got = &p
		}
	}))
	for _, c := range []struct {
		id   uuid.UUID
		want bool
	}{{active, true}, {blocked, false}, {uuid.Nil, false}, {active, true}} {
		tok, _, err := tokens.Issue(domain.Principal{UserID: c.id, Role: domain.RoleCandidate})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		serve(h, r)
		if (got != nil) != c.want {
			t.Fatalf("user %s: principal set = %v, want %v", c.id, got != nil, c.want)
		}
	}
	if calls != 3 {
		t.Fatalf("status must be cached per user, checks: %d", calls)
	}
	if !(*statusCache)(nil).active(context.Background(), active) {
		t.Fatal("without a checker every valid token is accepted")
	}
}

func TestUploadGuard(t *testing.T) {
	var body []byte
	h := uploadGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	req := func(role domain.Role, size int64) *http.Request {
		r := httptest.NewRequest(http.MethodPost, resumeUploadPath, io.LimitReader(zeroReader{}, size))
		r.ContentLength = -1
		if role != "" {
			r = r.WithContext(domain.WithPrincipal(r.Context(), domain.Principal{UserID: uuid.New(), Role: role}))
		}
		return r
	}
	if code := serve(h, req("", 10)).Code; code != http.StatusUnauthorized {
		t.Fatalf("anonymous upload must be rejected before reading the body: %d", code)
	}
	if code := serve(h, req(domain.RoleEmployer, 10)).Code; code != http.StatusForbidden {
		t.Fatalf("employer upload: %d", code)
	}
	if code := serve(h, req(domain.RoleCandidate, maxUploadBody+10)).Code; code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body must be cut: %d", code)
	}
	declared := req(domain.RoleCandidate, 10)
	declared.ContentLength = maxUploadBody + 1
	if code := serve(h, declared).Code; code != http.StatusRequestEntityTooLarge {
		t.Fatalf("declared oversized body: %d", code)
	}
	if code := serve(h, req(domain.RoleCandidate, 1000)).Code; code != http.StatusOK || len(body) != 1000 {
		t.Fatalf("normal upload: %d, %d bytes", code, len(body))
	}
	// Остальные маршруты не затрагиваются.
	other := httptest.NewRequest(http.MethodPost, APIPrefix+"/auth/login", strings.NewReader("{}"))
	if serve(h, other).Code != http.StatusOK {
		t.Fatal("guard must only apply to the resume upload")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = '0'
	}
	return len(p), nil
}

func TestSPAHandler(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "dist")
	for name, content := range map[string]string{
		"dist/index.html":       "<!doctype html><title>ФСП</title>",
		"dist/assets/app.js":    "console.log(1)",
		"dist/favicon.svg":      "<svg/>",
		"outside/secret.txt":    "TOP SECRET",
		"dist/assets/sub/x.css": "a{}",
	} {
		p := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := spaHandler(root)
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.URL.Path = path
		return serve(h, r)
	}
	if w := get("/assets/app.js"); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") || w.Body.String() != "console.log(1)" {
		t.Fatalf("asset: %d %v", w.Code, w.Header())
	}
	if w := get("/favicon.svg"); w.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("static file cache: %v", w.Header())
	}
	for _, p := range []string{"/../outside/secret.txt", "/assets/../../outside/secret.txt", "/assets/..%2f..%2foutside/secret.txt"} {
		if w := get(p); strings.Contains(w.Body.String(), "SECRET") || w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
			t.Fatalf("%s: traversal must never reach files outside the root: %d %q", p, w.Code, w.Body.String())
		}
	}
	for _, p := range []string{"/employer/needs/123", "/assets", "/candidate/test?x=1"} {
		w := get(p)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "SECRET") || !strings.Contains(w.Body.String(), "<title>ФСП</title>") {
			t.Fatalf("%s must fall back to index.html: %d %q", p, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-cache" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatalf("%s: index must not be cached and must carry the CSP: %v", p, w.Header())
		}
	}
	if w := serve(h, httptest.NewRequest(http.MethodPost, "/", nil)); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", w.Code)
	}
}

func TestMapError(t *testing.T) {
	cases := map[*domain.Error]int{
		domain.Validation("f", "c", "m"):            http.StatusUnprocessableEntity,
		domain.NotFound("x"):                        http.StatusNotFound,
		domain.Conflict("c", "m"):                   http.StatusConflict,
		domain.InvalidTransition("need", "a", "b"):  http.StatusConflict,
		domain.Forbidden("c", "m"):                  http.StatusForbidden,
		domain.E(domain.KindUnauthorized, "c", "m"): http.StatusUnauthorized,
		domain.E(domain.KindRateLimited, "c", "m"):  http.StatusTooManyRequests,
		domain.E(domain.KindCooldown, "c", "m"):     http.StatusLocked,
	}
	for de, want := range cases {
		var se huma.StatusError
		if !errors.As(MapError(de), &se) || se.GetStatus() != want {
			t.Errorf("%v → %v, want %d", de.Kind, se, want)
		}
	}
	var se huma.StatusError
	err := MapError(errors.New("pq: password authentication failed for user secret"))
	if !errors.As(err, &se) || se.GetStatus() != http.StatusInternalServerError || strings.Contains(err.Error(), "secret") {
		t.Fatalf("foreign errors must not leak: %v", err)
	}
	if MapError(nil) != nil {
		t.Fatal("nil")
	}
}
