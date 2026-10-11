// Package notifications — уведомления в приложении и письма о ключевых событиях.
// Уведомление никогда не ломает основной сценарий: ошибки только логируются.
package notifications

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
	"fsp/internal/infrastructure/mail"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Kind — тип уведомления (enum notification_kind).
type Kind string

// Виды уведомлений.
const (
	KindInvitationReceived  Kind = "invitation_received"
	KindInvitationStatus    Kind = "invitation_status"
	KindApplicationReceived Kind = "application_received"
	KindApplicationStatus   Kind = "application_status"
	KindTestResult          Kind = "test_result"
	KindSystem              Kind = "system"
)

// Event — что произошло; Link — путь во фронтенде, Email — продублировать письмом.
type Event struct {
	Kind    Kind
	Title   string
	Body    string
	Link    string
	Payload map[string]any
	Email   bool
}

// Service — лента уведомлений и рассылка.
type Service struct {
	q           *sqlcgen.Queries
	mailer      mail.Mailer
	frontendURL string
	log         *slog.Logger
}

// New создаёт сервис. mailer может быть nil — тогда письма не отправляются.
func New(pool *pgxpool.Pool, mailer mail.Mailer, frontendURL string, log *slog.Logger) *Service {
	return &Service{q: sqlcgen.New(pool), mailer: mailer, frontendURL: strings.TrimRight(frontendURL, "/"), log: log}
}

// Notify записывает уведомление пользователю и при необходимости шлёт письмо.
func (s *Service) Notify(ctx context.Context, userID uuid.UUID, ev Event) {
	payload, _ := json.Marshal(ev.Payload)
	if ev.Payload == nil {
		payload = []byte(`{}`)
	}
	var body, link *string
	if ev.Body != "" {
		body = &ev.Body
	}
	if ev.Link != "" {
		link = &ev.Link
	}
	if _, err := s.q.CreateNotification(ctx, sqlcgen.CreateNotificationParams{
		UserID: userID, Kind: sqlcgen.NotificationKind(ev.Kind), Title: ev.Title, Body: body, Link: link, Payload: payload,
	}); err != nil {
		s.log.Error("notification insert", "err", err, "user", userID, "kind", ev.Kind)
		return
	}
	if !ev.Email || s.mailer == nil {
		return
	}
	u, err := s.q.GetUserByID(ctx, userID)
	if err != nil || u.Status != sqlcgen.UserStatusActive {
		return
	}
	text := ev.Body
	if ev.Link != "" && s.frontendURL != "" {
		text += "\n\nОткрыть: " + s.frontendURL + ev.Link
	}
	if err := s.mailer.Send(ctx, mail.Message{To: u.Email, Subject: "ФСП: " + ev.Title, Text: text}); err != nil {
		s.log.Error("notification mail", "err", err, "user", userID)
	}
}

// NotifyCompany уведомляет всех пользователей компании.
func (s *Service) NotifyCompany(ctx context.Context, companyID uuid.UUID, ev Event) {
	ids, err := s.q.ListCompanyUserIDs(ctx, companyID)
	if err != nil {
		s.log.Error("company users", "err", err, "company", companyID)
		return
	}
	for _, id := range ids {
		s.Notify(ctx, id, ev)
	}
}

// Notification — элемент ленты.
type Notification struct {
	ID        uuid.UUID       `json:"id"`
	Kind      string          `json:"kind"`
	Title     string          `json:"title"`
	Body      string          `json:"body,omitempty"`
	Link      string          `json:"link,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	ReadAt    *time.Time      `json:"read_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// List — лента пользователя (новые сверху).
func (s *Service) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.q.ListNotifications(ctx, sqlcgen.ListNotificationsParams{UserID: userID, UnreadOnly: unreadOnly, Lim: int32(limit)}) //nolint:gosec // ≤ 200
	if err != nil {
		return nil, err
	}
	out := make([]Notification, 0, len(rows))
	for _, r := range rows {
		n := Notification{ID: r.ID, Kind: string(r.Kind), Title: r.Title, Payload: r.Payload, ReadAt: r.ReadAt, CreatedAt: r.CreatedAt}
		if r.Body != nil {
			n.Body = *r.Body
		}
		if r.Link != nil {
			n.Link = *r.Link
		}
		if len(n.Payload) == 0 {
			n.Payload = json.RawMessage(`{}`)
		}
		out = append(out, n)
	}
	return out, nil
}

// UnreadCount — число непрочитанных.
func (s *Service) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := s.q.CountUnreadNotifications(ctx, userID)
	return int(n), err
}

// MarkRead отмечает уведомление прочитанным (только своё).
func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	n, err := s.q.MarkNotificationRead(ctx, sqlcgen.MarkNotificationReadParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.NotFound("notification")
	}
	return nil
}

// MarkAllRead отмечает все уведомления прочитанными, возвращает число затронутых.
func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := s.q.MarkAllNotificationsRead(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return int(n), err
}
