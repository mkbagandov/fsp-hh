// Package admin — наблюдаемость и администрирование: метрики платформы, пользователи,
// статистика банка заданий, подозрительные сессии, работодатели, модерация вакансий, аудит.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/domain"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Service — сценарии администратора.
type Service struct {
	q   *sqlcgen.Queries
	log *slog.Logger
}

// New создаёт сервис.
func New(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{q: sqlcgen.New(pool), log: log}
}

// PromoteAdmins выдаёт роль admin пользователям с указанными e-mail (при старте API из ADMIN_EMAILS).
func (s *Service) PromoteAdmins(ctx context.Context, emails []string) (int, error) {
	clean := make([]string, 0, len(emails))
	for _, e := range emails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			clean = append(clean, e)
		}
	}
	if len(clean) == 0 {
		return 0, nil
	}
	rows, err := s.q.AdminPromote(ctx, clean)
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		s.log.Info("admin role granted", "email", r.Email)
	}
	return len(rows), nil
}

// Count — пара «ключ → число» для группировок по статусам.
type Count struct {
	Key   string `json:"key"`
	Total int64  `json:"total"`
}

// CategoryCount — число кандидатов в категории по статусу.
type CategoryCount struct {
	Specialization string `json:"specialization"`
	Grade          string `json:"grade"`
	Status         string `json:"status"`
	Total          int64  `json:"total"`
}

// SessionStat — статистика тест-сессий по статусу.
type SessionStat struct {
	Status     string  `json:"status"`
	Total      int64   `json:"total"`
	AvgItems   float64 `json:"avg_items"`
	AvgSeconds float64 `json:"avg_seconds"`
	Flagged    int64   `json:"flagged" doc:"Сессии с телеметрическими флагами"`
}

// Funnel — воронка кандидата и свежая активность.
type Funnel struct {
	Candidates    int64 `json:"candidates"`
	Surveyed      int64 `json:"surveyed"`
	Tested        int64 `json:"tested"`
	Confirmed     int64 `json:"confirmed"`
	Invited       int64 `json:"invited"`
	Accepted      int64 `json:"accepted"`
	Companies     int64 `json:"companies"`
	NewUsers7d    int64 `json:"new_users_7d"`
	Sessions7d    int64 `json:"sessions_7d"`
	Invitations7d int64 `json:"invitations_7d"`
}

// Day — активность за день.
type Day struct {
	Day           string `json:"day"`
	Registrations int64  `json:"registrations"`
	Sessions      int64  `json:"sessions"`
	Invitations   int64  `json:"invitations"`
	Applications  int64  `json:"applications"`
}

// Overview — сводка для дашборда.
type Overview struct {
	Users        []Count         `json:"users" doc:"role:status → total"`
	Categories   []CategoryCount `json:"categories"`
	Sessions     []SessionStat   `json:"sessions"`
	Invitations  []Count         `json:"invitations"`
	Applications []Count         `json:"applications"`
	Vacancies    []Count         `json:"vacancies"`
	Needs        []Count         `json:"needs"`
	Funnel       Funnel          `json:"funnel"`
	Daily        []Day           `json:"daily"`
	Families     int             `json:"families"`
	GeneratedAt  time.Time       `json:"generated_at"`
}

// Overview собирает все агрегаты одним запросом к API.
func (s *Service) Overview(ctx context.Context, days int) (Overview, error) {
	if days <= 0 || days > 90 {
		days = 14
	}
	out := Overview{GeneratedAt: time.Now(), Users: []Count{}, Categories: []CategoryCount{}, Sessions: []SessionStat{}, Invitations: []Count{}, Applications: []Count{}, Vacancies: []Count{}, Needs: []Count{}, Daily: []Day{}}
	users, err := s.q.AdminUserCounts(ctx)
	if err != nil {
		return out, err
	}
	for _, u := range users {
		out.Users = append(out.Users, Count{Key: string(u.Role) + ":" + string(u.Status), Total: u.Total})
	}
	cats, err := s.q.AdminCategoryCounts(ctx)
	if err != nil {
		return out, err
	}
	for _, c := range cats {
		out.Categories = append(out.Categories, CategoryCount{Specialization: string(c.Specialization), Grade: string(c.Grade), Status: string(c.Status), Total: c.Total})
	}
	sess, err := s.q.AdminSessionStats(ctx)
	if err != nil {
		return out, err
	}
	for _, x := range sess {
		out.Sessions = append(out.Sessions, SessionStat{Status: string(x.Status), Total: x.Total, AvgItems: x.AvgItems, AvgSeconds: x.AvgSeconds, Flagged: x.Flagged})
	}
	inv, err := s.q.AdminInvitationCounts(ctx)
	if err != nil {
		return out, err
	}
	for _, x := range inv {
		out.Invitations = append(out.Invitations, Count{Key: string(x.Status), Total: x.Total})
	}
	apps, err := s.q.AdminApplicationCounts(ctx)
	if err != nil {
		return out, err
	}
	for _, x := range apps {
		out.Applications = append(out.Applications, Count{Key: string(x.Status), Total: x.Total})
	}
	vac, err := s.q.AdminVacancyCounts(ctx)
	if err != nil {
		return out, err
	}
	for _, x := range vac {
		out.Vacancies = append(out.Vacancies, Count{Key: string(x.Status), Total: x.Total})
	}
	needs, err := s.q.AdminNeedCounts(ctx)
	if err != nil {
		return out, err
	}
	for _, x := range needs {
		out.Needs = append(out.Needs, Count{Key: string(x.Status), Total: x.Total})
	}
	f, err := s.q.AdminFunnel(ctx)
	if err != nil {
		return out, err
	}
	out.Funnel = Funnel{Candidates: f.Candidates, Surveyed: f.Surveyed, Tested: f.Tested, Confirmed: f.Confirmed, Invited: f.Invited, Accepted: f.Accepted, Companies: f.Companies, NewUsers7d: f.NewUsers7d, Sessions7d: f.Sessions7d, Invitations7d: f.Invitations7d}
	daily, err := s.q.AdminDailyActivity(ctx, int32(days)) //nolint:gosec // ≤ 90
	if err != nil {
		return out, err
	}
	for _, d := range daily {
		out.Daily = append(out.Daily, Day{Day: d.Day.Format("2006-01-02"), Registrations: d.Registrations, Sessions: d.Sessions, Invitations: d.Invitations, Applications: d.Applications})
	}
	fams, err := s.q.AdminItemFamilyStats(ctx)
	if err != nil {
		return out, err
	}
	out.Families = len(fams)
	return out, nil
}

// User — пользователь в списке админа.
type User struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
	EmailVerified bool       `json:"email_verified"`
}

// UserList — страница пользователей.
type UserList struct {
	Items []User `json:"items"`
	Total int    `json:"total"`
}

// UserFilter — фильтры списка пользователей.
type UserFilter struct {
	Query  string
	Role   string
	Status string
	Limit  int
	Offset int
}

// Users — список пользователей с поиском по e-mail.
func (s *Service) Users(ctx context.Context, f UserFilter) (UserList, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	p := sqlcgen.AdminListUsersParams{Lim: int32(f.Limit), Off: int32(max(0, f.Offset))} //nolint:gosec // ограничено
	if q := strings.TrimSpace(f.Query); q != "" {
		p.Q = &q
	}
	if f.Role != "" {
		r := sqlcgen.UserRole(f.Role)
		p.Role = &r
	}
	if f.Status != "" {
		st := sqlcgen.UserStatus(f.Status)
		p.Status = &st
	}
	rows, err := s.q.AdminListUsers(ctx, p)
	if err != nil {
		return UserList{}, err
	}
	out := UserList{Items: []User{}}
	for _, r := range rows {
		out.Total = int(r.Total)
		out.Items = append(out.Items, User{ID: r.ID, Email: r.Email, Role: string(r.Role), Status: string(r.Status), CreatedAt: r.CreatedAt, LastLoginAt: r.LastLoginAt, EmailVerified: r.EmailVerifiedAt != nil})
	}
	return out, nil
}

// SetUserStatus блокирует или разблокирует пользователя (не себя и не других админов).
func (s *Service) SetUserStatus(ctx context.Context, adminID, userID uuid.UUID, status string) (User, error) {
	if status != "active" && status != "blocked" {
		return User{}, domain.Validation("status", "status_invalid", "Допустимы active и blocked")
	}
	if adminID == userID {
		return User{}, domain.Conflict("self_block", "Нельзя менять статус собственной учётной записи")
	}
	u, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, domain.NotFound("user")
	}
	if err != nil {
		return User{}, err
	}
	if u.Role == sqlcgen.UserRoleAdmin {
		return User{}, domain.Forbidden("admin_protected", "Статус администратора меняется только в базе данных")
	}
	if u.Status == sqlcgen.UserStatusDeleted {
		return User{}, domain.Conflict("user_deleted", "Учётная запись удалена")
	}
	if err := s.q.UpdateUserStatus(ctx, sqlcgen.UpdateUserStatusParams{ID: userID, Status: sqlcgen.UserStatus(status)}); err != nil {
		return User{}, err
	}
	_ = s.q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: adminID, Valid: true}, Action: "admin.user." + status, ObjectType: "user", ObjectID: uuid.NullUUID{UUID: userID, Valid: true}, Details: json.RawMessage(`{}`)})
	u.Status = sqlcgen.UserStatus(status)
	return User{ID: u.ID, Email: u.Email, Role: string(u.Role), Status: string(u.Status), CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt, EmailVerified: u.EmailVerifiedAt != nil}, nil
}

// FamilyStat — статистика семейства заданий.
type FamilyStat struct {
	ID             string  `json:"id"`
	Specialization string  `json:"specialization"`
	Topic          string  `json:"topic"`
	Grade          string  `json:"grade"`
	Format         string  `json:"format"`
	A              float64 `json:"a"`
	B              float64 `json:"b"`
	BPrior         float64 `json:"b_prior"`
	C              float64 `json:"c"`
	Enabled        bool    `json:"enabled"`
	Served         int64   `json:"served"`
	Answered       int64   `json:"answered"`
	Correct        int64   `json:"correct"`
	CorrectShare   float64 `json:"correct_share"`
	AvgTimeSec     float64 `json:"avg_time_sec"`
	Discrimination float64 `json:"discrimination" doc:"Разница средней θ до ответа у ответивших верно и неверно; выше — лучше различает"`
	Exposure       float64 `json:"exposure" doc:"Доля сессий специализации, где выдавалось семейство"`
	Warning        string  `json:"warning,omitempty"`
}

// ItemStats — статистика банка: семейства с предупреждениями о слабых.
func (s *Service) ItemStats(ctx context.Context) ([]FamilyStat, error) {
	rows, err := s.q.AdminItemFamilyStats(ctx)
	if err != nil {
		return nil, err
	}
	bySpec, err := s.q.AdminSessionsBySpec(ctx)
	if err != nil {
		return nil, err
	}
	sessBySpec := map[string]int64{}
	var totalSessions int64
	for _, x := range bySpec {
		sessBySpec[string(x.Specialization)] = x.Total
		totalSessions += x.Total
	}
	// Знаменатель экспозиции — сессии той же специализации (общие семейства видят все сессии).
	denom := func(spec string) int64 {
		if spec == "common" {
			return totalSessions
		}
		return sessBySpec[spec]
	}
	// Средняя экспозиция по пулу специализации: при небольшом банке даже честный CAT выдаёт каждое семейство
	// в большой доле сессий, поэтому предупреждение — относительное (в 2 раза выше среднего по пулу).
	sumExp, cntExp := map[string]float64{}, map[string]int{}
	for _, r := range rows {
		if d := denom(string(r.Spec)); d > 0 {
			sumExp[string(r.Spec)] += float64(r.Served) / float64(d)
			cntExp[string(r.Spec)]++
		}
	}
	out := make([]FamilyStat, 0, len(rows))
	for _, r := range rows {
		fs := FamilyStat{
			ID: r.ID, Specialization: string(r.Spec), Topic: r.Topic, Grade: string(r.Grade), Format: r.Format,
			A: r.A, B: r.B, BPrior: r.BPrior, C: r.C, Enabled: r.Enabled, Served: r.Served, Answered: r.Answered, Correct: r.Correct,
			AvgTimeSec: r.AvgTimeMs / 1000,
		}
		if r.Answered > 0 {
			fs.CorrectShare = float64(r.Correct) / float64(r.Answered)
		}
		if r.HasBoth != nil && *r.HasBoth {
			fs.Discrimination = r.ThetaCorrect - r.ThetaIncorrect
		}
		if d := denom(string(r.Spec)); d > 0 {
			fs.Exposure = float64(r.Served) / float64(d)
		}
		avgExp := 0.0
		if cntExp[string(r.Spec)] > 0 {
			avgExp = sumExp[string(r.Spec)] / float64(cntExp[string(r.Spec)])
		}
		switch {
		case r.Answered >= 20 && fs.CorrectShare < 0.1:
			fs.Warning = "почти никто не отвечает верно — проверить формулировку или ответ"
		case r.Answered >= 20 && fs.CorrectShare > 0.95:
			fs.Warning = "слишком простое для своего грейда"
		case r.Answered >= 30 && r.HasBoth != nil && *r.HasBoth && fs.Discrimination < 0:
			fs.Warning = "отрицательная дискриминативность — верно отвечают более слабые"
		case totalSessions >= 50 && fs.Exposure >= 0.5 && fs.Exposure > 2*avgExp:
			fs.Warning = fmt.Sprintf("экспозиция %d %% — вдвое выше средней по специализации, риск утечки", int(fs.Exposure*100))
		}
		out = append(out, fs)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Warning != "") != (out[j].Warning != "") {
			return out[i].Warning != ""
		}
		return out[i].Served > out[j].Served
	})
	return out, nil
}

// SetFamilyEnabled включает или выключает семейство в банке.
func (s *Service) SetFamilyEnabled(ctx context.Context, adminID uuid.UUID, id string, enabled bool) error {
	if err := s.q.AdminSetFamilyEnabled(ctx, sqlcgen.AdminSetFamilyEnabledParams{ID: id, Enabled: enabled}); err != nil {
		return err
	}
	return s.q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: adminID, Valid: true}, Action: "admin.family.enabled", ObjectType: "item_family", Details: json.RawMessage(fmt.Sprintf(`{"id":%q,"enabled":%t}`, id, enabled))})
}

// FlaggedSession — сессия с телеметрическими флагами.
type FlaggedSession struct {
	ID             uuid.UUID      `json:"id"`
	UserID         uuid.UUID      `json:"user_id"`
	Email          string         `json:"email"`
	Specialization string         `json:"specialization"`
	TargetGrade    string         `json:"target_grade"`
	Status         string         `json:"status"`
	ItemsCount     int32          `json:"items_count"`
	CorrectCount   int32          `json:"correct_count"`
	Flags          map[string]int `json:"flags"`
	Decision       string         `json:"decision,omitempty"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	Suspicious     bool           `json:"suspicious" doc:"Много потерь фокуса или вставок при высокой доле верных"`
}

// FlaggedSessions — сессии с флагами телеметрии, подозрительные первыми.
func (s *Service) FlaggedSessions(ctx context.Context, limit int) ([]FlaggedSession, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.q.AdminFlaggedSessions(ctx, int32(limit)) //nolint:gosec // ≤ 500
	if err != nil {
		return nil, err
	}
	out := make([]FlaggedSession, 0, len(rows))
	for _, r := range rows {
		fs := FlaggedSession{ID: r.ID, UserID: r.UserID, Email: r.Email, Specialization: string(r.Specialization), TargetGrade: string(r.TargetGrade), Status: string(r.Status), ItemsCount: r.ItemsCount, CorrectCount: r.CorrectCount, Flags: map[string]int{}, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt}
		_ = json.Unmarshal(r.Flags, &fs.Flags)
		if r.Decision != nil {
			fs.Decision = string(*r.Decision)
		}
		events := fs.Flags["blur"] + fs.Flags["hidden"] + fs.Flags["paste"]*2 + fs.Flags["devtools"]*3
		share := 0.0
		if r.ItemsCount > 0 {
			share = float64(r.CorrectCount) / float64(r.ItemsCount)
		}
		fs.Suspicious = events >= 5 && share >= 0.8
		out = append(out, fs)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Suspicious && !out[j].Suspicious })
	return out, nil
}

// Employer — компания с репутацией.
type Employer struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Industry       string    `json:"industry,omitempty"`
	City           string    `json:"city,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	Users          int64     `json:"users"`
	Needs          int64     `json:"needs"`
	Vacancies      int64     `json:"vacancies"`
	Invitations    int64     `json:"invitations"`
	Accepted       int64     `json:"accepted"`
	Declined       int64     `json:"declined"`
	Withdrawn      int64     `json:"withdrawn"`
	AcceptanceRate float64   `json:"acceptance_rate"`
	Reputation     string    `json:"reputation" enum:"new,good,normal,low"`
}

// Employers — компании с показателями отклика на их приглашения.
func (s *Service) Employers(ctx context.Context) ([]Employer, error) {
	rows, err := s.q.AdminEmployers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Employer, 0, len(rows))
	for _, r := range rows {
		e := Employer{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt, Users: r.Users, Needs: r.Needs, Vacancies: r.Vacancies, Invitations: r.Invitations, Accepted: r.Accepted, Declined: r.Declined, Withdrawn: r.Withdrawn, Reputation: "new"}
		if r.Industry != nil {
			e.Industry = *r.Industry
		}
		if r.City != nil {
			e.City = *r.City
		}
		if answered := r.Accepted + r.Declined; answered > 0 {
			e.AcceptanceRate = float64(r.Accepted) / float64(answered)
			switch {
			case answered < 3:
				e.Reputation = "new"
			case e.AcceptanceRate >= 0.5:
				e.Reputation = "good"
			case e.AcceptanceRate >= 0.25:
				e.Reputation = "normal"
			default:
				e.Reputation = "low"
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// VacancyRow — вакансия для модерации.
type VacancyRow struct {
	ID             uuid.UUID  `json:"id"`
	CompanyID      uuid.UUID  `json:"company_id"`
	CompanyName    string     `json:"company_name"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	Specialization string     `json:"specialization"`
	GradeMin       string     `json:"grade_min"`
	GradeMax       string     `json:"grade_max"`
	SalaryFrom     int64      `json:"salary_from"`
	SalaryTo       int64      `json:"salary_to"`
	Applications   int64      `json:"applications"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Vacancies — все вакансии платформы.
func (s *Service) Vacancies(ctx context.Context) ([]VacancyRow, error) {
	rows, err := s.q.AdminVacancies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]VacancyRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, VacancyRow{ID: r.ID, CompanyID: r.CompanyID, CompanyName: r.CompanyName, Title: r.Title, Status: string(r.Status), Specialization: string(r.Specialization), GradeMin: string(r.GradeMin), GradeMax: string(r.GradeMax), SalaryFrom: int64(r.SalaryFrom), SalaryTo: int64(r.SalaryTo), Applications: r.Applications, PublishedAt: r.PublishedAt, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// HideVacancy снимает вакансию с публикации (модерация): статус closed, запись в аудит с причиной.
func (s *Service) HideVacancy(ctx context.Context, adminID, id uuid.UUID, reason string) error {
	v, err := s.q.GetVacancy(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound("vacancy")
	}
	if err != nil {
		return err
	}
	if v.Status != sqlcgen.VacancyStatusPublished {
		return domain.InvalidTransition("vacancy", string(v.Status), "closed")
	}
	if err := s.q.SetVacancyStatus(ctx, sqlcgen.SetVacancyStatusParams{ID: id, Status: sqlcgen.VacancyStatusClosed}); err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]string{"reason": strings.TrimSpace(reason)})
	return s.q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: adminID, Valid: true}, Action: "admin.vacancy.hide", ObjectType: "vacancy", ObjectID: uuid.NullUUID{UUID: id, Valid: true}, Details: details})
}

// AuditEntry — запись аудита.
type AuditEntry struct {
	ID         int64           `json:"id"`
	ActorEmail string          `json:"actor_email,omitempty"`
	ActorID    *uuid.UUID      `json:"actor_id,omitempty"`
	Action     string          `json:"action"`
	ObjectType string          `json:"object_type"`
	ObjectID   *uuid.UUID      `json:"object_id,omitempty"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AuditList — страница аудита.
type AuditList struct {
	Items []AuditEntry `json:"items"`
	Total int          `json:"total"`
}

// Audit — журнал действий с фильтрами по префиксу действия и e-mail актора.
func (s *Service) Audit(ctx context.Context, action, actor string, limit, offset int) (AuditList, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	p := sqlcgen.AdminAuditParams{Lim: int32(limit), Off: int32(max(0, offset))} //nolint:gosec // ограничено
	if a := strings.TrimSpace(action); a != "" {
		p.Action = &a
	}
	if a := strings.TrimSpace(actor); a != "" {
		p.Actor = &a
	}
	rows, err := s.q.AdminAudit(ctx, p)
	if err != nil {
		return AuditList{}, err
	}
	out := AuditList{Items: []AuditEntry{}}
	for _, r := range rows {
		out.Total = int(r.Total)
		e := AuditEntry{ID: r.ID, Action: r.Action, ObjectType: r.ObjectType, Details: r.Details, CreatedAt: r.CreatedAt}
		if len(e.Details) == 0 {
			e.Details = json.RawMessage(`{}`)
		}
		if r.ActorEmail != nil {
			e.ActorEmail = *r.ActorEmail
		}
		if r.ActorUserID.Valid {
			id := r.ActorUserID.UUID
			e.ActorID = &id
		}
		if r.ObjectID.Valid {
			id := r.ObjectID.UUID
			e.ObjectID = &id
		}
		out.Items = append(out.Items, e)
	}
	return out, nil
}

// Metrics — текст в формате Prometheus для /metrics.
func (s *Service) Metrics(ctx context.Context) (string, error) {
	var b strings.Builder
	users, err := s.q.AdminUserCounts(ctx)
	if err != nil {
		return "", err
	}
	b.WriteString("# HELP fsp_users Пользователи по роли и статусу\n# TYPE fsp_users gauge\n")
	for _, u := range users {
		fmt.Fprintf(&b, "fsp_users{role=%q,status=%q} %d\n", u.Role, u.Status, u.Total)
	}
	sess, err := s.q.AdminSessionStats(ctx)
	if err != nil {
		return "", err
	}
	b.WriteString("# HELP fsp_test_sessions Тест-сессии по статусу\n# TYPE fsp_test_sessions gauge\n")
	for _, x := range sess {
		fmt.Fprintf(&b, "fsp_test_sessions{status=%q} %d\n", x.Status, x.Total)
	}
	inv, err := s.q.AdminInvitationCounts(ctx)
	if err != nil {
		return "", err
	}
	b.WriteString("# HELP fsp_invitations Приглашения по статусу\n# TYPE fsp_invitations gauge\n")
	for _, x := range inv {
		fmt.Fprintf(&b, "fsp_invitations{status=%q} %d\n", x.Status, x.Total)
	}
	apps, err := s.q.AdminApplicationCounts(ctx)
	if err != nil {
		return "", err
	}
	b.WriteString("# HELP fsp_applications Отклики по статусу\n# TYPE fsp_applications gauge\n")
	for _, x := range apps {
		fmt.Fprintf(&b, "fsp_applications{status=%q} %d\n", x.Status, x.Total)
	}
	cats, err := s.q.AdminCategoryCounts(ctx)
	if err != nil {
		return "", err
	}
	b.WriteString("# HELP fsp_categories Кандидаты в категориях\n# TYPE fsp_categories gauge\n")
	for _, c := range cats {
		fmt.Fprintf(&b, "fsp_categories{specialization=%q,grade=%q,status=%q} %d\n", c.Specialization, c.Grade, c.Status, c.Total)
	}
	return b.String(), nil
}

// RecordAudit пишет действие администратора в журнал аудита.
func (s *Service) RecordAudit(ctx context.Context, adminID uuid.UUID, action, objectType string, details map[string]any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return s.q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: adminID, Valid: true}, Action: action, ObjectType: objectType, Details: raw})
}
