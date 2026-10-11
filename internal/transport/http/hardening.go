package httptransport

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/httprate"
	"github.com/google/uuid"
)

// --- адрес клиента за доверенным прокси ---

// realIP подменяет r.RemoteAddr адресом клиента из X-Forwarded-For, но только если соединение
// пришло от доверенного прокси (TRUSTED_PROXIES). Берётся самый правый адрес цепочки, не
// принадлежащий доверенным прокси: левую часть заголовка клиент может подделать. Без настроенных
// прокси заголовок игнорируется полностью. Лимиты, аудит и журнал используют результат.
func realIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	if len(trusted) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	isTrusted := func(a netip.Addr) bool {
		a = a.Unmap()
		for _, p := range trusted {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if peer, err := netip.ParseAddr(host); err == nil && isTrusted(peer) {
				if client, ok := clientFromXFF(r.Header.Values("X-Forwarded-For"), isTrusted); ok {
					r.RemoteAddr = net.JoinHostPort(client.String(), "0")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientFromXFF(values []string, isTrusted func(netip.Addr) bool) (netip.Addr, bool) {
	var hops []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			if s := strings.TrimSpace(p); s != "" {
				hops = append(hops, s)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(hops[i])
		if err != nil {
			return netip.Addr{}, false
		}
		if !isTrusted(a) {
			return a.Unmap(), true
		}
	}
	return netip.Addr{}, false
}

// --- лимиты на конкретные маршруты ---

// credentialPaths — маршруты, на которых перебирают пароли и коды: строгий лимит на IP.
// /auth/refresh, /auth/me и прочие идут под общим лимитом, иначе за общим NAT или прокси
// фоновые обновления сессии одних пользователей выбивали бы других.
var credentialPaths = []string{
	"/auth/register", "/auth/verify", "/auth/verify/resend", "/auth/login",
	"/auth/password/forgot", "/auth/password/reset", "/auth/password/change", "/auth/fsp/start",
}

// rateLimitPaths применяет отдельный лимит к перечисленным маршрутам API (точное совпадение пути).
func rateLimitPaths(paths []string, requestsPerMinute int) func(http.Handler) http.Handler {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[APIPrefix+p] = true
	}
	limiter := httprate.LimitBy(requestsPerMinute, time.Minute, clientKey)
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if set[r.URL.Path] {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- заголовки безопасности ---

// spaCSP — политика для страниц SPA: всё своё, без внешних источников и встроенных скриптов.
// 'unsafe-inline' только для стилей: React выставляет style-атрибуты (графики, прогресс).
const spaCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'"

// apiCSP — ответы API не должны исполняться как документ ни при каких Content-Type.
const apiCSP = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'"

// securityHeaders — базовые заголовки для всех ответов; HSTS — только за HTTPS (prod).
// Страница документации /api/v1/docs грузит Stoplight Elements с CDN, поэтому строгая CSP API
// на неё не ставится.
func securityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			if strings.HasPrefix(r.URL.Path, APIPrefix+"/") && r.URL.Path != APIPrefix+"/docs" {
				h.Set("Content-Security-Policy", apiCSP)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- /metrics ---

// metricsGuard: в dev без токена /metrics открыт; иначе нужен заголовок Authorization: Bearer <METRICS_TOKEN>.
// В prod без токена маршрут не публикуется вовсе (см. NewRouter).
func metricsGuard(token string, h http.Handler) http.Handler {
	if token == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if len(got) < 7 || !strings.EqualFold(got[:7], "bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got[7:])), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			writeProblem(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// --- статус пользователя для access-токенов ---

// UserActiveFunc сообщает, активна ли учётная запись (не заблокирована и не удалена).
type UserActiveFunc func(ctx context.Context, userID uuid.UUID) (bool, error)

// statusTTL — как быстро блокировка или удаление закрывают уже выданные access-токены.
const (
	statusTTL      = 30 * time.Second
	statusCacheMax = 50_000
)

// statusCache кэширует статус пользователя: блокировка администратором закрывает доступ не позже
// чем через statusTTL, а не по истечении access-токена. Ошибка проверки — доступ закрыт.
type statusCache struct {
	check UserActiveFunc
	mu    sync.Mutex
	items map[uuid.UUID]statusEntry
}

type statusEntry struct {
	active bool
	exp    time.Time
}

func newStatusCache(check UserActiveFunc) *statusCache {
	if check == nil {
		return nil
	}
	return &statusCache{check: check, items: map[uuid.UUID]statusEntry{}}
}

// forget сбрасывает кэш пользователя: удаление аккаунта и смена статуса администратором
// действуют на уже выданные access-токены сразу, а не через statusTTL.
func (c *statusCache) forget(id uuid.UUID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.items, id)
	c.mu.Unlock()
}

func (c *statusCache) active(ctx context.Context, id uuid.UUID) bool {
	if c == nil {
		return true
	}
	now := time.Now()
	c.mu.Lock()
	e, ok := c.items[id]
	c.mu.Unlock()
	if ok && now.Before(e.exp) {
		return e.active
	}
	active, err := c.check(ctx, id)
	if err != nil {
		return false
	}
	c.mu.Lock()
	if len(c.items) >= statusCacheMax {
		c.items = map[uuid.UUID]statusEntry{}
	}
	c.items[id] = statusEntry{active: active, exp: now.Add(statusTTL)}
	c.mu.Unlock()
	return active
}
