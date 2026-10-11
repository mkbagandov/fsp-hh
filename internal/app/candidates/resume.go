package candidates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/infrastructure/pdftext"
	"fsp/internal/storage/postgres/sqlcgen"
)

// ResumeDeps — зависимости разбора резюме; подключаются через SetResumeDeps.
type ResumeDeps struct {
	LLM       llm.Client
	UploadDir string
	// Extract извлекает текст из PDF. В API — изолированный дочерний процесс (pdftext.Isolated);
	// если не задан, разбор идёт в процессе (тесты, утилиты).
	Extract func(ctx context.Context, data []byte) (string, error)
}

// Квоты загрузки резюме: защита диска и CPU от массовых загрузок одним пользователем.
const (
	maxUploadsPerDay = 20
	keepUploads      = 5
)

// SetResumeDeps включает загрузку и разбор резюме.
func (s *Service) SetResumeDeps(d ResumeDeps) { s.resume = &d }

// ResumeUpload — результат загрузки.
type ResumeUpload struct {
	ID        uuid.UUID       `json:"id"`
	FileName  string          `json:"file_name"`
	SizeBytes int             `json:"size_bytes"`
	TextChars int             `json:"text_chars" doc:"Сколько символов текста извлечено"`
	Parser    string          `json:"parser" doc:"Провайдер разбора: mock (правила) или модель"`
	Draft     llm.ResumeDraft `json:"draft" doc:"Черновик — требует подтверждения пользователем"`
	CreatedAt time.Time       `json:"created_at"`
}

// UploadResume сохраняет PDF, извлекает текст и строит черновик профиля.
// Ничего не записывается в профиль автоматически.
func (s *Service) UploadResume(ctx context.Context, userID uuid.UUID, fileName string, data []byte) (ResumeUpload, error) {
	if s.resume == nil {
		return ResumeUpload{}, domain.E(domain.KindConflict, "resume_disabled", "Загрузка резюме не настроена")
	}
	if len(data) == 0 || len(data) > pdftext.MaxBytes {
		return ResumeUpload{}, domain.Validation("file", "file_size_invalid", "Файл пустой или больше 10 МБ")
	}
	if !pdftext.IsPDF(data) {
		return ResumeUpload{}, domain.Validation("file", "file_not_pdf", "Поддерживается только PDF")
	}
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" || fileName == "." {
		fileName = "resume.pdf"
	}
	if err := s.ensureRows(ctx, userID); err != nil {
		return ResumeUpload{}, err
	}
	var recent int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM resume_uploads WHERE user_id = $1 AND created_at > now() - interval '24 hours'`, userID).Scan(&recent); err != nil {
		return ResumeUpload{}, err
	}
	if recent >= maxUploadsPerDay {
		return ResumeUpload{}, domain.E(domain.KindRateLimited, "resume_upload_limit", "Слишком много загрузок резюме за сутки, попробуйте позже")
	}
	id := uuid.New()
	dir := filepath.Join(s.resume.UploadDir, "resumes", userID.String())
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return ResumeUpload{}, fmt.Errorf("upload dir: %w", err)
	}
	path := filepath.Join(dir, id.String()+".pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return ResumeUpload{}, fmt.Errorf("write upload: %w", err)
	}
	row, err := s.q.CreateResumeUpload(ctx, sqlcgen.CreateResumeUploadParams{
		UserID: userID, FileName: fileName, Mime: "application/pdf", SizeBytes: int32(len(data)), StoragePath: path, //nolint:gosec // ≤ 10 МБ
	})
	if err != nil {
		return ResumeUpload{}, err
	}
	// ID из базы — чтобы файл и запись совпадали, переименуем файл под ID записи.
	finalPath := filepath.Join(dir, row.ID.String()+".pdf")
	if err := os.Rename(path, finalPath); err == nil {
		path = finalPath
		_, _ = s.pool.Exec(ctx, `UPDATE resume_uploads SET storage_path = $2 WHERE id = $1`, row.ID, path)
	}

	extract := s.resume.Extract
	if extract == nil {
		extract = func(_ context.Context, b []byte) (string, error) { return pdftext.Extract(b) }
	}
	text, err := extract(ctx, data)
	if err != nil {
		s.log.Warn("resume text extraction failed", "user_id", userID, "err", err)
		text = ""
	}
	draft, usage, err := s.resume.LLM.ExtractResume(ctx, text)
	if err != nil {
		return ResumeUpload{}, fmt.Errorf("resume parse: %w", err)
	}
	if text == "" {
		draft.Warnings = append(draft.Warnings, "Из PDF не удалось извлечь текст (возможно, скан): заполните профиль вручную")
	}
	raw, _ := json.Marshal(draft)
	parser := usage.Provider
	if err := s.q.SetResumeParsed(ctx, sqlcgen.SetResumeParsedParams{ID: row.ID, ParseResult: raw, Parser: &parser}); err != nil {
		return ResumeUpload{}, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE candidate_profiles SET resume_pdf_path = $2 WHERE user_id = $1`, userID, path); err != nil {
		return ResumeUpload{}, err
	}
	s.pruneUploads(ctx, userID)
	return ResumeUpload{
		ID: row.ID, FileName: fileName, SizeBytes: len(data), TextChars: len([]rune(text)),
		Parser: parser, Draft: draft, CreatedAt: row.CreatedAt,
	}, nil
}

// GetResumeDraft возвращает сохранённый черновик.
func (s *Service) GetResumeDraft(ctx context.Context, userID, uploadID uuid.UUID) (ResumeUpload, error) {
	row, err := s.q.GetResumeUpload(ctx, sqlcgen.GetResumeUploadParams{ID: uploadID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResumeUpload{}, domain.NotFound("resume_upload")
	}
	if err != nil {
		return ResumeUpload{}, err
	}
	var draft llm.ResumeDraft
	if row.ParseResult != nil {
		_ = json.Unmarshal(row.ParseResult, &draft)
	}
	parser := ""
	if row.Parser != nil {
		parser = *row.Parser
	}
	return ResumeUpload{ID: row.ID, FileName: row.FileName, SizeBytes: int(row.SizeBytes), Parser: parser, Draft: draft, CreatedAt: row.CreatedAt}, nil
}

// ApplyInput — какие блоки черновика применить.
type ApplyInput struct {
	Profile     bool // ФИО, город, заголовок, «о себе», стаж, специализация, ссылки — только пустые поля, если не Overwrite
	Contacts    bool
	Experiences bool // заменить список
	Education   bool // заменить список
	Skills      bool // добавить как source=resume (заявленные сохраняются)
	Overwrite   bool
}

// ApplyResumeDraft переносит подтверждённые пользователем блоки черновика в профиль.
func (s *Service) ApplyResumeDraft(ctx context.Context, userID, uploadID uuid.UUID, in ApplyInput) (Profile, error) {
	up, err := s.GetResumeDraft(ctx, userID, uploadID)
	if err != nil {
		return Profile{}, err
	}
	d := up.Draft
	cur, err := s.Get(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	if in.Profile {
		u := UpdateInput{
			FirstName: pick(cur.FirstName, d.FirstName, in.Overwrite), LastName: pick(cur.LastName, d.LastName, in.Overwrite),
			MiddleName: pickPtr(cur.MiddleName, d.MiddleName, in.Overwrite), City: pickPtr(cur.City, d.City, in.Overwrite),
			WorkFormats: cur.WorkFormats, Relocation: cur.Relocation, SalaryMin: cur.SalaryMin, SalaryExpected: cur.SalaryExpected,
			Visibility: cur.Visibility, Headline: pickPtr(cur.Headline, d.Headline, in.Overwrite), About: pickPtr(cur.About, d.About, in.Overwrite),
			ExperienceYears: cur.ExperienceYears, PrimarySpecialization: pickPtr(cur.PrimarySpecialization, d.PrimarySpecialization, in.Overwrite),
			Links: map[string]string{},
		}
		if (u.ExperienceYears == nil || in.Overwrite) && d.ExperienceYears > 0 {
			v := d.ExperienceYears
			u.ExperienceYears = &v
		}
		for k, v := range cur.Links {
			u.Links[k] = v
		}
		for k, v := range d.Links {
			if _, exists := u.Links[k]; !exists || in.Overwrite {
				if allowedLinks[k] {
					u.Links[k] = v
				}
			}
		}
		if u.FirstName == "" || u.LastName == "" {
			return Profile{}, domain.Validation("profile", "name_missing", "В черновике нет имени или фамилии — заполните анкету вручную")
		}
		if _, err := s.Update(ctx, userID, u); err != nil {
			return Profile{}, err
		}
	}
	if in.Contacts {
		c, err := s.GetContacts(ctx, userID)
		if err != nil {
			return Profile{}, err
		}
		c.Phone = pickPtr(c.Phone, d.Phone, in.Overwrite)
		c.Telegram = pickPtr(c.Telegram, d.Telegram, in.Overwrite)
		c.Email = pickPtr(c.Email, d.Email, in.Overwrite)
		if _, err := s.UpdateContacts(ctx, userID, c); err != nil {
			return Profile{}, err
		}
	}
	if in.Experiences && len(d.Experiences) > 0 {
		items := make([]ExperienceInput, 0, len(d.Experiences))
		for _, e := range d.Experiences {
			if e.StartedOn == "" {
				continue
			}
			var ended *string
			if e.EndedOn != "" {
				v := e.EndedOn
				ended = &v
			}
			var desc *string
			if e.Description != "" {
				v := e.Description
				desc = &v
			}
			items = append(items, ExperienceInput{
				CompanyName: nzs(e.Company, "Компания не указана"), Position: nzs(e.Position, "Должность не указана"),
				StartedOn: e.StartedOn, EndedOn: ended, Description: desc, Stack: e.Stack,
			})
		}
		if _, err := s.ReplaceExperiences(ctx, userID, items); err != nil {
			return Profile{}, err
		}
	}
	if in.Education && len(d.Education) > 0 {
		items := make([]EducationInput, 0, len(d.Education))
		for _, e := range d.Education {
			var year *int32
			if e.GraduatedYear >= 1950 && e.GraduatedYear <= 2100 {
				v := int32(e.GraduatedYear) //nolint:gosec // проверено диапазоном
				year = &v
			}
			var degree, field *string
			if e.Degree != "" {
				v := e.Degree
				degree = &v
			}
			if e.Field != "" {
				v := e.Field
				field = &v
			}
			items = append(items, EducationInput{Institution: e.Institution, Degree: degree, Field: field, GraduatedYear: year})
		}
		if _, err := s.ReplaceEducation(ctx, userID, items); err != nil {
			return Profile{}, err
		}
	}
	if in.Skills && len(d.Skills) > 0 {
		// Объединяем с уже заявленными: SetSkills заменяет набор, поэтому передаём оба списка.
		seen := map[string]bool{}
		items := []SkillInput{}
		for _, sk := range cur.Skills {
			seen[sk.Code] = true
			items = append(items, SkillInput{Code: sk.Code, Level: sk.Level})
		}
		for _, sk := range d.Skills {
			if !seen[sk.Code] {
				items = append(items, SkillInput{Code: sk.Code})
			}
		}
		if _, err := s.SetSkills(ctx, userID, items, domain.SkillFromResume); err != nil {
			return Profile{}, err
		}
	}
	return s.Get(ctx, userID)
}

func pick(cur, draft string, overwrite bool) string {
	if (cur == "" || overwrite) && strings.TrimSpace(draft) != "" {
		return strings.TrimSpace(draft)
	}
	return cur
}

func pickPtr(cur *string, draft string, overwrite bool) *string {
	if (cur == nil || *cur == "" || overwrite) && strings.TrimSpace(draft) != "" {
		v := strings.TrimSpace(draft)
		return &v
	}
	return cur
}

func nzs(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// pruneUploads оставляет последние keepUploads загрузок пользователя; остальные удаляются вместе с файлами.
func (s *Service) pruneUploads(ctx context.Context, userID uuid.UUID) {
	rows, err := s.pool.Query(ctx, `DELETE FROM resume_uploads WHERE user_id = $1 AND id NOT IN (
		SELECT id FROM resume_uploads WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2) RETURNING storage_path`, userID, keepUploads)
	if err != nil {
		s.log.Warn("prune resume uploads", "user_id", userID, "err", err)
		return
	}
	paths, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		s.log.Warn("prune resume uploads", "user_id", userID, "err", err)
		return
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("remove old resume file", "user_id", userID, "err", err)
		}
	}
}
