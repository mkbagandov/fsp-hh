// Package applications — отклики кандидатов на вакансии. Отклик — явное действие кандидата,
// поэтому компания сразу получает грант на его контакты (правило ТЗ 2.2, docs/adr/0008).
package applications

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

	"fsp/internal/app/invitations"
	"fsp/internal/app/notifications"
	"fsp/internal/domain"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Notifier — уведомления (nil допустим).
type Notifier interface {
	Notify(ctx context.Context, userID uuid.UUID, ev notifications.Event)
	NotifyCompany(ctx context.Context, companyID uuid.UUID, ev notifications.Event)
}

// Service — сценарии откликов.
type Service struct {
	pool        *pgxpool.Pool
	q           *sqlcgen.Queries
	invitations *invitations.Service
	notifier    Notifier
	log         *slog.Logger
}

// New создаёт сервис.
func New(pool *pgxpool.Pool, inv *invitations.Service, log *slog.Logger) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool), invitations: inv, log: log}
}

// SetNotifier подключает уведомления.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// VacancyBrief — вакансия в отклике кандидата.
type VacancyBrief struct {
	ID             uuid.UUID `json:"id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	Specialization string    `json:"specialization"`
	SalaryFrom     int64     `json:"salary_from"`
	SalaryTo       int64     `json:"salary_to"`
	CompanyID      uuid.UUID `json:"company_id"`
	CompanyName    string    `json:"company_name"`
}

// Applicant — кандидат в списке откликов работодателя (с учётом приватности).
type Applicant struct {
	ID              uuid.UUID `json:"id"`
	DisplayName     string    `json:"display_name"`
	Headline        string    `json:"headline,omitempty"`
	City            string    `json:"city,omitempty"`
	SalaryExpected  *int64    `json:"salary_expected,omitempty"`
	ExperienceYears *float64  `json:"experience_years,omitempty"`
	Grade           string    `json:"grade,omitempty" doc:"Грейд в специализации вакансии"`
	GradeStatus     string    `json:"grade_status,omitempty"`
	Percentile      *float64  `json:"percentile,omitempty"`
}

// Application — отклик.
type Application struct {
	ID           uuid.UUID     `json:"id"`
	VacancyID    uuid.UUID     `json:"vacancy_id"`
	CandidateID  uuid.UUID     `json:"candidate_id"`
	CoverLetter  string        `json:"cover_letter,omitempty"`
	Status       string        `json:"status" enum:"sent,viewed,invited,rejected,withdrawn"`
	SentAt       time.Time     `json:"sent_at"`
	ViewedAt     *time.Time    `json:"viewed_at,omitempty"`
	RespondedAt  *time.Time    `json:"responded_at,omitempty"`
	RejectReason string        `json:"reject_reason,omitempty"`
	InvitationID *uuid.UUID    `json:"invitation_id,omitempty"`
	Vacancy      *VacancyBrief `json:"vacancy,omitempty"`
	Applicant    *Applicant    `json:"applicant,omitempty"`
}

// Apply — отклик на опубликованную вакансию; создаёт грант на контакты компании.
func (s *Service) Apply(ctx context.Context, candidateID, vacancyID uuid.UUID, cover string) (Application, error) {
	cover = strings.TrimSpace(cover)
	if len([]rune(cover)) > 4000 {
		return Application{}, domain.Validation("cover_letter", "cover_letter_too_long", "Сопроводительное письмо — до 4000 символов")
	}
	v, err := s.q.GetVacancy(ctx, vacancyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && v.Status != sqlcgen.VacancyStatusPublished) {
		return Application{}, domain.NotFound("vacancy")
	}
	if err != nil {
		return Application{}, err
	}
	prof, err := s.q.GetCandidateProfile(ctx, candidateID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && strings.TrimSpace(prof.FirstName) == "") {
		return Application{}, domain.Conflict("profile_required", "Сначала заполните анкету: работодатель увидит её вместе с откликом")
	}
	if err != nil {
		return Application{}, err
	}
	if existing, err := s.q.GetApplicationByVacancyAndCandidate(ctx, sqlcgen.GetApplicationByVacancyAndCandidateParams{VacancyID: vacancyID, CandidateUserID: candidateID}); err == nil {
		return s.view(existing, nil, nil), domain.Conflict("application_exists", "Вы уже откликались на эту вакансию")
	}
	var cl *string
	if cover != "" {
		cl = &cover
	}
	var row *sqlcgen.Application
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		r, err := q.CreateApplication(ctx, sqlcgen.CreateApplicationParams{VacancyID: vacancyID, CandidateUserID: candidateID, CoverLetter: cl})
		if err != nil {
			if strings.Contains(err.Error(), "23505") {
				return domain.Conflict("application_exists", "Вы уже откликались на эту вакансию")
			}
			return err
		}
		row = r
		if _, err := q.CreateContactGrant(ctx, sqlcgen.CreateContactGrantParams{
			CandidateUserID: candidateID, CompanyID: v.CompanyID, Source: sqlcgen.ContactGrantSourceApplicationSent, SourceID: r.ID,
		}); err != nil {
			return err
		}
		return q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: candidateID, Valid: true}, Action: "application.create", ObjectType: "application", ObjectID: uuid.NullUUID{UUID: r.ID, Valid: true}, Details: json.RawMessage(fmt.Sprintf(`{"vacancy_id":%q}`, vacancyID))})
	})
	if err != nil {
		return Application{}, err
	}
	if s.notifier != nil {
		s.notifier.NotifyCompany(ctx, v.CompanyID, notifications.Event{
			Kind: notifications.KindApplicationReceived, Title: "Новый отклик на вакансию «" + v.Title + "»",
			Body: "Кандидат откликнулся и открыл вам контакты. Посмотрите анкету и ответьте в течение нескольких дней.",
			Link: fmt.Sprintf("/employer/vacancies/%s/applications", v.ID), Payload: map[string]any{"vacancy_id": v.ID, "application_id": row.ID}, Email: true,
		})
	}
	return s.view(row, nil, nil), nil
}

// ListForCandidate — отклики кандидата с краткими данными вакансии.
func (s *Service) ListForCandidate(ctx context.Context, candidateID uuid.UUID) ([]Application, error) {
	rows, err := s.q.ListApplicationsByCandidate(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	out := make([]Application, 0, len(rows))
	for _, r := range rows {
		a := s.view(&sqlcgen.Application{ID: r.ID, VacancyID: r.VacancyID, CandidateUserID: r.CandidateUserID, CoverLetter: r.CoverLetter, Status: r.Status,
			SentAt: r.SentAt, ViewedAt: r.ViewedAt, RespondedAt: r.RespondedAt, RejectReason: r.RejectReason, WithdrawnAt: r.WithdrawnAt, InvitationID: r.InvitationID},
			&VacancyBrief{ID: r.VacancyID, Title: r.VacancyTitle, Status: string(r.VacancyStatus), Specialization: string(r.VacancySpecialization),
				SalaryFrom: int64(r.VacancySalaryFrom), SalaryTo: int64(r.VacancySalaryTo), CompanyID: r.CompanyID, CompanyName: r.CompanyName}, nil)
		out = append(out, a)
	}
	return out, nil
}

// Withdraw — кандидат отзывает отклик (грант на контакты при этом не отзывается: компания уже могла их увидеть; отзыв — через приватность).
func (s *Service) Withdraw(ctx context.Context, candidateID, id uuid.UUID) (Application, error) {
	row, err := s.q.GetApplication(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.CandidateUserID != candidateID) {
		return Application{}, domain.NotFound("application")
	}
	if err != nil {
		return Application{}, err
	}
	return s.transition(ctx, row, domain.ApplicationWithdrawn, nil, nil, candidateID)
}

// ListForVacancy — отклики на вакансию компании с анкетами кандидатов.
func (s *Service) ListForVacancy(ctx context.Context, companyID, vacancyID uuid.UUID) ([]Application, error) {
	v, err := s.q.GetVacancy(ctx, vacancyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && v.CompanyID != companyID) {
		return nil, domain.NotFound("vacancy")
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListApplicationsByVacancy(ctx, vacancyID)
	if err != nil {
		return nil, err
	}
	out := make([]Application, 0, len(rows))
	for _, r := range rows {
		ap := &Applicant{ID: r.CandidateUserID}
		if r.ShowFullName {
			ap.DisplayName = r.LastName + " " + r.FirstName
		} else {
			ap.DisplayName = r.FirstName
		}
		if r.Headline != nil {
			ap.Headline = *r.Headline
		}
		if r.ShowCity && r.City != nil {
			ap.City = *r.City
		}
		if r.ShowSalary && r.SalaryExpected != nil {
			v := int64(*r.SalaryExpected)
			ap.SalaryExpected = &v
		}
		if r.ShowExperience {
			ap.ExperienceYears = r.ExperienceYears
		}
		if r.Grade != nil {
			ap.Grade = string(*r.Grade)
		}
		if r.GradeStatus != nil {
			ap.GradeStatus = string(*r.GradeStatus)
		}
		ap.Percentile = r.Percentile
		out = append(out, s.view(&sqlcgen.Application{ID: r.ID, VacancyID: r.VacancyID, CandidateUserID: r.CandidateUserID, CoverLetter: r.CoverLetter, Status: r.Status,
			SentAt: r.SentAt, ViewedAt: r.ViewedAt, RespondedAt: r.RespondedAt, RejectReason: r.RejectReason, WithdrawnAt: r.WithdrawnAt, InvitationID: r.InvitationID}, nil, ap))
	}
	return out, nil
}

// View — работодатель открыл отклик (sent → viewed).
func (s *Service) View(ctx context.Context, userID, companyID, id uuid.UUID) (Application, error) {
	row, _, err := s.own(ctx, companyID, id)
	if err != nil {
		return Application{}, err
	}
	if row.Status != sqlcgen.ApplicationStatusSent {
		return s.view(row, nil, nil), nil
	}
	return s.transition(ctx, row, domain.ApplicationViewed, nil, nil, userID)
}

// Reject — отказ по отклику с причиной (видна кандидату).
func (s *Service) Reject(ctx context.Context, userID, companyID, id uuid.UUID, reason string) (Application, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 1000 {
		return Application{}, domain.Validation("reason", "reason_invalid", "Укажите причину отказа (до 1000 символов) — кандидат её увидит")
	}
	row, v, err := s.own(ctx, companyID, id)
	if err != nil {
		return Application{}, err
	}
	if row.Status == sqlcgen.ApplicationStatusSent {
		if _, err := s.transition(ctx, row, domain.ApplicationViewed, nil, nil, userID); err != nil {
			return Application{}, err
		}
		row.Status = sqlcgen.ApplicationStatusViewed
	}
	a, err := s.transition(ctx, row, domain.ApplicationRejected, &reason, nil, userID)
	if err != nil {
		return Application{}, err
	}
	if s.notifier != nil {
		s.notifier.Notify(ctx, row.CandidateUserID, notifications.Event{
			Kind: notifications.KindApplicationStatus, Title: "Отказ по отклику на «" + v.Title + "»", Body: "Причина: " + reason,
			Link: "/candidate/applications", Payload: map[string]any{"application_id": row.ID, "status": "rejected"}, Email: true,
		})
	}
	return a, nil
}

// InviteInput — условия приглашения из отклика.
type InviteInput struct {
	Message       string
	SalaryFrom    int64
	SalaryTo      int64
	ContactMethod invitations.ContactMethod
}

// InviteResult — отклик и созданное приглашение.
type InviteResult struct {
	Application Application            `json:"application"`
	Invitation  invitations.Invitation `json:"invitation"`
}

// Invite — пригласить кандидата из отклика: создаётся приглашение с вилкой, отклик переходит в invited.
func (s *Service) Invite(ctx context.Context, userID, companyID, id uuid.UUID, in InviteInput) (InviteResult, error) {
	row, v, err := s.own(ctx, companyID, id)
	if err != nil {
		return InviteResult{}, err
	}
	if row.Status == sqlcgen.ApplicationStatusSent {
		if _, err := s.transition(ctx, row, domain.ApplicationViewed, nil, nil, userID); err != nil {
			return InviteResult{}, err
		}
		row.Status = sqlcgen.ApplicationStatusViewed
	}
	if !domain.ApplicationStatus(row.Status).CanTransitionTo(domain.ApplicationInvited) {
		return InviteResult{}, domain.InvalidTransition("application", string(row.Status), string(domain.ApplicationInvited))
	}
	var needID *uuid.UUID
	if v.NeedID.Valid {
		n := v.NeedID.UUID
		needID = &n
	}
	vid := v.ID
	inv, err := s.invitations.Create(ctx, userID, companyID, invitations.CreateInput{
		CandidateID: row.CandidateUserID, NeedID: needID, VacancyID: &vid, Message: in.Message, SalaryFrom: in.SalaryFrom, SalaryTo: in.SalaryTo, ContactMethod: in.ContactMethod,
	})
	if err != nil {
		return InviteResult{}, err
	}
	a, err := s.transition(ctx, row, domain.ApplicationInvited, nil, &inv.ID, userID)
	if err != nil {
		return InviteResult{}, err
	}
	return InviteResult{Application: a, Invitation: inv}, nil
}

func (s *Service) own(ctx context.Context, companyID, id uuid.UUID) (*sqlcgen.Application, *sqlcgen.Vacancy, error) {
	row, err := s.q.GetApplication(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, domain.NotFound("application")
	}
	if err != nil {
		return nil, nil, err
	}
	v, err := s.q.GetVacancy(ctx, row.VacancyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && v.CompanyID != companyID) {
		return nil, nil, domain.NotFound("application")
	}
	if err != nil {
		return nil, nil, err
	}
	return row, v, nil
}

func (s *Service) transition(ctx context.Context, row *sqlcgen.Application, to domain.ApplicationStatus, reason *string, invitationID *uuid.UUID, actor uuid.UUID) (Application, error) {
	from := domain.ApplicationStatus(row.Status)
	if !from.CanTransitionTo(to) {
		return Application{}, domain.InvalidTransition("application", string(from), string(to))
	}
	var inv uuid.NullUUID
	if invitationID != nil {
		inv = uuid.NullUUID{UUID: *invitationID, Valid: true}
	}
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdateApplicationStatus(ctx, sqlcgen.UpdateApplicationStatusParams{ID: row.ID, Status: sqlcgen.ApplicationStatus(to), RejectReason: reason, InvitationID: inv}); err != nil {
			return err
		}
		return q.InsertAudit(ctx, sqlcgen.InsertAuditParams{ActorUserID: uuid.NullUUID{UUID: actor, Valid: true}, Action: "application." + string(to), ObjectType: "application", ObjectID: uuid.NullUUID{UUID: row.ID, Valid: true}, Details: json.RawMessage(`{}`)})
	})
	if err != nil {
		return Application{}, err
	}
	updated, err := s.q.GetApplication(ctx, row.ID)
	if err != nil {
		return Application{}, err
	}
	return s.view(updated, nil, nil), nil
}

func (s *Service) view(r *sqlcgen.Application, v *VacancyBrief, ap *Applicant) Application {
	a := Application{ID: r.ID, VacancyID: r.VacancyID, CandidateID: r.CandidateUserID, Status: string(r.Status), SentAt: r.SentAt, ViewedAt: r.ViewedAt, RespondedAt: r.RespondedAt, Vacancy: v, Applicant: ap}
	if r.CoverLetter != nil {
		a.CoverLetter = *r.CoverLetter
	}
	if r.RejectReason != nil {
		a.RejectReason = *r.RejectReason
	}
	if r.InvitationID.Valid {
		id := r.InvitationID.UUID
		a.InvitationID = &id
	}
	return a
}
