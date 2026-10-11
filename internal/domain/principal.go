package domain

import (
	"context"

	"github.com/google/uuid"
)

// Principal — аутентифицированный субъект запроса.
type Principal struct {
	UserID uuid.UUID
	Role   Role
	Email  string
}

type principalKey struct{}

// WithPrincipal кладёт субъект в контекст запроса.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom достаёт субъект из контекста.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// RequirePrincipal возвращает субъект или ошибку 401.
func RequirePrincipal(ctx context.Context) (Principal, error) {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return Principal{}, E(KindUnauthorized, "unauthorized", "Требуется вход в систему")
	}
	return p, nil
}

// RequireRole возвращает субъект, если его роль — одна из допустимых.
func RequireRole(ctx context.Context, roles ...Role) (Principal, error) {
	p, err := RequirePrincipal(ctx)
	if err != nil {
		return Principal{}, err
	}
	for _, r := range roles {
		if p.Role == r {
			return p, nil
		}
	}
	return Principal{}, Forbidden("role_forbidden", "Действие недоступно для вашей роли")
}
