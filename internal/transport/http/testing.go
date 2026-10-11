package httptransport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	apptesting "fsp/internal/app/testing"
	"fsp/internal/domain"
	"fsp/pkg/itembank"
)

// TestingDeps — модуль опроса и адаптивного тестирования.
type TestingDeps struct {
	Service *apptesting.Service
}

func registerTesting(api huma.API, d TestingDeps) {
	type surveyInput struct {
		Body struct {
			Specialization  string   `json:"specialization" enum:"backend,frontend,data_ml,devops,qa,security"`
			Industry        string   `json:"industry,omitempty" maxLength:"120"`
			PrimaryStack    []string `json:"primary_stack,omitempty" maxItems:"20" doc:"Коды навыков основного стека"`
			ClaimedGrade    string   `json:"claimed_grade" enum:"junior,middle,senior" doc:"Грейд, который кандидат заявляет и будет подтверждать тестом"`
			ExperienceYears *float64 `json:"experience_years,omitempty" minimum:"0" maximum:"60"`
		}
	}
	type surveyOutput struct {
		Status int
		Body   apptesting.Survey
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-survey-submit", Method: http.MethodPost, Path: "/candidate/survey",
		Summary: "Опрос: отрасль, специализация, стек, заявленный грейд", Tags: []string{"testing"}, Security: bearerSecurity,
		DefaultStatus: http.StatusCreated,
		Description:   "Обязательный шаг перед тестом. Заявленный грейд не становится категорией — его подтверждает тест.",
	}, func(ctx context.Context, in *surveyInput) (*surveyOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		sv, err := d.Service.SubmitSurvey(ctx, p.UserID, apptesting.SurveyInput{
			Specialization: in.Body.Specialization, Industry: in.Body.Industry, PrimaryStack: in.Body.PrimaryStack,
			ClaimedGrade: in.Body.ClaimedGrade, ExperienceYears: in.Body.ExperienceYears,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &surveyOutput{Status: http.StatusCreated, Body: sv}, nil
	})

	type surveyQuery struct {
		Specialization string `query:"specialization" enum:"backend,frontend,data_ml,devops,qa,security" required:"true"`
	}
	type surveyGetOutput struct {
		Body apptesting.Survey
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-survey-get", Method: http.MethodGet, Path: "/candidate/survey",
		Summary: "Последний опрос по специализации", Tags: []string{"testing"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *surveyQuery) (*surveyGetOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		sv, err := d.Service.LatestSurvey(ctx, p.UserID, in.Specialization)
		if err != nil {
			return nil, MapError(err)
		}
		return &surveyGetOutput{Body: sv}, nil
	})

	type testStartInput struct {
		Body struct {
			Specialization string `json:"specialization" enum:"backend,frontend,data_ml,devops,qa,security"`
			TargetGrade    string `json:"target_grade" enum:"junior,middle,senior"`
		}
	}
	type testStartOutput struct {
		Status int
		Body   struct {
			Session apptesting.SessionView `json:"session"`
			Item    *apptesting.ItemView   `json:"item,omitempty" doc:"Первое (или текущее) задание"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-test-start", Method: http.MethodPost, Path: "/candidate/tests",
		Summary: "Начать адаптивный тест на заявленный грейд", Tags: []string{"testing"}, Security: bearerSecurity,
		DefaultStatus: http.StatusCreated,
		Description: "Если активная сессия уже есть — возвращается она. Ошибки: 409 survey_required, 409 grade_already_confirmed, " +
			"423 grade_cooldown, 409 bank_too_small.",
	}, func(ctx context.Context, in *testStartInput) (*testStartOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		view, item, err := d.Service.StartTest(ctx, p.UserID, apptesting.StartInput{Specialization: in.Body.Specialization, TargetGrade: in.Body.TargetGrade})
		if err != nil {
			return nil, MapError(err)
		}
		out := &testStartOutput{Status: http.StatusCreated}
		out.Body.Session, out.Body.Item = view, item
		return out, nil
	})

	type sessionsOutput struct {
		Body struct {
			Items []apptesting.SessionView `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-tests-list", Method: http.MethodGet, Path: "/candidate/tests",
		Summary: "История тест-сессий", Tags: []string{"testing"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*sessionsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		items, err := d.Service.ListSessions(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &sessionsOutput{}
		out.Body.Items = items
		return out, nil
	})

	type sessionPath struct {
		ID uuid.UUID `path:"id"`
	}
	type sessionOutput struct {
		Body apptesting.SessionView
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-test-get", Method: http.MethodGet, Path: "/candidate/tests/{id}",
		Summary: "Состояние сессии и результат", Tags: []string{"testing"}, Security: bearerSecurity,
		Description: "После завершения содержит результат (θ, SE, решение, процентиль) и разбор заданий с пояснениями.",
	}, func(ctx context.Context, in *sessionPath) (*sessionOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		v, err := d.Service.GetSession(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &sessionOutput{Body: v}, nil
	})

	type itemOutput struct {
		Body struct {
			Item     *apptesting.ItemView    `json:"item,omitempty"`
			Finished bool                    `json:"finished"`
			Session  *apptesting.SessionView `json:"session,omitempty" doc:"Итог, если тест завершён"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-test-item", Method: http.MethodGet, Path: "/candidate/tests/{id}/item",
		Summary: "Текущее задание", Tags: []string{"testing"}, Security: bearerSecurity,
		Description: "Возвращает неотвеченное задание или выдаёт следующее по адаптивному алгоритму; при выполнении правила остановки завершает сессию.",
	}, func(ctx context.Context, in *sessionPath) (*itemOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		item, err := d.Service.NextItem(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &itemOutput{}
		out.Body.Item = item
		if item == nil {
			out.Body.Finished = true
			v, err := d.Service.GetSession(ctx, p.UserID, in.ID)
			if err != nil {
				return nil, MapError(err)
			}
			out.Body.Session = &v
		}
		return out, nil
	})

	type answerInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			ItemID     uuid.UUID `json:"item_id"`
			Submission struct {
				Keys   []string   `json:"keys,omitempty" doc:"single_choice / multi_choice"`
				Number *float64   `json:"number,omitempty" doc:"numeric"`
				Text   string     `json:"text,omitempty" doc:"short_text / code_output"`
				Order  []string   `json:"order,omitempty" doc:"ordering"`
				Rows   [][]string `json:"rows,omitempty" doc:"sql_result"`
			} `json:"submission"`
		}
	}
	type answerOutput struct {
		Body apptesting.AnswerResult
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-test-answer", Method: http.MethodPost, Path: "/candidate/tests/{id}/answer",
		Summary: "Ответить на задание", Tags: []string{"testing"}, Security: bearerSecurity,
		Description: "Правильность не раскрывается до завершения сессии. Ответ после дедлайна принимается, но засчитывается как неверный. " +
			"Ошибки: 409 item_already_answered, 409 session_closed, 409 session_expired.",
	}, func(ctx context.Context, in *answerInput) (*answerOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		s := in.Body.Submission
		res, err := d.Service.Answer(ctx, p.UserID, in.ID, in.Body.ItemID, itembank.Submission{Keys: s.Keys, Number: s.Number, Text: s.Text, Order: s.Order, Rows: s.Rows})
		if err != nil {
			return nil, MapError(err)
		}
		return &answerOutput{Body: res}, nil
	})

	type eventInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Kind string `json:"kind" enum:"blur,paste,copy,hidden,resize,devtools"`
		}
	}
	type acceptedOut struct {
		Status int
		Body   struct {
			Message string `json:"message"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-test-event", Method: http.MethodPost, Path: "/candidate/tests/{id}/events",
		Summary: "Событие поведения во время теста (телеметрия)", Tags: []string{"testing"}, Security: bearerSecurity, DefaultStatus: http.StatusAccepted,
		Description: "Потеря фокуса, вставка из буфера и т. п. Не влияет на результат автоматически — только флаги для проверки.",
	}, func(ctx context.Context, in *eventInput) (*acceptedOut, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.ReportEvent(ctx, p.UserID, in.ID, in.Body.Kind); err != nil {
			return nil, MapError(err)
		}
		out := &acceptedOut{Status: http.StatusAccepted}
		out.Body.Message = "Событие записано"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "candidate-test-abandon", Method: http.MethodPost, Path: "/candidate/tests/{id}/abandon",
		Summary: "Прервать тест", Tags: []string{"testing"}, Security: bearerSecurity, DefaultStatus: http.StatusAccepted,
		Description: "Результат не засчитывается, cooldown не тратится.",
	}, func(ctx context.Context, in *sessionPath) (*acceptedOut, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.Abandon(ctx, p.UserID, in.ID); err != nil {
			return nil, MapError(err)
		}
		out := &acceptedOut{Status: http.StatusAccepted}
		out.Body.Message = "Сессия прервана"
		return out, nil
	})

	type categoriesOutput struct {
		Body struct {
			Categories []apptesting.Category     `json:"categories"`
			History    []apptesting.HistoryEntry `json:"history"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-categories", Method: http.MethodGet, Path: "/candidate/categories",
		Summary: "Категории кандидата и история грейдов", Tags: []string{"testing"}, Security: bearerSecurity,
		Description: "Категория = специализация × подтверждённый грейд; показывает, можно ли сейчас пройти тест выше или ниже.",
	}, func(ctx context.Context, _ *struct{}) (*categoriesOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		cats, hist, err := d.Service.Categories(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &categoriesOutput{}
		out.Body.Categories, out.Body.History = cats, hist
		if out.Body.Categories == nil {
			out.Body.Categories = []apptesting.Category{}
		}
		if out.Body.History == nil {
			out.Body.History = []apptesting.HistoryEntry{}
		}
		return out, nil
	})
}
