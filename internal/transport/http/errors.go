package httptransport

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"fsp/internal/domain"
)

// MapError переводит доменную ошибку в ответ problem+json (RFC 9457) с нужным кодом.
// Все обработчики возвращают ошибки через эту функцию, чтобы коды и формат были едиными
// и совпадали с описанием в OpenAPI.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	var de *domain.Error
	if !errors.As(err, &de) {
		// Чужая ошибка: 500 без деталей наружу, текст остаётся в логах.
		return huma.Error500InternalServerError("Внутренняя ошибка сервера")
	}
	detail := &huma.ErrorDetail{Message: de.Message, Location: de.Field, Value: de.Code}
	switch de.Kind {
	case domain.KindValidation:
		return huma.Error422UnprocessableEntity(de.Message, detail)
	case domain.KindNotFound:
		return huma.Error404NotFound(de.Message, detail)
	case domain.KindConflict, domain.KindInvalidTransition:
		return huma.Error409Conflict(de.Message, detail)
	case domain.KindForbidden:
		return huma.Error403Forbidden(de.Message, detail)
	case domain.KindUnauthorized:
		return huma.Error401Unauthorized(de.Message, detail)
	case domain.KindRateLimited:
		return huma.Error429TooManyRequests(de.Message, detail)
	case domain.KindCooldown:
		return huma.NewError(http.StatusLocked, de.Message, detail)
	default:
		return huma.Error500InternalServerError("Внутренняя ошибка сервера")
	}
}

// asDomainError извлекает доменную ошибку из цепочки.
func asDomainError(err error, target **domain.Error) bool { return errors.As(err, target) }

// writeProblem пишет problem+json вручную — для маршрутов вне huma (404/405 роутера).
func writeProblem(w http.ResponseWriter, status int, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"status":` + itoa(status) + `,"title":"` + title + `"}`))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}
