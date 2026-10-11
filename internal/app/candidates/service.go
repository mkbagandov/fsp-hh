// Package candidates — профиль кандидата: анкета, контакты, приватность, опыт,
// образование, навыки. Правила видимости — docs/01-domain.md, раздел 3.
package candidates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Service — сценарии профиля кандидата.
type Service struct {
	pool   *pgxpool.Pool
	q      *sqlcgen.Queries
	log    *slog.Logger
	resume *ResumeDeps
	pdf    *PDFDeps
}

// New создаёт сервис.
func New(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool), log: log}
}

// Experience — место работы.
type Experience struct {
	ID             uuid.UUID  `json:"id"`
	CompanyName    string     `json:"company_name"`
	Position       string     `json:"position"`
	Specialization *string    `json:"specialization,omitempty" enum:"backend,frontend,data_ml,devops,qa,security"`
	StartedOn      string     `json:"started_on" doc:"YYYY-MM-DD"`
	EndedOn        *string    `json:"ended_on,omitempty" doc:"YYYY-MM-DD; пусто — по настоящее время"`
	Description    *string    `json:"description,omitempty"`
	Stack          []string   `json:"stack"`
	SortOrder      int32      `json:"sort_order"`
	startedAt      time.Time  // служебное
	endedAt        *time.Time // служебное
}

// Education — образование.
type Education struct {
	ID            uuid.UUID `json:"id"`
	Institution   string    `json:"institution"`
	Degree        *string   `json:"degree,omitempty"`
	Field         *string   `json:"field,omitempty"`
	GraduatedYear *int32    `json:"graduated_year,omitempty"`
}

// Skill — навык кандидата.
type Skill struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Group  string `json:"group"`
	Level  *int16 `json:"level,omitempty" minimum:"1" maximum:"5" doc:"Самооценка 1–5"`
	Source string `json:"source" enum:"declared,resume,test_confirmed" doc:"Откуда навык: указан, из резюме, подтверждён тестом"`
}

// Profile — полный профиль кандидата (без контактов).
type Profile struct {
	UserID                uuid.UUID         `json:"user_id"`
	FirstName             string            `json:"first_name"`
	LastName              string            `json:"last_name"`
	MiddleName            *string           `json:"middle_name,omitempty"`
	City                  *string           `json:"city,omitempty"`
	WorkFormats           []string          `json:"work_formats"`
	Relocation            bool              `json:"relocation"`
	SalaryMin             *int64            `json:"salary_min,omitempty" doc:"Минимальная зарплата (порог), рубли"`
	SalaryExpected        *int64            `json:"salary_expected,omitempty" doc:"Ожидаемая зарплата, рубли"`
	Visibility            string            `json:"visibility" enum:"open,passive,hidden"`
	Headline              *string           `json:"headline,omitempty"`
	About                 *string           `json:"about,omitempty"`
	ExperienceYears       *float64          `json:"experience_years,omitempty"`
	PrimarySpecialization *string           `json:"primary_specialization,omitempty"`
	Links                 map[string]string `json:"links"`
	ResumeUploaded        bool              `json:"resume_uploaded"`
	Experiences           []Experience      `json:"experiences"`
	Education             []Education       `json:"education"`
	Skills                []Skill           `json:"skills"`
	Completeness          int               `json:"completeness" doc:"Заполненность профиля, %"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// Get возвращает профиль, создавая пустые строки для старых учётных записей.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (Profile, error) {
	p, err := s.q.GetCandidateProfile(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := s.ensureRows(ctx, userID); err != nil {
			return Profile{}, err
		}
		p, err = s.q.GetCandidateProfile(ctx, userID)
	}
	if err != nil {
		return Profile{}, err
	}
	exps, err := s.q.ListCandidateExperiences(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	edu, err := s.q.ListCandidateEducation(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	skills, err := s.q.ListCandidateSkills(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	out := toProfile(p)
	for _, e := range exps {
		out.Experiences = append(out.Experiences, toExperience(e))
	}
	for _, e := range edu {
		out.Education = append(out.Education, Education{ID: e.ID, Institution: e.Institution, Degree: e.Degree, Field: e.Field, GraduatedYear: e.GraduatedYear})
	}
	for _, sk := range skills {
		out.Skills = append(out.Skills, Skill{Code: sk.Code, Name: sk.Name, Group: sk.GroupCode, Level: sk.Level, Source: string(sk.Source)})
	}
	out.Completeness = completeness(out)
	return out, nil
}

func (s *Service) ensureRows(ctx context.Context, userID uuid.UUID) error {
	return postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.EnsureCandidateProfile(ctx, userID); err != nil {
			return err
		}
		if err := q.EnsureCandidateContacts(ctx, sqlcgen.EnsureCandidateContactsParams{UserID: userID}); err != nil {
			return err
		}
		return q.EnsurePrivacySettings(ctx, userID)
	})
}

// UpdateInput — редактируемые поля профиля.
type UpdateInput struct {
	FirstName             string
	LastName              string
	MiddleName            *string
	City                  *string
	WorkFormats           []string
	Relocation            bool
	SalaryMin             *int64
	SalaryExpected        *int64
	Visibility            string
	Headline              *string
	About                 *string
	ExperienceYears       *float64
	PrimarySpecialization *string
	Links                 map[string]string
}

var allowedLinks = map[string]bool{"github": true, "gitlab": true, "site": true, "linkedin": true, "hh": true, "habr": true, "other": true}

// Update сохраняет анкету после проверки инвариантов.
func (s *Service) Update(ctx context.Context, userID uuid.UUID, in UpdateInput) (Profile, error) {
	in.FirstName, in.LastName = strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName)
	if in.FirstName == "" || len([]rune(in.FirstName)) > 80 {
		return Profile{}, domain.Validation("first_name", "first_name_invalid", "Укажите имя (до 80 символов)")
	}
	if in.LastName == "" || len([]rune(in.LastName)) > 80 {
		return Profile{}, domain.Validation("last_name", "last_name_invalid", "Укажите фамилию (до 80 символов)")
	}
	vis := domain.Visibility(in.Visibility)
	if in.Visibility == "" {
		vis = domain.VisibilityOpen
	}
	if !vis.Valid() {
		return Profile{}, domain.Validation("visibility", "visibility_invalid", "Видимость: open, passive или hidden")
	}
	formats := make([]sqlcgen.WorkFormat, 0, len(in.WorkFormats))
	seen := map[string]bool{}
	for _, f := range in.WorkFormats {
		if !refdata.IsWorkFormat(f) {
			return Profile{}, domain.Validation("work_formats", "work_format_invalid", "Неизвестный формат работы: "+f)
		}
		if !seen[f] {
			seen[f] = true
			formats = append(formats, sqlcgen.WorkFormat(f))
		}
	}
	if in.SalaryMin != nil && (*in.SalaryMin <= 0 || *in.SalaryMin > domain.MaxSalary) {
		return Profile{}, domain.Validation("salary_min", "salary_invalid", "Минимальная зарплата должна быть положительным числом в рублях")
	}
	if in.SalaryExpected != nil && (*in.SalaryExpected <= 0 || *in.SalaryExpected > domain.MaxSalary) {
		return Profile{}, domain.Validation("salary_expected", "salary_invalid", "Ожидаемая зарплата должна быть положительным числом в рублях")
	}
	if in.SalaryMin != nil && in.SalaryExpected != nil && *in.SalaryMin > *in.SalaryExpected {
		return Profile{}, domain.Validation("salary_min", "salary_min_above_expected", "Минимальная зарплата не может быть выше ожидаемой")
	}
	if in.ExperienceYears != nil && (*in.ExperienceYears < 0 || *in.ExperienceYears > 60) {
		return Profile{}, domain.Validation("experience_years", "experience_years_invalid", "Стаж указан некорректно")
	}
	var spec *sqlcgen.SpecializationCode
	if in.PrimarySpecialization != nil && *in.PrimarySpecialization != "" {
		if !refdata.IsSelectableSpecialization(*in.PrimarySpecialization) {
			return Profile{}, domain.Validation("primary_specialization", "specialization_invalid", "Неизвестная специализация")
		}
		v := sqlcgen.SpecializationCode(*in.PrimarySpecialization)
		spec = &v
	}
	links := map[string]string{}
	for k, v := range in.Links {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !allowedLinks[k] {
			return Profile{}, domain.Validation("links", "link_key_invalid", "Недопустимый тип ссылки: "+k)
		}
		if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || len(v) > 300 {
			return Profile{}, domain.Validation("links."+k, "link_invalid", "Ссылка должна начинаться с http(s)://")
		}
		links[k] = v
	}
	linksJSON, _ := json.Marshal(links)
	if in.Headline != nil && len([]rune(*in.Headline)) > 160 {
		return Profile{}, domain.Validation("headline", "headline_too_long", "Заголовок — до 160 символов")
	}
	if in.About != nil && len([]rune(*in.About)) > 4000 {
		return Profile{}, domain.Validation("about", "about_too_long", "«О себе» — до 4000 символов")
	}

	if err := s.ensureRows(ctx, userID); err != nil {
		return Profile{}, err
	}
	_, err := s.q.UpdateCandidateProfile(ctx, sqlcgen.UpdateCandidateProfileParams{
		UserID: userID, FirstName: in.FirstName, LastName: in.LastName,
		MiddleName: trimPtr(in.MiddleName, 80), City: trimPtr(in.City, 80),
		WorkFormats: formats, Relocation: in.Relocation,
		SalaryMin: toInt32Ptr(in.SalaryMin), SalaryExpected: toInt32Ptr(in.SalaryExpected),
		Visibility: sqlcgen.CandidateVisibility(vis), Headline: trimPtr(in.Headline, 160), About: trimPtr(in.About, 4000),
		ExperienceYears: in.ExperienceYears, PrimarySpecialization: spec, Links: linksJSON,
	})
	if err != nil {
		return Profile{}, fmt.Errorf("update profile: %w", err)
	}
	return s.Get(ctx, userID)
}

// Contacts — контакты кандидата (только владельцу и по гранту).
type Contacts struct {
	Phone    *string           `json:"phone,omitempty"`
	Telegram *string           `json:"telegram,omitempty"`
	Email    *string           `json:"email,omitempty"`
	Other    map[string]string `json:"other"`
}

var (
	rePhone    = regexp.MustCompile(`^\+?[0-9][0-9 ()\-]{6,30}$`)
	reTelegram = regexp.MustCompile(`^@?[A-Za-z0-9_]{4,32}$`)
)

// GetContacts возвращает контакты владельцу.
func (s *Service) GetContacts(ctx context.Context, userID uuid.UUID) (Contacts, error) {
	c, err := s.q.GetCandidateContacts(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := s.ensureRows(ctx, userID); err != nil {
			return Contacts{}, err
		}
		c, err = s.q.GetCandidateContacts(ctx, userID)
	}
	if err != nil {
		return Contacts{}, err
	}
	return toContacts(c), nil
}

// UpdateContacts сохраняет контакты владельца.
func (s *Service) UpdateContacts(ctx context.Context, userID uuid.UUID, in Contacts) (Contacts, error) {
	if in.Phone != nil && *in.Phone != "" && !rePhone.MatchString(strings.TrimSpace(*in.Phone)) {
		return Contacts{}, domain.Validation("phone", "phone_invalid", "Телефон указан некорректно")
	}
	if in.Telegram != nil && *in.Telegram != "" {
		t := strings.TrimPrefix(strings.TrimSpace(*in.Telegram), "@")
		if !reTelegram.MatchString(t) {
			return Contacts{}, domain.Validation("telegram", "telegram_invalid", "Telegram: 4–32 латинских символа, цифры или _")
		}
		in.Telegram = &t
	}
	if in.Email != nil && *in.Email != "" {
		e := strings.ToLower(strings.TrimSpace(*in.Email))
		if !strings.Contains(e, "@") || len(e) > 254 {
			return Contacts{}, domain.Validation("email", "email_invalid", "E-mail указан некорректно")
		}
		in.Email = &e
	}
	if len(in.Other) > 10 {
		return Contacts{}, domain.Validation("other", "contacts_too_many", "Не больше 10 дополнительных контактов")
	}
	other := in.Other
	if other == nil {
		other = map[string]string{}
	}
	otherJSON, _ := json.Marshal(other)
	if err := s.ensureRows(ctx, userID); err != nil {
		return Contacts{}, err
	}
	c, err := s.q.UpdateCandidateContacts(ctx, sqlcgen.UpdateCandidateContactsParams{
		UserID: userID, Phone: emptyToNil(in.Phone), Telegram: emptyToNil(in.Telegram), Email: emptyToNil(in.Email), Other: otherJSON,
	})
	if err != nil {
		return Contacts{}, err
	}
	return toContacts(c), nil
}

// Privacy — что видно работодателю до раскрытия контактов.
type Privacy struct {
	ShowFullName       bool `json:"show_full_name"`
	ShowExperience     bool `json:"show_experience"`
	ShowFSP            bool `json:"show_fsp"`
	ShowSalaryExpected bool `json:"show_salary_expected"`
	ShowCity           bool `json:"show_city"`
}

// GetPrivacy возвращает настройки приватности.
func (s *Service) GetPrivacy(ctx context.Context, userID uuid.UUID) (Privacy, error) {
	p, err := s.q.GetPrivacySettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := s.ensureRows(ctx, userID); err != nil {
			return Privacy{}, err
		}
		p, err = s.q.GetPrivacySettings(ctx, userID)
	}
	if err != nil {
		return Privacy{}, err
	}
	return Privacy{ShowFullName: p.ShowFullName, ShowExperience: p.ShowExperience, ShowFSP: p.ShowFsp, ShowSalaryExpected: p.ShowSalaryExpected, ShowCity: p.ShowCity}, nil
}

// UpdatePrivacy сохраняет настройки приватности с записью в аудит.
func (s *Service) UpdatePrivacy(ctx context.Context, userID uuid.UUID, in Privacy) (Privacy, error) {
	if err := s.ensureRows(ctx, userID); err != nil {
		return Privacy{}, err
	}
	var out Privacy
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, err := q.UpdatePrivacySettings(ctx, sqlcgen.UpdatePrivacySettingsParams{
			UserID: userID, ShowFullName: in.ShowFullName, ShowExperience: in.ShowExperience, ShowFsp: in.ShowFSP,
			ShowSalaryExpected: in.ShowSalaryExpected, ShowCity: in.ShowCity,
		})
		if err != nil {
			return err
		}
		out = Privacy{ShowFullName: p.ShowFullName, ShowExperience: p.ShowExperience, ShowFSP: p.ShowFsp, ShowSalaryExpected: p.ShowSalaryExpected, ShowCity: p.ShowCity}
		details, _ := json.Marshal(in)
		return q.InsertAudit(ctx, sqlcgen.InsertAuditParams{
			ActorUserID: uuid.NullUUID{UUID: userID, Valid: true}, Action: "privacy.update", ObjectType: "user",
			ObjectID: uuid.NullUUID{UUID: userID, Valid: true}, Details: details,
		})
	})
	return out, err
}

// ExperienceInput — место работы на вход.
type ExperienceInput struct {
	CompanyName    string
	Position       string
	Specialization *string
	StartedOn      string
	EndedOn        *string
	Description    *string
	Stack          []string
}

// ReplaceExperiences заменяет список мест работы целиком (порядок — как передан).
func (s *Service) ReplaceExperiences(ctx context.Context, userID uuid.UUID, in []ExperienceInput) ([]Experience, error) {
	if len(in) > 30 {
		return nil, domain.Validation("experiences", "too_many", "Не больше 30 мест работы")
	}
	params := make([]sqlcgen.InsertCandidateExperienceParams, 0, len(in))
	for i, e := range in {
		field := fmt.Sprintf("experiences[%d]", i)
		e.CompanyName, e.Position = strings.TrimSpace(e.CompanyName), strings.TrimSpace(e.Position)
		if e.CompanyName == "" || len([]rune(e.CompanyName)) > 200 {
			return nil, domain.Validation(field+".company_name", "company_required", "Укажите компанию")
		}
		if e.Position == "" || len([]rune(e.Position)) > 200 {
			return nil, domain.Validation(field+".position", "position_required", "Укажите должность")
		}
		started, err := time.Parse("2006-01-02", e.StartedOn)
		if err != nil {
			return nil, domain.Validation(field+".started_on", "date_invalid", "Дата начала в формате YYYY-MM-DD")
		}
		var ended *time.Time
		if e.EndedOn != nil && *e.EndedOn != "" {
			t, err := time.Parse("2006-01-02", *e.EndedOn)
			if err != nil || t.Before(started) {
				return nil, domain.Validation(field+".ended_on", "date_invalid", "Дата окончания не раньше даты начала")
			}
			ended = &t
		}
		var spec *sqlcgen.SpecializationCode
		if e.Specialization != nil && *e.Specialization != "" {
			if !refdata.IsSelectableSpecialization(*e.Specialization) {
				return nil, domain.Validation(field+".specialization", "specialization_invalid", "Неизвестная специализация")
			}
			v := sqlcgen.SpecializationCode(*e.Specialization)
			spec = &v
		}
		if len(e.Stack) > 30 {
			e.Stack = e.Stack[:30]
		}
		stack := e.Stack
		if stack == nil {
			stack = []string{}
		}
		params = append(params, sqlcgen.InsertCandidateExperienceParams{
			UserID: userID, CompanyName: e.CompanyName, Position: e.Position, Specialization: spec,
			StartedOn: started, EndedOn: ended, Description: trimPtr(e.Description, 4000), Stack: stack, SortOrder: int32(i), //nolint:gosec // i ≤ 30
		})
	}
	if err := s.ensureRows(ctx, userID); err != nil {
		return nil, err
	}
	var out []Experience
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.ReplaceCandidateExperiencesDelete(ctx, userID); err != nil {
			return err
		}
		out = make([]Experience, 0, len(params))
		for _, p := range params {
			row, err := q.InsertCandidateExperience(ctx, p)
			if err != nil {
				return err
			}
			out = append(out, toExperience(row))
		}
		return nil
	})
	return out, err
}

// EducationInput — образование на вход.
type EducationInput struct {
	Institution   string
	Degree        *string
	Field         *string
	GraduatedYear *int32
}

// ReplaceEducation заменяет список образования целиком.
func (s *Service) ReplaceEducation(ctx context.Context, userID uuid.UUID, in []EducationInput) ([]Education, error) {
	if len(in) > 10 {
		return nil, domain.Validation("education", "too_many", "Не больше 10 записей об образовании")
	}
	for i, e := range in {
		if strings.TrimSpace(e.Institution) == "" || len([]rune(e.Institution)) > 200 {
			return nil, domain.Validation(fmt.Sprintf("education[%d].institution", i), "institution_required", "Укажите учебное заведение")
		}
		if e.GraduatedYear != nil && (*e.GraduatedYear < 1950 || *e.GraduatedYear > 2100) {
			return nil, domain.Validation(fmt.Sprintf("education[%d].graduated_year", i), "year_invalid", "Год окончания указан некорректно")
		}
	}
	if err := s.ensureRows(ctx, userID); err != nil {
		return nil, err
	}
	var out []Education
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteCandidateEducation(ctx, userID); err != nil {
			return err
		}
		out = make([]Education, 0, len(in))
		for _, e := range in {
			row, err := q.InsertCandidateEducation(ctx, sqlcgen.InsertCandidateEducationParams{
				UserID: userID, Institution: strings.TrimSpace(e.Institution), Degree: trimPtr(e.Degree, 100), Field: trimPtr(e.Field, 200), GraduatedYear: e.GraduatedYear,
			})
			if err != nil {
				return err
			}
			out = append(out, Education{ID: row.ID, Institution: row.Institution, Degree: row.Degree, Field: row.Field, GraduatedYear: row.GraduatedYear})
		}
		return nil
	})
	return out, err
}

// SkillInput — навык на вход (код из справочника и самооценка).
type SkillInput struct {
	Code  string
	Level *int16
}

// SetSkills заменяет заявленные навыки; подтверждённые тестом не трогаются.
func (s *Service) SetSkills(ctx context.Context, userID uuid.UUID, in []SkillInput, source domain.SkillSource) ([]Skill, error) {
	if len(in) > 100 {
		return nil, domain.Validation("skills", "too_many", "Не больше 100 навыков")
	}
	if source != domain.SkillDeclared && source != domain.SkillFromResume {
		source = domain.SkillDeclared
	}
	codes := make([]string, 0, len(in))
	levels := map[string]*int16{}
	for _, sk := range in {
		c := strings.ToLower(strings.TrimSpace(sk.Code))
		if c == "" {
			continue
		}
		if sk.Level != nil && (*sk.Level < 1 || *sk.Level > 5) {
			return nil, domain.Validation("skills", "level_invalid", "Уровень навыка — от 1 до 5")
		}
		codes = append(codes, c)
		levels[c] = sk.Level
	}
	rows, err := s.q.GetSkillsByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	found := map[string]int32{}
	for _, r := range rows {
		found[r.Code] = r.ID
	}
	var unknown []string
	for _, c := range codes {
		if _, ok := found[c]; !ok {
			unknown = append(unknown, c)
		}
	}
	if len(unknown) > 0 {
		return nil, domain.Validation("skills", "skill_unknown", "Неизвестные навыки: "+strings.Join(unknown, ", "))
	}
	if err := s.ensureRows(ctx, userID); err != nil {
		return nil, err
	}
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		keep := make([]int32, 0, len(found))
		for _, c := range codes {
			id := found[c]
			keep = append(keep, id)
			if err := q.UpsertCandidateSkill(ctx, sqlcgen.UpsertCandidateSkillParams{
				UserID: userID, SkillID: id, Level: levels[c], Source: sqlcgen.SkillSource(source),
			}); err != nil {
				return err
			}
		}
		return q.DeleteCandidateSkillsNotIn(ctx, sqlcgen.DeleteCandidateSkillsNotInParams{UserID: userID, KeepIds: keep})
	})
	if err != nil {
		return nil, err
	}
	list, err := s.q.ListCandidateSkills(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(list))
	for _, sk := range list {
		out = append(out, Skill{Code: sk.Code, Name: sk.Name, Group: sk.GroupCode, Level: sk.Level, Source: string(sk.Source)})
	}
	return out, nil
}

// --- преобразования ---

func toProfile(p *sqlcgen.CandidateProfile) Profile {
	links := map[string]string{}
	_ = json.Unmarshal(p.Links, &links)
	formats := make([]string, 0, len(p.WorkFormats))
	for _, f := range p.WorkFormats {
		formats = append(formats, string(f))
	}
	var spec *string
	if p.PrimarySpecialization != nil {
		v := string(*p.PrimarySpecialization)
		spec = &v
	}
	return Profile{
		UserID: p.UserID, FirstName: p.FirstName, LastName: p.LastName, MiddleName: p.MiddleName, City: p.City,
		WorkFormats: formats, Relocation: p.Relocation,
		SalaryMin: toInt64Ptr(p.SalaryMin), SalaryExpected: toInt64Ptr(p.SalaryExpected),
		Visibility: string(p.Visibility), Headline: p.Headline, About: p.About, ExperienceYears: p.ExperienceYears,
		PrimarySpecialization: spec, Links: links, ResumeUploaded: p.ResumePdfPath != nil,
		Experiences: []Experience{}, Education: []Education{}, Skills: []Skill{}, UpdatedAt: p.UpdatedAt,
	}
}

func toExperience(e *sqlcgen.CandidateExperience) Experience {
	var spec *string
	if e.Specialization != nil {
		v := string(*e.Specialization)
		spec = &v
	}
	var ended *string
	if e.EndedOn != nil {
		v := e.EndedOn.Format("2006-01-02")
		ended = &v
	}
	stack := e.Stack
	if stack == nil {
		stack = []string{}
	}
	return Experience{
		ID: e.ID, CompanyName: e.CompanyName, Position: e.Position, Specialization: spec,
		StartedOn: e.StartedOn.Format("2006-01-02"), EndedOn: ended, Description: e.Description, Stack: stack, SortOrder: e.SortOrder,
		startedAt: e.StartedOn, endedAt: e.EndedOn,
	}
}

func toContacts(c *sqlcgen.CandidateContact) Contacts {
	other := map[string]string{}
	_ = json.Unmarshal(c.Other, &other)
	return Contacts{Phone: c.Phone, Telegram: c.Telegram, Email: c.Email, Other: other}
}

func completeness(p Profile) int {
	score := 0
	if p.FirstName != "" && p.LastName != "" {
		score += 15
	}
	if p.City != nil && *p.City != "" {
		score += 5
	}
	if len(p.WorkFormats) > 0 {
		score += 5
	}
	if p.SalaryExpected != nil {
		score += 10
	}
	if p.Headline != nil && *p.Headline != "" {
		score += 10
	}
	if p.About != nil && len(*p.About) >= 50 {
		score += 10
	}
	if p.PrimarySpecialization != nil {
		score += 10
	}
	if len(p.Experiences) > 0 {
		score += 15
	}
	if len(p.Skills) >= 3 {
		score += 15
	}
	if len(p.Education) > 0 {
		score += 5
	}
	return score
}

func trimPtr(s *string, maxRunes int) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	r := []rune(v)
	if len(r) > maxRunes {
		v = string(r[:maxRunes])
	}
	return &v
}

func emptyToNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}

func toInt32Ptr(v *int64) *int32 {
	if v == nil {
		return nil
	}
	x := int32(*v) //nolint:gosec // ограничено MaxSalary
	return &x
}

func toInt64Ptr(v *int32) *int64 {
	if v == nil {
		return nil
	}
	x := int64(*v)
	return &x
}
