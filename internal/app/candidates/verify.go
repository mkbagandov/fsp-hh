package candidates

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"fsp/internal/domain"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Verification — публичное подтверждение категорий по QR-коду из PDF-профиля. Без персональных данных:
// только категории, их статусы и связь с ФСП ID; адрес страницы содержит непредсказуемый UUID из PDF.
type Verification struct {
	Categories  []VerifiedCategory `json:"categories"`
	FSPLinked   bool               `json:"fsp_linked" doc:"Аккаунт связан с ФСП ID"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// VerifiedCategory — категория на странице подтверждения.
type VerifiedCategory struct {
	Specialization string     `json:"specialization"`
	Grade          string     `json:"grade"`
	Status         string     `json:"status" enum:"claimed,confirmed,not_confirmed,upgrade_suggested"`
	IsPrimary      bool       `json:"is_primary"`
	Percentile     *float64   `json:"percentile,omitempty"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
}

// Verify собирает данные страницы подтверждения. Правила видимости те же, что в подборе: активный
// кандидат, действующее согласие на публикацию, профиль не скрыт. Иначе — «не найдено», чтобы ссылка
// не раскрывала даже факт регистрации.
func (s *Service) Verify(ctx context.Context, userID uuid.UUID) (Verification, error) {
	notFound := domain.NotFound("verification")
	u, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, notFound
	}
	if err != nil {
		return Verification{}, err
	}
	if u.Role != sqlcgen.UserRoleCandidate || u.Status != sqlcgen.UserStatusActive {
		return Verification{}, notFound
	}
	p, err := s.q.GetCandidateProfile(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, notFound
	}
	if err != nil {
		return Verification{}, err
	}
	if p.Visibility == sqlcgen.CandidateVisibilityHidden {
		return Verification{}, notFound
	}
	if _, err := s.q.GetActiveConsent(ctx, sqlcgen.GetActiveConsentParams{UserID: userID, Kind: sqlcgen.ConsentKindPublication}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Verification{}, notFound
		}
		return Verification{}, err
	}
	rows, err := s.q.ListCandidateCategories(ctx, userID)
	if err != nil {
		return Verification{}, err
	}
	if len(rows) == 0 {
		return Verification{}, notFound
	}
	v := Verification{Categories: make([]VerifiedCategory, 0, len(rows)), GeneratedAt: time.Now()}
	for _, r := range rows {
		v.Categories = append(v.Categories, VerifiedCategory{
			Specialization: string(r.Specialization), Grade: string(r.Grade), Status: string(r.Status), IsPrimary: r.IsPrimary,
			Percentile: r.Percentile, ConfirmedAt: r.ConfirmedAt,
		})
	}
	if s.pdf != nil && s.pdf.FSPLinked != nil {
		if linked, err := s.pdf.FSPLinked(ctx, userID); err == nil {
			v.FSPLinked = linked
		}
	}
	return v, nil
}
