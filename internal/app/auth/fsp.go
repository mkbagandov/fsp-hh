package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/oidc"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// FSPSyncer — синхронизация достижений после входа/привязки (internal/app/fsp).
type FSPSyncer interface {
	Sync(ctx context.Context, userID uuid.UUID, fspID, accessToken string) (int, error)
}

// SetFSPSyncer подключает синхронизатор достижений (необязательно).
func (s *Service) SetFSPSyncer(sync FSPSyncer) { s.fspSync = sync }

// FSPLinkInfo — состояние привязки ФСП ID.
type FSPLinkInfo struct {
	Linked     bool       `json:"linked"`
	FSPID      string     `json:"fsp_id,omitempty"`
	LinkedAt   *time.Time `json:"linked_at,omitempty"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	SyncError  *string    `json:"sync_error,omitempty"`
}

// CompleteFSPLogin завершает вход через ФСП ID: находит привязанного пользователя
// или регистрирует нового кандидата (нужно согласие на обработку данных).
func (s *Service) CompleteFSPLogin(ctx context.Context, id oidc.Identity, consentProcessing bool, meta RequestMeta) (Session, error) {
	if link, err := s.q.GetFSPLinkBySubject(ctx, sqlcgen.GetFSPLinkBySubjectParams{Issuer: id.Issuer, Subject: id.Subject}); err == nil {
		u, err := s.q.GetUserByID(ctx, link.UserID)
		if err != nil {
			return Session{}, domain.NotFound("user")
		}
		if u.Status != sqlcgen.UserStatusActive {
			return Session{}, domain.Forbidden("user_blocked", "Учётная запись заблокирована")
		}
		s.syncAfterLogin(ctx, u.ID, link.FspID, id.AccessToken)
		return s.openSession(ctx, u, meta)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, err
	}

	// Новый пользователь.
	if !consentProcessing {
		return Session{}, domain.Validation("consents.processing", "consent_required",
			"Для создания учётной записи через ФСП ID требуется согласие на обработку персональных данных")
	}
	email := normalizeEmail(id.Email)
	if email == "" {
		return Session{}, domain.Validation("email", "email_required", "ФСП ID не передал e-mail; зарегистрируйтесь по e-mail и привяжите ФСП ID в настройках")
	}
	var user *sqlcgen.User
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		existing, err := q.GetUserByEmail(ctx, email)
		switch {
		case err == nil:
			// E-mail уже занят. ФСП ID — доверенный провайдер федерации: если он подтвердил
			// адрес, привязываем к существующему кандидату; иначе просим войти и привязать вручную.
			if !id.EmailVerified || existing.Role != sqlcgen.UserRoleCandidate || existing.Status != sqlcgen.UserStatusActive {
				return domain.Conflict("email_taken_link_manually",
					"Этот e-mail уже зарегистрирован: войдите по паролю и привяжите ФСП ID в настройках")
			}
			if _, err := q.GetFSPLinkByUser(ctx, existing.ID); err == nil {
				return domain.Conflict("fsp_already_linked", "К этой учётной записи уже привязан другой ФСП ID")
			}
			user = existing
			if user.EmailVerifiedAt == nil {
				if err := q.SetEmailVerified(ctx, user.ID); err != nil {
					return err
				}
			}
		case errors.Is(err, pgx.ErrNoRows):
			u, err := q.CreateUser(ctx, sqlcgen.CreateUserParams{Email: email, PasswordHash: nil, Role: sqlcgen.UserRoleCandidate})
			if err != nil {
				return err
			}
			if id.EmailVerified {
				if err := q.SetEmailVerified(ctx, u.ID); err != nil {
					return err
				}
			}
			if err := q.EnsureCandidateProfile(ctx, u.ID); err != nil {
				return err
			}
			if err := q.EnsureCandidateContacts(ctx, sqlcgen.EnsureCandidateContactsParams{UserID: u.ID, Email: &email}); err != nil {
				return err
			}
			if err := q.EnsurePrivacySettings(ctx, u.ID); err != nil {
				return err
			}
			if id.GivenName != "" || id.FamilyName != "" {
				if _, err := tx.Exec(ctx, `UPDATE candidate_profiles SET first_name = $2, last_name = $3 WHERE user_id = $1`, u.ID, id.GivenName, id.FamilyName); err != nil {
					return err
				}
			}
			if _, err := q.CreateConsent(ctx, sqlcgen.CreateConsentParams{
				UserID: u.ID, Kind: sqlcgen.ConsentKindProcessing, Version: s.cfg.ConsentVersion,
				Ip: meta.ipPrefix(), UserAgent: meta.userAgent(),
			}); err != nil {
				return err
			}
			user = u
		default:
			return err
		}
		if err := s.createLink(ctx, q, user.ID, id, meta); err != nil {
			return err
		}
		return s.audit(ctx, q, &user.ID, "user.register_fsp", "user", &user.ID, map[string]any{"fsp_id": id.FSPID}, meta)
	})
	if err != nil {
		return Session{}, err
	}
	user, err = s.q.GetUserByID(ctx, user.ID)
	if err != nil {
		return Session{}, err
	}
	s.syncAfterLogin(ctx, user.ID, id.FSPID, id.AccessToken)
	return s.openSession(ctx, user, meta)
}

// LinkFSP привязывает ФСП ID к текущему пользователю.
func (s *Service) LinkFSP(ctx context.Context, userID uuid.UUID, id oidc.Identity, meta RequestMeta) (FSPLinkInfo, error) {
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if existing, err := q.GetFSPLinkByUser(ctx, userID); err == nil {
			if existing.Issuer == id.Issuer && existing.Subject == id.Subject {
				return nil // идемпотентно
			}
			return domain.Conflict("fsp_already_linked", "К учётной записи уже привязан другой ФСП ID; сначала отвяжите его")
		}
		if other, err := q.GetFSPLinkBySubject(ctx, sqlcgen.GetFSPLinkBySubjectParams{Issuer: id.Issuer, Subject: id.Subject}); err == nil && other.UserID != userID {
			return domain.Conflict("fsp_id_linked_elsewhere", "Этот ФСП ID уже привязан к другой учётной записи")
		}
		if err := s.createLink(ctx, q, userID, id, meta); err != nil {
			return err
		}
		return s.audit(ctx, q, &userID, "fsp.linked", "user", &userID, map[string]any{"fsp_id": id.FSPID}, meta)
	})
	if err != nil {
		return FSPLinkInfo{}, err
	}
	s.syncAfterLogin(ctx, userID, id.FSPID, id.AccessToken)
	return s.FSPLink(ctx, userID)
}

func (s *Service) createLink(ctx context.Context, q *sqlcgen.Queries, userID uuid.UUID, id oidc.Identity, meta RequestMeta) error {
	consent, err := q.CreateConsent(ctx, sqlcgen.CreateConsentParams{
		UserID: userID, Kind: sqlcgen.ConsentKindFspData, Version: s.cfg.ConsentVersion,
		Ip: meta.ipPrefix(), UserAgent: meta.userAgent(),
	})
	if err != nil {
		return err
	}
	_, err = q.CreateFSPLink(ctx, sqlcgen.CreateFSPLinkParams{
		UserID: userID, FspID: id.FSPID, Issuer: id.Issuer, Subject: id.Subject,
		ConsentID: uuid.NullUUID{UUID: consent.ID, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Conflict("fsp_id_linked_elsewhere", "Этот ФСП ID уже привязан к другой учётной записи")
		}
		return err
	}
	return nil
}

func (s *Service) syncAfterLogin(ctx context.Context, userID uuid.UUID, fspID, accessToken string) {
	if s.fspSync == nil || accessToken == "" {
		return
	}
	if n, err := s.fspSync.Sync(ctx, userID, fspID, accessToken); err != nil {
		s.log.Warn("fsp achievements sync failed", "user_id", userID, "err", err)
	} else {
		s.log.Info("fsp achievements synced", "user_id", userID, "count", n)
	}
}

// FSPLink возвращает состояние привязки.
func (s *Service) FSPLink(ctx context.Context, userID uuid.UUID) (FSPLinkInfo, error) {
	link, err := s.q.GetFSPLinkByUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return FSPLinkInfo{Linked: false}, nil
	}
	if err != nil {
		return FSPLinkInfo{}, err
	}
	linkedAt := link.LinkedAt
	return FSPLinkInfo{Linked: true, FSPID: link.FspID, LinkedAt: &linkedAt, LastSyncAt: link.LastSyncAt, SyncError: link.SyncError}, nil
}

// UnlinkFSP отвязывает ФСП ID, удаляет кэш достижений и отзывает согласие fsp_data.
func (s *Service) UnlinkFSP(ctx context.Context, userID uuid.UUID, meta RequestMeta) error {
	return postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.DeleteFSPLink(ctx, userID)
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.NotFound("fsp_link")
		}
		if err := q.DeleteFSPAchievements(ctx, userID); err != nil {
			return err
		}
		if _, err := q.WithdrawConsent(ctx, sqlcgen.WithdrawConsentParams{UserID: userID, Kind: sqlcgen.ConsentKindFspData}); err != nil {
			return err
		}
		return s.audit(ctx, q, &userID, "fsp.unlinked", "user", &userID, nil, meta)
	})
}
