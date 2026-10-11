package fspmock

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func startMock(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	issuer := "http://" + ts.Listener.Addr().String() + "/realms/fsp"
	s, err := New(Config{Issuer: issuer, ClientID: "fsp-platform", ClientSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = s.Handler()
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, s
}

func TestDiscoveryAndCodeFlow(t *testing.T) {
	ts, s := startMock(t)
	var disc map[string]any
	resp, err := http.Get(s.cfg.Issuer + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		t.Fatal(err)
	}
	if disc["issuer"] != s.cfg.Issuer || disc["token_endpoint"] != s.cfg.Issuer+"/protocol/openid-connect/token" {
		t.Fatalf("discovery: %v", disc)
	}

	verifier := "0123456789abcdef0123456789abcdef0123456789abcdef"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	form := url.Values{
		"client_id": {"fsp-platform"}, "redirect_uri": {"http://app.local/cb"}, "response_type": {"code"},
		"scope": {"openid profile email fsp.achievements"}, "state": {"st1"}, "nonce": {"n1"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"},
		"username": {"ivanov"}, "password": {"fsp"},
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp2, err := client.PostForm(s.cfg.Issuer+"/protocol/openid-connect/auth", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect, got %d", resp2.StatusCode)
	}
	loc, _ := url.Parse(resp2.Header.Get("Location"))
	code := loc.Query().Get("code")
	if code == "" || loc.Query().Get("state") != "st1" || !strings.HasPrefix(loc.String(), "http://app.local/cb") {
		t.Fatalf("bad redirect: %s", loc)
	}

	// Неверный verifier → invalid_grant.
	bad, _ := http.PostForm(s.cfg.Issuer+"/protocol/openid-connect/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"http://app.local/cb"},
		"client_id": {"fsp-platform"}, "client_secret": {"secret"}, "code_verifier": {"wrong"},
	})
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong verifier must fail, got %d", bad.StatusCode)
	}
	bad.Body.Close()

	// Код одноразовый: после неудачи он уже удалён — получаем новый.
	resp3, _ := client.PostForm(s.cfg.Issuer+"/protocol/openid-connect/auth", form)
	loc3, _ := url.Parse(resp3.Header.Get("Location"))
	resp3.Body.Close()
	code = loc3.Query().Get("code")

	tokResp, err := http.PostForm(s.cfg.Issuer+"/protocol/openid-connect/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"http://app.local/cb"},
		"client_id": {"fsp-platform"}, "client_secret": {"secret"}, "code_verifier": {verifier},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tokResp.Body.Close()
	var tok map[string]any
	if err := json.NewDecoder(tokResp.Body).Decode(&tok); err != nil || tokResp.StatusCode != 200 {
		t.Fatalf("token: %d %v %v", tokResp.StatusCode, tok, err)
	}
	access, _ := tok["access_token"].(string)
	if access == "" || tok["id_token"] == "" {
		t.Fatalf("tokens missing: %v", tok)
	}

	// userinfo и реестр.
	req, _ := http.NewRequest(http.MethodGet, s.cfg.Issuer+"/protocol/openid-connect/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	ui, err := http.DefaultClient.Do(req)
	if err != nil || ui.StatusCode != 200 {
		t.Fatalf("userinfo: %v %v", ui, err)
	}
	var info map[string]any
	_ = json.NewDecoder(ui.Body).Decode(&info)
	ui.Body.Close()
	if info["fsp_id"] != "FSP-000123" || info["email_verified"] != true {
		t.Fatalf("userinfo claims: %v", info)
	}

	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/registry/v1/participants/FSP-000123/achievements", nil)
	req2.Header.Set("Authorization", "Bearer "+access)
	ach, err := http.DefaultClient.Do(req2)
	if err != nil || ach.StatusCode != 200 {
		t.Fatalf("achievements: %v %v", ach, err)
	}
	var list struct {
		Items []Achievement `json:"items"`
		Total int           `json:"total"`
	}
	_ = json.NewDecoder(ach.Body).Decode(&list)
	ach.Body.Close()
	if list.Total != 3 || len(list.Items) != 3 {
		t.Fatalf("expected 3 achievements, got %+v", list)
	}

	// Чужой участник по токену Иванова — запрещено.
	req3, _ := http.NewRequest(http.MethodGet, ts.URL+"/registry/v1/participants/FSP-000456/achievements", nil)
	req3.Header.Set("Authorization", "Bearer "+access)
	other, _ := http.DefaultClient.Do(req3)
	other.Body.Close()
	if other.StatusCode != http.StatusForbidden {
		t.Fatalf("other participant must be forbidden, got %d", other.StatusCode)
	}
	// Без токена — 401.
	noauth, _ := http.Get(ts.URL + "/registry/v1/participants/FSP-000123/achievements")
	noauth.Body.Close()
	if noauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token must be 401, got %d", noauth.StatusCode)
	}
}

func TestAuthValidation(t *testing.T) {
	_, s := startMock(t)
	resp, _ := http.Get(s.cfg.Issuer + "/protocol/openid-connect/auth?client_id=other&response_type=code&redirect_uri=http://x&scope=openid")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown client must be 400, got %d", resp.StatusCode)
	}
	resp2, _ := http.Get(s.cfg.Issuer + "/protocol/openid-connect/auth?client_id=fsp-platform&response_type=code&redirect_uri=http://x&scope=openid&code_challenge=abc&code_challenge_method=S256")
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("valid auth request must render login page, got %d", resp2.StatusCode)
	}
}
