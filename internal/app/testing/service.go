// Package testing — опрос, адаптивный тест, решение по грейду и категория кандидата
// (docs/04-testing.md). Банк семейств — pkg/itembank, модель — pkg/irt.
//
// Инварианты (docs/01-domain.md): категория меняется только здесь по результату сессии;
// грейд не понижается принудительно; смена грейда ограничена cooldown; ответ на задание
// не отдаётся наружу до завершения сессии.
package testing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
	"fsp/pkg/irt"
	"fsp/pkg/itembank"
)

// Config — параметры сервиса.
type Config struct {
	SessionTTL      time.Duration // общий лимит сессии (45 минут)
	GradeCooldown   time.Duration // интервал между попытками сменить грейд (90 дней)
	ConfirmMinP     float64       // порог подтверждения (0.70)
	UpgradeMinP     float64       // порог предложения повышения (0.80)
	AnswerGrace     time.Duration // допуск к дедлайну задания (5 секунд)
	CAT             irt.Config
	MinBankFamilies int // минимум семейств в пуле теста (специализация + общий блок) (8)
	// MinSpecFamilies — минимум собственных семейств специализации без общего блока (10): без них
	// тест проверял бы только общие знания, и категория «QA» или «ИБ» означала бы не то, что написано.
	MinSpecFamilies int
}

func (c Config) withDefaults() Config {
	if c.SessionTTL <= 0 {
		c.SessionTTL = 45 * time.Minute
	}
	if c.GradeCooldown <= 0 {
		c.GradeCooldown = 90 * 24 * time.Hour
	}
	if c.ConfirmMinP <= 0 {
		c.ConfirmMinP = 0.70
	}
	if c.UpgradeMinP <= 0 {
		c.UpgradeMinP = 0.80
	}
	if c.AnswerGrace <= 0 {
		c.AnswerGrace = 5 * time.Second
	}
	if c.CAT.MaxItems == 0 {
		c.CAT = irt.DefaultConfig()
	}
	if c.MinBankFamilies <= 0 {
		c.MinBankFamilies = 8
	}
	if c.MinSpecFamilies <= 0 {
		c.MinSpecFamilies = DefaultMinSpecFamilies
	}
	return c
}

// Service — сценарии тестирования.
type Service struct {
	cfg    Config
	pool   *pgxpool.Pool
	q      *sqlcgen.Queries
	engine *irt.Engine
	log    *slog.Logger
	now    func() time.Time
}

// New создаёт сервис.
func New(cfg Config, pool *pgxpool.Pool, log *slog.Logger) *Service {
	cfg = cfg.withDefaults()
	return &Service{cfg: cfg, pool: pool, q: sqlcgen.New(pool), engine: irt.NewEngine(cfg.CAT), log: log, now: time.Now}
}

// Bands — полосы грейдов из справочника.
func Bands() []irt.Band {
	out := make([]irt.Band, 0, len(refdata.Grades))
	for _, g := range refdata.Grades {
		out = append(out, irt.Band{Code: g.Code, Lower: g.LowerTheta, MinConfirm: g.MinConfirmTheta})
	}
	return out
}

// SyncFamilies переносит паспорта зарегистрированных семейств в item_families
// (калиброванные параметры семейств с показами сохраняются).
func SyncFamilies(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	q := sqlcgen.New(pool)
	n := 0
	for _, f := range itembank.All() {
		m := f.Meta()
		if err := q.UpsertItemFamily(ctx, sqlcgen.UpsertItemFamilyParams{
			ID: m.ID, Spec: sqlcgen.SpecializationCode(m.Spec), Topic: m.Topic, Grade: sqlcgen.GradeCode(m.Grade),
			Format: string(m.Format), A: m.IRT.A, B: m.IRT.B, C: m.IRT.C,
		}); err != nil {
			return n, fmt.Errorf("sync family %s: %w", m.ID, err)
		}
		n++
	}
	return n, nil
}

// --- Опрос ---

// SurveyInput — анкета при регистрации.
type SurveyInput struct {
	Specialization  string
	Industry        string
	PrimaryStack    []string
	ClaimedGrade    string
	ExperienceYears *float64
}

// Survey — сохранённая анкета.
type Survey struct {
	ID              uuid.UUID `json:"id"`
	Specialization  string    `json:"specialization"`
	Industry        string    `json:"industry,omitempty"`
	PrimaryStack    []string  `json:"primary_stack"`
	ClaimedGrade    string    `json:"claimed_grade"`
	ExperienceYears *float64  `json:"experience_years,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// SubmitSurvey сохраняет анкету; специализация профиля заполняется, если была пустой.
func (s *Service) SubmitSurvey(ctx context.Context, userID uuid.UUID, in SurveyInput) (Survey, error) {
	if !refdata.IsSelectableSpecialization(in.Specialization) {
		return Survey{}, domain.Validation("specialization", "specialization_invalid", "Выберите специализацию из справочника")
	}
	if !refdata.IsGrade(in.ClaimedGrade) {
		return Survey{}, domain.Validation("claimed_grade", "grade_invalid", "Выберите грейд: junior, middle или senior")
	}
	if in.ExperienceYears != nil && (*in.ExperienceYears < 0 || *in.ExperienceYears > 60) {
		return Survey{}, domain.Validation("experience_years", "experience_years_invalid", "Стаж указан некорректно")
	}
	if len(in.PrimaryStack) > 20 {
		in.PrimaryStack = in.PrimaryStack[:20]
	}
	stack := in.PrimaryStack
	if stack == nil {
		stack = []string{}
	}
	var industry *string
	if in.Industry != "" {
		v := in.Industry
		industry = &v
	}
	var out Survey
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		row, err := q.CreateSurvey(ctx, sqlcgen.CreateSurveyParams{
			UserID: userID, Specialization: sqlcgen.SpecializationCode(in.Specialization), Industry: industry,
			PrimaryStack: stack, ClaimedGrade: sqlcgen.GradeCode(in.ClaimedGrade), ExperienceYears: in.ExperienceYears,
		})
		if err != nil {
			return err
		}
		out = toSurvey(row)
		return q.SetProfileSpecializationIfEmpty(ctx, sqlcgen.SetProfileSpecializationIfEmptyParams{
			UserID: userID, PrimarySpecialization: ptrSpec(in.Specialization),
		})
	})
	return out, err
}

// LatestSurvey возвращает последнюю анкету по специализации.
func (s *Service) LatestSurvey(ctx context.Context, userID uuid.UUID, spec string) (Survey, error) {
	row, err := s.q.GetLatestSurvey(ctx, sqlcgen.GetLatestSurveyParams{UserID: userID, Specialization: sqlcgen.SpecializationCode(spec)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Survey{}, domain.NotFound("survey")
	}
	if err != nil {
		return Survey{}, err
	}
	return toSurvey(row), nil
}

// --- Сессии ---

// SessionView — публичное состояние сессии.
type SessionView struct {
	ID             uuid.UUID    `json:"id"`
	Specialization string       `json:"specialization"`
	TargetGrade    string       `json:"target_grade"`
	Status         string       `json:"status" enum:"created,in_progress,completed,abandoned,expired"`
	ItemsAnswered  int          `json:"items_answered"`
	MinItems       int          `json:"min_items"`
	MaxItems       int          `json:"max_items"`
	ExpiresAt      time.Time    `json:"expires_at"`
	StartedAt      *time.Time   `json:"started_at,omitempty"`
	FinishedAt     *time.Time   `json:"finished_at,omitempty"`
	Result         *Result      `json:"result,omitempty" doc:"Только после завершения"`
	Review         []ItemReview `json:"review,omitempty" doc:"Разбор заданий — только после завершения"`
}

// Result — итог теста.
type Result struct {
	Theta         float64    `json:"theta" doc:"Оценка способности на единой шкале"`
	SE            float64    `json:"se" doc:"Стандартная ошибка оценки"`
	Percentile    float64    `json:"percentile" doc:"Доля кандидатов специализации с меньшей θ, 0..1"`
	Decision      string     `json:"decision" enum:"confirmed,not_confirmed,upgrade_suggested"`
	ResultGrade   string     `json:"result_grade,omitempty" doc:"Подтверждённый грейд"`
	PConfirm      float64    `json:"p_confirm" doc:"Вероятность, что θ не ниже заявленного грейда"`
	PUpgrade      float64    `json:"p_upgrade" doc:"Вероятность, что θ не ниже следующего грейда"`
	CorrectCount  int        `json:"correct_count"`
	ItemsCount    int        `json:"items_count"`
	NextAllowedAt *time.Time `json:"next_test_allowed_at,omitempty"`
	Message       string     `json:"message"`
}

// ItemReview — разбор задания после завершения.
type ItemReview struct {
	Position    int                  `json:"position"`
	Topic       string               `json:"topic"`
	Format      string               `json:"format"`
	Stem        string               `json:"stem"`
	Code        *itembank.CodeBlock  `json:"code,omitempty"`
	Options     []itembank.Option    `json:"options,omitempty"`
	Answer      itembank.Answer      `json:"answer"`
	Submission  *itembank.Submission `json:"submission,omitempty"`
	IsCorrect   *bool                `json:"is_correct,omitempty"`
	Explanation string               `json:"explanation"`
}

// ItemView — задание для кандидата (без ответа и пояснения).
type ItemView struct {
	ID            uuid.UUID           `json:"id"`
	Position      int                 `json:"position"`
	Topic         string              `json:"topic"`
	Format        string              `json:"format" enum:"single_choice,multi_choice,numeric,short_text,code_output,ordering,sql_result"`
	Stem          string              `json:"stem"`
	Code          *itembank.CodeBlock `json:"code,omitempty"`
	Options       []itembank.Option   `json:"options,omitempty"`
	TimeLimitSec  int                 `json:"time_limit_sec"`
	ServedAt      time.Time           `json:"served_at"`
	DeadlineAt    time.Time           `json:"deadline_at"`
	ItemsAnswered int                 `json:"items_answered"`
	MaxItems      int                 `json:"max_items"`
}

// StartInput — запуск теста.
type StartInput struct {
	Specialization string
	TargetGrade    string
}

// StartTest создаёт сессию (или возвращает активную) и выдаёт первое задание.
func (s *Service) StartTest(ctx context.Context, userID uuid.UUID, in StartInput) (SessionView, *ItemView, error) {
	if !refdata.IsSelectableSpecialization(in.Specialization) {
		return SessionView{}, nil, domain.Validation("specialization", "specialization_invalid", "Выберите специализацию из справочника")
	}
	if !refdata.IsGrade(in.TargetGrade) {
		return SessionView{}, nil, domain.Validation("target_grade", "grade_invalid", "Выберите грейд")
	}
	if active, err := s.q.GetActiveTestSession(ctx, userID); err == nil {
		if s.expireIfNeeded(ctx, active) {
			// истекла — продолжаем создание новой
		} else {
			view, err := s.sessionView(ctx, active, false)
			if err != nil {
				return SessionView{}, nil, err
			}
			item, err := s.NextItem(ctx, userID, active.ID)
			return view, item, err
		}
	}
	if _, err := s.q.GetLatestSurvey(ctx, sqlcgen.GetLatestSurveyParams{UserID: userID, Specialization: sqlcgen.SpecializationCode(in.Specialization)}); err != nil {
		return SessionView{}, nil, domain.Conflict("survey_required", "Сначала пройдите опрос по специализации")
	}
	if err := s.checkEligibility(ctx, userID, in.Specialization, in.TargetGrade); err != nil {
		return SessionView{}, nil, err
	}
	fams := itembank.BySpec(itembank.Spec(in.Specialization))
	if len(fams) < s.cfg.MinBankFamilies || specificFamilies(in.Specialization) < s.cfg.MinSpecFamilies {
		return SessionView{}, nil, domain.Conflict("bank_too_small",
			"Банк заданий по этой специализации ещё формируется; тест доступен для: "+testableNames(s.cfg.MinSpecFamilies))
	}
	survey, _ := s.q.GetLatestSurvey(ctx, sqlcgen.GetLatestSurveyParams{UserID: userID, Specialization: sqlcgen.SpecializationCode(in.Specialization)})
	seed := int64(rand.Uint64() >> 1) //nolint:gosec // seed сессии нужен только для детерминизма вариантов
	row, err := s.q.CreateTestSession(ctx, sqlcgen.CreateTestSessionParams{
		UserID: userID, SurveyID: uuid.NullUUID{UUID: survey.ID, Valid: true},
		Specialization: sqlcgen.SpecializationCode(in.Specialization), TargetGrade: sqlcgen.GradeCode(in.TargetGrade),
		Seed: seed, ExpiresAt: s.now().Add(s.cfg.SessionTTL),
	})
	if err != nil {
		return SessionView{}, nil, fmt.Errorf("create session: %w", err)
	}
	view, err := s.sessionView(ctx, row, false)
	if err != nil {
		return SessionView{}, nil, err
	}
	item, err := s.NextItem(ctx, userID, row.ID)
	return view, item, err
}

// checkEligibility реализует правила cooldown и запрета бессмысленных тестов.
func (s *Service) checkEligibility(ctx context.Context, userID uuid.UUID, spec, target string) error {
	cat, err := s.q.GetCandidateCategory(ctx, sqlcgen.GetCandidateCategoryParams{UserID: userID, Specialization: sqlcgen.SpecializationCode(spec)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	cur := string(cat.Grade)
	targetOrder, curOrder := refdata.GradeOrder(target), refdata.GradeOrder(cur)
	cooldownOK := cat.NextTestAllowedAt == nil || !s.now().Before(*cat.NextTestAllowedAt)
	switch domain.GradeStatus(cat.Status) {
	case domain.GradeNotConfirmed:
		if targetOrder < curOrder {
			return nil // ниже — сразу
		}
		if !cooldownOK {
			return cooldownErr(cat.NextTestAllowedAt)
		}
		return nil
	case domain.GradeConfirmed, domain.GradeUpgradeSuggested:
		if targetOrder <= curOrder {
			return domain.Conflict("grade_already_confirmed", fmt.Sprintf("Грейд %s уже подтверждён; пройти тест можно на более высокий грейд", refdataGradeName(cur)))
		}
		if cat.Status == sqlcgen.GradeStatusUpgradeSuggested || cooldownOK {
			return nil
		}
		return cooldownErr(cat.NextTestAllowedAt)
	}
	return nil
}

func cooldownErr(until *time.Time) error {
	msg := "Повторный тест на этот или более высокий грейд пока недоступен"
	if until != nil {
		msg += " (до " + until.Format("02.01.2006") + ")"
	}
	return domain.E(domain.KindCooldown, "grade_cooldown", msg)
}

// NextItem возвращает текущее неотвеченное задание или выдаёт новое; при выполнении
// правила остановки завершает сессию и возвращает nil.
func (s *Service) NextItem(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID) (*ItemView, error) {
	sess, err := s.ownSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if s.expireIfNeeded(ctx, sess) {
		return nil, domain.Conflict("session_expired", "Время сессии истекло; результат зафиксирован по отвеченным заданиям")
	}
	if !domain.TestSessionStatus(sess.Status).IsOpen() {
		return nil, domain.Conflict("session_closed", "Сессия завершена")
	}
	items, err := s.q.ListTestItems(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.AnsweredAt == nil {
			return s.itemView(it, len(items)-1), nil
		}
	}
	st, pool, err := s.rebuildState(ctx, sess, items)
	if err != nil {
		return nil, err
	}
	if s.engine.ShouldStop(&st) {
		if err := s.finish(ctx, sess, st, items, sqlcgen.TestSessionStatusCompleted); err != nil {
			return nil, err
		}
		return nil, nil
	}
	session := rand.New(rand.NewPCG(uint64(sess.Seed), uint64(len(items)+1))) //nolint:gosec // детерминированный выбор по seed сессии
	cand, ok := s.engine.Next(&st, pool, session)
	if !ok {
		if err := s.finish(ctx, sess, st, items, sqlcgen.TestSessionStatusCompleted); err != nil {
			return nil, err
		}
		return nil, nil
	}
	fam, ok := itembank.Get(cand.ID)
	if !ok {
		return nil, fmt.Errorf("family %s not registered", cand.ID)
	}
	position := len(items) + 1
	itemSeed := uint64(sess.Seed) ^ (uint64(position) * 0x9E3779B97F4A7C15) //nolint:gosec // детерминированный seed варианта
	inst, err := fam.Generate(itemSeed)
	if err != nil {
		return nil, fmt.Errorf("generate %s: %w", cand.ID, err)
	}
	raw, _ := json.Marshal(inst)
	theta := st.Theta
	var row *sqlcgen.TestItem
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if sess.Status == sqlcgen.TestSessionStatusCreated {
			if err := q.StartTestSession(ctx, sess.ID); err != nil {
				return err
			}
		}
		r, err := q.CreateTestItem(ctx, sqlcgen.CreateTestItemParams{
			SessionID: sess.ID, Position: int32(position), FamilyID: cand.ID, Seed: int64(itemSeed >> 1), //nolint:gosec // позиция ≤ 20
			Instance: raw, DeadlineAt: s.now().Add(fam.Meta().TimeLimit + s.cfg.AnswerGrace), ThetaBefore: &theta,
		})
		if err != nil {
			return err
		}
		row = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.itemView(row, len(items)), nil
}

// AnswerResult — ответ на отправку решения.
type AnswerResult struct {
	Accepted      bool         `json:"accepted"`
	Late          bool         `json:"late" doc:"Ответ пришёл после дедлайна задания и засчитан как неверный"`
	ItemsAnswered int          `json:"items_answered"`
	Finished      bool         `json:"finished"`
	Session       *SessionView `json:"session,omitempty" doc:"Итог, если сессия завершилась"`
}

// Answer принимает решение по текущему заданию и обновляет оценку.
func (s *Service) Answer(ctx context.Context, userID, sessionID, itemID uuid.UUID, sub itembank.Submission) (AnswerResult, error) {
	sess, err := s.ownSession(ctx, userID, sessionID)
	if err != nil {
		return AnswerResult{}, err
	}
	if s.expireIfNeeded(ctx, sess) {
		return AnswerResult{}, domain.Conflict("session_expired", "Время сессии истекло")
	}
	if !domain.TestSessionStatus(sess.Status).IsOpen() {
		return AnswerResult{}, domain.Conflict("session_closed", "Сессия завершена")
	}
	item, err := s.q.GetTestItem(ctx, sqlcgen.GetTestItemParams{ID: itemID, SessionID: sess.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return AnswerResult{}, domain.NotFound("test_item")
	}
	if err != nil {
		return AnswerResult{}, err
	}
	if item.AnsweredAt != nil {
		return AnswerResult{}, domain.Conflict("item_already_answered", "Ответ на это задание уже принят")
	}
	var inst itembank.Instance
	if err := json.Unmarshal(item.Instance, &inst); err != nil {
		return AnswerResult{}, fmt.Errorf("decode instance: %w", err)
	}
	fam, ok := itembank.Get(item.FamilyID)
	if !ok {
		return AnswerResult{}, fmt.Errorf("family %s not registered", item.FamilyID)
	}
	now := s.now()
	late := now.After(item.DeadlineAt)
	correct := !late && itembank.Check(fam.Meta().Format, inst.Answer, sub)

	items, err := s.q.ListTestItems(ctx, sess.ID)
	if err != nil {
		return AnswerResult{}, err
	}
	st, _, err := s.rebuildState(ctx, sess, items)
	if err != nil {
		return AnswerResult{}, err
	}
	params := s.paramsFor(ctx, item.FamilyID, fam.Meta())
	s.engine.Update(&st, item.FamilyID, fam.Meta().Topic, params, correct)
	subRaw, _ := json.Marshal(sub)
	spent := int32(now.Sub(item.ServedAt).Milliseconds()) //nolint:gosec // миллисекунды сессии ≤ 45 минут
	theta, se := st.Theta, st.SE
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.AnswerTestItem(ctx, sqlcgen.AnswerTestItemParams{
			ID: item.ID, SessionID: sess.ID, Submission: subRaw, IsCorrect: &correct, TimeSpentMs: &spent, ThetaAfter: &theta, SeAfter: &se,
		}); err != nil {
			return err
		}
		correctCount := sess.CorrectCount
		if correct {
			correctCount++
		}
		if err := q.UpdateTestSessionProgress(ctx, sqlcgen.UpdateTestSessionProgressParams{
			ID: sess.ID, Theta: &theta, Se: &se, ItemsCount: sess.ItemsCount + 1, CorrectCount: correctCount,
		}); err != nil {
			return err
		}
		return q.IncrementFamilyStats(ctx, sqlcgen.IncrementFamilyStatsParams{ID: item.FamilyID, Correct: correct})
	})
	if err != nil {
		return AnswerResult{}, err
	}
	res := AnswerResult{Accepted: true, Late: late, ItemsAnswered: len(st.Responses)}
	if s.engine.ShouldStop(&st) {
		sess, _ = s.q.GetTestSession(ctx, sess.ID)
		items, _ = s.q.ListTestItems(ctx, sess.ID)
		if err := s.finish(ctx, sess, st, items, sqlcgen.TestSessionStatusCompleted); err != nil {
			return AnswerResult{}, err
		}
		sess, _ = s.q.GetTestSession(ctx, sess.ID)
		view, err := s.sessionView(ctx, sess, true)
		if err != nil {
			return AnswerResult{}, err
		}
		res.Finished = true
		res.Session = &view
	}
	return res, nil
}

// Abandon прерывает сессию; результат не засчитывается, cooldown не тратится.
func (s *Service) Abandon(ctx context.Context, userID, sessionID uuid.UUID) error {
	sess, err := s.ownSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if !domain.TestSessionStatus(sess.Status).IsOpen() {
		return domain.Conflict("session_closed", "Сессия уже завершена")
	}
	return s.q.FinishTestSession(ctx, sqlcgen.FinishTestSessionParams{ID: sess.ID, Status: sqlcgen.TestSessionStatusAbandoned, Theta: sess.Theta, Se: sess.Se})
}

// ReportEvent записывает событие поведения (телеметрия): blur, paste, visibility и т. п.
func (s *Service) ReportEvent(ctx context.Context, userID, sessionID uuid.UUID, kind string) error {
	allowed := map[string]bool{"blur": true, "paste": true, "copy": true, "hidden": true, "resize": true, "devtools": true}
	if !allowed[kind] {
		return domain.Validation("kind", "event_kind_invalid", "Неизвестный тип события")
	}
	sess, err := s.ownSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	flags := map[string]int{}
	_ = json.Unmarshal(sess.Flags, &flags)
	flags[kind]++
	raw, _ := json.Marshal(flags)
	return s.q.UpdateTestSessionFlags(ctx, sqlcgen.UpdateTestSessionFlagsParams{ID: sess.ID, Flags: raw})
}

// GetSession возвращает состояние сессии (с разбором после завершения).
func (s *Service) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (SessionView, error) {
	sess, err := s.ownSession(ctx, userID, sessionID)
	if err != nil {
		return SessionView{}, err
	}
	if s.expireIfNeeded(ctx, sess) {
		sess, _ = s.q.GetTestSession(ctx, sess.ID)
	}
	return s.sessionView(ctx, sess, sess.Status == sqlcgen.TestSessionStatusCompleted || sess.Status == sqlcgen.TestSessionStatusExpired)
}

// ListSessions — история сессий кандидата.
func (s *Service) ListSessions(ctx context.Context, userID uuid.UUID) ([]SessionView, error) {
	rows, err := s.q.ListTestSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(rows))
	for _, r := range rows {
		v, err := s.sessionView(ctx, r, false)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Category — категория кандидата.
type Category struct {
	Specialization    string     `json:"specialization"`
	Grade             string     `json:"grade"`
	Status            string     `json:"status" enum:"claimed,confirmed,not_confirmed,upgrade_suggested"`
	IsPrimary         bool       `json:"is_primary"`
	Theta             *float64   `json:"theta,omitempty"`
	SE                *float64   `json:"se,omitempty"`
	Percentile        *float64   `json:"percentile,omitempty"`
	ConfirmedAt       *time.Time `json:"confirmed_at,omitempty"`
	NextTestAllowedAt *time.Time `json:"next_test_allowed_at,omitempty"`
	CanTestHigher     bool       `json:"can_test_higher" doc:"Можно ли сейчас пройти тест на грейд выше"`
	CanTestLower      bool       `json:"can_test_lower"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// HistoryEntry — запись истории грейдов.
type HistoryEntry struct {
	Specialization string     `json:"specialization"`
	ClaimedGrade   string     `json:"claimed_grade"`
	Decision       string     `json:"decision"`
	ResultGrade    *string    `json:"result_grade,omitempty"`
	Theta          *float64   `json:"theta,omitempty"`
	SE             *float64   `json:"se,omitempty"`
	SessionID      *uuid.UUID `json:"session_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Categories возвращает категории и историю кандидата.
func (s *Service) Categories(ctx context.Context, userID uuid.UUID) ([]Category, []HistoryEntry, error) {
	rows, err := s.q.ListCandidateCategories(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	cats := make([]Category, 0, len(rows))
	for _, r := range rows {
		c := Category{
			Specialization: string(r.Specialization), Grade: string(r.Grade), Status: string(r.Status), IsPrimary: r.IsPrimary,
			Theta: r.Theta, SE: r.Se, Percentile: r.Percentile, ConfirmedAt: r.ConfirmedAt, NextTestAllowedAt: r.NextTestAllowedAt, UpdatedAt: r.UpdatedAt,
		}
		if next, ok := refdata.NextGrade(string(r.Grade)); ok {
			c.CanTestHigher = s.checkEligibility(ctx, userID, string(r.Specialization), next.Code) == nil
		}
		if prev, ok := refdata.PrevGrade(string(r.Grade)); ok {
			c.CanTestLower = s.checkEligibility(ctx, userID, string(r.Specialization), prev.Code) == nil
		}
		cats = append(cats, c)
	}
	hist, err := s.q.ListGradeHistory(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	entries := make([]HistoryEntry, 0, len(hist))
	for _, h := range hist {
		e := HistoryEntry{Specialization: string(h.Specialization), ClaimedGrade: string(h.ClaimedGrade), Decision: string(h.Decision), Theta: h.Theta, SE: h.Se, CreatedAt: h.CreatedAt}
		if h.ResultGrade != nil {
			v := string(*h.ResultGrade)
			e.ResultGrade = &v
		}
		if h.SessionID.Valid {
			id := h.SessionID.UUID
			e.SessionID = &id
		}
		entries = append(entries, e)
	}
	return cats, entries, nil
}

// --- внутреннее ---

func (s *Service) ownSession(ctx context.Context, userID, sessionID uuid.UUID) (*sqlcgen.TestSession, error) {
	sess, err := s.q.GetTestSession(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && sess.UserID != userID) {
		return nil, domain.NotFound("test_session")
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// expireIfNeeded завершает просроченную сессию по отвеченным заданиям. Возвращает true, если сессия истекла.
func (s *Service) expireIfNeeded(ctx context.Context, sess *sqlcgen.TestSession) bool {
	if !domain.TestSessionStatus(sess.Status).IsOpen() || !s.now().After(sess.ExpiresAt) {
		return false
	}
	items, err := s.q.ListTestItems(ctx, sess.ID)
	if err != nil {
		return true
	}
	st, _, err := s.rebuildState(ctx, sess, items)
	if err != nil {
		return true
	}
	if err := s.finish(ctx, sess, st, items, sqlcgen.TestSessionStatusExpired); err != nil {
		s.log.Warn("expire session", "session_id", sess.ID, "err", err)
	}
	return true
}

// rebuildState восстанавливает состояние CAT из сохранённых заданий и возвращает пул кандидатов.
func (s *Service) rebuildState(ctx context.Context, sess *sqlcgen.TestSession, items []*sqlcgen.TestItem) (irt.State, []irt.Candidate, error) {
	prior := 0.0
	if g, ok := refdata.GradeByCode(string(sess.TargetGrade)); ok {
		prior = g.PriorTheta
	}
	st := irt.NewState(prior, s.cfg.CAT.PriorSigma)
	fams, err := s.q.ListItemFamiliesBySpec(ctx, sess.Specialization)
	if err != nil {
		return st, nil, err
	}
	params := map[string]irt.Params{}
	topics := map[string]string{}
	for _, f := range fams {
		params[f.ID] = irt.Params{A: f.A, B: f.B, C: f.C}
		topics[f.ID] = f.Topic
	}
	for _, it := range items {
		st.AskedIDs = append(st.AskedIDs, it.FamilyID)
		topic := topics[it.FamilyID]
		if it.AnsweredAt != nil && it.IsCorrect != nil {
			p, ok := params[it.FamilyID]
			if !ok {
				if fam, found := itembank.Get(it.FamilyID); found {
					m := fam.Meta()
					p = irt.Params{A: m.IRT.A, B: m.IRT.B, C: m.IRT.C}
					topic = m.Topic
				}
			}
			st.Responses = append(st.Responses, irt.Response{Params: p, Correct: *it.IsCorrect})
			st.Topics[topic]++
		}
	}
	if len(st.Responses) > 0 {
		_, st.Theta, st.SE = irt.EAP(s.engine.Grid, st.PriorMu, s.cfg.CAT.PriorSigma, st.Responses)
	}
	rates := map[string]float64{}
	if total, err := s.q.CountRecentSessions(ctx, sess.Specialization); err == nil && total > 0 {
		if rows, err := s.q.GetExposureCounts(ctx, sess.Specialization); err == nil {
			for _, r := range rows {
				rates[r.FamilyID] = float64(r.SessionsWithFamily) / float64(total)
			}
		}
	}
	registered := map[string]bool{}
	for _, f := range itembank.BySpec(itembank.Spec(sess.Specialization)) {
		registered[f.Meta().ID] = true
	}
	pool := make([]irt.Candidate, 0, len(fams))
	for _, f := range fams {
		if !registered[f.ID] {
			continue // семейство удалено из кода
		}
		pool = append(pool, irt.Candidate{ID: f.ID, Topic: f.Topic, Params: params[f.ID], Exposure: rates[f.ID]})
	}
	return st, pool, nil
}

func (s *Service) paramsFor(ctx context.Context, familyID string, meta itembank.Meta) irt.Params {
	fams, err := s.q.ListItemFamiliesBySpec(ctx, sqlcgen.SpecializationCode(meta.Spec))
	if err == nil {
		for _, f := range fams {
			if f.ID == familyID {
				return irt.Params{A: f.A, B: f.B, C: f.C}
			}
		}
	}
	return irt.Params{A: meta.IRT.A, B: meta.IRT.B, C: meta.IRT.C}
}

// finish фиксирует результат, решение по грейду и категорию.
func (s *Service) finish(ctx context.Context, sess *sqlcgen.TestSession, st irt.State, items []*sqlcgen.TestItem, status sqlcgen.TestSessionStatus) error {
	post := s.engine.Posterior(&st)
	claimed := string(sess.TargetGrade)
	d := irt.Decide(post, claimed, Bands(), s.cfg.ConfirmMinP, s.cfg.UpgradeMinP)
	if len(st.Responses) == 0 {
		d.Status = "not_confirmed"
		d.Result = ""
	}
	theta, se := st.Theta, st.SE
	pct, err := s.q.CategoryPercentile(ctx, sqlcgen.CategoryPercentileParams{Specialization: sess.Specialization, Theta: theta})
	if err != nil {
		pct = 0.5
	}
	decision := sqlcgen.GradeStatus(d.Status)
	now := s.now()
	nextAllowed := now.Add(s.cfg.GradeCooldown)
	var correctTopics []string
	topicCorrect := map[string][2]int{}
	for _, it := range items {
		if it.IsCorrect == nil {
			continue
		}
		fam, ok := itembank.Get(it.FamilyID)
		if !ok {
			continue
		}
		t := fam.Meta().Topic
		c := topicCorrect[t]
		c[1]++
		if *it.IsCorrect {
			c[0]++
		}
		topicCorrect[t] = c
	}
	for t, c := range topicCorrect {
		if c[0]*2 >= c[1] && c[0] > 0 {
			correctTopics = append(correctTopics, t)
		}
	}
	return postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.FinishTestSession(ctx, sqlcgen.FinishTestSessionParams{
			ID: sess.ID, Status: status, Theta: &theta, Se: &se, Decision: &decision, Percentile: &pct,
		}); err != nil {
			return err
		}
		existing, err := q.GetCandidateCategory(ctx, sqlcgen.GetCandidateCategoryParams{UserID: sess.UserID, Specialization: sess.Specialization})
		hasExisting := err == nil
		primaryCount, _ := q.CountPrimaryCategories(ctx, sess.UserID)
		params := sqlcgen.UpsertCandidateCategoryParams{
			UserID: sess.UserID, Specialization: sess.Specialization, Grade: sess.TargetGrade, Status: decision,
			IsPrimary: primaryCount == 0, Theta: &theta, Se: &se, Percentile: &pct,
			LastSessionID: uuid.NullUUID{UUID: sess.ID, Valid: true}, NextTestAllowedAt: &nextAllowed,
		}
		var resultGrade *sqlcgen.GradeCode
		switch decision {
		case sqlcgen.GradeStatusConfirmed, sqlcgen.GradeStatusUpgradeSuggested:
			params.ConfirmedAt = &now
			g := sess.TargetGrade
			resultGrade = &g
		default:
			// Не подтверждён: ранее подтверждённая категория ниже сохраняется (грейд не понижается).
			if hasExisting && (existing.Status == sqlcgen.GradeStatusConfirmed || existing.Status == sqlcgen.GradeStatusUpgradeSuggested) &&
				refdata.GradeOrder(string(existing.Grade)) < refdata.GradeOrder(string(sess.TargetGrade)) {
				params.Grade = existing.Grade
				params.Status = existing.Status
				params.Theta, params.Se, params.Percentile = existing.Theta, existing.Se, existing.Percentile
				params.LastSessionID = existing.LastSessionID
				params.ConfirmedAt = existing.ConfirmedAt
			}
		}
		if _, err := q.UpsertCandidateCategory(ctx, params); err != nil {
			return err
		}
		if err := q.InsertGradeHistory(ctx, sqlcgen.InsertGradeHistoryParams{
			UserID: sess.UserID, Specialization: sess.Specialization, ClaimedGrade: sess.TargetGrade, Decision: decision,
			ResultGrade: resultGrade, Theta: &theta, Se: &se, SessionID: uuid.NullUUID{UUID: sess.ID, Valid: true},
		}); err != nil {
			return err
		}
		if len(correctTopics) > 0 {
			if _, err := q.ConfirmSkillsByTopics(ctx, sqlcgen.ConfirmSkillsByTopicsParams{UserID: sess.UserID, SessionID: sess.ID, Topics: correctTopics}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) sessionView(ctx context.Context, sess *sqlcgen.TestSession, withReview bool) (SessionView, error) {
	v := SessionView{
		ID: sess.ID, Specialization: string(sess.Specialization), TargetGrade: string(sess.TargetGrade), Status: string(sess.Status),
		ItemsAnswered: int(sess.ItemsCount), MinItems: s.cfg.CAT.MinItems, MaxItems: s.cfg.CAT.MaxItems,
		ExpiresAt: sess.ExpiresAt, StartedAt: sess.StartedAt, FinishedAt: sess.FinishedAt,
	}
	if sess.Decision != nil && sess.Theta != nil {
		items, err := s.q.ListTestItems(ctx, sess.ID)
		if err != nil {
			return v, err
		}
		st, _, err := s.rebuildState(ctx, sess, items)
		if err != nil {
			return v, err
		}
		d := irt.Decide(s.engine.Posterior(&st), string(sess.TargetGrade), Bands(), s.cfg.ConfirmMinP, s.cfg.UpgradeMinP)
		res := &Result{
			Theta: *sess.Theta, Decision: string(*sess.Decision), PConfirm: d.PConfirm, PUpgrade: d.PUpgrade,
			CorrectCount: int(sess.CorrectCount), ItemsCount: int(sess.ItemsCount),
		}
		if sess.Se != nil {
			res.SE = *sess.Se
		}
		if sess.Percentile != nil {
			res.Percentile = *sess.Percentile
		}
		if cat, err := s.q.GetCandidateCategory(ctx, sqlcgen.GetCandidateCategoryParams{UserID: sess.UserID, Specialization: sess.Specialization}); err == nil {
			res.NextAllowedAt = cat.NextTestAllowedAt
		}
		switch res.Decision {
		case "confirmed":
			res.ResultGrade = string(sess.TargetGrade)
			res.Message = fmt.Sprintf("Грейд %s подтверждён: вероятность %.0f%%.", refdataGradeName(res.ResultGrade), d.PConfirm*100)
		case "upgrade_suggested":
			res.ResultGrade = string(sess.TargetGrade)
			if next, ok := refdata.NextGrade(res.ResultGrade); ok {
				res.Message = fmt.Sprintf("Грейд %s подтверждён, и результат уверенно выше: можно пройти тест на %s.", refdataGradeName(res.ResultGrade), next.Name)
			}
		default:
			res.Message = fmt.Sprintf("Грейд %s не подтверждён (вероятность %.0f%%). Можно сразу пройти тест на уровень ниже.", refdataGradeName(string(sess.TargetGrade)), d.PConfirm*100)
		}
		v.Result = res
		if withReview {
			for _, it := range items {
				var inst itembank.Instance
				_ = json.Unmarshal(it.Instance, &inst)
				topic, format := "", ""
				if fam, ok := itembank.Get(it.FamilyID); ok {
					topic, format = fam.Meta().Topic, string(fam.Meta().Format)
				}
				rv := ItemReview{Position: int(it.Position), Topic: topic, Format: format, Stem: inst.Stem, Code: inst.Code, Options: inst.Options, Answer: inst.Answer, IsCorrect: it.IsCorrect, Explanation: inst.Explanation}
				if it.Submission != nil {
					var sub itembank.Submission
					if json.Unmarshal(it.Submission, &sub) == nil {
						rv.Submission = &sub
					}
				}
				v.Review = append(v.Review, rv)
			}
		}
	}
	return v, nil
}

func (s *Service) itemView(it *sqlcgen.TestItem, answered int) *ItemView {
	var inst itembank.Instance
	_ = json.Unmarshal(it.Instance, &inst)
	topic, format, limit := "", "", 90
	if fam, ok := itembank.Get(it.FamilyID); ok {
		m := fam.Meta()
		topic, format, limit = m.Topic, string(m.Format), int(m.TimeLimit.Seconds())
	}
	return &ItemView{
		ID: it.ID, Position: int(it.Position), Topic: topic, Format: format, Stem: inst.Stem, Code: inst.Code, Options: inst.Options,
		TimeLimitSec: limit, ServedAt: it.ServedAt, DeadlineAt: it.DeadlineAt, ItemsAnswered: answered, MaxItems: s.cfg.CAT.MaxItems,
	}
}

func toSurvey(row *sqlcgen.Survey) Survey {
	out := Survey{ID: row.ID, Specialization: string(row.Specialization), PrimaryStack: row.PrimaryStack, ClaimedGrade: string(row.ClaimedGrade), ExperienceYears: row.ExperienceYears, CreatedAt: row.CreatedAt}
	if row.Industry != nil {
		out.Industry = *row.Industry
	}
	if out.PrimaryStack == nil {
		out.PrimaryStack = []string{}
	}
	return out
}

func ptrSpec(code string) *sqlcgen.SpecializationCode {
	v := sqlcgen.SpecializationCode(code)
	return &v
}

func refdataGradeName(code string) string {
	if g, ok := refdata.GradeByCode(code); ok {
		return g.Name
	}
	return code
}

// DefaultMinSpecFamilies — сколько собственных семейств нужно специализации, чтобы по ней был тест.
const DefaultMinSpecFamilies = 10

func specificFamilies(spec string) int {
	n := 0
	for _, f := range itembank.BySpec(itembank.Spec(spec)) {
		if f.Meta().Spec != itembank.SpecCommon {
			n++
		}
	}
	return n
}

// TestableSpecializations — выбираемые специализации, у которых есть собственный банк заданий.
func TestableSpecializations(minSpec int) []string {
	var out []string
	for _, s := range refdata.Specializations {
		if s.Selectable && specificFamilies(s.Code) >= minSpec {
			out = append(out, s.Code)
		}
	}
	return out
}

func testableNames(minSpec int) string {
	var names []string
	for _, code := range TestableSpecializations(minSpec) {
		for _, s := range refdata.Specializations {
			if s.Code == code {
				names = append(names, s.Short)
			}
		}
	}
	return strings.Join(names, ", ")
}
