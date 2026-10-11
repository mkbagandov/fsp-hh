// Package domain — типы, статусы, инварианты и ошибки предметной области.
// Пакет не зависит от HTTP, БД и инфраструктуры.
package domain

import (
	"errors"
	"fmt"
)

// Kind — класс доменной ошибки; транспорт отображает его на HTTP-код.
type Kind int

const (
	KindUnknown           Kind = iota
	KindValidation             // 422: неверные данные
	KindNotFound               // 404
	KindConflict               // 409: нарушение уникальности, повтор операции
	KindInvalidTransition      // 409: недопустимый переход статуса
	KindForbidden              // 403: нет прав или контакты закрыты
	KindUnauthorized           // 401
	KindRateLimited            // 429: квоты и лимиты
	KindCooldown               // 423: действие заблокировано до срока
)

// Error — доменная ошибка с машиночитаемым кодом.
type Error struct {
	Kind    Kind
	Code    string // стабильный код для клиента, snake_case: "salary_range_invalid"
	Message string // текст на русском для пользователя
	Field   string // поле запроса, если ошибка относится к полю
	Err     error  // причина
}

func (e *Error) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s (%s): %s", e.Code, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// E создаёт ошибку заданного класса.
func E(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Wrap добавляет причину.
func (e *Error) Wrap(err error) *Error {
	cp := *e
	cp.Err = err
	return &cp
}

// Validation — ошибка данных по конкретному полю.
func Validation(field, code, message string) *Error {
	return &Error{Kind: KindValidation, Code: code, Message: message, Field: field}
}

// NotFound — сущность не найдена.
func NotFound(what string) *Error {
	return &Error{Kind: KindNotFound, Code: what + "_not_found", Message: "Не найдено: " + what}
}

// Forbidden — операция запрещена для текущего пользователя.
func Forbidden(code, message string) *Error {
	return &Error{Kind: KindForbidden, Code: code, Message: message}
}

// Conflict — конфликт состояния.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// InvalidTransition — недопустимый переход статуса.
func InvalidTransition(entity, from, to string) *Error {
	return &Error{
		Kind:    KindInvalidTransition,
		Code:    entity + "_invalid_transition",
		Message: fmt.Sprintf("Переход %s: %s → %s недопустим", entity, from, to),
	}
}

// KindOf возвращает класс ошибки (KindUnknown для чужих ошибок).
func KindOf(err error) Kind {
	var de *Error
	if errors.As(err, &de) {
		return de.Kind
	}
	return KindUnknown
}

// Is сообщает, относится ли ошибка к классу.
func Is(err error, kind Kind) bool { return KindOf(err) == kind }
