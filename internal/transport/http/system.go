package httptransport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// HealthOutput — ответ операции get-health.
type HealthOutput struct {
	Body struct {
		Status  string    `json:"status" enum:"ok,degraded" doc:"Общее состояние сервиса"`
		Version string    `json:"version" doc:"Версия сборки"`
		DB      string    `json:"db" enum:"ok,down" doc:"Доступность PostgreSQL"`
		Time    time.Time `json:"time" doc:"Серверное время (UTC)"`
	}
}

func registerSystem(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Состояние сервиса",
		Description: "Проверка доступности API и базы данных. Используется стендом и E2E-тестами.",
		Tags:        []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		out.Body.Version = d.Version
		out.Body.Time = time.Now().UTC()
		out.Body.DB = "ok"

		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if d.Pool == nil || d.Pool.Ping(pingCtx) != nil {
			out.Body.DB = "down"
			out.Body.Status = "degraded"
		}
		return out, nil
	})
}
