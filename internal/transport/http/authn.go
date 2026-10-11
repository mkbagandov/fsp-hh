package httptransport

import (
	"net/http"
	"strings"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/authtoken"
)

// authenticate извлекает bearer-токен и кладёт субъекта в контекст.
// Отсутствие или невалидность токена — не ошибка на этом уровне: операции,
// требующие входа, сами вызывают domain.RequirePrincipal и получают 401.
func authenticate(tokens *authtoken.Issuer, status *statusCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tokens != nil {
				h := r.Header.Get("Authorization")
				if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
					// Токен заблокированного или удалённого пользователя не действует (кэш статуса — секунды).
					if p, err := tokens.Parse(strings.TrimSpace(h[7:])); err == nil && status.active(r.Context(), p.UserID) {
						r = r.WithContext(domain.WithPrincipal(r.Context(), p))
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
