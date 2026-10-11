package httptransport

import (
	"context"
	"net/http"
)

type reqMetaKey struct{}

type reqMeta struct {
	ip        string
	userAgent string
}

// requestMeta сохраняет адрес и User-Agent в контексте: huma-обработчики
// не видят *http.Request, а аудиту и refresh-токенам эти данные нужны.
func requestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), reqMetaKey{}, reqMeta{ip: r.RemoteAddr, userAgent: r.UserAgent()})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func clientIPFromCtx(ctx context.Context) string {
	m, _ := ctx.Value(reqMetaKey{}).(reqMeta)
	return m.ip
}

func userAgentFromCtx(ctx context.Context) string {
	m, _ := ctx.Value(reqMetaKey{}).(reqMeta)
	return m.userAgent
}
