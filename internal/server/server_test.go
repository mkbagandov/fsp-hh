package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"fsp/internal/app/skills"
	apptesting "fsp/internal/app/testing"
	"fsp/internal/config"
	"fsp/internal/infrastructure/pdftext"
	"fsp/internal/server"
	"fsp/internal/storage/postgres/pgtest"
	_ "fsp/pkg/itembank/families/all"
)

const password = "Srv-Passw0rd!"

type client struct {
	t     *testing.T
	base  string
	http  *http.Client
	token string
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

// raw выполняет запрос и возвращает ответ с прочитанным телом.
func (c *client) raw(method, path string, body io.Reader, contentType string) (*http.Response, []byte) {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+"/api/v1"+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

// call отправляет JSON и проверяет код ответа; тело разбирается в map.
func (c *client) call(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	res, b := c.raw(method, path, r, "application/json")
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, b)
	}
	out := map[string]any{}
	if len(b) > 0 && b[0] == '{' {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

var codeRe = regexp.MustCompile(`\b(\d{6})\b`)

func (c *client) outboxCode(email string) string {
	c.t.Helper()
	res := c.call(http.MethodGet, "/dev/outbox", nil, http.StatusOK)
	msgs, _ := res["messages"].([]any)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i].(map[string]any)
		if m["to"] == email {
			if code := codeRe.FindString(m["text"].(string)); code != "" {
				return code
			}
		}
	}
	c.t.Fatalf("no code for %s", email)
	return ""
}

func (c *client) signup(role, email string) string {
	c.t.Helper()
	c.call(http.MethodPost, "/auth/register", map[string]any{"email": email, "password": password, "role": role, "consents": map[string]bool{"processing": true}}, http.StatusCreated)
	sess := c.call(http.MethodPost, "/auth/verify", map[string]any{"email": email, "code": c.outboxCode(email)}, http.StatusOK)
	c.token = sess["access_token"].(string)
	return sess["user"].(map[string]any)["id"].(string)
}

func items(m map[string]any, key string) []map[string]any {
	raw, _ := m[key].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		out = append(out, it.(map[string]any))
	}
	return out
}

// Сквозной сценарий через настоящий HTTP-обработчик (та же сборка, что в cmd/api): маршрутизация,
// middleware, валидация huma, маппинг ошибок, cookie, приватность контактов и удаление аккаунта.
func TestAPIEndToEnd(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := context.Background()
	clean := func() {
		for _, q := range []string{
			`DELETE FROM companies WHERE id IN (SELECT eu.company_id FROM employer_users eu JOIN users u ON u.id = eu.user_id WHERE u.email LIKE 'srv-%@test.local')`,
			`DELETE FROM users WHERE email LIKE 'srv-%@test.local' OR id IN (SELECT user_id FROM candidate_profiles WHERE city LIKE 'Сервер-%')`,
		} {
			if _, err := pool.Exec(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
	if _, err := skills.Seed(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := apptesting.SyncFamilies(ctx, pool); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		AppEnv: "dev", JWTSecret: strings.Repeat("k", 40), AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour,
		BaseURL: "http://fsp.test", FrontendURL: "http://fsp.test", UploadDir: t.TempDir(), LLMProvider: "mock",
		MailProvider: "console", ConsentVersion: "2026-10-01", GradeCooldownDays: 90, WebDir: "-",
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	app, err := server.Build(ctx, cfg, pool, log, server.Options{Version: "test",
		Extract: func(_ context.Context, b []byte) (string, error) { return pdftext.Extract(b) }})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler)
	defer ts.Close()
	suffix := uuid.NewString()[:6]
	city := "Сервер-" + suffix

	// --- аутентификация ---
	anon := newClient(t, ts.URL)
	res, body := anon.raw(http.MethodGet, "/candidate/profile", nil, "")
	if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/problem+json") ||
		!strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("anonymous profile: %d %v %s", res.StatusCode, res.Header, body)
	}
	cand := newClient(t, ts.URL)
	candEmail := "srv-cand-" + suffix + "@test.local"
	candID := cand.signup("candidate", candEmail)
	// Регрессия критической уязвимости: повторное подтверждение не открывает сессию даже с верным кодом.
	for _, code := range []string{"000000", cand.outboxCode(candEmail)} {
		anon.call(http.MethodPost, "/auth/verify", map[string]any{"email": candEmail, "code": code}, http.StatusUnprocessableEntity)
	}
	refreshed := cand.call(http.MethodPost, "/auth/refresh", nil, http.StatusOK) // refresh-токен — из HttpOnly-cookie
	if cand.token = refreshed["access_token"].(string); cand.token == "" {
		t.Fatal("refresh must issue a new access token")
	}

	// --- кандидат: анкета, согласие, контакты, опрос, адаптивный тест ---
	cand.call(http.MethodPut, "/candidate/profile", map[string]any{
		"first_name": "Сервер", "last_name": "Тестовый", "city": city, "work_formats": []string{"remote"},
		"salary_expected": 240000, "visibility": "open", "primary_specialization": "backend", "headline": "Backend на Go",
	}, http.StatusOK)
	cand.call(http.MethodPost, "/auth/consents", map[string]any{"kind": "publication"}, http.StatusOK)
	cand.call(http.MethodPut, "/candidate/contacts", map[string]any{"telegram": "@srv_" + suffix, "other": map[string]string{}}, http.StatusOK)
	cand.call(http.MethodPost, "/candidate/survey", map[string]any{"specialization": "backend", "claimed_grade": "middle"}, http.StatusCreated)
	start := cand.call(http.MethodPost, "/candidate/tests", map[string]any{"specialization": "backend", "target_grade": "middle"}, http.StatusCreated)
	session := start["session"].(map[string]any)["id"].(string)
	for i := 0; i < 40; i++ {
		item := cand.call(http.MethodGet, "/candidate/tests/"+session+"/item", nil, http.StatusOK)
		if item["finished"] == true || item["item"] == nil {
			break
		}
		it := item["item"].(map[string]any)
		var keys []string
		for _, o := range items(it, "options") {
			keys = append(keys, o["key"].(string))
		}
		submission := map[string]any{"text": "1"}
		switch it["format"] {
		case "single_choice", "multi_choice":
			submission = map[string]any{"keys": keys[:1]}
		case "ordering":
			submission = map[string]any{"order": keys}
		case "numeric":
			submission = map[string]any{"number": 1}
		case "sql_result":
			submission = map[string]any{"rows": [][]string{{"1"}}}
		}
		if ans := cand.call(http.MethodPost, "/candidate/tests/"+session+"/answer", map[string]any{"item_id": it["id"], "submission": submission}, http.StatusOK); ans["finished"] == true {
			break
		}
	}
	cats := cand.call(http.MethodGet, "/candidate/categories", nil, http.StatusOK)
	if len(items(cats, "categories")) == 0 {
		t.Fatalf("finished test must place the candidate into a category: %v", cats)
	}

	// --- работодатель: компания, потребность, подборка, приглашение ---
	emp := newClient(t, ts.URL)
	emp.signup("employer", "srv-emp-"+suffix+"@test.local")
	emp.call(http.MethodGet, "/candidate/profile", nil, http.StatusForbidden)
	emp.call(http.MethodPut, "/employer/company", map[string]any{"name": "Серверная " + suffix}, http.StatusOK)
	need := emp.call(http.MethodPost, "/employer/needs", map[string]any{
		"title": "Go-разработчик", "specialization": "backend", "grade_min": "junior", "grade_max": "senior",
		"must_skills": []string{}, "salary_from": 200000, "salary_to": 300000, "work_formats": []string{"remote"},
	}, http.StatusCreated)
	needID := need["id"].(string)
	sel := emp.call(http.MethodPost, "/employer/needs/"+needID+"/selection", map[string]any{"city": city}, http.StatusCreated)
	found := items(sel, "items")
	if len(found) != 1 || found[0]["candidate"].(map[string]any)["id"] != candID || len(items(found[0], "facts")) == 0 {
		t.Fatalf("selection by unique city must contain exactly our candidate with explanations: %v", sel)
	}
	denied := emp.call(http.MethodGet, "/employer/candidates/"+candID+"/contacts", nil, http.StatusForbidden)
	if errs := items(denied, "errors"); len(errs) == 0 || errs[0]["value"] != "contacts_not_granted" {
		t.Fatalf("contacts before acceptance: %v", denied)
	}
	emp.call(http.MethodPost, "/employer/invitations", map[string]any{"candidate_id": candID, "need_id": needID, "message": "Ждём",
		"salary_from": 300000, "salary_to": 200000, "contact_method": map[string]string{"kind": "telegram", "value": "@hr"}}, http.StatusUnprocessableEntity)
	inv := emp.call(http.MethodPost, "/employer/invitations", map[string]any{"candidate_id": candID, "need_id": needID, "message": "Ждём на собеседование",
		"salary_from": 200000, "salary_to": 300000, "contact_method": map[string]string{"kind": "telegram", "value": "@hr"}}, http.StatusCreated)
	mine := items(cand.call(http.MethodGet, "/candidate/invitations", nil, http.StatusOK), "items")
	if len(mine) != 1 || mine[0]["contact_method"] != nil {
		t.Fatalf("candidate sees the invitation but not the contact method before accepting: %v", mine)
	}
	cand.call(http.MethodPost, "/candidate/invitations/"+inv["id"].(string)+"/accept", nil, http.StatusOK)
	granted := emp.call(http.MethodGet, "/employer/candidates/"+candID+"/contacts", nil, http.StatusOK)
	if granted["telegram"] != "srv_"+suffix {
		t.Fatalf("contacts after acceptance: %v", granted)
	}

	// --- резюме: аноним отсекается до разбора тела, кандидат получает черновик ---
	res, pdf := cand.raw(http.MethodGet, "/candidate/profile.pdf", nil, "")
	if res.StatusCode != http.StatusOK || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("profile pdf: %d", res.StatusCode)
	}
	upload := func(c *client) (*http.Response, []byte) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, _ := mw.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="file"; filename="cv.pdf"`}, "Content-Type": {"application/pdf"},
		})
		_, _ = part.Write(pdf)
		_ = mw.Close()
		return c.raw(http.MethodPost, "/candidate/resume", &buf, mw.FormDataContentType())
	}
	if res, _ := upload(anon); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous upload: %d", res.StatusCode)
	}
	res, body = upload(cand)
	if res.StatusCode != http.StatusCreated || !strings.Contains(string(body), `"parser":"mock"`) || !strings.Contains(string(body), "Тестовый") {
		t.Fatalf("upload: %d %s", res.StatusCode, body)
	}

	// --- метрики в dev открыты ---
	mres, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	mb, _ := io.ReadAll(mres.Body)
	_ = mres.Body.Close()
	if mres.StatusCode != http.StatusOK || !strings.Contains(string(mb), "fsp_users") {
		t.Fatalf("metrics: %d", mres.StatusCode)
	}

	// --- удаление аккаунта закрывает контакты и уже выданный access-токен ---
	cand.call(http.MethodDelete, "/auth/account", nil, http.StatusOK)
	emp.call(http.MethodGet, "/employer/candidates/"+candID+"/contacts", nil, http.StatusForbidden)
	cand.call(http.MethodGet, "/candidate/profile", nil, http.StatusUnauthorized)
}
