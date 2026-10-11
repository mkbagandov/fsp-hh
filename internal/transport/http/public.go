package httptransport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"fsp/internal/app/candidates"
)

// registerPublicVerify — страница подтверждения категории: сюда ведёт QR-код из PDF-профиля кандидата.
func registerPublicVerify(api huma.API, d CandidateDeps) {
	type verifyOutput struct {
		Body candidates.Verification
	}
	huma.Register(api, huma.Operation{
		OperationID: "public-verify", Method: http.MethodGet, Path: "/public/verify/{id}",
		Summary: "Подтверждение категории по QR-коду", Tags: []string{"candidate"},
		Description: "Без аутентификации и без персональных данных: категории кандидата, их статусы и связь с ФСП ID. " +
			"404 — кандидат скрыл профиль, отозвал согласие на публикацию или ещё не проходил тест.",
		Errors: []int{http.StatusNotFound},
	}, func(ctx context.Context, in *idPath) (*verifyOutput, error) {
		v, err := d.Service.Verify(ctx, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &verifyOutput{Body: v}, nil
	})
}
