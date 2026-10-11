package httptransport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"fsp/internal/app/admin"
	apptesting "fsp/internal/app/testing"
	"fsp/internal/domain"
)

// AdminDeps — администрирование и наблюдаемость.
type AdminDeps struct {
	Service *admin.Service
	// Testing — пересчёт калибровки банка заданий (nil — операция отвечает 409).
	Testing *apptesting.Service
}

func registerAdmin(api huma.API, d AdminDeps, status *statusCache) {
	type adminAcceptedOut struct {
		Status int
		Body   struct {
			Message string `json:"message"`
		}
	}
	type overviewInput struct {
		Days int `query:"days" minimum:"1" maximum:"90" doc:"Глубина дневной активности, по умолчанию 14"`
	}
	type overviewOutput struct {
		Body admin.Overview
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-overview", Method: http.MethodGet, Path: "/admin/overview",
		Summary: "Сводка платформы", Tags: []string{"admin"}, Security: bearerSecurity,
		Description: "Пользователи, категории, тест-сессии, приглашения, отклики, вакансии, воронка кандидата и активность по дням.",
	}, func(ctx context.Context, in *overviewInput) (*overviewOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleAdmin); err != nil {
			return nil, MapError(err)
		}
		o, err := d.Service.Overview(ctx, in.Days)
		if err != nil {
			return nil, MapError(err)
		}
		return &overviewOutput{Body: o}, nil
	})

	type usersInput struct {
		Q      string `query:"q" maxLength:"120" doc:"Часть e-mail"`
		Role   string `query:"role" enum:"candidate,employer,admin,"`
		Status string `query:"status" enum:"active,blocked,deleted,"`
		Limit  int    `query:"limit" minimum:"1" maximum:"200"`
		Offset int    `query:"offset" minimum:"0"`
	}
	type usersOutput struct {
		Body admin.UserList
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-users", Method: http.MethodGet, Path: "/admin/users",
		Summary: "Пользователи", Tags: []string{"admin"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *usersInput) (*usersOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleAdmin); err != nil {
			return nil, MapError(err)
		}
		list, err := d.Service.Users(ctx, admin.UserFilter{Query: in.Q, Role: in.Role, Status: in.Status, Limit: in.Limit, Offset: in.Offset})
		if err != nil {
			return nil, MapError(err)
		}
		return &usersOutput{Body: list}, nil
	})

	type userStatusInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Status string `json:"status" enum:"active,blocked"`
		}
	}
	type userOutput struct {
		Body admin.User
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-user-status", Method: http.MethodPost, Path: "/admin/users/{id}/status",
		Summary: "Заблокировать или разблокировать", Tags: []string{"admin"}, Security: bearerSecurity,
		Description: "Нельзя менять себя и других администраторов. Действие пишется в аудит.",
	}, func(ctx context.Context, in *userStatusInput) (*userOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleAdmin)
		if err != nil {
			return nil, MapError(err)
		}
		u, err := d.Service.SetUserStatus(ctx, p.UserID, in.ID, in.Body.Status)
		if err != nil {
			return nil, MapError(err)
		}
		status.forget(in.ID)
		return &userOutput{Body: u}, nil
	})

	type familiesOutput struct {
		Body struct {
			Items []admin.FamilyStat `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-item-stats", Method: http.MethodGet, Path: "/admin/items",
		Summary: "Статистика банка заданий", Tags: []string{"admin"}, Security: bearerSecurity,
		Description: "Экспозиция, доля верных, среднее время, оценка дискриминативности (разница θ верно/неверно ответивших) и предупреждения о слабых семействах — первыми.",
	}, func(ctx context.Context, _ *struct{}) (*familiesOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleAdmin); err != nil {
			return nil, MapError(err)
		}
		items, err := d.Service.ItemStats(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		out := &familiesOutput{}
		out.Body.Items = items
		return out, nil
	})

	type familyEnabledInput struct {
		ID   string `path:"id" maxLength:"200"`
		Body struct {
			Enabled bool `json:"enabled,omitempty"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-item-enabled", Method: http.MethodPost, Path: "/admin/items/{id}/enabled",
		Summary: "Включить или выключить семейство", Tags: []string{"admin"}, Security: bearerSecurity, DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *familyEnabledInput) (*adminAcceptedOut, error) {
		p, err := domain.RequireRole(ctx, domain.RoleAdmin)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.SetFamilyEnabled(ctx, p.UserID, in.ID, in.Body.Enabled); err != nil {
			return nil, MapError(err)
		}
		out := &adminAcceptedOut{Status: http.StatusAccepted}
		out.Body.Message = "Сохранено"
		return out, nil
	})

	type recalibrateOutput struct {
		Body apptesting.CalibrationReport
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-items-recalibrate", Method: http.MethodPost, Path: "/admin/items/recalibrate",
		Summary: "Пересчитать сложность заданий по живым ответам", Tags: []string{"admin"}, Security: bearerSecurity,
		Description: "Байесовская калибровка сложности b по ответам завершённых сессий: κ = 20, от 30 ответов на семейство — " +
			"тот же расчёт, что проверен валидацией. Всегда от априорной сложности, поэтому повторный запуск безопасен. " +
			"Выполняется и автоматически раз в сутки.",
	}, func(ctx context.Context, _ *struct{}) (*recalibrateOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleAdmin)
		if err != nil {
			return nil, MapError(err)
		}
		if d.Testing == nil {
			return nil, MapError(domain.Conflict("calibration_unavailable", "Сервис тестирования не подключён"))
		}
		rep, err := d.Testing.Recalibrate(ctx, nil)
		if err != nil {
			return nil, MapError(err)
		}
		_ = d.Service.RecordAudit(ctx, p.UserID, "admin.items.recalibrated", "item_family", map[string]any{"families": rep.Families, "updated": rep.Updated, "answers": rep.Answers})
		return &recalibrateOutput{Body: rep}, nil
	})

	type sessionsInput struct {
		Limit int `query:"limit" minimum:"1" maximum:"500"`
	}
	type sessionsOutput struct {
		Body struct {
			Items []admin.FlaggedSession `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-flagged-sessions", Method: http.MethodGet, Path: "/admin/sessions/flagged",
		Summary: "Сессии с флагами телеметрии", Tags: []string{"admin"}, Security: bearerSecurity,
		Description: "Потери фокуса, скрытие вкладки, вставка. Подозрительные (много событий при высокой доле верных) — первыми. Автоматических санкций нет.",
	}, func(ctx context.Context, in *sessionsInput) (*sessionsOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleAdmin); err != nil {
			return nil, MapError(err)
		}
		items, err := d.Service.FlaggedSessions(ctx, in.Limit)
		if err != nil {
			return nil, MapError(err)
		}
		out := &sessionsOutput{}
		out.Body.Items = items
		return out, nil
	})

	type employersOutput struct {
		Body struct {
			Items []admin.Employer `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-employers", Method: http.MethodGet, Path: "/admin/employers",
		Summary: "Работодатели и репутация", Tags: []string{"admin"}, Security: bearerSecurity,
		Description: "Доля принятых приглашений среди ответивших: good ≥ 50 %, normal ≥ 25 %, low — ниже; new — меньше трёх ответов.",
	}, func(ctx context.Context, _ *struct{}) (*employersOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleAdmin); err != nil {
			return nil, MapError(err)
		}
		items, err := d.Service.Employers(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		out := &employersOutput{}
		out.Body.Items = items
		return out, nil
	})

	type vacanciesOutput struct {
		Body struct {
			Items []admin.VacancyRow `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-vacancies", Method: http.MethodGet, Path: "/admin/vacancies",
		Summary: "Вакансии платформы (модерация)", Tags: []string{"admin"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*vacanciesOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleAdmin); err != nil {
			return nil, MapError(err)
		}
		items, err := d.Service.Vacancies(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		out := &vacanciesOutput{}
		out.Body.Items = items
		return out, nil
	})

	type hideInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Reason string `json:"reason" minLength:"1" maxLength:"1000"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-vacancy-hide", Method: http.MethodPost, Path: "/admin/vacancies/{id}/hide",
		Summary: "Снять вакансию с публикации", Tags: []string{"admin"}, Security: bearerSecurity, DefaultStatus: http.StatusAccepted,
		Description: "Статус становится closed; причина — в аудите. 409 invalid_transition, если вакансия не опубликована.",
	}, func(ctx context.Context, in *hideInput) (*adminAcceptedOut, error) {
		p, err := domain.RequireRole(ctx, domain.RoleAdmin)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.HideVacancy(ctx, p.UserID, in.ID, in.Body.Reason); err != nil {
			return nil, MapError(err)
		}
		out := &adminAcceptedOut{Status: http.StatusAccepted}
		out.Body.Message = "Вакансия снята с публикации"
		return out, nil
	})

	type auditInput struct {
		Action string `query:"action" maxLength:"80" doc:"Префикс действия: invitation., application., admin."`
		Actor  string `query:"actor" maxLength:"120" doc:"Часть e-mail актора"`
		Limit  int    `query:"limit" minimum:"1" maximum:"200"`
		Offset int    `query:"offset" minimum:"0"`
	}
	type auditOutput struct {
		Body admin.AuditList
	}
	huma.Register(api, huma.Operation{
		OperationID: "admin-audit", Method: http.MethodGet, Path: "/admin/audit",
		Summary: "Журнал аудита", Tags: []string{"admin"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *auditInput) (*auditOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleAdmin); err != nil {
			return nil, MapError(err)
		}
		list, err := d.Service.Audit(ctx, in.Action, in.Actor, in.Limit, in.Offset)
		if err != nil {
			return nil, MapError(err)
		}
		return &auditOutput{Body: list}, nil
	})
}

// metricsHandler — /metrics в текстовом формате Prometheus (без аутентификации: только агрегаты).
func metricsHandler(svc *admin.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		text, err := svc.Metrics(r.Context())
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "Internal Server Error")
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(text))
	})
}
