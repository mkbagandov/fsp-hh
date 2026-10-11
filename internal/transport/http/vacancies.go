package httptransport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"fsp/internal/app/applications"
	"fsp/internal/app/employers"
	"fsp/internal/app/invitations"
	"fsp/internal/app/matching"
	"fsp/internal/app/notifications"
	"fsp/internal/app/vacancies"
	"fsp/internal/domain"
)

// VacancyDeps — вакансии, отклики и уведомления.
type VacancyDeps struct {
	Employers     *employers.Service
	Vacancies     *vacancies.Service
	Applications  *applications.Service
	Matching      *matching.Service
	Notifications *notifications.Service
}

func (d VacancyDeps) employer(ctx context.Context) (domain.Principal, uuid.UUID, error) {
	p, err := domain.RequireRole(ctx, domain.RoleEmployer)
	if err != nil {
		return p, uuid.Nil, MapError(err)
	}
	companyID, err := d.Employers.CompanyIDFor(ctx, p.UserID)
	if err != nil {
		return p, uuid.Nil, MapError(err)
	}
	return p, companyID, nil
}

func registerVacancies(api huma.API, d VacancyDeps) {
	registerPublicVacancies(api, d)
	registerEmployerVacancies(api, d)
	registerCandidateApplications(api, d)
	registerNotifications(api, d)
}

func registerPublicVacancies(api huma.API, d VacancyDeps) {
	type publicVacanciesInput struct {
		Specialization string `query:"specialization" enum:"backend,frontend,data_ml,devops,qa,security,"`
		Grade          string `query:"grade" enum:"junior,middle,senior," doc:"Грейд входит в диапазон вакансии"`
		WorkFormat     string `query:"work_format" enum:"office,hybrid,remote,"`
		City           string `query:"city" maxLength:"80" doc:"Город; удалённые вакансии показываются всегда"`
		SalaryMin      int64  `query:"salary_min" minimum:"0" doc:"Верх вилки не ниже этой суммы"`
		Q              string `query:"q" maxLength:"100" doc:"Поиск по названию вакансии и компании"`
		Limit          int    `query:"limit" minimum:"1" maximum:"100"`
		Offset         int    `query:"offset" minimum:"0"`
	}
	type publicVacanciesOutput struct {
		Body vacancies.PublicList
	}
	huma.Register(api, huma.Operation{
		OperationID: "vacancies-list", Method: http.MethodGet, Path: "/vacancies",
		Summary: "Опубликованные вакансии", Tags: []string{"vacancies"},
		Description: "Публичный список без аутентификации. Вилка показывается всегда — это обязательное условие платформы.",
	}, func(ctx context.Context, in *publicVacanciesInput) (*publicVacanciesOutput, error) {
		list, err := d.Vacancies.ListPublic(ctx, vacancies.PublicFilter{
			Specialization: in.Specialization, Grade: in.Grade, WorkFormat: in.WorkFormat, City: in.City, SalaryMin: in.SalaryMin, Query: in.Q, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &publicVacanciesOutput{Body: list}, nil
	})

	type publicOutput struct {
		Body vacancies.PublicVacancy
	}
	huma.Register(api, huma.Operation{
		OperationID: "vacancy-get", Method: http.MethodGet, Path: "/vacancies/{id}",
		Summary: "Вакансия", Tags: []string{"vacancies"},
	}, func(ctx context.Context, in *idPath) (*publicOutput, error) {
		v, err := d.Vacancies.GetPublic(ctx, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &publicOutput{Body: v}, nil
	})
}

type vacancyBody struct {
	Title          string     `json:"title" minLength:"1" maxLength:"160"`
	Description    string     `json:"description" minLength:"20" maxLength:"20000"`
	Specialization string     `json:"specialization" enum:"backend,frontend,data_ml,devops,qa,security"`
	GradeMin       string     `json:"grade_min" enum:"junior,middle,senior"`
	GradeMax       string     `json:"grade_max" enum:"junior,middle,senior"`
	MustSkills     []string   `json:"must_skills,omitempty" doc:"Коды навыков"`
	NiceSkills     []string   `json:"nice_skills,omitempty"`
	SalaryFrom     int64      `json:"salary_from" minimum:"1" doc:"Вилка обязательна"`
	SalaryTo       int64      `json:"salary_to" minimum:"1"`
	WorkFormats    []string   `json:"work_formats,omitempty" doc:"office, hybrid, remote"`
	City           string     `json:"city,omitempty" maxLength:"80"`
	RemoteOK       bool       `json:"remote_ok,omitempty"`
	NeedID         *uuid.UUID `json:"need_id,omitempty" doc:"Связанная потребность (для подборки и приглашений)"`
}

func (b vacancyBody) toInput() vacancies.Input {
	return vacancies.Input{
		Title: b.Title, Description: b.Description, Specialization: b.Specialization, GradeMin: b.GradeMin, GradeMax: b.GradeMax,
		MustSkills: b.MustSkills, NiceSkills: b.NiceSkills, SalaryFrom: b.SalaryFrom, SalaryTo: b.SalaryTo, WorkFormats: b.WorkFormats,
		City: b.City, RemoteOK: b.RemoteOK, NeedID: b.NeedID,
	}
}

func registerEmployerVacancies(api huma.API, d VacancyDeps) {
	type vacancyOutput struct {
		Body vacancies.Vacancy
	}
	type vacanciesOutput struct {
		Body struct {
			Items []vacancies.Vacancy `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-vacancies-list", Method: http.MethodGet, Path: "/employer/vacancies",
		Summary: "Вакансии компании", Tags: []string{"vacancies"}, Security: bearerSecurity,
		Description: "Со счётчиками откликов (всего и непросмотренных).",
	}, func(ctx context.Context, _ *struct{}) (*vacanciesOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		items, err := d.Vacancies.ListForCompany(ctx, companyID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &vacanciesOutput{}
		out.Body.Items = items
		return out, nil
	})

	type vacancyCreateInput struct {
		Body vacancyBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-vacancy-create", Method: http.MethodPost, Path: "/employer/vacancies",
		Summary: "Создать вакансию (черновик)", Tags: []string{"vacancies"}, Security: bearerSecurity, DefaultStatus: http.StatusCreated,
		Description: "Вилка обязательна. Публикация — отдельной операцией смены статуса.",
	}, func(ctx context.Context, in *vacancyCreateInput) (*vacancyOutput, error) {
		p, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		v, err := d.Vacancies.Create(ctx, p.UserID, companyID, in.Body.toInput())
		if err != nil {
			return nil, MapError(err)
		}
		return &vacancyOutput{Body: v}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "employer-vacancy-get", Method: http.MethodGet, Path: "/employer/vacancies/{id}",
		Summary: "Вакансия компании", Tags: []string{"vacancies"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *idPath) (*vacancyOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		v, err := d.Vacancies.Get(ctx, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &vacancyOutput{Body: v}, nil
	})

	type updateInput struct {
		ID   uuid.UUID `path:"id"`
		Body vacancyBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-vacancy-update", Method: http.MethodPut, Path: "/employer/vacancies/{id}",
		Summary: "Обновить вакансию", Tags: []string{"vacancies"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *updateInput) (*vacancyOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		v, err := d.Vacancies.Update(ctx, companyID, in.ID, in.Body.toInput())
		if err != nil {
			return nil, MapError(err)
		}
		return &vacancyOutput{Body: v}, nil
	})

	type vacancyStatusInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Status string `json:"status" enum:"draft,published,closed"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-vacancy-status", Method: http.MethodPost, Path: "/employer/vacancies/{id}/status",
		Summary: "Опубликовать, вернуть в черновик или закрыть", Tags: []string{"vacancies"}, Security: bearerSecurity,
		Description: "Переходы: draft→published, published→draft|closed. Иначе 409 invalid_transition.",
	}, func(ctx context.Context, in *vacancyStatusInput) (*vacancyOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		v, err := d.Vacancies.SetStatus(ctx, companyID, in.ID, in.Body.Status)
		if err != nil {
			return nil, MapError(err)
		}
		return &vacancyOutput{Body: v}, nil
	})

	type applicationsOutput struct {
		Body struct {
			Items []applications.Application `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-vacancy-applications", Method: http.MethodGet, Path: "/employer/vacancies/{id}/applications",
		Summary: "Отклики на вакансию", Tags: []string{"applications"}, Security: bearerSecurity,
		Description: "Анкеты с учётом приватности; контакты откликнувшихся доступны в карточке кандидата (грант создаётся откликом).",
	}, func(ctx context.Context, in *idPath) (*applicationsOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		items, err := d.Applications.ListForVacancy(ctx, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &applicationsOutput{}
		out.Body.Items = items
		return out, nil
	})

	type previewInput struct {
		ID   uuid.UUID `path:"id"`
		Body matching.SelectionFilters
	}
	type previewOutput struct {
		Body struct {
			Items []matching.SelectionItem `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-vacancy-candidates", Method: http.MethodPost, Path: "/employer/vacancies/{id}/candidates",
		Summary: "Подходящие кандидаты из банка под вакансию", Tags: []string{"matching"}, Security: bearerSecurity,
		Description: "Та же объяснимая модель ранжирования, что и для потребности, без сохранения версии. Приглашать можно прямо отсюда.",
	}, func(ctx context.Context, in *previewInput) (*previewOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		row, err := d.Vacancies.Own(ctx, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		items, err := d.Matching.Preview(ctx, vacancies.NeedFor(row), in.Body)
		if err != nil {
			return nil, MapError(err)
		}
		out := &previewOutput{}
		out.Body.Items = items
		return out, nil
	})

	type employerApplicationOutput struct {
		Body applications.Application
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-application-view", Method: http.MethodPost, Path: "/employer/applications/{id}/view",
		Summary: "Отметить отклик просмотренным", Tags: []string{"applications"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *idPath) (*employerApplicationOutput, error) {
		p, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		a, err := d.Applications.View(ctx, p.UserID, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &employerApplicationOutput{Body: a}, nil
	})

	type rejectInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Reason string `json:"reason" minLength:"1" maxLength:"1000" doc:"Кандидат увидит причину"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-application-reject", Method: http.MethodPost, Path: "/employer/applications/{id}/reject",
		Summary: "Отказать по отклику", Tags: []string{"applications"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *rejectInput) (*employerApplicationOutput, error) {
		p, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		a, err := d.Applications.Reject(ctx, p.UserID, companyID, in.ID, in.Body.Reason)
		if err != nil {
			return nil, MapError(err)
		}
		return &employerApplicationOutput{Body: a}, nil
	})

	type inviteInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Message       string                    `json:"message" minLength:"1" maxLength:"2000"`
			SalaryFrom    int64                     `json:"salary_from" minimum:"1"`
			SalaryTo      int64                     `json:"salary_to" minimum:"1"`
			ContactMethod invitations.ContactMethod `json:"contact_method"`
		}
	}
	type inviteOutput struct {
		Body applications.InviteResult
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-application-invite", Method: http.MethodPost, Path: "/employer/applications/{id}/invite",
		Summary: "Пригласить кандидата из отклика", Tags: []string{"applications"}, Security: bearerSecurity, DefaultStatus: http.StatusCreated,
		Description: "Создаёт приглашение с вилкой (привязано к вакансии) и переводит отклик в invited.",
	}, func(ctx context.Context, in *inviteInput) (*inviteOutput, error) {
		p, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		res, err := d.Applications.Invite(ctx, p.UserID, companyID, in.ID, applications.InviteInput{
			Message: in.Body.Message, SalaryFrom: in.Body.SalaryFrom, SalaryTo: in.Body.SalaryTo, ContactMethod: in.Body.ContactMethod,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &inviteOutput{Body: res}, nil
	})
}

func registerCandidateApplications(api huma.API, d VacancyDeps) {
	type candidateApplicationOutput struct {
		Body applications.Application
	}
	type vacancyApplyInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			CoverLetter string `json:"cover_letter,omitempty" maxLength:"4000"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "vacancy-apply", Method: http.MethodPost, Path: "/vacancies/{id}/apply",
		Summary: "Откликнуться на вакансию", Tags: []string{"applications"}, Security: bearerSecurity, DefaultStatus: http.StatusCreated,
		Description: "Отклик открывает компании контакты кандидата (правило ТЗ 2.2). Ошибки: 409 profile_required, 409 application_exists, 404 vacancy.",
	}, func(ctx context.Context, in *vacancyApplyInput) (*candidateApplicationOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		a, err := d.Applications.Apply(ctx, p.UserID, in.ID, in.Body.CoverLetter)
		if err != nil {
			return nil, MapError(err)
		}
		return &candidateApplicationOutput{Body: a}, nil
	})

	type candidateApplicationsOutput struct {
		Body struct {
			Items []applications.Application `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-applications-list", Method: http.MethodGet, Path: "/candidate/applications",
		Summary: "Мои отклики", Tags: []string{"applications"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*candidateApplicationsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		items, err := d.Applications.ListForCandidate(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &candidateApplicationsOutput{}
		out.Body.Items = items
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "candidate-application-withdraw", Method: http.MethodPost, Path: "/candidate/applications/{id}/withdraw",
		Summary: "Отозвать отклик", Tags: []string{"applications"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *idPath) (*candidateApplicationOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		a, err := d.Applications.Withdraw(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &candidateApplicationOutput{Body: a}, nil
	})
}

func registerNotifications(api huma.API, d VacancyDeps) {
	type notificationsListInput struct {
		Unread bool `query:"unread" doc:"Только непрочитанные"`
		Limit  int  `query:"limit" minimum:"1" maximum:"200"`
	}
	type notificationsListOutput struct {
		Body struct {
			Items       []notifications.Notification `json:"items"`
			UnreadCount int                          `json:"unread_count"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "notifications-list", Method: http.MethodGet, Path: "/notifications",
		Summary: "Уведомления", Tags: []string{"notifications"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *notificationsListInput) (*notificationsListOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		items, err := d.Notifications.List(ctx, p.UserID, in.Unread, in.Limit)
		if err != nil {
			return nil, MapError(err)
		}
		unread, err := d.Notifications.UnreadCount(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &notificationsListOutput{}
		out.Body.Items = items
		out.Body.UnreadCount = unread
		return out, nil
	})

	type countOutput struct {
		Body struct {
			UnreadCount int `json:"unread_count"`
			Marked      int `json:"marked,omitempty"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "notification-read", Method: http.MethodPost, Path: "/notifications/{id}/read",
		Summary: "Отметить прочитанным", Tags: []string{"notifications"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *idPath) (*countOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Notifications.MarkRead(ctx, p.UserID, in.ID); err != nil {
			return nil, MapError(err)
		}
		unread, err := d.Notifications.UnreadCount(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &countOutput{}
		out.Body.UnreadCount = unread
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "notifications-read-all", Method: http.MethodPost, Path: "/notifications/read-all",
		Summary: "Отметить все прочитанными", Tags: []string{"notifications"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*countOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		n, err := d.Notifications.MarkAllRead(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &countOutput{}
		out.Body.Marked = n
		return out, nil
	})
}
