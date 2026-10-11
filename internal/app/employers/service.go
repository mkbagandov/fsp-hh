// Package employers — компания работодателя и её потребности (docs/03-matching.md, раздел 2).
package employers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Service — сценарии работодателя.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
	llm  llm.Client
	log  *slog.Logger
}

// New создаёт сервис.
func New(pool *pgxpool.Pool, client llm.Client, log *slog.Logger) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool), llm: client, log: log}
}

// Company — профиль компании.
type Company struct {
	ID             uuid.UUID         `json:"id"`
	Name           string            `json:"name"`
	INN            string            `json:"inn,omitempty"`
	Description    string            `json:"description,omitempty"`
	Industry       string            `json:"industry,omitempty"`
	Website        string            `json:"website,omitempty"`
	City           string            `json:"city,omitempty"`
	PublicContacts map[string]string `json:"public_contacts"`
	Role           string            `json:"role,omitempty" doc:"Роль пользователя в компании"`
	CreatedAt      time.Time         `json:"created_at"`
}

// CompanyInput — редактируемые поля.
type CompanyInput struct {
	Name           string
	INN            string
	Description    string
	Industry       string
	Website        string
	City           string
	PublicContacts map[string]string
	Position       string
}

// CompanyIDFor возвращает компанию пользователя-работодателя.
func (s *Service) CompanyIDFor(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	eu, err := s.q.GetEmployerUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.Conflict("company_required", "Сначала заполните профиль компании")
	}
	if err != nil {
		return uuid.Nil, err
	}
	return eu.CompanyID, nil
}

// MyCompany возвращает компанию пользователя.
func (s *Service) MyCompany(ctx context.Context, userID uuid.UUID) (Company, error) {
	eu, err := s.q.GetEmployerUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Company{}, domain.NotFound("company")
	}
	if err != nil {
		return Company{}, err
	}
	row, err := s.q.GetCompany(ctx, eu.CompanyID)
	if err != nil {
		return Company{}, err
	}
	c := toCompany(row)
	c.Role = eu.CompanyRole
	return c, nil
}

// UpsertCompany создаёт компанию при первом обращении или обновляет существующую.
func (s *Service) UpsertCompany(ctx context.Context, userID uuid.UUID, in CompanyInput) (Company, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 200 {
		return Company{}, domain.Validation("name", "company_name_invalid", "Укажите название компании (до 200 символов)")
	}
	if in.INN != "" && (len(in.INN) != 10 && len(in.INN) != 12 || strings.Trim(in.INN, "0123456789") != "") {
		return Company{}, domain.Validation("inn", "inn_invalid", "ИНН — 10 или 12 цифр")
	}
	if in.Website != "" && !strings.HasPrefix(in.Website, "http://") && !strings.HasPrefix(in.Website, "https://") {
		return Company{}, domain.Validation("website", "website_invalid", "Сайт должен начинаться с http(s)://")
	}
	contacts := in.PublicContacts
	if contacts == nil {
		contacts = map[string]string{}
	}
	if len(contacts) > 10 {
		return Company{}, domain.Validation("public_contacts", "contacts_too_many", "Не больше 10 контактов")
	}
	cJSON, _ := json.Marshal(contacts)
	var out Company
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		eu, err := q.GetEmployerUser(ctx, userID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			row, err := q.CreateCompany(ctx, sqlcgen.CreateCompanyParams{
				Name: in.Name, Inn: opt(in.INN), Description: opt(in.Description), Industry: opt(in.Industry), Website: opt(in.Website), City: opt(in.City), PublicContacts: cJSON,
			})
			if err != nil {
				return err
			}
			if err := q.CreateEmployerUser(ctx, sqlcgen.CreateEmployerUserParams{UserID: userID, CompanyID: row.ID, CompanyRole: "owner", Position: opt(in.Position)}); err != nil {
				return err
			}
			out = toCompany(row)
			out.Role = "owner"
			return q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: userID, Valid: true}, Action: "company.create", ObjectType: "company", ObjectID: uuid.NullUUID{UUID: row.ID, Valid: true}, Details: json.RawMessage(`{}`)})
		case err != nil:
			return err
		default:
			row, err := q.UpdateCompany(ctx, sqlcgen.UpdateCompanyParams{
				ID: eu.CompanyID, Name: in.Name, Inn: opt(in.INN), Description: opt(in.Description), Industry: opt(in.Industry), Website: opt(in.Website), City: opt(in.City), PublicContacts: cJSON,
			})
			if err != nil {
				return err
			}
			out = toCompany(row)
			out.Role = eu.CompanyRole
			return nil
		}
	})
	return out, err
}

// SkillRef — навык в потребности.
type SkillRef struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Need — потребность.
type Need struct {
	ID              uuid.UUID  `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description,omitempty"`
	TeamDescription string     `json:"team_description,omitempty"`
	Specialization  string     `json:"specialization"`
	GradeMin        string     `json:"grade_min"`
	GradeMax        string     `json:"grade_max"`
	MustSkills      []SkillRef `json:"must_skills"`
	NiceSkills      []SkillRef `json:"nice_skills"`
	SalaryFrom      int64      `json:"salary_from"`
	SalaryTo        int64      `json:"salary_to"`
	WorkFormats     []string   `json:"work_formats"`
	City            string     `json:"city,omitempty"`
	RemoteOK        bool       `json:"remote_ok"`
	Status          string     `json:"status" enum:"draft,active,archived"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// NeedInput — создание/обновление потребности.
type NeedInput struct {
	Title           string
	Description     string
	TeamDescription string
	Specialization  string
	GradeMin        string
	GradeMax        string
	MustSkills      []string
	NiceSkills      []string
	SalaryFrom      int64
	SalaryTo        int64
	WorkFormats     []string
	City            string
	RemoteOK        bool
	Extraction      json.RawMessage
}

func (s *Service) validateNeed(ctx context.Context, in *NeedInput) ([]int32, []int32, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len([]rune(in.Title)) > 160 {
		return nil, nil, domain.Validation("title", "title_invalid", "Укажите название потребности (до 160 символов)")
	}
	if !refdata.IsSelectableSpecialization(in.Specialization) {
		return nil, nil, domain.Validation("specialization", "specialization_invalid", "Выберите специализацию")
	}
	if !refdata.IsGrade(in.GradeMin) || !refdata.IsGrade(in.GradeMax) {
		return nil, nil, domain.Validation("grade", "grade_invalid", "Укажите диапазон грейдов")
	}
	if refdata.GradeOrder(in.GradeMin) > refdata.GradeOrder(in.GradeMax) {
		in.GradeMin, in.GradeMax = in.GradeMax, in.GradeMin
	}
	if err := (domain.SalaryRange{From: in.SalaryFrom, To: in.SalaryTo}).Validate(); err != nil {
		return nil, nil, err
	}
	for _, f := range in.WorkFormats {
		if !refdata.IsWorkFormat(f) {
			return nil, nil, domain.Validation("work_formats", "work_format_invalid", "Неизвестный формат работы: "+f)
		}
	}
	must, err := s.skillIDs(ctx, in.MustSkills, "must_skills")
	if err != nil {
		return nil, nil, err
	}
	nice, err := s.skillIDs(ctx, in.NiceSkills, "nice_skills")
	if err != nil {
		return nil, nil, err
	}
	return must, nice, nil
}

func (s *Service) skillIDs(ctx context.Context, codes []string, field string) ([]int32, error) {
	return ResolveSkillCodes(ctx, s.q, codes, field)
}

// ResolveSkillCodes переводит коды навыков справочника в идентификаторы; неизвестные коды — ошибка валидации поля field.
func ResolveSkillCodes(ctx context.Context, q *sqlcgen.Queries, codes []string, field string) ([]int32, error) {
	if len(codes) == 0 {
		return []int32{}, nil
	}
	if len(codes) > 30 {
		return nil, domain.Validation(field, "too_many", "Не больше 30 навыков")
	}
	rows, err := q.GetSkillsByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	found := map[string]int32{}
	for _, r := range rows {
		found[r.Code] = r.ID
	}
	ids := make([]int32, 0, len(codes))
	var unknown []string
	seen := map[int32]bool{}
	for _, c := range codes {
		id, ok := found[c]
		if !ok {
			unknown = append(unknown, c)
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(unknown) > 0 {
		return nil, domain.Validation(field, "skill_unknown", "Неизвестные навыки: "+strings.Join(unknown, ", "))
	}
	return ids, nil
}

// CreateNeed создаёт активную потребность.
func (s *Service) CreateNeed(ctx context.Context, userID uuid.UUID, in NeedInput) (Need, error) {
	companyID, err := s.CompanyIDFor(ctx, userID)
	if err != nil {
		return Need{}, err
	}
	must, nice, err := s.validateNeed(ctx, &in)
	if err != nil {
		return Need{}, err
	}
	formats := make([]sqlcgen.WorkFormat, 0, len(in.WorkFormats))
	for _, f := range in.WorkFormats {
		formats = append(formats, sqlcgen.WorkFormat(f))
	}
	row, err := s.q.CreateNeed(ctx, sqlcgen.CreateNeedParams{
		CompanyID: companyID, CreatedBy: userID, Title: in.Title, Description: opt(in.Description), TeamDescription: opt(in.TeamDescription),
		Specialization: sqlcgen.SpecializationCode(in.Specialization), GradeMin: sqlcgen.GradeCode(in.GradeMin), GradeMax: sqlcgen.GradeCode(in.GradeMax),
		MustSkillIds: must, NiceSkillIds: nice, SalaryFrom: int32(in.SalaryFrom), SalaryTo: int32(in.SalaryTo), //nolint:gosec // ≤ MaxSalary
		WorkFormats: formats, City: opt(in.City), RemoteOk: in.RemoteOK, Status: sqlcgen.NeedStatusActive, Extraction: in.Extraction,
	})
	if err != nil {
		return Need{}, err
	}
	return s.toNeed(ctx, row)
}

// UpdateNeed обновляет потребность компании.
func (s *Service) UpdateNeed(ctx context.Context, userID, needID uuid.UUID, in NeedInput) (Need, error) {
	row, err := s.OwnNeed(ctx, userID, needID)
	if err != nil {
		return Need{}, err
	}
	must, nice, err := s.validateNeed(ctx, &in)
	if err != nil {
		return Need{}, err
	}
	formats := make([]sqlcgen.WorkFormat, 0, len(in.WorkFormats))
	for _, f := range in.WorkFormats {
		formats = append(formats, sqlcgen.WorkFormat(f))
	}
	updated, err := s.q.UpdateNeed(ctx, sqlcgen.UpdateNeedParams{
		ID: row.ID, Title: in.Title, Description: opt(in.Description), TeamDescription: opt(in.TeamDescription),
		Specialization: sqlcgen.SpecializationCode(in.Specialization), GradeMin: sqlcgen.GradeCode(in.GradeMin), GradeMax: sqlcgen.GradeCode(in.GradeMax),
		MustSkillIds: must, NiceSkillIds: nice, SalaryFrom: int32(in.SalaryFrom), SalaryTo: int32(in.SalaryTo), //nolint:gosec // ≤ MaxSalary
		WorkFormats: formats, City: opt(in.City), RemoteOk: in.RemoteOK,
	})
	if err != nil {
		return Need{}, err
	}
	return s.toNeed(ctx, updated)
}

// SetStatus меняет статус потребности по допустимым переходам.
func (s *Service) SetStatus(ctx context.Context, userID, needID uuid.UUID, status string) (Need, error) {
	row, err := s.OwnNeed(ctx, userID, needID)
	if err != nil {
		return Need{}, err
	}
	from, to := domain.NeedStatus(row.Status), domain.NeedStatus(status)
	if !from.CanTransitionTo(to) {
		return Need{}, domain.InvalidTransition("need", string(from), string(to))
	}
	if err := s.q.SetNeedStatus(ctx, sqlcgen.SetNeedStatusParams{ID: row.ID, Status: sqlcgen.NeedStatus(to)}); err != nil {
		return Need{}, err
	}
	row.Status = sqlcgen.NeedStatus(to)
	return s.toNeed(ctx, row)
}

// OwnNeed возвращает потребность, если она принадлежит компании пользователя.
func (s *Service) OwnNeed(ctx context.Context, userID, needID uuid.UUID) (*sqlcgen.Need, error) {
	companyID, err := s.CompanyIDFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.q.GetNeed(ctx, needID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.CompanyID != companyID) {
		return nil, domain.NotFound("need")
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetNeed — потребность компании.
func (s *Service) GetNeed(ctx context.Context, userID, needID uuid.UUID) (Need, error) {
	row, err := s.OwnNeed(ctx, userID, needID)
	if err != nil {
		return Need{}, err
	}
	return s.toNeed(ctx, row)
}

// ListNeeds — потребности компании.
func (s *Service) ListNeeds(ctx context.Context, userID uuid.UUID) ([]Need, error) {
	companyID, err := s.CompanyIDFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListNeeds(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]Need, 0, len(rows))
	for _, r := range rows {
		n, err := s.toNeed(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// Extraction — разбор свободного текста потребности (черновик для подтверждения).
type Extraction struct {
	Draft      llm.NeedDraft `json:"draft"`
	MustSkills []SkillRef    `json:"must_skills"`
	NiceSkills []SkillRef    `json:"nice_skills"`
	Provider   string        `json:"provider"`
}

// ExtractNeed разбирает текст через LLM-провайдер (в MVP — эмулятор на правилах).
func (s *Service) ExtractNeed(ctx context.Context, text string) (Extraction, error) {
	text = strings.TrimSpace(text)
	if len([]rune(text)) < 20 {
		return Extraction{}, domain.Validation("text", "text_too_short", "Опишите потребность хотя бы в одном-двух предложениях")
	}
	if len([]rune(text)) > 8000 {
		return Extraction{}, domain.Validation("text", "text_too_long", "Текст слишком длинный (до 8000 символов)")
	}
	draft, usage, err := s.llm.ExtractNeed(ctx, text)
	if err != nil {
		return Extraction{}, err
	}
	out := Extraction{Draft: draft, Provider: usage.Provider, MustSkills: []SkillRef{}, NiceSkills: []SkillRef{}}
	resolve := func(codes []string) []SkillRef {
		if len(codes) == 0 {
			return []SkillRef{}
		}
		rows, err := s.q.GetSkillsByCodes(ctx, codes)
		if err != nil {
			return []SkillRef{}
		}
		refs := make([]SkillRef, 0, len(rows))
		for _, r := range rows {
			refs = append(refs, SkillRef{Code: r.Code, Name: r.Name})
		}
		return refs
	}
	out.MustSkills = resolve(draft.MustSkills)
	out.NiceSkills = resolve(draft.NiceSkills)
	return out, nil
}

func (s *Service) toNeed(ctx context.Context, r *sqlcgen.Need) (Need, error) {
	n := Need{
		ID: r.ID, Title: r.Title, Specialization: string(r.Specialization), GradeMin: string(r.GradeMin), GradeMax: string(r.GradeMax),
		SalaryFrom: int64(r.SalaryFrom), SalaryTo: int64(r.SalaryTo), RemoteOK: r.RemoteOk, Status: string(r.Status),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, MustSkills: []SkillRef{}, NiceSkills: []SkillRef{}, WorkFormats: []string{},
	}
	if r.Description != nil {
		n.Description = *r.Description
	}
	if r.TeamDescription != nil {
		n.TeamDescription = *r.TeamDescription
	}
	if r.City != nil {
		n.City = *r.City
	}
	for _, f := range r.WorkFormats {
		n.WorkFormats = append(n.WorkFormats, string(f))
	}
	ids := append(append([]int32{}, r.MustSkillIds...), r.NiceSkillIds...)
	if len(ids) > 0 {
		rows, err := s.q.GetSkillsByIDs(ctx, ids)
		if err != nil {
			return Need{}, err
		}
		byID := map[int32]SkillRef{}
		for _, sk := range rows {
			byID[sk.ID] = SkillRef{Code: sk.Code, Name: sk.Name}
		}
		for _, id := range r.MustSkillIds {
			if ref, ok := byID[id]; ok {
				n.MustSkills = append(n.MustSkills, ref)
			}
		}
		for _, id := range r.NiceSkillIds {
			if ref, ok := byID[id]; ok {
				n.NiceSkills = append(n.NiceSkills, ref)
			}
		}
	}
	return n, nil
}

func toCompany(r *sqlcgen.Company) Company {
	c := Company{ID: r.ID, Name: r.Name, PublicContacts: map[string]string{}, CreatedAt: r.CreatedAt}
	_ = json.Unmarshal(r.PublicContacts, &c.PublicContacts)
	if r.Inn != nil {
		c.INN = *r.Inn
	}
	if r.Description != nil {
		c.Description = *r.Description
	}
	if r.Industry != nil {
		c.Industry = *r.Industry
	}
	if r.Website != nil {
		c.Website = *r.Website
	}
	if r.City != nil {
		c.City = *r.City
	}
	return c
}

func opt(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// SkillRefsByIDs возвращает код и имя навыков по идентификаторам (неизвестные пропускаются).
func SkillRefsByIDs(ctx context.Context, q *sqlcgen.Queries, ids []int32) (map[int32]SkillRef, error) {
	out := map[int32]SkillRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.GetSkillsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, sk := range rows {
		out[sk.ID] = SkillRef{Code: sk.Code, Name: sk.Name}
	}
	return out, nil
}
