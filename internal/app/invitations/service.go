// Package invitations — приглашения работодателя кандидату: вилка обязательна, статусы,
// раскрытие контактов через гранты, причины отказа как обратная связь, квоты
// (docs/03-matching.md, разделы 6–8; docs/adr/0008).
package invitations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/app/notifications"
	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Config — квоты и сроки.
type Config struct {
	TTL                 time.Duration // срок ответа (14 дней)
	MaxActivePerNeed    int           // активных приглашений на потребность (20)
	MaxPerCompanyPerDay int           // отправок компании в сутки (50)
}

func (c Config) withDefaults() Config {
	if c.TTL <= 0 {
		c.TTL = 14 * 24 * time.Hour
	}
	if c.MaxActivePerNeed <= 0 {
		c.MaxActivePerNeed = 20
	}
	if c.MaxPerCompanyPerDay <= 0 {
		c.MaxPerCompanyPerDay = 50
	}
	return c
}

// Notifier — уведомления о событиях приглашения (nil допустим).
type Notifier interface {
	Notify(ctx context.Context, userID uuid.UUID, ev notifications.Event)
	NotifyCompany(ctx context.Context, companyID uuid.UUID, ev notifications.Event)
}

// Service — сценарии приглашений.
type Service struct {
	cfg      Config
	pool     *pgxpool.Pool
	q        *sqlcgen.Queries
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
}

// SetNotifier подключает уведомления.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// New создаёт сервис.
func New(cfg Config, pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{cfg: cfg.withDefaults(), pool: pool, q: sqlcgen.New(pool), log: log, now: time.Now}
}

// ContactMethod — способ связи, который работодатель открывает кандидату при принятии.
type ContactMethod struct {
	Kind  string `json:"kind" enum:"email,telegram,phone,link"`
	Value string `json:"value"`
}

// Invitation — приглашение (вид для обеих сторон).
type Invitation struct {
	ID                uuid.UUID      `json:"id"`
	CompanyID         uuid.UUID      `json:"company_id"`
	CompanyName       string         `json:"company_name,omitempty"`
	NeedID            *uuid.UUID     `json:"need_id,omitempty"`
	VacancyID         *uuid.UUID     `json:"vacancy_id,omitempty"`
	CandidateID       uuid.UUID      `json:"candidate_id"`
	Message           string         `json:"message"`
	SalaryFrom        int64          `json:"salary_from"`
	SalaryTo          int64          `json:"salary_to"`
	ContactMethod     *ContactMethod `json:"contact_method,omitempty" doc:"Кандидату — только после принятия"`
	Status            string         `json:"status" enum:"sent,viewed,accepted,declined,withdrawn,expired"`
	SentAt            time.Time      `json:"sent_at"`
	ViewedAt          *time.Time     `json:"viewed_at,omitempty"`
	RespondedAt       *time.Time     `json:"responded_at,omitempty"`
	DeclineReason     *string        `json:"decline_reason,omitempty"`
	DeclineComment    *string        `json:"decline_comment,omitempty"`
	ExpiresAt         time.Time      `json:"expires_at"`
	BelowCandidateMin bool           `json:"below_candidate_min,omitempty" doc:"Вилка ниже минимума кандидата (видит работодатель)"`
}

// CreateInput — новое приглашение.
type CreateInput struct {
	CandidateID   uuid.UUID
	NeedID        *uuid.UUID
	VacancyID     *uuid.UUID
	Message       string
	SalaryFrom    int64
	SalaryTo      int64
	ContactMethod ContactMethod
}

// Create отправляет приглашение с проверкой вилки, доступности кандидата и квот.
func (s *Service) Create(ctx context.Context, userID, companyID uuid.UUID, in CreateInput) (Invitation, error) {
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" || len([]rune(in.Message)) > 2000 {
		return Invitation{}, domain.Validation("message", "message_invalid", "Опишите предложение (до 2000 символов)")
	}
	rng := domain.SalaryRange{From: in.SalaryFrom, To: in.SalaryTo}
	if err := rng.Validate(); err != nil {
		return Invitation{}, err
	}
	switch in.ContactMethod.Kind {
	case "email", "telegram", "phone", "link":
	default:
		return Invitation{}, domain.Validation("contact_method.kind", "contact_kind_invalid", "Способ связи: email, telegram, phone или link")
	}
	if strings.TrimSpace(in.ContactMethod.Value) == "" || len(in.ContactMethod.Value) > 200 {
		return Invitation{}, domain.Validation("contact_method.value", "contact_value_invalid", "Укажите контакт для связи")
	}
	// Пригласить можно видимого кандидата с согласием на публикацию — либо того, кто сам открылся
	// этой компании (откликнулся или уже принимал приглашение): у компании есть активный грант.
	if _, err := s.q.GetCandidateCard(ctx, in.CandidateID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, err
		}
		if _, gerr := s.q.GetActiveContactGrant(ctx, sqlcgen.GetActiveContactGrantParams{CandidateUserID: in.CandidateID, CompanyID: companyID}); gerr != nil {
			return Invitation{}, domain.NotFound("candidate")
		}
		if _, aerr := s.q.GetCandidateCardAny(ctx, in.CandidateID); aerr != nil {
			return Invitation{}, domain.NotFound("candidate")
		}
	}
	var needID uuid.NullUUID
	if in.NeedID != nil {
		need, err := s.q.GetNeed(ctx, *in.NeedID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && need.CompanyID != companyID) {
			return Invitation{}, domain.NotFound("need")
		}
		if err != nil {
			return Invitation{}, err
		}
		needID = uuid.NullUUID{UUID: need.ID, Valid: true}
		active, err := s.q.CountActiveInvitationsForNeed(ctx, needID)
		if err != nil {
			return Invitation{}, err
		}
		if int(active) >= s.cfg.MaxActivePerNeed {
			return Invitation{}, domain.E(domain.KindRateLimited, "need_quota_exceeded", fmt.Sprintf("По этой потребности уже %d активных приглашений; дождитесь ответов", s.cfg.MaxActivePerNeed))
		}
	}
	today, err := s.q.CountInvitationsSentToday(ctx, companyID)
	if err != nil {
		return Invitation{}, err
	}
	if int(today) >= s.cfg.MaxPerCompanyPerDay {
		return Invitation{}, domain.E(domain.KindRateLimited, "company_quota_exceeded", "Суточный лимит приглашений компании исчерпан")
	}
	var vacancyID uuid.NullUUID
	if in.VacancyID != nil {
		vacancyID = uuid.NullUUID{UUID: *in.VacancyID, Valid: true}
	}
	cmJSON, _ := json.Marshal(in.ContactMethod)
	var row *sqlcgen.Invitation
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		r, err := q.CreateInvitation(ctx, sqlcgen.CreateInvitationParams{
			CompanyID: companyID, NeedID: needID, VacancyID: vacancyID, CandidateUserID: in.CandidateID, CreatedBy: userID, Message: in.Message,
			SalaryFrom: int32(in.SalaryFrom), SalaryTo: int32(in.SalaryTo), ContactMethod: cmJSON, ExpiresAt: s.now().Add(s.cfg.TTL), //nolint:gosec // ≤ MaxSalary
		})
		if err != nil {
			if isUnique(err) {
				return domain.Conflict("invitation_active_exists", "У этого кандидата уже есть активное приглашение от вашей компании")
			}
			return err
		}
		row = r
		return q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: userID, Valid: true}, Action: "invitation.create", ObjectType: "invitation", ObjectID: uuid.NullUUID{UUID: r.ID, Valid: true}, Details: json.RawMessage(fmt.Sprintf(`{"candidate_id":%q}`, in.CandidateID))})
	})
	if err != nil {
		return Invitation{}, err
	}
	inv := toInvitation(row, true)
	// salary_min хранится в профиле; карточка его не отдаёт — читаем напрямую для предупреждения.
	if prof, err := s.q.GetCandidateProfile(ctx, in.CandidateID); err == nil && prof.SalaryMin != nil && int64(*prof.SalaryMin) > in.SalaryTo {
		inv.BelowCandidateMin = true
	}
	if s.notifier != nil {
		s.notifier.Notify(ctx, in.CandidateID, notifications.Event{
			Kind: notifications.KindInvitationReceived, Title: "Новое приглашение от компании " + s.companyName(ctx, companyID),
			Body: fmt.Sprintf("Вилка %d–%d ₽. %s", in.SalaryFrom, in.SalaryTo, in.Message),
			Link: "/candidate/invitations/" + row.ID.String(), Payload: map[string]any{"invitation_id": row.ID}, Email: true,
		})
	}
	return inv, nil
}

// ListForCompany — приглашения компании (опционально по потребности).
func (s *Service) ListForCompany(ctx context.Context, companyID uuid.UUID, needID *uuid.UUID) ([]Invitation, error) {
	_, _ = s.q.ExpireInvitations(ctx)
	var nid uuid.NullUUID
	if needID != nil {
		nid = uuid.NullUUID{UUID: *needID, Valid: true}
	}
	rows, err := s.q.ListInvitationsByCompany(ctx, sqlcgen.ListInvitationsByCompanyParams{CompanyID: companyID, NeedID: nid})
	if err != nil {
		return nil, err
	}
	out := make([]Invitation, 0, len(rows))
	for _, r := range rows {
		out = append(out, toInvitation(r, true))
	}
	return out, nil
}

// Withdraw отзывает приглашение до ответа.
func (s *Service) Withdraw(ctx context.Context, userID, companyID, id uuid.UUID) (Invitation, error) {
	row, err := s.q.GetInvitation(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.CompanyID != companyID) {
		return Invitation{}, domain.NotFound("invitation")
	}
	if err != nil {
		return Invitation{}, err
	}
	return s.transition(ctx, row, domain.InvitationWithdrawn, nil, nil, userID, true)
}

// ListForCandidate — входящие приглашения кандидата.
func (s *Service) ListForCandidate(ctx context.Context, userID uuid.UUID) ([]Invitation, error) {
	_, _ = s.q.ExpireInvitations(ctx)
	rows, err := s.q.ListInvitationsByCandidate(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Invitation, 0, len(rows))
	for _, r := range rows {
		inv := toInvitation(fromCandidateRow(r), r.Status == sqlcgen.InvitationStatusAccepted)
		inv.CompanyName = r.CompanyName
		out = append(out, inv)
	}
	return out, nil
}

// GetForCandidate возвращает приглашение и отмечает просмотр.
func (s *Service) GetForCandidate(ctx context.Context, userID, id uuid.UUID) (Invitation, error) {
	row, err := s.ownByCandidate(ctx, userID, id)
	if err != nil {
		return Invitation{}, err
	}
	if row.Status == sqlcgen.InvitationStatusSent {
		if err := s.q.UpdateInvitationStatus(ctx, sqlcgen.UpdateInvitationStatusParams{ID: row.ID, Status: sqlcgen.InvitationStatusViewed}); err != nil {
			return Invitation{}, err
		}
		row, _ = s.q.GetInvitation(ctx, row.ID)
	}
	inv := toInvitation(row, row.Status == sqlcgen.InvitationStatusAccepted)
	if c, err := s.q.GetCompanyPublicContacts(ctx, row.CompanyID); err == nil {
		inv.CompanyName = c.Name
	}
	return inv, nil
}

// AcceptResult — ответ при принятии: контакты компании.
type AcceptResult struct {
	Invitation      Invitation        `json:"invitation"`
	ContactMethod   ContactMethod     `json:"contact_method"`
	CompanyContacts map[string]string `json:"company_contacts"`
}

// Accept принимает приглашение: создаёт грант на контакты и раскрывает способ связи компании.
func (s *Service) Accept(ctx context.Context, userID, id uuid.UUID) (AcceptResult, error) {
	row, err := s.ownByCandidate(ctx, userID, id)
	if err != nil {
		return AcceptResult{}, err
	}
	if row.Status == sqlcgen.InvitationStatusSent {
		// Принятие подразумевает просмотр условий: sent → viewed → accepted.
		if err := s.q.UpdateInvitationStatus(ctx, sqlcgen.UpdateInvitationStatusParams{ID: row.ID, Status: sqlcgen.InvitationStatusViewed}); err != nil {
			return AcceptResult{}, err
		}
		row.Status = sqlcgen.InvitationStatusViewed
	}
	inv, err := s.transition(ctx, row, domain.InvitationAccepted, nil, nil, userID, true)
	if err != nil {
		return AcceptResult{}, err
	}
	res := AcceptResult{Invitation: inv, CompanyContacts: map[string]string{}}
	if inv.ContactMethod != nil {
		res.ContactMethod = *inv.ContactMethod
	}
	if c, err := s.q.GetCompanyPublicContacts(ctx, row.CompanyID); err == nil {
		res.Invitation.CompanyName = c.Name
		_ = json.Unmarshal(c.PublicContacts, &res.CompanyContacts)
	}
	return res, nil
}

// Decline отклоняет приглашение с причиной.
func (s *Service) Decline(ctx context.Context, userID, id uuid.UUID, reason, comment string) (Invitation, error) {
	if !refdata.IsDeclineReason(reason) {
		return Invitation{}, domain.Validation("reason", "decline_reason_invalid", "Выберите причину из справочника")
	}
	if len([]rune(comment)) > 1000 {
		return Invitation{}, domain.Validation("comment", "comment_too_long", "Комментарий — до 1000 символов")
	}
	row, err := s.ownByCandidate(ctx, userID, id)
	if err != nil {
		return Invitation{}, err
	}
	if row.Status == sqlcgen.InvitationStatusSent {
		if err := s.q.UpdateInvitationStatus(ctx, sqlcgen.UpdateInvitationStatusParams{ID: row.ID, Status: sqlcgen.InvitationStatusViewed}); err != nil {
			return Invitation{}, err
		}
		row.Status = sqlcgen.InvitationStatusViewed
	}
	r := sqlcgen.DeclineReason(reason)
	var c *string
	if strings.TrimSpace(comment) != "" {
		v := strings.TrimSpace(comment)
		c = &v
	}
	return s.transition(ctx, row, domain.InvitationDeclined, &r, c, userID, false)
}

// Feedback — агрегированная обратная связь по потребности.
type Feedback struct {
	Total          int            `json:"total"`
	ByStatus       map[string]int `json:"by_status"`
	AcceptanceRate float64        `json:"acceptance_rate"`
	DeclineReasons map[string]int `json:"decline_reasons"`
	Hints          []string       `json:"hints"`
}

// FeedbackForNeed — статусы и причины отказов по потребности с подсказками.
func (s *Service) FeedbackForNeed(ctx context.Context, companyID, needID uuid.UUID) (Feedback, error) {
	rows, err := s.q.ListInvitationsByCompany(ctx, sqlcgen.ListInvitationsByCompanyParams{CompanyID: companyID, NeedID: uuid.NullUUID{UUID: needID, Valid: true}})
	if err != nil {
		return Feedback{}, err
	}
	fb := Feedback{ByStatus: map[string]int{}, DeclineReasons: map[string]int{}, Hints: []string{}}
	answered, accepted := 0, 0
	for _, r := range rows {
		fb.Total++
		fb.ByStatus[string(r.Status)]++
		switch r.Status {
		case sqlcgen.InvitationStatusAccepted:
			answered++
			accepted++
		case sqlcgen.InvitationStatusDeclined:
			answered++
			if r.DeclineReason != nil {
				fb.DeclineReasons[string(*r.DeclineReason)]++
			}
		}
	}
	if answered > 0 {
		fb.AcceptanceRate = float64(accepted) / float64(answered)
	}
	declined := fb.ByStatus["declined"]
	if declined > 0 {
		if n := fb.DeclineReasons["salary_below"]; n*2 >= declined {
			fb.Hints = append(fb.Hints, fmt.Sprintf("%d%% отказов — вилка ниже ожиданий: посмотрите карту категорий и поднимите верхнюю границу", n*100/declined))
		}
		if n := fb.DeclineReasons["format_mismatch"]; n*2 >= declined {
			fb.Hints = append(fb.Hints, "Чаще всего не подходит формат или локация: рассмотрите удалённый формат")
		}
		if n := fb.DeclineReasons["role_mismatch"]; n*2 >= declined {
			fb.Hints = append(fb.Hints, "Кандидаты не видят себя в роли: уточните обязательные навыки и описание задач")
		}
	}
	return fb, nil
}

// --- внутреннее ---

func (s *Service) ownByCandidate(ctx context.Context, userID, id uuid.UUID) (*sqlcgen.Invitation, error) {
	_, _ = s.q.ExpireInvitations(ctx)
	row, err := s.q.GetInvitation(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.CandidateUserID != userID) {
		return nil, domain.NotFound("invitation")
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) transition(ctx context.Context, row *sqlcgen.Invitation, to domain.InvitationStatus, reason *sqlcgen.DeclineReason, comment *string, actor uuid.UUID, withContact bool) (Invitation, error) {
	from := domain.InvitationStatus(row.Status)
	if !from.CanTransitionTo(to) {
		return Invitation{}, domain.InvalidTransition("invitation", string(from), string(to))
	}
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdateInvitationStatus(ctx, sqlcgen.UpdateInvitationStatusParams{ID: row.ID, Status: sqlcgen.InvitationStatus(to), DeclineReason: reason, DeclineComment: comment}); err != nil {
			return err
		}
		if to == domain.InvitationAccepted {
			if _, err := q.CreateContactGrant(ctx, sqlcgen.CreateContactGrantParams{
				CandidateUserID: row.CandidateUserID, CompanyID: row.CompanyID, Source: sqlcgen.ContactGrantSourceInvitationAccepted, SourceID: row.ID,
			}); err != nil {
				return err
			}
		}
		return q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: actor, Valid: true}, Action: "invitation." + string(to), ObjectType: "invitation", ObjectID: uuid.NullUUID{UUID: row.ID, Valid: true}, Details: json.RawMessage(`{}`)})
	})
	if err != nil {
		return Invitation{}, err
	}
	updated, err := s.q.GetInvitation(ctx, row.ID)
	if err != nil {
		return Invitation{}, err
	}
	s.notifyTransition(ctx, updated, to, reason)
	return toInvitation(updated, withContact), nil
}

// notifyTransition сообщает другой стороне об изменении статуса.
func (s *Service) notifyTransition(ctx context.Context, inv *sqlcgen.Invitation, to domain.InvitationStatus, reason *sqlcgen.DeclineReason) {
	if s.notifier == nil {
		return
	}
	payload := map[string]any{"invitation_id": inv.ID, "status": string(to)}
	switch to {
	case domain.InvitationAccepted:
		s.notifier.NotifyCompany(ctx, inv.CompanyID, notifications.Event{Kind: notifications.KindInvitationStatus, Title: "Кандидат принял приглашение",
			Body: "Контакты кандидата открыты в его карточке.", Link: "/employer/candidates/" + inv.CandidateUserID.String(), Payload: payload, Email: true})
	case domain.InvitationDeclined:
		text := "Кандидат отклонил приглашение"
		if reason != nil {
			text += ": " + refdata.DeclineReasonName(string(*reason))
		}
		s.notifier.NotifyCompany(ctx, inv.CompanyID, notifications.Event{Kind: notifications.KindInvitationStatus, Title: text,
			Body: "Причины отказов по потребности собираются в обратной связи.", Link: "/employer/invitations", Payload: payload})
	case domain.InvitationWithdrawn:
		s.notifier.Notify(ctx, inv.CandidateUserID, notifications.Event{Kind: notifications.KindInvitationStatus, Title: "Компания отозвала приглашение",
			Link: "/candidate/invitations", Payload: payload})
	default:
	}
}

func (s *Service) companyName(ctx context.Context, companyID uuid.UUID) string {
	if c, err := s.q.GetCompanyPublicContacts(ctx, companyID); err == nil {
		return c.Name
	}
	return ""
}

func toInvitation(r *sqlcgen.Invitation, withContact bool) Invitation {
	inv := Invitation{
		ID: r.ID, CompanyID: r.CompanyID, CandidateID: r.CandidateUserID, Message: r.Message, SalaryFrom: int64(r.SalaryFrom), SalaryTo: int64(r.SalaryTo),
		Status: string(r.Status), SentAt: r.SentAt, ViewedAt: r.ViewedAt, RespondedAt: r.RespondedAt, DeclineComment: r.DeclineComment, ExpiresAt: r.ExpiresAt,
	}
	if r.NeedID.Valid {
		id := r.NeedID.UUID
		inv.NeedID = &id
	}
	if r.VacancyID.Valid {
		id := r.VacancyID.UUID
		inv.VacancyID = &id
	}
	if r.DeclineReason != nil {
		v := string(*r.DeclineReason)
		inv.DeclineReason = &v
	}
	if withContact {
		var cm ContactMethod
		if json.Unmarshal(r.ContactMethod, &cm) == nil {
			inv.ContactMethod = &cm
		}
	}
	return inv
}

// fromCandidateRow собирает модель приглашения из строки с данными компании.
func fromCandidateRow(r *sqlcgen.ListInvitationsByCandidateRow) *sqlcgen.Invitation {
	return &sqlcgen.Invitation{
		ID: r.ID, CompanyID: r.CompanyID, NeedID: r.NeedID, VacancyID: r.VacancyID, CandidateUserID: r.CandidateUserID, CreatedBy: r.CreatedBy,
		Message: r.Message, SalaryFrom: r.SalaryFrom, SalaryTo: r.SalaryTo, ContactMethod: r.ContactMethod, Status: r.Status,
		SentAt: r.SentAt, ViewedAt: r.ViewedAt, RespondedAt: r.RespondedAt, DeclineReason: r.DeclineReason, DeclineComment: r.DeclineComment,
		WithdrawnAt: r.WithdrawnAt, ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt,
	}
}

func isUnique(err error) bool {
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key")
}

// ExpireDue переводит просроченные приглашения в expired (фоновая задача; чтение списков делает то же лениво).
func (s *Service) ExpireDue(ctx context.Context) (int64, error) {
	return s.q.ExpireInvitations(ctx)
}
