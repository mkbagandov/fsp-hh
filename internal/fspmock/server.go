// Package fspmock — эмулятор ФСП ID (OpenID Connect провайдер с путями Keycloak)
// и реестра достижений ФСП. Используется для разработки, демонстрации и тестов.
//
// Пути повторяют Keycloak (realm "fsp"): discovery, auth, token, userinfo, certs, logout.
// Поддерживается Authorization Code + PKCE (S256), client_secret_post и client_secret_basic.
// Токены — RS256, ключ генерируется при старте (JWKS на /certs).
//
// Реестр: GET /registry/v1/participants/{fsp_id} и .../achievements, требует access-токен
// со scope fsp.achievements. Это наше предположение о будущем API ФСП (docs/05-fsp-integration.md).
package fspmock

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// User — тестовый участник ФСП.
type User struct {
	Username     string
	Password     string
	Subject      string // OIDC sub
	FSPID        string
	GivenName    string
	FamilyName   string
	Email        string
	Achievements []Achievement
}

// Achievement — запись реестра достижений.
type Achievement struct {
	ExternalID string `json:"external_id"`
	Discipline string `json:"discipline"`
	Level      string `json:"level"`
	EventName  string `json:"event_name"`
	EventDate  string `json:"event_date"`
	ResultKind string `json:"result_kind"`
	Place      *int   `json:"place,omitempty"`
	RankTitle  string `json:"rank_title,omitempty"`
	Rating     *int   `json:"rating,omitempty"`
	Role       string `json:"role,omitempty"`
	TeamName   string `json:"team_name,omitempty"`
}

// Config — параметры эмулятора.
type Config struct {
	Issuer       string   // полный issuer, например http://127.0.0.1:8090/realms/fsp
	ClientID     string   // клиент платформы
	ClientSecret string   // секрет клиента
	RedirectURIs []string // допустимые redirect_uri (пусто — любые; только для dev)
	Users        []User   // тестовые пользователи (пусто — DefaultUsers)
	TokenTTL     time.Duration
	Now          func() time.Time
}

// Server — HTTP-обработчик эмулятора.
type Server struct {
	cfg    Config
	prefix string // путь issuer, например /realms/fsp
	key    *rsa.PrivateKey
	kid    string
	mux    *http.ServeMux
	mu     sync.Mutex
	codes  map[string]authCode
	users  map[string]*User
	bySub  map[string]*User
	byFSP  map[string]*User
}

type authCode struct {
	user        *User
	redirectURI string
	scope       string
	nonce       string
	challenge   string
	expires     time.Time
}

// New создаёт эмулятор и генерирует ключ подписи.
func New(cfg Config) (*Server, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("fspmock: issuer required")
	}
	u, err := url.Parse(cfg.Issuer)
	if err != nil || u.Path == "" {
		return nil, fmt.Errorf("fspmock: issuer must be a URL with a path, got %q", cfg.Issuer)
	}
	if cfg.ClientID == "" {
		cfg.ClientID = "fsp-platform"
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = "fsp-platform-secret"
	}
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = time.Hour
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if len(cfg.Users) == 0 {
		cfg.Users = DefaultUsers()
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("fspmock: rsa key: %w", err)
	}
	s := &Server{
		cfg:    cfg,
		prefix: strings.TrimRight(u.Path, "/"),
		key:    key,
		kid:    randomID(8),
		codes:  map[string]authCode{},
		users:  map[string]*User{},
		bySub:  map[string]*User{},
		byFSP:  map[string]*User{},
	}
	for i := range cfg.Users {
		usr := &cfg.Users[i]
		s.users[usr.Username] = usr
		s.bySub[usr.Subject] = usr
		s.byFSP[usr.FSPID] = usr
	}
	s.routes()
	return s, nil
}

// Handler возвращает HTTP-обработчик.
func (s *Server) Handler() http.Handler { return s.mux }

// Users возвращает тестовых пользователей (для документации и тестов).
func (s *Server) Users() []User { return s.cfg.Users }

func (s *Server) routes() {
	m := http.NewServeMux()
	p := s.prefix
	m.HandleFunc("GET /", s.index)
	m.HandleFunc("GET "+p+"/.well-known/openid-configuration", s.discovery)
	m.HandleFunc("GET "+p+"/protocol/openid-connect/auth", s.authGet)
	m.HandleFunc("POST "+p+"/protocol/openid-connect/auth", s.authPost)
	m.HandleFunc("POST "+p+"/protocol/openid-connect/token", s.token)
	m.HandleFunc("GET "+p+"/protocol/openid-connect/userinfo", s.userinfo)
	m.HandleFunc("GET "+p+"/protocol/openid-connect/certs", s.certs)
	m.HandleFunc("GET "+p+"/protocol/openid-connect/logout", s.logout)
	m.HandleFunc("GET /registry/v1/participants/{fspID}", s.participant)
	m.HandleFunc("GET /registry/v1/participants/{fspID}/achievements", s.achievements)
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	s.mux = m
}

func (s *Server) discovery(w http.ResponseWriter, _ *http.Request) {
	iss := s.cfg.Issuer
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                iss,
		"authorization_endpoint":                iss + "/protocol/openid-connect/auth",
		"token_endpoint":                        iss + "/protocol/openid-connect/token",
		"userinfo_endpoint":                     iss + "/protocol/openid-connect/userinfo",
		"jwks_uri":                              iss + "/protocol/openid-connect/certs",
		"end_session_endpoint":                  iss + "/protocol/openid-connect/logout",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "fsp.achievements"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"code_challenge_methods_supported":      []string{"S256"},
		"grant_types_supported":                 []string{"authorization_code"},
		"claims_supported":                      []string{"sub", "fsp_id", "name", "given_name", "family_name", "email", "email_verified", "preferred_username"},
	})
}

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><title>ФСП ID — вход (эмулятор)</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
body{margin:0;font-family:"JetBrains Mono",ui-monospace,Consolas,monospace;background:#EDEDED;color:#1B1C21}
.card{max-width:420px;margin:48px auto;background:#fff;border:1px solid #C8C9CA;border-radius:10px;padding:28px}
h1{font-size:20px;margin:0 0 4px}.sub{font-size:12px;color:#6b6e78;margin-bottom:20px;letter-spacing:.08em;text-transform:uppercase}
label{display:block;font-size:12px;margin:12px 0 4px}input{width:100%;box-sizing:border-box;padding:10px;border:1px solid #C8C9CA;border-radius:6px;font:inherit}
button{margin-top:18px;width:100%;padding:12px;border:0;border-radius:6px;background:#402FFF;color:#fff;font:inherit;font-weight:600;cursor:pointer}
.err{background:#fde4e7;color:#c8102e;padding:10px;border-radius:6px;font-size:13px;margin-top:12px}
.users{margin-top:20px;font-size:12px;color:#6b6e78}.users code{background:#EDEDED;padding:2px 4px;border-radius:4px}
.mark{color:#EC1D35}
</style></head><body><div class="card">
<h1><span class="mark">★</span> ФСП ID</h1><div class="sub">эмулятор Keycloak · realm fsp</div>
<form method="post">
{{range $k,$v := .Hidden}}<input type="hidden" name="{{$k}}" value="{{$v}}">{{end}}
<label>Логин</label><input name="username" autofocus value="{{.Username}}">
<label>Пароль</label><input name="password" type="password">
<button type="submit">Войти и передать данные в «{{.Client}}»</button>
{{if .Error}}<div class="err">{{.Error}}</div>{{end}}
</form>
<div class="users">Тестовые учётные записи (пароль <code>fsp</code>):{{range .Users}}<br><code>{{.Username}}</code> — {{.GivenName}} {{.FamilyName}}, {{.FSPID}}, достижений: {{len .Achievements}}{{end}}</div>
</div></body></html>`))

type loginView struct {
	Hidden   map[string]string
	Username string
	Client   string
	Error    string
	Users    []User
}

func (s *Server) authGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if msg := s.validateAuthRequest(q); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	s.renderLogin(w, q, "", "")
}

func (s *Server) renderLogin(w http.ResponseWriter, q url.Values, username, errMsg string) {
	hidden := map[string]string{}
	for _, k := range []string{"client_id", "redirect_uri", "response_type", "scope", "state", "nonce", "code_challenge", "code_challenge_method"} {
		if v := q.Get(k); v != "" {
			hidden[k] = v
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginPage.Execute(w, loginView{Hidden: hidden, Username: username, Client: q.Get("client_id"), Error: errMsg, Users: s.cfg.Users})
}

func (s *Server) validateAuthRequest(q url.Values) string {
	if q.Get("client_id") != s.cfg.ClientID {
		return "unknown client_id"
	}
	if q.Get("response_type") != "code" {
		return "response_type must be code"
	}
	ru := q.Get("redirect_uri")
	if ru == "" {
		return "redirect_uri required"
	}
	if len(s.cfg.RedirectURIs) > 0 {
		ok := false
		for _, allowed := range s.cfg.RedirectURIs {
			if allowed == ru {
				ok = true
				break
			}
		}
		if !ok {
			return "redirect_uri not allowed"
		}
	}
	if !strings.Contains(" "+q.Get("scope")+" ", " openid ") {
		return "scope must include openid"
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		return "PKCE (S256) is required"
	}
	return ""
}

func (s *Server) authPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	q := r.PostForm
	if msg := s.validateAuthRequest(q); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	user, ok := s.users[strings.TrimSpace(q.Get("username"))]
	if !ok || subtle.ConstantTimeCompare([]byte(user.Password), []byte(q.Get("password"))) != 1 {
		s.renderLogin(w, q, q.Get("username"), "Неверный логин или пароль")
		return
	}
	code := randomID(24)
	s.mu.Lock()
	s.codes[code] = authCode{
		user: user, redirectURI: q.Get("redirect_uri"), scope: q.Get("scope"),
		nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), expires: s.cfg.Now().Add(5 * time.Minute),
	}
	s.mu.Unlock()
	target, _ := url.Parse(q.Get("redirect_uri"))
	tq := target.Query()
	tq.Set("code", code)
	if st := q.Get("state"); st != "" {
		tq.Set("state", st)
	}
	target.RawQuery = tq.Encode()
	// redirect_uri проверен validateAuthRequest по allowlist (в dev — любой): это штатное поведение OIDC-провайдера.
	http.Redirect(w, r, target.String(), http.StatusFound) //nolint:gosec // см. комментарий выше
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, "invalid_request", "bad form")
		return
	}
	clientID, secret := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	if id, pw, ok := r.BasicAuth(); ok {
		clientID, secret = id, pw
	}
	if clientID != s.cfg.ClientID || subtle.ConstantTimeCompare([]byte(secret), []byte(s.cfg.ClientSecret)) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="fsp"`)
		oauthErrorStatus(w, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	s.mu.Lock()
	ac, ok := s.codes[r.PostForm.Get("code")]
	delete(s.codes, r.PostForm.Get("code"))
	s.mu.Unlock()
	if !ok || s.cfg.Now().After(ac.expires) {
		oauthError(w, "invalid_grant", "code is invalid or expired")
		return
	}
	if ac.redirectURI != r.PostForm.Get("redirect_uri") {
		oauthError(w, "invalid_grant", "redirect_uri mismatch")
		return
	}
	verifier := r.PostForm.Get("code_verifier")
	sum := sha256.Sum256([]byte(verifier))
	if verifier == "" || base64.RawURLEncoding.EncodeToString(sum[:]) != ac.challenge {
		oauthError(w, "invalid_grant", "PKCE verification failed")
		return
	}
	now := s.cfg.Now()
	access, err := s.sign(jwt.MapClaims{
		"iss": s.cfg.Issuer, "sub": ac.user.Subject, "aud": []string{s.cfg.ClientID, "account"},
		"exp": now.Add(s.cfg.TokenTTL).Unix(), "iat": now.Unix(), "typ": "Bearer", "azp": s.cfg.ClientID,
		"scope": ac.scope, "fsp_id": ac.user.FSPID, "preferred_username": ac.user.Username,
	})
	if err != nil {
		oauthErrorStatus(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	idClaims := jwt.MapClaims{
		"iss": s.cfg.Issuer, "sub": ac.user.Subject, "aud": s.cfg.ClientID,
		"exp": now.Add(s.cfg.TokenTTL).Unix(), "iat": now.Unix(), "auth_time": now.Unix(),
		"fsp_id": ac.user.FSPID, "name": ac.user.GivenName + " " + ac.user.FamilyName,
		"given_name": ac.user.GivenName, "family_name": ac.user.FamilyName,
		"email": ac.user.Email, "email_verified": true, "preferred_username": ac.user.Username,
	}
	if ac.nonce != "" {
		idClaims["nonce"] = ac.nonce
	}
	idToken, err := s.sign(idClaims)
	if err != nil {
		oauthErrorStatus(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": int(s.cfg.TokenTTL.Seconds()),
		"id_token": idToken, "scope": ac.scope,
	})
}

func (s *Server) userinfo(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.bearer(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="fsp"`)
		oauthErrorStatus(w, http.StatusUnauthorized, "invalid_token", "missing or invalid access token")
		return
	}
	u := s.bySub[claims["sub"].(string)]
	writeJSON(w, http.StatusOK, map[string]any{
		"sub": u.Subject, "fsp_id": u.FSPID, "name": u.GivenName + " " + u.FamilyName,
		"given_name": u.GivenName, "family_name": u.FamilyName, "email": u.Email, "email_verified": true,
		"preferred_username": u.Username,
	})
}

func (s *Server) certs(w http.ResponseWriter, _ *http.Request) {
	pub := s.key.PublicKey
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]any{{
		"kty": "RSA", "kid": s.kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if to := r.URL.Query().Get("post_logout_redirect_uri"); to != "" {
		http.Redirect(w, r, to, http.StatusFound) //nolint:gosec // эмулятор для разработки: post_logout_redirect_uri не ограничивается
		return
	}
	_, _ = w.Write([]byte("logged out"))
}

func (s *Server) participant(w http.ResponseWriter, r *http.Request) {
	u, ok := s.registryAuth(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fsp_id": u.FSPID, "given_name": u.GivenName, "family_name": u.FamilyName,
		"achievements_count": len(u.Achievements),
	})
}

func (s *Server) achievements(w http.ResponseWriter, r *http.Request) {
	u, ok := s.registryAuth(w, r)
	if !ok {
		return
	}
	items := u.Achievements
	if items == nil {
		items = []Achievement{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"fsp_id": u.FSPID, "items": items, "total": len(items)})
}

// registryAuth проверяет токен, scope и соответствие fsp_id в пути субъекту токена.
func (s *Server) registryAuth(w http.ResponseWriter, r *http.Request) (*User, bool) {
	claims, ok := s.bearer(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="fsp"`)
		oauthErrorStatus(w, http.StatusUnauthorized, "invalid_token", "missing or invalid access token")
		return nil, false
	}
	scope, _ := claims["scope"].(string)
	if !strings.Contains(" "+scope+" ", " fsp.achievements ") {
		oauthErrorStatus(w, http.StatusForbidden, "insufficient_scope", "scope fsp.achievements required")
		return nil, false
	}
	fspID := r.PathValue("fspID")
	u, ok := s.byFSP[fspID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "error_description": "participant not found"})
		return nil, false
	}
	if sub, _ := claims["sub"].(string); sub != u.Subject {
		oauthErrorStatus(w, http.StatusForbidden, "access_denied", "token subject does not match participant")
		return nil, false
	}
	return u, true
}

func (s *Server) bearer(r *http.Request) (jwt.MapClaims, bool) {
	h := r.Header.Get("Authorization")
	if len(h) < 8 || !strings.EqualFold(h[:7], "bearer ") {
		return nil, false
	}
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(strings.TrimSpace(h[7:]), claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected alg")
		}
		return &s.key.PublicKey, nil
	}, jwt.WithIssuer(s.cfg.Issuer), jwt.WithTimeFunc(s.cfg.Now))
	if err != nil || !tok.Valid {
		return nil, false
	}
	if _, ok := claims["sub"].(string); !ok {
		return nil, false
	}
	if _, ok := s.bySub[claims["sub"].(string)]; !ok {
		return nil, false
	}
	return claims, true
}

func (s *Server) sign(claims jwt.MapClaims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = s.kid
	return t.SignedString(s.key)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "Эмулятор ФСП ID (Keycloak-совместимый) и реестра достижений\n\nIssuer: %s\nDiscovery: %s/.well-known/openid-configuration\nКлиент: %s\n\nТестовые пользователи (пароль «fsp»):\n",
		s.cfg.Issuer, s.cfg.Issuer, s.cfg.ClientID)
	for _, u := range s.cfg.Users {
		_, _ = fmt.Fprintf(w, "  %-10s %s %s, %s, достижений: %d\n", u.Username, u.GivenName, u.FamilyName, u.FSPID, len(u.Achievements))
	}
}

// DefaultUsers — тестовые участники: с богатой историей, без истории, с устаревшей историей.
func DefaultUsers() []User {
	p := func(i int) *int { return &i }
	return []User{
		{
			Username: "ivanov", Password: "fsp", Subject: "7d3f3f64-0b8c-4e5a-9a7b-1a2b3c4d5e01", FSPID: "FSP-000123",
			GivenName: "Иван", FamilyName: "Иванов", Email: "ivanov@fsp-id.example",
			Achievements: []Achievement{
				{ExternalID: "ev-2025-hack-07", Discipline: "product", Level: "national", EventName: "Всероссийский хакатон «Цифровой прорыв»", EventDate: "2025-11-16", ResultKind: "place", Place: p(2), Role: "backend", TeamName: "Байтоеды"},
				{ExternalID: "ev-2026-icpc-q", Discipline: "algorithmic", Level: "regional", EventName: "Отборочный этап ICPC, Москва", EventDate: "2026-04-12", ResultKind: "place", Place: p(7), Role: "участник"},
				{ExternalID: "ev-2024-ctf-03", Discipline: "security", Level: "national", EventName: "Кубок России по спортивному программированию, CTF", EventDate: "2024-09-21", ResultKind: "rank", RankTitle: "КМС"},
			},
		},
		{
			Username: "petrova", Password: "fsp", Subject: "9a1e2d3c-5b6a-4f7e-8d9c-0b1a2c3d4e02", FSPID: "FSP-000456",
			GivenName: "Анна", FamilyName: "Петрова", Email: "petrova@fsp-id.example",
		},
		{
			Username: "sidorov", Password: "fsp", Subject: "c4d5e6f7-8a9b-4c0d-9e1f-2a3b4c5d6e03", FSPID: "FSP-000789",
			GivenName: "Сергей", FamilyName: "Сидоров", Email: "sidorov@fsp-id.example",
			Achievements: []Achievement{
				{ExternalID: "ev-2021-robo-01", Discipline: "robotics", Level: "regional", EventName: "Региональные соревнования по робототехнике", EventDate: "2021-05-30", ResultKind: "place", Place: p(1), TeamName: "РобоКот"},
				{ExternalID: "ev-2022-uav-02", Discipline: "uav", Level: "national", EventName: "Первенство России по программированию БАС", EventDate: "2022-08-14", ResultKind: "participation"},
			},
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, code, desc string) {
	oauthErrorStatus(w, http.StatusBadRequest, code, desc)
}

func oauthErrorStatus(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
