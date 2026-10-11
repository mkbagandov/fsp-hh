package httptransport

import (
	"context"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"fsp/internal/app/candidates"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/pdftext"
)

func registerCandidateResume(api huma.API, d CandidateDeps) {
	type uploadInput struct {
		RawBody huma.MultipartFormFiles[struct {
			File huma.FormFile `form:"file" contentType:"application/pdf" required:"true" doc:"PDF-резюме до 10 МБ"`
		}]
	}
	type uploadOutput struct {
		Status int
		Body   candidates.ResumeUpload
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-resume-upload", Method: http.MethodPost, Path: "/candidate/resume",
		Summary: "Загрузить PDF-резюме и получить черновик профиля", Tags: []string{"candidate"}, Security: bearerSecurity,
		DefaultStatus: http.StatusCreated,
		Description: "Текст извлекается из PDF, разбирается провайдером LLM (в MVP — детерминированный эмулятор на правилах) " +
			"и возвращается как черновик. В профиль ничего не записывается до подтверждения (/candidate/resume/{id}/apply). " +
			"Текст извлекается в изолированном процессе с тайм-аутом; хранятся 5 последних загрузок, не больше 20 в сутки. " +
			"Ошибки: 401/403 (до чтения тела), 413 (тело больше 11 МБ), 422 file_not_pdf, 422 file_size_invalid, 429 resume_upload_limit.",
	}, func(ctx context.Context, in *uploadInput) (*uploadOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		if in.RawBody.Form != nil {
			defer func() { _ = in.RawBody.Form.RemoveAll() }()
		}
		f := in.RawBody.Data().File
		if !f.IsSet {
			return nil, MapError(domain.Validation("file", "file_required", "Приложите PDF-файл в поле file"))
		}
		data, err := io.ReadAll(io.LimitReader(f.File, pdftext.MaxBytes+1))
		if err != nil {
			return nil, MapError(domain.Validation("file", "file_read_error", "Не удалось прочитать файл"))
		}
		up, err := d.Service.UploadResume(ctx, p.UserID, f.Filename, data)
		if err != nil {
			return nil, MapError(err)
		}
		return &uploadOutput{Status: http.StatusCreated, Body: up}, nil
	})

	type draftPath struct {
		ID uuid.UUID `path:"id"`
	}
	type draftOutput struct {
		Body candidates.ResumeUpload
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-resume-draft", Method: http.MethodGet, Path: "/candidate/resume/{id}",
		Summary: "Черновик из загруженного резюме", Tags: []string{"candidate"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *draftPath) (*draftOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		up, err := d.Service.GetResumeDraft(ctx, p.UserID, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &draftOutput{Body: up}, nil
	})

	type resumeApplyInput struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			Profile     bool `json:"profile,omitempty" doc:"ФИО, город, заголовок, о себе, стаж, специализация, ссылки"`
			Contacts    bool `json:"contacts,omitempty"`
			Experiences bool `json:"experiences,omitempty" doc:"Заменить список мест работы"`
			Education   bool `json:"education,omitempty" doc:"Заменить список образования"`
			Skills      bool `json:"skills,omitempty" doc:"Добавить навыки из резюме (source=resume)"`
			Overwrite   bool `json:"overwrite,omitempty" doc:"Перезаписывать уже заполненные поля"`
		}
	}
	type resumeProfileOutput struct {
		Body candidates.Profile
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-resume-apply", Method: http.MethodPost, Path: "/candidate/resume/{id}/apply",
		Summary: "Применить подтверждённые блоки черновика к профилю", Tags: []string{"candidate"}, Security: bearerSecurity,
	}, func(ctx context.Context, in *resumeApplyInput) (*resumeProfileOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		prof, err := d.Service.ApplyResumeDraft(ctx, p.UserID, in.ID, candidates.ApplyInput{
			Profile: in.Body.Profile, Contacts: in.Body.Contacts, Experiences: in.Body.Experiences,
			Education: in.Body.Education, Skills: in.Body.Skills, Overwrite: in.Body.Overwrite,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &resumeProfileOutput{Body: prof}, nil
	})

	type pdfOutput struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-profile-pdf", Method: http.MethodGet, Path: "/candidate/profile.pdf",
		Summary: "Стандартизированный PDF-профиль (с контактами, для владельца)", Tags: []string{"candidate"}, Security: bearerSecurity,
		Responses: map[string]*huma.Response{"200": {Description: "PDF-документ", Content: map[string]*huma.MediaType{"application/pdf": {}}}},
	}, func(ctx context.Context, _ *struct{}) (*pdfOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		data, err := d.Service.ProfilePDF(ctx, p.UserID, true)
		if err != nil {
			return nil, MapError(err)
		}
		return &pdfOutput{ContentType: "application/pdf", ContentDisposition: `inline; filename="fsp-profile.pdf"`, Body: data}, nil
	})
}
