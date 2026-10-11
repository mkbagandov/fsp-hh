// Package auth — регистрация, подтверждение e-mail, вход, обновление сессии,
// сброс пароля, согласия 152-ФЗ. Правила доступа и инварианты — docs/01-domain.md.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/domain"
	"fsp/internal/infrastructure/authtoken"
	"fsp/internal/infrastructure/mail"
	"fsp/internal/infrastructure/secrets"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Config — параметры сервиса.
type Config struct {
	RefreshTTL        time.Duration // срок refresh-токена (по умолчанию 14 дней)
	CodeTTL           time.Duration // срок кода подтверждения (15 минут)
	CodeMaxAttempts   int           // попыток ввода кода (5)
	ResendLimit       int           // писем за окно (3)
	ResendWindow      time.Duration // окно для лимита (15 минут)
	ConsentVersion    string        // текущая версия текста согласия
	BaseURL           string        // для ссылок в письмах
	MinPasswordLength int           // 8
}

func (c Config) withDefaults() Config {
	if c.RefreshTTL <= 0 {
		c.RefreshTTL = 14 * 24 * time.Hour
	}
	if c.CodeTTL <= 0 {
		c.CodeTTL = 15 * time.Minute
	}
	if c.CodeMaxAttempts <= 0 {
		c.CodeMaxAttempts = 5
	}
	if c.ResendLimit <= 0 {
		c.ResendLimit = 3
	}
	if c.ResendWindow <= 0 {
		c.ResendWindow = 15 * time.Minute
	}
	if c.ConsentVersion == "" {
		c.ConsentVersion = "2026-10-01"
	}
	if c.MinPasswordLength <= 0 {
		c.MinPasswordLength = 8
	}
	return c
}

// Service — сценарии аутентификации.
type Service struct {
	cfg     Config
	pool    *pgxpool.Pool
	q       *sqlcgen.Queries
	tokens  *authtoken.Issuer
	mailer  mail.Mailer
	log     *slog.Logger
	now     func() time.Time
	fspSync FSPSyncer
}

// New собирает сервис.
func New(cfg Config, pool *pgxpool.Pool, tokens *authtoken.Issuer, mailer mail.Mailer, log *slog.Logger) *Service {
	return &Service{
		cfg:    cfg.withDefaults(),
		pool:   pool,
		q:      sqlcgen.New(pool),
		tokens: tokens,
		mailer: mailer,
		log:    log,
		now:    time.Now,
	}
}

// RequestMeta — метаданные запроса для аудита и токенов.
type RequestMeta struct {
	IP        string
	UserAgent string
}

func (m RequestMeta) ipPrefix() *netip.Prefix {
	host := m.IP
	if i := strings.LastIndex(host, ":"); i > 0 && strings.Count(host, ":") == 1 {
		host = host[:i]
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return nil
	}
	p := netip.PrefixFrom(addr, addr.BitLen())
	return &p
}

func (m RequestMeta) userAgent() *string {
	if m.UserAgent == "" {
		return nil
	}
	ua := m.UserAgent
	if len(ua) > 512 {
		ua = ua[:512]
	}
	return &ua
}

// UserInfo — публичное представление пользователя.
type UserInfo struct {
	ID            uuid.UUID   `json:"id"`
	Email         string      `json:"email"`
	Role          domain.Role `json:"role" enum:"candidate,employer,admin"`
	EmailVerified bool        `json:"email_verified"`
	CreatedAt     time.Time   `json:"created_at"`
}

// Session — результат входа.
type Session struct {
	User         UserInfo
	AccessToken  string
	AccessExpiry time.Time
	RefreshToken string // сырой токен для cookie
	RefreshExp   time.Time
}

// RegisterInput — данные регистрации.
type RegisterInput struct {
	Email             string
	Password          string
	Role              domain.Role
	ProcessingConsent bool
}

// Register создаёт пользователя, записывает согласие и отправляет код подтверждения.
func (s *Service) Register(ctx context.Context, in RegisterInput, meta RequestMeta) (UserInfo, error) {
	email := normalizeEmail(in.Email)
	if err := validateEmail(email); err != nil {
		return UserInfo{}, err
	}
	if err := s.validatePassword(in.Password); err != nil {
		return UserInfo{}, err
	}
	if in.Role != domain.RoleCandidate && in.Role != domain.RoleEmployer {
		return UserInfo{}, domain.Validation("role", "role_invalid", "Роль должна быть candidate или employer")
	}
	if !in.ProcessingConsent {
		return UserInfo{}, domain.Validation("consents.processing", "consent_required",
			"Для регистрации требуется согласие на обработку персональных данных")
	}
	hash, err := secrets.HashPassword(in.Password)
	if err != nil {
		return UserInfo{}, err
	}

	var user *sqlcgen.User
	var code string
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		u, err := q.CreateUser(ctx, sqlcgen.CreateUserParams{
			Email: email, PasswordHash: &hash, Role: sqlcgen.UserRole(in.Role),
		})
		if err != nil {
			if isUniqueViolation(err) {
				return domain.Conflict("email_taken", "Пользователь с таким e-mail уже зарегистрирован")
			}
			return fmt.Errorf("create user: %w", err)
		}
		user = u
		if in.Role == domain.RoleCandidate {
			if err := q.EnsureCandidateProfile(ctx, u.ID); err != nil {
				return err
			}
			if err := q.EnsureCandidateContacts(ctx, sqlcgen.EnsureCandidateContactsParams{UserID: u.ID, Email: &email}); err != nil {
				return err
			}
			if err := q.EnsurePrivacySettings(ctx, u.ID); err != nil {
				return err
			}
		}
		if _, err := q.CreateConsent(ctx, sqlcgen.CreateConsentParams{
			UserID: u.ID, Kind: sqlcgen.ConsentKindProcessing, Version: s.cfg.ConsentVersion,
			Ip: meta.ipPrefix(), UserAgent: meta.userAgent(),
		}); err != nil {
			return err
		}
		c, err := s.issueCode(ctx, q, u.ID, sqlcgen.VerificationPurposeVerifyEmail)
		if err != nil {
			return err
		}
		code = c
		return s.audit(ctx, q, &u.ID, "user.register", "user", &u.ID, map[string]any{"role": in.Role}, meta)
	})
	if err != nil {
		return UserInfo{}, err
	}
	s.sendVerification(ctx, email, code)
	return toUserInfo(user), nil
}

// ResendVerification повторно отправляет код. Ответ одинаков для любого e-mail (без перечисления).
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil || u.EmailVerifiedAt != nil || u.Status != sqlcgen.UserStatusActive {
		return nil
	}
	var code string
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.LockUserCodes(ctx, u.ID); err != nil {
			return err
		}
		n, err := q.CountRecentVerifications(ctx, sqlcgen.CountRecentVerificationsParams{
			UserID: u.ID, Purpose: sqlcgen.VerificationPurposeVerifyEmail, Since: s.now().Add(-s.cfg.ResendWindow),
		})
		if err != nil {
			return err
		}
		if int(n) >= s.cfg.ResendLimit {
			return domain.E(domain.KindRateLimited, "resend_limit", "Слишком много запросов кода, попробуйте позже")
		}
		code, err = s.issueCode(ctx, q, u.ID, sqlcgen.VerificationPurposeVerifyEmail)
		return err
	})
	if err != nil {
		return err
	}
	s.sendVerification(ctx, email, code)
	return nil
}

// VerifyEmail проверяет код подтверждения и при успехе открывает сессию.
//
// Для уже подтверждённого адреса сессия не открывается никогда: иначе вход выполнялся бы по одному
// e-mail без пароля и кода. Ответ при этом тот же, что и на неверный код, — без перечисления аккаунтов.
func (s *Service) VerifyEmail(ctx context.Context, email, code string, meta RequestMeta) (Session, error) {
	email = normalizeEmail(email)
	invalid := domain.Validation("code", "code_invalid", "Неверный или просроченный код")
	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil || u.EmailVerifiedAt != nil || u.Status != sqlcgen.UserStatusActive {
		return Session{}, invalid
	}
	if err := s.consumeCode(ctx, u.ID, sqlcgen.VerificationPurposeVerifyEmail, code); err != nil {
		return Session{}, err
	}
	if err := s.q.SetEmailVerified(ctx, u.ID); err != nil {
		return Session{}, err
	}
	_ = s.audit(ctx, s.q, &u.ID, "user.email_verified", "user", &u.ID, nil, meta)
	u, err = s.q.GetUserByID(ctx, u.ID)
	if err != nil {
		return Session{}, err
	}
	return s.openSession(ctx, u, meta)
}

// Login проверяет пароль и открывает сессию.
func (s *Service) Login(ctx context.Context, email, password string, meta RequestMeta) (Session, error) {
	email = normalizeEmail(email)
	invalid := domain.E(domain.KindUnauthorized, "invalid_credentials", "Неверный e-mail или пароль")
	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil || u.PasswordHash == nil {
		// Выравниваем время ответа, чтобы не выдавать существование аккаунта.
		_, _ = secrets.VerifyPassword(password, dummyHash)
		return Session{}, invalid
	}
	ok, err := secrets.VerifyPassword(password, *u.PasswordHash)
	if err != nil || !ok {
		return Session{}, invalid
	}
	if u.Status != sqlcgen.UserStatusActive {
		return Session{}, domain.Forbidden("user_blocked", "Учётная запись заблокирована")
	}
	if u.EmailVerifiedAt == nil {
		return Session{}, domain.Forbidden("email_not_verified", "Подтвердите e-mail: код отправлен при регистрации")
	}
	return s.openSession(ctx, u, meta)
}

// Refresh ротирует refresh-токен и выпускает новый access-токен.
func (s *Service) Refresh(ctx context.Context, rawRefresh string, meta RequestMeta) (Session, error) {
	invalid := domain.E(domain.KindUnauthorized, "refresh_invalid", "Сессия недействительна, войдите заново")
	if rawRefresh == "" {
		return Session{}, invalid
	}
	rt, err := s.q.GetRefreshTokenByHash(ctx, secrets.HashToken("refresh", rawRefresh))
	if err != nil {
		return Session{}, invalid
	}
	if rt.RevokedAt != nil {
		// Повторное использование отозванного токена — признак кражи: закрываем все сессии.
		_ = s.q.RevokeAllRefreshTokens(ctx, rt.UserID)
		_ = s.audit(ctx, s.q, &rt.UserID, "session.reuse_detected", "user", &rt.UserID, nil, meta)
		return Session{}, invalid
	}
	if rt.ExpiresAt.Before(s.now()) {
		return Session{}, invalid
	}
	u, err := s.q.GetUserByID(ctx, rt.UserID)
	if err != nil || u.Status != sqlcgen.UserStatusActive {
		return Session{}, invalid
	}
	var sess Session
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		next, err := s.createRefresh(ctx, q, u.ID, meta)
		if err != nil {
			return err
		}
		n, err := q.RotateRefreshToken(ctx, sqlcgen.RotateRefreshTokenParams{
			ID: rt.ID, ReplacedBy: uuid.NullUUID{UUID: next.id, Valid: true},
		})
		if err != nil {
			return err
		}
		if n != 1 {
			// Параллельный запрос уже обменял этот токен: вторая цепочка сессий не создаётся.
			return invalid
		}
		sess, err = s.buildSession(u, next)
		return err
	})
	return sess, err
}

// Logout отзывает refresh-токен текущей сессии.
func (s *Service) Logout(ctx context.Context, rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}
	rt, err := s.q.GetRefreshTokenByHash(ctx, secrets.HashToken("refresh", rawRefresh))
	if err != nil {
		return nil
	}
	return s.q.RevokeRefreshToken(ctx, sqlcgen.RevokeRefreshTokenParams{ID: rt.ID})
}

// Me возвращает пользователя и его действующие согласия.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (UserInfo, []ConsentInfo, error) {
	u, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return UserInfo{}, nil, domain.NotFound("user")
	}
	rows, err := s.q.ListUserConsents(ctx, userID)
	if err != nil {
		return UserInfo{}, nil, err
	}
	consents := make([]ConsentInfo, 0, len(rows))
	for _, c := range rows {
		consents = append(consents, toConsentInfo(c))
	}
	return toUserInfo(u), consents, nil
}

// ChangePassword меняет пароль и закрывает остальные сессии.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, current, next string, meta RequestMeta) error {
	u, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return domain.NotFound("user")
	}
	if u.PasswordHash != nil {
		ok, _ := secrets.VerifyPassword(current, *u.PasswordHash)
		if !ok {
			return domain.Validation("current_password", "password_wrong", "Текущий пароль указан неверно")
		}
	}
	if err := s.validatePassword(next); err != nil {
		return err
	}
	hash, err := secrets.HashPassword(next)
	if err != nil {
		return err
	}
	return postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdateUserPassword(ctx, sqlcgen.UpdateUserPasswordParams{ID: userID, PasswordHash: &hash}); err != nil {
			return err
		}
		if err := q.RevokeAllRefreshTokens(ctx, userID); err != nil {
			return err
		}
		return s.audit(ctx, q, &userID, "user.password_changed", "user", &userID, nil, meta)
	})
}

// ForgotPassword отправляет код сброса. Ответ одинаков для любого e-mail.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil || u.Status != sqlcgen.UserStatusActive {
		return nil
	}
	var code string
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.LockUserCodes(ctx, u.ID); err != nil {
			return err
		}
		n, err := q.CountRecentVerifications(ctx, sqlcgen.CountRecentVerificationsParams{
			UserID: u.ID, Purpose: sqlcgen.VerificationPurposeResetPassword, Since: s.now().Add(-s.cfg.ResendWindow),
		})
		if err != nil {
			return err
		}
		if int(n) >= s.cfg.ResendLimit {
			return domain.E(domain.KindRateLimited, "resend_limit", "Слишком много запросов кода, попробуйте позже")
		}
		code, err = s.issueCode(ctx, q, u.ID, sqlcgen.VerificationPurposeResetPassword)
		return err
	})
	if err != nil {
		return err
	}
	_ = s.mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: "ФСП: сброс пароля",
		Text: fmt.Sprintf("Код для сброса пароля: %s\nКод действует %d минут. Если вы не запрашивали сброс, проигнорируйте письмо.",
			code, int(s.cfg.CodeTTL.Minutes())),
	})
	return nil
}

// ResetPassword устанавливает новый пароль по коду и закрывает все сессии.
func (s *Service) ResetPassword(ctx context.Context, email, code, next string, meta RequestMeta) error {
	email = normalizeEmail(email)
	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return domain.Validation("code", "code_invalid", "Неверный или просроченный код")
	}
	if err := s.validatePassword(next); err != nil {
		return err
	}
	if err := s.consumeCode(ctx, u.ID, sqlcgen.VerificationPurposeResetPassword, code); err != nil {
		return err
	}
	hash, err := secrets.HashPassword(next)
	if err != nil {
		return err
	}
	return postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdateUserPassword(ctx, sqlcgen.UpdateUserPasswordParams{ID: u.ID, PasswordHash: &hash}); err != nil {
			return err
		}
		if u.EmailVerifiedAt == nil {
			// Владение почтой доказано кодом — считаем e-mail подтверждённым.
			if err := q.SetEmailVerified(ctx, u.ID); err != nil {
				return err
			}
		}
		if err := q.RevokeAllRefreshTokens(ctx, u.ID); err != nil {
			return err
		}
		return s.audit(ctx, q, &u.ID, "user.password_reset", "user", &u.ID, nil, meta)
	})
}

// ConsentInfo — согласие в ответе API.
type ConsentInfo struct {
	Kind        domain.ConsentKind `json:"kind" enum:"processing,publication,fsp_data"`
	Version     string             `json:"version"`
	AcceptedAt  time.Time          `json:"accepted_at"`
	WithdrawnAt *time.Time         `json:"withdrawn_at,omitempty"`
}

// GiveConsent фиксирует согласие заданного вида (повторная выдача — новая запись).
func (s *Service) GiveConsent(ctx context.Context, userID uuid.UUID, kind domain.ConsentKind, meta RequestMeta) (ConsentInfo, error) {
	switch kind {
	case domain.ConsentProcessing, domain.ConsentPublication, domain.ConsentFSPData:
	default:
		return ConsentInfo{}, domain.Validation("kind", "consent_kind_invalid", "Неизвестный вид согласия")
	}
	var out ConsentInfo
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.WithdrawConsent(ctx, sqlcgen.WithdrawConsentParams{UserID: userID, Kind: sqlcgen.ConsentKind(kind)}); err != nil {
			return err
		}
		c, err := q.CreateConsent(ctx, sqlcgen.CreateConsentParams{
			UserID: userID, Kind: sqlcgen.ConsentKind(kind), Version: s.cfg.ConsentVersion,
			Ip: meta.ipPrefix(), UserAgent: meta.userAgent(),
		})
		if err != nil {
			return err
		}
		out = toConsentInfo(c)
		return s.audit(ctx, q, &userID, "consent.given", "consent", &c.ID, map[string]any{"kind": kind}, meta)
	})
	return out, err
}

// WithdrawConsent отзывает согласие. Согласие на обработку отозвать нельзя — только удалить аккаунт.
func (s *Service) WithdrawConsent(ctx context.Context, userID uuid.UUID, kind domain.ConsentKind, meta RequestMeta) error {
	if kind == domain.ConsentProcessing {
		return domain.Conflict("consent_processing_required",
			"Согласие на обработку данных необходимо для работы аккаунта; вместо отзыва удалите аккаунт")
	}
	return postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.WithdrawConsent(ctx, sqlcgen.WithdrawConsentParams{UserID: userID, Kind: sqlcgen.ConsentKind(kind)})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.NotFound("consent")
		}
		return s.audit(ctx, q, &userID, "consent.withdrawn", "user", &userID, map[string]any{"kind": kind}, meta)
	})
}

// wipeStatements удаляют или обезличивают персональные данные удаляемого аккаунта (152-ФЗ):
// контакты, резюме, опыт, образование, навыки, данные ФСП, анкеты и уведомления; профиль скрывается,
// гранты на контакты отзываются, активные приглашения и отклики закрываются. Результаты тестов
// остаются без привязки к личности: на них держится калибровка банка заданий.
var wipeStatements = []string{
	`DELETE FROM candidate_contacts WHERE user_id = $1`,
	`DELETE FROM candidate_experiences WHERE user_id = $1`,
	`DELETE FROM candidate_education WHERE user_id = $1`,
	`DELETE FROM candidate_skills WHERE user_id = $1`,
	`DELETE FROM resume_uploads WHERE user_id = $1`,
	`DELETE FROM fsp_achievements WHERE user_id = $1`,
	`DELETE FROM fsp_links WHERE user_id = $1`,
	`DELETE FROM surveys WHERE user_id = $1`,
	`DELETE FROM notifications WHERE user_id = $1`,
	`UPDATE candidate_profiles SET first_name = '', last_name = '', middle_name = NULL, city = NULL, headline = NULL,
	   about = NULL, salary_min = NULL, salary_expected = NULL, links = '{}'::jsonb, resume_pdf_path = NULL,
	   visibility = 'hidden' WHERE user_id = $1`,
	`UPDATE contact_grants SET revoked_at = now() WHERE candidate_user_id = $1 AND revoked_at IS NULL`,
	`UPDATE invitations SET status = 'expired' WHERE candidate_user_id = $1 AND status IN ('sent', 'viewed')`,
	`UPDATE applications SET status = 'withdrawn' WHERE candidate_user_id = $1 AND status IN ('sent', 'viewed', 'invited')`,
	`UPDATE users SET email = 'deleted+' || id::text || '@invalid.local', password_hash = NULL WHERE id = $1`,
}

// DeleteAccount удаляет аккаунт: статус deleted, обезличенный e-mail, закрытые сессии и стёртые
// персональные данные (см. wipeStatements). Файлы резюме удаляются с диска после фиксации транзакции.
func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID, meta RequestMeta) error {
	var files []string
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		rows, err := tx.Query(ctx, `SELECT storage_path FROM resume_uploads WHERE user_id = $1`, userID)
		if err != nil {
			return err
		}
		files, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if err := q.UpdateUserStatus(ctx, sqlcgen.UpdateUserStatusParams{ID: userID, Status: sqlcgen.UserStatusDeleted}); err != nil {
			return err
		}
		for _, stmt := range wipeStatements {
			if _, err := tx.Exec(ctx, stmt, userID); err != nil {
				return fmt.Errorf("wipe account data: %w", err)
			}
		}
		if err := q.RevokeAllRefreshTokens(ctx, userID); err != nil {
			return err
		}
		return s.audit(ctx, q, &userID, "user.deleted", "user", &userID, map[string]any{"resume_files": len(files)}, meta)
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("resume file not removed", "user_id", userID, "err", err)
		}
	}
	return nil
}

// ExportAccount возвращает данные учётной записи и согласий в машиночитаемом виде (152-ФЗ).
func (s *Service) ExportAccount(ctx context.Context, userID uuid.UUID) (map[string]any, error) {
	u, consents, err := s.Me(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"user": u, "consents": consents, "exported_at": s.now().UTC()}, nil
}

// --- внутренние помощники ---

type refreshRecord struct {
	id  uuid.UUID
	raw string
	exp time.Time
}

func (s *Service) openSession(ctx context.Context, u *sqlcgen.User, meta RequestMeta) (Session, error) {
	if u.Status != sqlcgen.UserStatusActive {
		return Session{}, domain.Forbidden("user_blocked", "Учётная запись заблокирована")
	}
	var sess Session
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		rec, err := s.createRefresh(ctx, q, u.ID, meta)
		if err != nil {
			return err
		}
		if err := q.TouchLastLogin(ctx, u.ID); err != nil {
			return err
		}
		if err := s.audit(ctx, q, &u.ID, "user.login", "user", &u.ID, nil, meta); err != nil {
			return err
		}
		sess, err = s.buildSession(u, rec)
		return err
	})
	return sess, err
}

func (s *Service) createRefresh(ctx context.Context, q *sqlcgen.Queries, userID uuid.UUID, meta RequestMeta) (refreshRecord, error) {
	raw, err := secrets.RandomToken(32)
	if err != nil {
		return refreshRecord{}, err
	}
	exp := s.now().Add(s.cfg.RefreshTTL)
	row, err := q.CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		UserID: userID, TokenHash: secrets.HashToken("refresh", raw), ExpiresAt: exp,
		UserAgent: meta.userAgent(), Ip: meta.ipPrefix(),
	})
	if err != nil {
		return refreshRecord{}, fmt.Errorf("create refresh: %w", err)
	}
	return refreshRecord{id: row.ID, raw: raw, exp: exp}, nil
}

func (s *Service) buildSession(u *sqlcgen.User, rec refreshRecord) (Session, error) {
	p := domain.Principal{UserID: u.ID, Role: domain.Role(u.Role), Email: u.Email}
	access, exp, err := s.tokens.Issue(p)
	if err != nil {
		return Session{}, err
	}
	return Session{
		User: toUserInfo(u), AccessToken: access, AccessExpiry: exp,
		RefreshToken: rec.raw, RefreshExp: rec.exp,
	}, nil
}

func (s *Service) issueCode(ctx context.Context, q *sqlcgen.Queries, userID uuid.UUID, purpose sqlcgen.VerificationPurpose) (string, error) {
	if err := q.InvalidateVerifications(ctx, sqlcgen.InvalidateVerificationsParams{UserID: userID, Purpose: purpose}); err != nil {
		return "", err
	}
	code, err := secrets.NumericCode(6)
	if err != nil {
		return "", err
	}
	_, err = q.CreateVerification(ctx, sqlcgen.CreateVerificationParams{
		UserID: userID, Purpose: purpose,
		CodeHash:  secrets.HashToken(userID.String(), code),
		ExpiresAt: s.now().Add(s.cfg.CodeTTL),
	})
	if err != nil {
		return "", fmt.Errorf("create verification: %w", err)
	}
	return code, nil
}

func (s *Service) consumeCode(ctx context.Context, userID uuid.UUID, purpose sqlcgen.VerificationPurpose, code string) error {
	invalid := domain.Validation("code", "code_invalid", "Неверный или просроченный код")
	// Попытка засчитывается до сравнения и атомарно: параллельные запросы не обходят лимит.
	v, err := s.q.AttemptVerification(ctx, sqlcgen.AttemptVerificationParams{
		UserID: userID, Purpose: purpose, MaxAttempts: int32(s.cfg.CodeMaxAttempts), //nolint:gosec // лимит — малое число из конфига
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if active, gerr := s.q.GetActiveVerification(ctx, sqlcgen.GetActiveVerificationParams{UserID: userID, Purpose: purpose}); gerr == nil &&
			int(active.Attempts) >= s.cfg.CodeMaxAttempts {
			_, _ = s.q.ConsumeVerification(ctx, active.ID)
			return domain.E(domain.KindRateLimited, "code_attempts_exceeded", "Превышено число попыток, запросите новый код")
		}
		return invalid
	}
	if err != nil {
		return err
	}
	if !secrets.EqualHash(v.CodeHash, secrets.HashToken(userID.String(), strings.TrimSpace(code))) {
		if int(v.Attempts) >= s.cfg.CodeMaxAttempts {
			_, _ = s.q.ConsumeVerification(ctx, v.ID)
		}
		return invalid
	}
	// Верный код погашается ровно одним запросом.
	n, err := s.q.ConsumeVerification(ctx, v.ID)
	if err != nil {
		return err
	}
	if n != 1 {
		return invalid
	}
	return nil
}

func (s *Service) sendVerification(ctx context.Context, email, code string) {
	link := ""
	if s.cfg.BaseURL != "" {
		link = fmt.Sprintf("\nИли перейдите по ссылке: %s/verify?email=%s&code=%s", strings.TrimRight(s.cfg.BaseURL, "/"), email, code)
	}
	if err := s.mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: "ФСП: подтверждение e-mail",
		Text: fmt.Sprintf("Ваш код подтверждения: %s\nКод действует %d минут.%s",
			code, int(s.cfg.CodeTTL.Minutes()), link),
	}); err != nil {
		s.log.Error("send verification mail", "err", err)
	}
}

func (s *Service) audit(ctx context.Context, q *sqlcgen.Queries, actor *uuid.UUID, action, objectType string, objectID *uuid.UUID, details map[string]any, meta RequestMeta) error {
	var actorID, objID uuid.NullUUID
	if actor != nil {
		actorID = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	if objectID != nil {
		objID = uuid.NullUUID{UUID: *objectID, Valid: true}
	}
	raw := json.RawMessage(`{}`)
	if details != nil {
		b, err := json.Marshal(details)
		if err == nil {
			raw = b
		}
	}
	return q.InsertAudit(ctx, sqlcgen.InsertAuditParams{
		ActorUserID: actorID, Action: action, ObjectType: objectType, ObjectID: objID, Details: raw, Ip: meta.ipPrefix(),
	})
}

func (s *Service) validatePassword(p string) error {
	if len(p) < s.cfg.MinPasswordLength {
		return domain.Validation("password", "password_too_short",
			fmt.Sprintf("Пароль должен быть не короче %d символов", s.cfg.MinPasswordLength))
	}
	if len(p) > 128 {
		return domain.Validation("password", "password_too_long", "Пароль слишком длинный")
	}
	return nil
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func validateEmail(e string) error {
	at := strings.LastIndex(e, "@")
	if len(e) < 5 || len(e) > 254 || at < 1 || at == len(e)-1 || !strings.Contains(e[at:], ".") || strings.ContainsAny(e, " \t\n") {
		return domain.Validation("email", "email_invalid", "Укажите корректный e-mail")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func toUserInfo(u *sqlcgen.User) UserInfo {
	return UserInfo{
		ID: u.ID, Email: u.Email, Role: domain.Role(u.Role),
		EmailVerified: u.EmailVerifiedAt != nil, CreatedAt: u.CreatedAt,
	}
}

func toConsentInfo(c *sqlcgen.Consent) ConsentInfo {
	return ConsentInfo{Kind: domain.ConsentKind(c.Kind), Version: c.Version, AcceptedAt: c.AcceptedAt, WithdrawnAt: c.WithdrawnAt}
}

// dummyHash используется для выравнивания времени ответа при неизвестном e-mail.
var dummyHash = func() string {
	h, _ := secrets.HashPassword("dummy-password-for-timing")
	return h
}()

// IsActive сообщает, активна ли учётная запись: используется HTTP-слоем, чтобы блокировка
// и удаление закрывали уже выданные access-токены (с кэшем на несколько секунд).
func (s *Service) IsActive(ctx context.Context, userID uuid.UUID) (bool, error) {
	u, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return u.Status == sqlcgen.UserStatusActive, nil
}
