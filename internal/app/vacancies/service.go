// Package vacancies — классический путь: вакансия с обязательной вилкой, публичный список, подборка под вакансию.
package vacancies

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/app/employers"
	"fsp/internal/app/matching"
	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Service — сценарии вакансий.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
	log  *slog.Logger
}

// New создаёт сервис.
func New(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool), log: log}
}

// Input — поля вакансии при создании и обновлении.
type Input struct {
	Title          string
	Description    string
	Specialization string
	GradeMin       string
	GradeMax       string
	MustSkills     []string
	NiceSkills     []string
	SalaryFrom     int64
	SalaryTo       int64
	WorkFormats    []string
	City           string
	RemoteOK       bool
	NeedID         *uuid.UUID
}

// ApplicationCounts — сколько откликов у вакансии.
type ApplicationCounts struct {
	Total int `json:"total"`
	New   int `json:"new" doc:"Непросмотренные"`
}

// Vacancy — вакансия для работодателя.
type Vacancy struct {
	ID             uuid.UUID            `json:"id"`
	CompanyID      uuid.UUID            `json:"company_id"`
	NeedID         *uuid.UUID           `json:"need_id,omitempty"`
	Title          string               `json:"title"`
	Description    string               `json:"description"`
	Specialization string               `json:"specialization"`
	GradeMin       string               `json:"grade_min"`
	GradeMax       string               `json:"grade_max"`
	MustSkills     []employers.SkillRef `json:"must_skills"`
	NiceSkills     []employers.SkillRef `json:"nice_skills"`
	SalaryFrom     int64                `json:"salary_from"`
	SalaryTo       int64                `json:"salary_to"`
	WorkFormats    []string             `json:"work_formats"`
	City           string               `json:"city,omitempty"`
	RemoteOK       bool                 `json:"remote_ok"`
	Status         string               `json:"status" enum:"draft,published,closed"`
	PublishedAt    *time.Time           `json:"published_at,omitempty"`
	ClosedAt       *time.Time           `json:"closed_at,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	Applications   *ApplicationCounts   `json:"applications,omitempty"`
}

// CompanyBrief — компания в публичной вакансии.
type CompanyBrief struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Industry    string    `json:"industry,omitempty"`
	City        string    `json:"city,omitempty"`
	Description string    `json:"description,omitempty"`
	Website     string    `json:"website,omitempty"`
}

// PublicVacancy — вакансия в публичном списке/карточке.
type PublicVacancy struct {
	Vacancy
	Company CompanyBrief `json:"company"`
}

// PublicFilter — фильтры публичного списка.
type PublicFilter struct {
	Specialization string
	Grade          string
	WorkFormat     string
	City           string
	SalaryMin      int64
	Query          string
	Limit          int
	Offset         int
}

// PublicList — страница публичного списка.
type PublicList struct {
	Items  []PublicVacancy `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

func (s *Service) validate(ctx context.Context, in *Input) ([]int32, []int32, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if in.Title == "" || len([]rune(in.Title)) > 160 {
		return nil, nil, domain.Validation("title", "title_invalid", "Укажите название вакансии (до 160 символов)")
	}
	if len([]rune(in.Description)) < 20 || len([]rune(in.Description)) > 20000 {
		return nil, nil, domain.Validation("description", "description_invalid", "Опишите вакансию (от 20 до 20 000 символов)")
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
	must, err := employers.ResolveSkillCodes(ctx, s.q, in.MustSkills, "must_skills")
	if err != nil {
		return nil, nil, err
	}
	nice, err := employers.ResolveSkillCodes(ctx, s.q, in.NiceSkills, "nice_skills")
	if err != nil {
		return nil, nil, err
	}
	return must, nice, nil
}

func formats(in []string) []sqlcgen.WorkFormat {
	out := make([]sqlcgen.WorkFormat, 0, len(in))
	for _, f := range in {
		out = append(out, sqlcgen.WorkFormat(f))
	}
	return out
}

func (s *Service) needRef(ctx context.Context, companyID uuid.UUID, needID *uuid.UUID) (uuid.NullUUID, error) {
	if needID == nil {
		return uuid.NullUUID{}, nil
	}
	n, err := s.q.GetNeed(ctx, *needID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && n.CompanyID != companyID) {
		return uuid.NullUUID{}, domain.NotFound("need")
	}
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: n.ID, Valid: true}, nil
}

// Create создаёт черновик вакансии.
func (s *Service) Create(ctx context.Context, userID, companyID uuid.UUID, in Input) (Vacancy, error) {
	must, nice, err := s.validate(ctx, &in)
	if err != nil {
		return Vacancy{}, err
	}
	needID, err := s.needRef(ctx, companyID, in.NeedID)
	if err != nil {
		return Vacancy{}, err
	}
	row, err := s.q.CreateVacancy(ctx, sqlcgen.CreateVacancyParams{
		CompanyID: companyID, CreatedBy: userID, NeedID: needID, Title: in.Title, Description: in.Description,
		Specialization: sqlcgen.SpecializationCode(in.Specialization), GradeMin: sqlcgen.GradeCode(in.GradeMin), GradeMax: sqlcgen.GradeCode(in.GradeMax),
		MustSkillIds: must, NiceSkillIds: nice, SalaryFrom: int32(in.SalaryFrom), SalaryTo: int32(in.SalaryTo), //nolint:gosec // ≤ MaxSalary
		WorkFormats: formats(in.WorkFormats), City: opt(in.City), RemoteOk: in.RemoteOK,
	})
	if err != nil {
		return Vacancy{}, err
	}
	return s.toVacancy(ctx, row)
}

// Update обновляет поля вакансии (статус не меняет).
func (s *Service) Update(ctx context.Context, companyID, id uuid.UUID, in Input) (Vacancy, error) {
	row, err := s.Own(ctx, companyID, id)
	if err != nil {
		return Vacancy{}, err
	}
	must, nice, err := s.validate(ctx, &in)
	if err != nil {
		return Vacancy{}, err
	}
	needID, err := s.needRef(ctx, companyID, in.NeedID)
	if err != nil {
		return Vacancy{}, err
	}
	updated, err := s.q.UpdateVacancy(ctx, sqlcgen.UpdateVacancyParams{
		ID: row.ID, Title: in.Title, Description: in.Description,
		Specialization: sqlcgen.SpecializationCode(in.Specialization), GradeMin: sqlcgen.GradeCode(in.GradeMin), GradeMax: sqlcgen.GradeCode(in.GradeMax),
		MustSkillIds: must, NiceSkillIds: nice, SalaryFrom: int32(in.SalaryFrom), SalaryTo: int32(in.SalaryTo), //nolint:gosec // ≤ MaxSalary
		WorkFormats: formats(in.WorkFormats), City: opt(in.City), RemoteOk: in.RemoteOK, NeedID: needID,
	})
	if err != nil {
		return Vacancy{}, err
	}
	return s.toVacancy(ctx, updated)
}

// SetStatus — публикация, возврат в черновик, закрытие (по допустимым переходам).
func (s *Service) SetStatus(ctx context.Context, companyID, id uuid.UUID, status string) (Vacancy, error) {
	row, err := s.Own(ctx, companyID, id)
	if err != nil {
		return Vacancy{}, err
	}
	from, to := domain.VacancyStatus(row.Status), domain.VacancyStatus(status)
	if !from.CanTransitionTo(to) {
		return Vacancy{}, domain.InvalidTransition("vacancy", string(from), string(to))
	}
	if err := s.q.SetVacancyStatus(ctx, sqlcgen.SetVacancyStatusParams{ID: row.ID, Status: sqlcgen.VacancyStatus(to)}); err != nil {
		return Vacancy{}, err
	}
	row, err = s.q.GetVacancy(ctx, row.ID)
	if err != nil {
		return Vacancy{}, err
	}
	return s.toVacancy(ctx, row)
}

// Own возвращает вакансию, если она принадлежит компании.
func (s *Service) Own(ctx context.Context, companyID, id uuid.UUID) (*sqlcgen.Vacancy, error) {
	row, err := s.q.GetVacancy(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.CompanyID != companyID) {
		return nil, domain.NotFound("vacancy")
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Get — вакансия компании со счётчиками откликов.
func (s *Service) Get(ctx context.Context, companyID, id uuid.UUID) (Vacancy, error) {
	row, err := s.Own(ctx, companyID, id)
	if err != nil {
		return Vacancy{}, err
	}
	v, err := s.toVacancy(ctx, row)
	if err != nil {
		return Vacancy{}, err
	}
	counts, err := s.counts(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return Vacancy{}, err
	}
	c := counts[row.ID]
	v.Applications = &c
	return v, nil
}

// ListForCompany — вакансии компании со счётчиками откликов.
func (s *Service) ListForCompany(ctx context.Context, companyID uuid.UUID) ([]Vacancy, error) {
	rows, err := s.q.ListVacanciesByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	counts, err := s.counts(ctx, ids)
	if err != nil {
		return nil, err
	}
	refs, err := s.skillRefs(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]Vacancy, 0, len(rows))
	for _, r := range rows {
		v := vacancyFrom(r, refs)
		c := counts[r.ID]
		v.Applications = &c
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) counts(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ApplicationCounts, error) {
	out := map[uuid.UUID]ApplicationCounts{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.CountApplicationsForVacancies(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.VacancyID] = ApplicationCounts{Total: int(r.Total), New: int(r.NewCount)}
	}
	return out, nil
}

// ListPublic — опубликованные вакансии с фильтрами и пагинацией (без аутентификации).
func (s *Service) ListPublic(ctx context.Context, f PublicFilter) (PublicList, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	p := sqlcgen.ListPublicVacanciesParams{Lim: int32(f.Limit), Off: int32(f.Offset)} //nolint:gosec // ограничено выше
	if f.Specialization != "" {
		if !refdata.IsSelectableSpecialization(f.Specialization) {
			return PublicList{}, domain.Validation("specialization", "specialization_invalid", "Неизвестная специализация")
		}
		v := sqlcgen.SpecializationCode(f.Specialization)
		p.Specialization = &v
	}
	if f.Grade != "" {
		if !refdata.IsGrade(f.Grade) {
			return PublicList{}, domain.Validation("grade", "grade_invalid", "Неизвестный грейд")
		}
		v := sqlcgen.GradeCode(f.Grade)
		p.Grade = &v
	}
	if f.WorkFormat != "" {
		if !refdata.IsWorkFormat(f.WorkFormat) {
			return PublicList{}, domain.Validation("work_format", "work_format_invalid", "Неизвестный формат")
		}
		v := sqlcgen.WorkFormat(f.WorkFormat)
		p.WorkFormat = &v
	}
	if c := strings.TrimSpace(f.City); c != "" {
		p.City = &c
	}
	if f.SalaryMin > 0 {
		v := int32(min(f.SalaryMin, domain.MaxSalary)) //nolint:gosec // ≤ MaxSalary
		p.SalaryMin = &v
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		if len([]rune(q)) > 100 {
			q = string([]rune(q)[:100])
		}
		p.Q = &q
	}
	rows, err := s.q.ListPublicVacancies(ctx, p)
	if err != nil {
		return PublicList{}, err
	}
	out := PublicList{Items: []PublicVacancy{}, Limit: f.Limit, Offset: f.Offset}
	vacs := make([]*sqlcgen.Vacancy, 0, len(rows))
	for _, r := range rows {
		vacs = append(vacs, &sqlcgen.Vacancy{
			ID: r.ID, CompanyID: r.CompanyID, CreatedBy: r.CreatedBy, NeedID: r.NeedID, Title: r.Title, Description: r.Description,
			Specialization: r.Specialization, GradeMin: r.GradeMin, GradeMax: r.GradeMax, MustSkillIds: r.MustSkillIds, NiceSkillIds: r.NiceSkillIds,
			SalaryFrom: r.SalaryFrom, SalaryTo: r.SalaryTo, WorkFormats: r.WorkFormats, City: r.City, RemoteOk: r.RemoteOk, Status: r.Status,
			PublishedAt: r.PublishedAt, ClosedAt: r.ClosedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	// Навыки всех вакансий страницы — одним запросом, а не по запросу на вакансию.
	refs, err := s.skillRefs(ctx, vacs)
	if err != nil {
		return PublicList{}, err
	}
	for i, r := range rows {
		out.Total = int(r.Total)
		v := vacancyFrom(vacs[i], refs)
		cb := CompanyBrief{ID: r.CompanyID, Name: r.CompanyName}
		if r.CompanyIndustry != nil {
			cb.Industry = *r.CompanyIndustry
		}
		if r.CompanyCity != nil {
			cb.City = *r.CompanyCity
		}
		out.Items = append(out.Items, PublicVacancy{Vacancy: v, Company: cb})
	}
	return out, nil
}

// GetPublic — опубликованная вакансия с данными компании.
func (s *Service) GetPublic(ctx context.Context, id uuid.UUID) (PublicVacancy, error) {
	r, err := s.q.GetPublicVacancy(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicVacancy{}, domain.NotFound("vacancy")
	}
	if err != nil {
		return PublicVacancy{}, err
	}
	v, err := s.toVacancy(ctx, &sqlcgen.Vacancy{
		ID: r.ID, CompanyID: r.CompanyID, CreatedBy: r.CreatedBy, NeedID: r.NeedID, Title: r.Title, Description: r.Description,
		Specialization: r.Specialization, GradeMin: r.GradeMin, GradeMax: r.GradeMax, MustSkillIds: r.MustSkillIds, NiceSkillIds: r.NiceSkillIds,
		SalaryFrom: r.SalaryFrom, SalaryTo: r.SalaryTo, WorkFormats: r.WorkFormats, City: r.City, RemoteOk: r.RemoteOk, Status: r.Status,
		PublishedAt: r.PublishedAt, ClosedAt: r.ClosedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
	if err != nil {
		return PublicVacancy{}, err
	}
	cb := CompanyBrief{ID: r.CompanyID, Name: r.CompanyName}
	if r.CompanyIndustry != nil {
		cb.Industry = *r.CompanyIndustry
	}
	if r.CompanyCity != nil {
		cb.City = *r.CompanyCity
	}
	if r.CompanyDescription != nil {
		cb.Description = *r.CompanyDescription
	}
	if r.CompanyWebsite != nil {
		cb.Website = *r.CompanyWebsite
	}
	return PublicVacancy{Vacancy: v, Company: cb}, nil
}

// NeedFor переводит вакансию в потребность для подбора.
func NeedFor(v *sqlcgen.Vacancy) matching.Need {
	fs := make([]string, 0, len(v.WorkFormats))
	for _, f := range v.WorkFormats {
		fs = append(fs, string(f))
	}
	city := ""
	if v.City != nil {
		city = *v.City
	}
	return matching.Need{
		CompanyID: v.CompanyID, Specialization: string(v.Specialization), GradeMin: string(v.GradeMin), GradeMax: string(v.GradeMax),
		MustSkills: v.MustSkillIds, NiceSkills: v.NiceSkillIds, Salary: domain.SalaryRange{From: int64(v.SalaryFrom), To: int64(v.SalaryTo)},
		WorkFormats: fs, City: city, RemoteOK: v.RemoteOk,
	}
}

func (s *Service) toVacancy(ctx context.Context, r *sqlcgen.Vacancy) (Vacancy, error) {
	refs, err := s.skillRefs(ctx, []*sqlcgen.Vacancy{r})
	if err != nil {
		return Vacancy{}, err
	}
	return vacancyFrom(r, refs), nil
}

// skillRefs загружает навыки всех переданных вакансий одним запросом.
func (s *Service) skillRefs(ctx context.Context, rows []*sqlcgen.Vacancy) (map[int32]employers.SkillRef, error) {
	var ids []int32
	for _, r := range rows {
		ids = append(ids, r.MustSkillIds...)
		ids = append(ids, r.NiceSkillIds...)
	}
	return employers.SkillRefsByIDs(ctx, s.q, ids)
}

// vacancyFrom собирает вакансию из строки базы и заранее загруженных навыков.
func vacancyFrom(r *sqlcgen.Vacancy, refs map[int32]employers.SkillRef) Vacancy {
	v := Vacancy{
		ID: r.ID, CompanyID: r.CompanyID, Title: r.Title, Description: r.Description, Specialization: string(r.Specialization),
		GradeMin: string(r.GradeMin), GradeMax: string(r.GradeMax), SalaryFrom: int64(r.SalaryFrom), SalaryTo: int64(r.SalaryTo),
		RemoteOK: r.RemoteOk, Status: string(r.Status), PublishedAt: r.PublishedAt, ClosedAt: r.ClosedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		MustSkills: []employers.SkillRef{}, NiceSkills: []employers.SkillRef{}, WorkFormats: []string{},
	}
	if r.NeedID.Valid {
		id := r.NeedID.UUID
		v.NeedID = &id
	}
	if r.City != nil {
		v.City = *r.City
	}
	for _, f := range r.WorkFormats {
		v.WorkFormats = append(v.WorkFormats, string(f))
	}
	for _, id := range r.MustSkillIds {
		if ref, ok := refs[id]; ok {
			v.MustSkills = append(v.MustSkills, ref)
		}
	}
	for _, id := range r.NiceSkillIds {
		if ref, ok := refs[id]; ok {
			v.NiceSkills = append(v.NiceSkills, ref)
		}
	}
	return v
}

func opt(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
