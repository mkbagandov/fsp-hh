package httptransport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"fsp/internal/infrastructure/mail"
)

// registerDev — диагностические операции только для профиля dev:
// /dev/outbox показывает последние письма консольного мейлера, чтобы пройти
// подтверждение e-mail без SMTP.
func registerDev(api huma.API, outbox mail.Outbox) {
	type outboxOutput struct {
		Body struct {
			Messages []mail.Message `json:"messages"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "dev-outbox", Method: http.MethodGet, Path: "/dev/outbox",
		Summary: "Последние письма (только dev)", Tags: []string{"dev"},
		Description: "Доступно только при APP_ENV=dev. Содержит коды подтверждения — никогда не включать в prod.",
	}, func(_ context.Context, _ *struct{}) (*outboxOutput, error) {
		out := &outboxOutput{}
		out.Body.Messages = outbox.Recent(50)
		if out.Body.Messages == nil {
			out.Body.Messages = []mail.Message{}
		}
		return out, nil
	})
}
