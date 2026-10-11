package httptransport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"fsp/internal/app/candidates"
	"fsp/internal/domain"
)

// CandidateDeps — модуль профиля кандидата.
type CandidateDeps struct {
	Service *candidates.Service
}

func registerCandidate(api huma.API, d CandidateDeps) {
	type profileOutput struct {
		Body candidates.Profile
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-profile-get", Method: http.MethodGet, Path: "/candidate/profile",
		Summary: "Профиль кандидата", Tags: []string{"candidate"}, Security: bearerSecurity,
		Description: "Полный профиль владельца без контактов (контакты — отдельная операция).",
	}, func(ctx context.Context, _ *struct{}) (*profileOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		prof, err := d.Service.Get(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		return &profileOutput{Body: prof}, nil
	})

	type profileInput struct {
		Body struct {
			FirstName             string            `json:"first_name" minLength:"1" maxLength:"80"`
			LastName              string            `json:"last_name" minLength:"1" maxLength:"80"`
			MiddleName            *string           `json:"middle_name,omitempty" maxLength:"80"`
			City                  *string           `json:"city,omitempty" maxLength:"80"`
			WorkFormats           []string          `json:"work_formats,omitempty" doc:"office, hybrid, remote"`
			Relocation            bool              `json:"relocation,omitempty"`
			SalaryMin             *int64            `json:"salary_min,omitempty" minimum:"1" doc:"Порог: приглашения с вилкой ниже помечаются"`
			SalaryExpected        *int64            `json:"salary_expected,omitempty" minimum:"1"`
			Visibility            string            `json:"visibility,omitempty" enum:"open,passive,hidden"`
			Headline              *string           `json:"headline,omitempty" maxLength:"160"`
			About                 *string           `json:"about,omitempty" maxLength:"4000"`
			ExperienceYears       *float64          `json:"experience_years,omitempty" minimum:"0" maximum:"60"`
			PrimarySpecialization *string           `json:"primary_specialization,omitempty" enum:"backend,frontend,data_ml,devops,qa,security"`
			Links                 map[string]string `json:"links,omitempty" doc:"github, gitlab, site, linkedin, hh, habr, other"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-profile-update", Method: http.MethodPut, Path: "/candidate/profile",
		Summary: "Обновить анкету", Tags: []string{"candidate"}, Security: bearerSecurity,
		Description: "Полная замена анкетных полей. Категория и грейд здесь не меняются — только тестом.",
	}, func(ctx context.Context, in *profileInput) (*profileOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		b := in.Body
		prof, err := d.Service.Update(ctx, p.UserID, candidates.UpdateInput{
			FirstName: b.FirstName, LastName: b.LastName, MiddleName: b.MiddleName, City: b.City,
			WorkFormats: b.WorkFormats, Relocation: b.Relocation, SalaryMin: b.SalaryMin, SalaryExpected: b.SalaryExpected,
			Visibility: b.Visibility, Headline: b.Headline, About: b.About, ExperienceYears: b.ExperienceYears,
			PrimarySpecialization: b.PrimarySpecialization, Links: b.Links,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &profileOutput{Body: prof}, nil
	})

	type contactsOutput struct {
		Body candidates.Contacts
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-contacts-get", Method: http.MethodGet, Path: "/candidate/contacts",
		Summary: "Контакты кандидата (владелец)", Tags: []string{"candidate"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*contactsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		c, err := d.Service.GetContacts(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		return &contactsOutput{Body: c}, nil
	})

	type contactsInput struct {
		Body candidates.Contacts
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-contacts-update", Method: http.MethodPut, Path: "/candidate/contacts",
		Summary: "Обновить контакты", Tags: []string{"candidate"}, Security: bearerSecurity,
		Description: "Контакты хранятся отдельно и раскрываются работодателю только после принятия приглашения или отклика.",
	}, func(ctx context.Context, in *contactsInput) (*contactsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		c, err := d.Service.UpdateContacts(ctx, p.UserID, in.Body)
		if err != nil {
			return nil, MapError(err)
		}
		return &contactsOutput{Body: c}, nil
	})

	type privacyOutput struct {
		Body candidates.Privacy
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-privacy-get", Method: http.MethodGet, Path: "/candidate/privacy",
		Summary: "Настройки приватности", Tags: []string{"candidate"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*privacyOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		pr, err := d.Service.GetPrivacy(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		return &privacyOutput{Body: pr}, nil
	})

	type privacyInput struct {
		Body candidates.Privacy
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-privacy-update", Method: http.MethodPut, Path: "/candidate/privacy",
		Summary: "Обновить приватность", Tags: []string{"candidate"}, Security: bearerSecurity,
		Description: "Что работодатель видит в карточке до раскрытия контактов. Изменение пишется в аудит.",
	}, func(ctx context.Context, in *privacyInput) (*privacyOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		pr, err := d.Service.UpdatePrivacy(ctx, p.UserID, in.Body)
		if err != nil {
			return nil, MapError(err)
		}
		return &privacyOutput{Body: pr}, nil
	})

	type experiencesInput struct {
		Body struct {
			Items []struct {
				CompanyName    string   `json:"company_name" minLength:"1" maxLength:"200"`
				Position       string   `json:"position" minLength:"1" maxLength:"200"`
				Specialization *string  `json:"specialization,omitempty" enum:"backend,frontend,data_ml,devops,qa,security"`
				StartedOn      string   `json:"started_on" format:"date"`
				EndedOn        *string  `json:"ended_on,omitempty" format:"date"`
				Description    *string  `json:"description,omitempty" maxLength:"4000"`
				Stack          []string `json:"stack,omitempty" maxItems:"30"`
			} `json:"items" maxItems:"30"`
		}
	}
	type experiencesOutput struct {
		Body struct {
			Items []candidates.Experience `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-experiences-replace", Method: http.MethodPut, Path: "/candidate/experiences",
		Summary: "Заменить список мест работы", Tags: []string{"candidate"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *experiencesInput) (*experiencesOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		items := make([]candidates.ExperienceInput, 0, len(in.Body.Items))
		for _, e := range in.Body.Items {
			items = append(items, candidates.ExperienceInput{
				CompanyName: e.CompanyName, Position: e.Position, Specialization: e.Specialization,
				StartedOn: e.StartedOn, EndedOn: e.EndedOn, Description: e.Description, Stack: e.Stack,
			})
		}
		out := &experiencesOutput{}
		out.Body.Items, err = d.Service.ReplaceExperiences(ctx, p.UserID, items)
		if err != nil {
			return nil, MapError(err)
		}
		return out, nil
	})

	type educationInput struct {
		Body struct {
			Items []struct {
				Institution   string  `json:"institution" minLength:"1" maxLength:"200"`
				Degree        *string `json:"degree,omitempty" maxLength:"100"`
				Field         *string `json:"field,omitempty" maxLength:"200"`
				GraduatedYear *int32  `json:"graduated_year,omitempty" minimum:"1950" maximum:"2100"`
			} `json:"items" maxItems:"10"`
		}
	}
	type educationOutput struct {
		Body struct {
			Items []candidates.Education `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-education-replace", Method: http.MethodPut, Path: "/candidate/education",
		Summary: "Заменить список образования", Tags: []string{"candidate"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *educationInput) (*educationOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		items := make([]candidates.EducationInput, 0, len(in.Body.Items))
		for _, e := range in.Body.Items {
			items = append(items, candidates.EducationInput{Institution: e.Institution, Degree: e.Degree, Field: e.Field, GraduatedYear: e.GraduatedYear})
		}
		out := &educationOutput{}
		out.Body.Items, err = d.Service.ReplaceEducation(ctx, p.UserID, items)
		if err != nil {
			return nil, MapError(err)
		}
		return out, nil
	})

	type skillsInput struct {
		Body struct {
			Items []struct {
				Code  string `json:"code" minLength:"1" maxLength:"64"`
				Level *int16 `json:"level,omitempty" minimum:"1" maximum:"5"`
			} `json:"items" maxItems:"100"`
		}
	}
	type candidateSkillsOutput struct {
		Body struct {
			Items []candidates.Skill `json:"items"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-skills-replace", Method: http.MethodPut, Path: "/candidate/skills",
		Summary: "Заменить заявленные навыки", Tags: []string{"candidate"}, Security: bearerSecurity,
		Description: "Коды — из справочника /skills. Навыки, подтверждённые тестом, не удаляются и не понижаются до «заявлен». Ошибка 422 skill_unknown.",
	}, func(ctx context.Context, in *skillsInput) (*candidateSkillsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		items := make([]candidates.SkillInput, 0, len(in.Body.Items))
		for _, s := range in.Body.Items {
			items = append(items, candidates.SkillInput{Code: s.Code, Level: s.Level})
		}
		out := &candidateSkillsOutput{}
		out.Body.Items, err = d.Service.SetSkills(ctx, p.UserID, items, domain.SkillDeclared)
		if err != nil {
			return nil, MapError(err)
		}
		return out, nil
	})
}
