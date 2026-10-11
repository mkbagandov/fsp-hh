package httptransport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"fsp/internal/app/employers"
	"fsp/internal/app/invitations"
	"fsp/internal/app/matching"
	"fsp/internal/domain"
)

// EmployerDeps — модуль работодателя: компания, потребности, подбор, приглашения.
type EmployerDeps struct {
	Employers   *employers.Service
	Matching    *matching.Service
	Invitations *invitations.Service
}

type idPath struct {
	ID uuid.UUID `path:"id"`
}

// employer проверяет роль и возвращает субъекта и компанию (409 company_required, если компании ещё нет).
func (d EmployerDeps) employer(ctx context.Context) (domain.Principal, uuid.UUID, error) {
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

func registerEmployer(api huma.API, d EmployerDeps) {
	registerEmployerCompany(api, d)
	registerEmployerNeeds(api, d)
	registerEmployerSelection(api, d)
	registerEmployerCandidates(api, d)
	registerEmployerInvitations(api, d)
	registerCandidateInvitations(api, d)
}

func registerEmployerCompany(api huma.API, d EmployerDeps) {
	type companyOutput struct {
		Body employers.Company
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-company-get", Method: http.MethodGet, Path: "/employer/company",
		Summary: "Компания работодателя", Tags: []string{"employer"}, Security: bearerSecurity,
		Description: "404 company — компания ещё не создана: заполните её через PUT.",
	}, func(ctx context.Context, _ *struct{}) (*companyOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		c, err := d.Employers.MyCompany(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		return &companyOutput{Body: c}, nil
	})

	type companyInput struct {
		Body struct {
			Name           string            `json:"name" minLength:"1" maxLength:"200"`
			INN            string            `json:"inn,omitempty" maxLength:"12" doc:"10 или 12 цифр"`
			Description    string            `json:"description,omitempty" maxLength:"4000"`
			Industry       string            `json:"industry,omitempty" maxLength:"120"`
			Website        string            `json:"website,omitempty" maxLength:"200"`
			City           string            `json:"city,omitempty" maxLength:"80"`
			PublicContacts map[string]string `json:"public_contacts,omitempty" doc:"Контакты, которые кандидат видит после принятия приглашения"`
			Position       string            `json:"position,omitempty" maxLength:"120" doc:"Должность пользователя в компании"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-company-upsert", Method: http.MethodPut, Path: "/employer/company",
		Summary: "Создать или обновить компанию", Tags: []string{"employer"}, Security: bearerSecurity,
		Description: "При первом вызове создаёт компанию и делает пользователя её владельцем. Ошибки: 422 company_name_invalid, inn_invalid, website_invalid.",
	}, func(ctx context.Context, in *companyInput) (*companyOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		b := in.Body
		c, err := d.Employers.UpsertCompany(ctx, p.UserID, employers.CompanyInput{
			Name: b.Name, INN: b.INN, Description: b.Description, Industry: b.Industry, Website: b.Website, City: b.City,
			PublicContacts: b.PublicContacts, Position: b.Position,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &companyOutput{Body: c}, nil
	})
}

type needBody struct {
	Title           string         `json:"title" minLength:"1" maxLength:"160"`
	Description     string         `json:"description,omitempty" maxLength:"8000"`
	TeamDescription string         `json:"team_description,omitempty" maxLength:"4000"`
	Specialization  string         `json:"specialization" enum:"backend,frontend,data_ml,devops,qa,security"`
	GradeMin        string         `json:"grade_min" enum:"junior,middle,senior"`
	GradeMax        string         `json:"grade_max" enum:"junior,middle,senior"`
	MustSkills      []string       `json:"must_skills,omitempty" doc:"Коды навыков из /refdata/skills"`
	NiceSkills      []string       `json:"nice_skills,omitempty"`
	SalaryFrom      int64          `json:"salary_from" minimum:"1" doc:"Рубли в месяц, обязательна"`
	SalaryTo        int64          `json:"salary_to" minimum:"1"`
	WorkFormats     []string       `json:"work_formats,omitempty" doc:"office, hybrid, remote"`
	City            string         `json:"city,omitempty" maxLength:"80"`
	RemoteOK        bool           `json:"remote_ok,omitempty"`
	Extraction      map[string]any `json:"extraction,omitempty" doc:"Черновик разбора текста (для аудита «что предложил помощник»)"`
}

func (b needBody) toInput() employers.NeedInput {
	in := employers.NeedInput{
		Title: b.Title, Description: b.Description, TeamDescription: b.TeamDescription, Specialization: b.Specialization,
		GradeMin: b.GradeMin, GradeMax: b.GradeMax, MustSkills: b.MustSkills, NiceSkills: b.NiceSkills,
		SalaryFrom: b.SalaryFrom, SalaryTo: b.SalaryTo, WorkFormats: b.WorkFormats, City: b.City, RemoteOK: b.RemoteOK,
	}
	if b.Extraction != nil {
		in.Extraction, _ = json.Marshal(b.Extraction)
	}
	return in
}

func registerEmployerNeeds(api huma.API, d EmployerDeps) {
	type needOutput struct {
		Body employers.Need
	}
	type needsOutput struct {
		Body struct {
			Items []employers.Need `json:"items"`
		}
	}
	type extractInput struct {
		Body struct {
			Text string `json:"text" minLength:"20" maxLength:"8000" doc:"Свободное описание: кого ищем, задачи, стек, вилка, формат"`
		}
	}
	type extractOutput struct {
		Body employers.Extraction
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-need-extract", Method: http.MethodPost, Path: "/employer/needs/extract",
		Summary: "Разобрать текст потребности в структуру", Tags: []string{"employer"}, Security: bearerSecurity,
		Description: "Помощник предлагает специализацию, грейды, навыки, вилку и формат; результат — черновик для подтверждения, ничего не сохраняется.",
	}, func(ctx context.Context, in *extractInput) (*extractOutput, error) {
		if _, err := domain.RequireRole(ctx, domain.RoleEmployer); err != nil {
			return nil, MapError(err)
		}
		ex, err := d.Employers.ExtractNeed(ctx, in.Body.Text)
		if err != nil {
			return nil, MapError(err)
		}
		return &extractOutput{Body: ex}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "employer-needs-list", Method: http.MethodGet, Path: "/employer/needs",
		Summary: "Потребности компании", Tags: []string{"employer"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*needsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		items, err := d.Employers.ListNeeds(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &needsOutput{}
		out.Body.Items = items
		return out, nil
	})

	type needCreateInput struct {
		Body needBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-need-create", Method: http.MethodPost, Path: "/employer/needs",
		Summary: "Создать потребность", Tags: []string{"employer"}, Security: bearerSecurity, DefaultStatus: http.StatusCreated,
		Description: "Вилка обязательна (salary_from ≤ salary_to). Ошибки: 409 company_required, 422 salary_*, skill_unknown.",
	}, func(ctx context.Context, in *needCreateInput) (*needOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		n, err := d.Employers.CreateNeed(ctx, p.UserID, in.Body.toInput())
		if err != nil {
			return nil, MapError(err)
		}
		return &needOutput{Body: n}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "employer-need-get", Method: http.MethodGet, Path: "/employer/needs/{id}",
		Summary: "Потребность", Tags: []string{"employer"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *idPath) (*needOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		n, err := d.Employers.GetNeed(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &needOutput{Body: n}, nil
	})

	type needUpdateInput struct {
		ID   uuid.UUID `path:"id"`
		Body needBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-need-update", Method: http.MethodPut, Path: "/employer/needs/{id}",
		Summary: "Обновить потребность", Tags: []string{"employer"}, Security: bearerSecurity,
		Description: "Изменение не трогает сохранённые версии подборки — постройте новую версию.",
	}, func(ctx context.Context, in *needUpdateInput) (*needOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		n, err := d.Employers.UpdateNeed(ctx, p.UserID, in.ID, in.Body.toInput())
		if err != nil {
			return nil, MapError(err)
		}
		return &needOutput{Body: n}, nil
	})

	type needStatusInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Status string `json:"status" enum:"draft,active,archived"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-need-status", Method: http.MethodPost, Path: "/employer/needs/{id}/status",
		Summary: "Сменить статус потребности", Tags: []string{"employer"}, Security: bearerSecurity,
		Description: "Переходы: draft→active|archived, active→archived|draft, archived→active. Иначе 409 invalid_transition.",
	}, func(ctx context.Context, in *needStatusInput) (*needOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		n, err := d.Employers.SetStatus(ctx, p.UserID, in.ID, in.Body.Status)
		if err != nil {
			return nil, MapError(err)
		}
		return &needOutput{Body: n}, nil
	})

	type feedbackOutput struct {
		Body invitations.Feedback
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-need-feedback", Method: http.MethodGet, Path: "/employer/needs/{id}/feedback",
		Summary: "Обратная связь по приглашениям потребности", Tags: []string{"employer"}, Security: bearerSecurity,
		Description: "Статусы приглашений, доля принятых, причины отказов и подсказки, что поправить в потребности.",
	}, func(ctx context.Context, in *idPath) (*feedbackOutput, error) {
		p, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := d.Employers.OwnNeed(ctx, p.UserID, in.ID); err != nil {
			return nil, MapError(err)
		}
		fb, err := d.Invitations.FeedbackForNeed(ctx, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &feedbackOutput{Body: fb}, nil
	})
}

func registerEmployerSelection(api huma.API, d EmployerDeps) {
	type mapOutput struct {
		Body struct {
			Weights matching.Weights        `json:"weights" doc:"Веса компонентов релевантности"`
			Map     []matching.CategoryStat `json:"map"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-need-map", Method: http.MethodGet, Path: "/employer/needs/{id}/map",
		Summary: "Карта категорий под потребность", Tags: []string{"matching"}, Security: bearerSecurity,
		Description: "По каждому грейду специализации: сколько кандидатов, доля подтверждённых и с достижениями ФСП, квартили ожиданий, покрытие вилкой и подсказки по вилке.",
	}, func(ctx context.Context, in *idPath) (*mapOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		row, err := d.Employers.OwnNeed(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		m, err := d.Matching.CategoryMapFor(ctx, matching.NeedFromRow(row))
		if err != nil {
			return nil, MapError(err)
		}
		out := &mapOutput{}
		out.Body.Weights = d.Matching.Weights()
		out.Body.Map = m
		return out, nil
	})

	type versionOutput struct {
		Body matching.SelectionVersion
	}
	type buildInput struct {
		ID   uuid.UUID `path:"id"`
		Body matching.SelectionFilters
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-selection-build", Method: http.MethodPost, Path: "/employer/needs/{id}/selection",
		Summary: "Построить подборку (новая версия)", Tags: []string{"matching"}, Security: bearerSecurity, DefaultStatus: http.StatusCreated,
		Description: "Ранжирует кандидатов категорий потребности: relevance = Σ wᵢ·sᵢ, score = relevance × (0.6 + 0.4·p_accept); " +
			"каждая строка содержит компоненты и факты-объяснения. Фильтры сохраняются в версии; ответ включает отличия от предыдущей версии.",
	}, func(ctx context.Context, in *buildInput) (*versionOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		row, err := d.Employers.OwnNeed(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		v, err := d.Matching.BuildVersion(ctx, row, p.UserID, in.Body)
		if err != nil {
			return nil, MapError(err)
		}
		return &versionOutput{Body: v}, nil
	})

	type getVersionInput struct {
		ID      uuid.UUID `path:"id"`
		Version int       `query:"version" minimum:"0" doc:"Номер версии; 0 или пусто — последняя"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-selection-get", Method: http.MethodGet, Path: "/employer/needs/{id}/selection",
		Summary: "Сохранённая версия подборки", Tags: []string{"matching"}, Security: bearerSecurity,
		Description: "Баллы и объяснения — снимок на момент построения; карточки кандидатов — актуальные с учётом приватности. 404 selection_version — версий ещё нет.",
	}, func(ctx context.Context, in *getVersionInput) (*versionOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		row, err := d.Employers.OwnNeed(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		v, err := d.Matching.GetVersion(ctx, row, in.Version)
		if err != nil {
			return nil, MapError(err)
		}
		return &versionOutput{Body: v}, nil
	})

	type versionsOutput struct {
		Body struct {
			Items []matching.SelectionVersionSummary `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-selection-versions", Method: http.MethodGet, Path: "/employer/needs/{id}/selection/versions",
		Summary: "История версий подборки", Tags: []string{"matching"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *idPath) (*versionsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleEmployer)
		if err != nil {
			return nil, MapError(err)
		}
		if _, err := d.Employers.OwnNeed(ctx, p.UserID, in.ID); err != nil {
			return nil, MapError(err)
		}
		items, err := d.Matching.ListVersions(ctx, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &versionsOutput{}
		out.Body.Items = items
		return out, nil
	})
}

func registerEmployerCandidates(api huma.API, d EmployerDeps) {
	type searchInput struct {
		Specialization string   `query:"specialization" enum:"backend,frontend,data_ml,devops,qa,security"`
		Grades         []string `query:"grades" doc:"junior, middle, senior (через запятую); пусто — все"`
		Skills         []string `query:"skills" doc:"Коды навыков, обязательных в профиле"`
		City           string   `query:"city" maxLength:"80"`
		Formats        []string `query:"formats" doc:"office, hybrid, remote"`
		HasFSP         string   `query:"has_fsp" enum:"true,false" doc:"Только с достижениями ФСП (true) или только без (false)"`
		GradeStatuses  []string `query:"grade_statuses" doc:"confirmed, upgrade_suggested, not_confirmed"`
		Limit          int      `query:"limit" minimum:"1" maximum:"200" doc:"По умолчанию 50"`
	}
	type itemsOutput struct {
		Body struct {
			Items []matching.SelectionItem `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-candidates-search", Method: http.MethodGet, Path: "/employer/candidates/search",
		Summary: "Поиск по банку кандидатов", Tags: []string{"matching"}, Security: bearerSecurity,
		Description: "Ранжирование без потребности: условия (вилка) нейтральны, учитываются тест, навыки, ФСП и свежесть. Только видимые кандидаты с согласием на публикацию.",
	}, func(ctx context.Context, in *searchInput) (*itemsOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		var hasFSP *bool
		if in.HasFSP != "" {
			v := in.HasFSP == "true"
			hasFSP = &v
		}
		items, err := d.Matching.Search(ctx, companyID, matching.SearchInput{
			Specialization: in.Specialization, Grades: in.Grades, SkillCodes: in.Skills, City: in.City, Formats: in.Formats,
			HasFSP: hasFSP, GradeStatuses: in.GradeStatuses, Limit: in.Limit,
		})
		if err != nil {
			return nil, MapError(err)
		}
		out := &itemsOutput{}
		out.Body.Items = items
		return out, nil
	})

	type cardOutput struct {
		Body matching.CandidateCardView
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-candidate-card", Method: http.MethodGet, Path: "/employer/candidates/{id}",
		Summary: "Карточка кандидата", Tags: []string{"matching"}, Security: bearerSecurity,
		Description: "Без контактов, с учётом настроек приватности кандидата. Поле contacts появляется только при активном гранте (принятое приглашение); просмотр контактов пишется в аудит.",
	}, func(ctx context.Context, in *idPath) (*cardOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		card, err := d.Matching.CandidateCard(ctx, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &cardOutput{Body: card}, nil
	})

	type candidateContactsForEmployerOutput struct {
		Body matching.CardContacts
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-candidate-contacts", Method: http.MethodGet, Path: "/employer/candidates/{id}/contacts",
		Summary: "Контакты кандидата (по гранту)", Tags: []string{"matching"}, Security: bearerSecurity,
		Description: "403 contacts_not_granted — кандидат ещё не принял приглашение компании.",
	}, func(ctx context.Context, in *idPath) (*candidateContactsForEmployerOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		c, err := d.Matching.Contacts(ctx, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &candidateContactsForEmployerOutput{Body: c}, nil
	})
}

func registerEmployerInvitations(api huma.API, d EmployerDeps) {
	type employerInvitationOutput struct {
		Body invitations.Invitation
	}
	type employerInvitationsOutput struct {
		Body struct {
			Items []invitations.Invitation `json:"items"`
		}
	}
	type invitationCreateInput struct {
		Body struct {
			CandidateID   uuid.UUID                 `json:"candidate_id"`
			NeedID        *uuid.UUID                `json:"need_id,omitempty" doc:"Потребность, по которой приглашаем (квота 20 активных)"`
			Message       string                    `json:"message" minLength:"1" maxLength:"2000"`
			SalaryFrom    int64                     `json:"salary_from" minimum:"1"`
			SalaryTo      int64                     `json:"salary_to" minimum:"1"`
			ContactMethod invitations.ContactMethod `json:"contact_method" doc:"Как связаться с компанией — кандидат увидит после принятия"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-invitation-create", Method: http.MethodPost, Path: "/employer/invitations",
		Summary: "Пригласить кандидата", Tags: []string{"invitations"}, Security: bearerSecurity, DefaultStatus: http.StatusCreated,
		Description: "Вилка и способ связи обязательны. Ошибки: 409 invitation_active_exists, 429 need_quota_exceeded / company_quota_exceeded, 404 candidate (скрыт или без согласия).",
	}, func(ctx context.Context, in *invitationCreateInput) (*employerInvitationOutput, error) {
		p, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		b := in.Body
		inv, err := d.Invitations.Create(ctx, p.UserID, companyID, invitations.CreateInput{
			CandidateID: b.CandidateID, NeedID: b.NeedID, Message: b.Message, SalaryFrom: b.SalaryFrom, SalaryTo: b.SalaryTo, ContactMethod: b.ContactMethod,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &employerInvitationOutput{Body: inv}, nil
	})

	type employerInvitationsListInput struct {
		NeedID uuid.UUID `query:"need_id" doc:"Фильтр по потребности; пусто — все приглашения компании"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "employer-invitations-list", Method: http.MethodGet, Path: "/employer/invitations",
		Summary: "Приглашения компании", Tags: []string{"invitations"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *employerInvitationsListInput) (*employerInvitationsOutput, error) {
		_, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		var needID *uuid.UUID
		if in.NeedID != uuid.Nil {
			needID = &in.NeedID
		}
		items, err := d.Invitations.ListForCompany(ctx, companyID, needID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &employerInvitationsOutput{}
		out.Body.Items = items
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "employer-invitation-withdraw", Method: http.MethodPost, Path: "/employer/invitations/{id}/withdraw",
		Summary: "Отозвать приглашение", Tags: []string{"invitations"}, Security: bearerSecurity,
		Description: "Только до ответа кандидата; иначе 409 invalid_transition.",
	}, func(ctx context.Context, in *idPath) (*employerInvitationOutput, error) {
		p, companyID, err := d.employer(ctx)
		if err != nil {
			return nil, err
		}
		inv, err := d.Invitations.Withdraw(ctx, p.UserID, companyID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &employerInvitationOutput{Body: inv}, nil
	})
}

func registerCandidateInvitations(api huma.API, d EmployerDeps) {
	type candidateInvitationOutput struct {
		Body invitations.Invitation
	}
	type candidateInvitationsOutput struct {
		Body struct {
			Items []invitations.Invitation `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-invitations-list", Method: http.MethodGet, Path: "/candidate/invitations",
		Summary: "Входящие приглашения", Tags: []string{"invitations"}, Security: bearerSecurity,
		Description: "Вилка видна сразу; способ связи компании — только после принятия.",
	}, func(ctx context.Context, _ *struct{}) (*candidateInvitationsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		items, err := d.Invitations.ListForCandidate(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &candidateInvitationsOutput{}
		out.Body.Items = items
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "candidate-invitation-get", Method: http.MethodGet, Path: "/candidate/invitations/{id}",
		Summary: "Приглашение (отмечает просмотр)", Tags: []string{"invitations"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *idPath) (*candidateInvitationOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		inv, err := d.Invitations.GetForCandidate(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &candidateInvitationOutput{Body: inv}, nil
	})

	type acceptOutput struct {
		Body invitations.AcceptResult
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-invitation-accept", Method: http.MethodPost, Path: "/candidate/invitations/{id}/accept",
		Summary: "Принять приглашение", Tags: []string{"invitations"}, Security: bearerSecurity,
		Description: "Создаёт грант: компания получает контакты кандидата, кандидат — способ связи и публичные контакты компании.",
	}, func(ctx context.Context, in *idPath) (*acceptOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		res, err := d.Invitations.Accept(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &acceptOutput{Body: res}, nil
	})

	type declineInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Reason  string `json:"reason" doc:"Код из справочника причин отказа (/refdata)"`
			Comment string `json:"comment,omitempty" maxLength:"1000"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-invitation-decline", Method: http.MethodPost, Path: "/candidate/invitations/{id}/decline",
		Summary: "Отклонить приглашение", Tags: []string{"invitations"}, Security: bearerSecurity,
		Description: "Причина обязательна — она агрегируется для работодателя как обратная связь по потребности. Контакты не раскрываются.",
	}, func(ctx context.Context, in *declineInput) (*candidateInvitationOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		inv, err := d.Invitations.Decline(ctx, p.UserID, in.ID, in.Body.Reason, in.Body.Comment)
		if err != nil {
			return nil, MapError(err)
		}
		return &candidateInvitationOutput{Body: inv}, nil
	})
}
