package httptransport

import (
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
)

// corsMiddleware разрешает фронтенду с других источников (dev-сервер Vite) ходить в API.
// В prod SPA отдаётся тем же бинарником, и список источников пуст.
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"Link", "X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

// rateLimit — защита от перебора кодов и паролей и от случайных штормов:
// лимит на IP для всего API, более строгий — для auth-маршрутов (навешивается отдельно).
// Модель доверия: ключ — r.RemoteAddr. Локально и на стенде API стоит либо без прокси,
// либо за Caddy на той же машине; на этапе 17 перед этим middleware добавляется
// chi middleware.ClientIPFromXFF с перечислением доверенных прокси.
func rateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	return httprate.LimitBy(requestsPerMinute, time.Minute, clientKey)
}

// clientKey — ключ лимита: адрес TCP-соединения (не подделывается заголовками).
// IPv6 группируется по /64, чтобы один хост не получал тысячи «разных» адресов.
func clientKey(r *http.Request) (string, error) {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host, nil //nolint:nilerr // неразборчивый адрес — отдельный бакет, не ошибка запроса
	}
	if addr.Is6() && !addr.Is4In6() {
		if p, err := addr.Prefix(64); err == nil {
			return p.Masked().Addr().String(), nil
		}
	}
	return addr.Unmap().String(), nil
}
