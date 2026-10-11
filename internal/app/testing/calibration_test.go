package testing

import (
	"context"
	"math"
	"math/rand/v2"
	stdtesting "testing"
	"time"

	"github.com/google/uuid"

	"fsp/internal/storage/postgres/sqlcgen"
	"fsp/pkg/irt"
)

// observations моделирует ответы по 3PL с «истинной» сложностью bTrue.
func observations(rng *rand.Rand, n int, p irt.Params) []irt.Observation {
	out := make([]irt.Observation, 0, n)
	for i := 0; i < n; i++ {
		theta := rng.NormFloat64()
		out = append(out, irt.Observation{Theta: theta, Correct: rng.Float64() < irt.Prob(theta, p)})
	}
	return out
}

func TestCalibrateMovesTowardsTruth(t *stdtesting.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	prior := familyPrior{ID: "f.hard", A: 1.2, BPrior: 0, C: 0, BCurrent: 0}
	few := familyPrior{ID: "f.few", A: 1.2, BPrior: 0, C: 0, BCurrent: 0}
	obs := map[string][]irt.Observation{
		"f.hard": observations(rng, 400, irt.Params{A: 1.2, B: 0.8}),
		"f.few":  observations(rng, CalibrationMinObs-1, irt.Params{A: 1.2, B: 0.8}),
	}
	updates, families := calibrate([]familyPrior{prior, few}, obs)
	if families != 1 || len(updates) != 1 || updates[0].ID != "f.hard" {
		t.Fatalf("only families with enough answers are calibrated: %+v (%d)", updates, families)
	}
	// Ожидаемо (κ·0 + n·b_emp)/(κ+n) ≈ 0,76 при b_emp ≈ 0,8; допускаем шум выборки.
	if b := updates[0].B; b < 0.55 || b > 0.95 {
		t.Fatalf("calibrated b = %.3f, want close to the true 0.8", b)
	}
	// Повторный запуск от той же априорной сложности ничего не меняет.
	prior.BCurrent = updates[0].B
	if again, _ := calibrate([]familyPrior{prior}, obs); len(again) != 0 {
		t.Fatalf("recalibration must be idempotent: %+v", again)
	}
}

// Интеграция: ответы завершённых сессий в базе → новая сложность семейства. Используется отдельное
// тестовое семейство, которого нет в коде: адаптивный тест его не выдаёт, а общий банк не меняется.
func TestRecalibrateFromStoredAnswers(t *stdtesting.T) {
	svc, uid := newService(t)
	ctx := context.Background()
	famID := "test.calibration." + uuid.NewString()[:8]
	if _, err := svc.pool.Exec(ctx, `INSERT INTO item_families (id, spec, topic, grade, format, a, b, c, a_prior, b_prior)
		VALUES ($1, 'backend', 'sql', 'middle', 'numeric', 1.2, 0, 0, 1.2, 0)`, famID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = svc.pool.Exec(context.Background(), `DELETE FROM test_sessions WHERE user_id = $1`, uid)
		_, _ = svc.pool.Exec(context.Background(), `DELETE FROM item_families WHERE id = $1`, famID)
		_, _ = svc.pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, uid)
	})
	rng := rand.New(rand.NewPCG(3, 5))
	truth := irt.Params{A: 1.2, B: 0.8}
	for i, o := range observations(rng, 120, truth) {
		var sid uuid.UUID
		if err := svc.pool.QueryRow(ctx, `INSERT INTO test_sessions (user_id, specialization, target_grade, status, seed, expires_at, finished_at, theta, se)
			VALUES ($1, 'backend', 'middle', 'completed', $2, now(), now(), $3, 0.3) RETURNING id`, uid, i, o.Theta).Scan(&sid); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.pool.Exec(ctx, `INSERT INTO test_items (session_id, position, family_id, seed, instance, deadline_at, answered_at, is_correct)
			VALUES ($1, 1, $2, 1, '{}'::jsonb, $3, now(), $4)`, sid, famID, time.Now().Add(time.Minute), o.Correct); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := svc.Recalibrate(ctx, []string{famID})
	if err != nil || rep.Families != 1 || rep.Updated != 1 || rep.Answers != 120 {
		t.Fatalf("report: %+v %v", rep, err)
	}
	fams, err := svc.q.ListItemFamilies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got *sqlcgen.ItemFamily
	for _, f := range fams {
		if f.ID == famID {
			got = f
		}
	}
	if got == nil || got.B < 0.4 || got.B > 1.0 || got.BPrior != 0 || math.Abs(got.A-1.2) > 1e-9 {
		t.Fatalf("family after calibration: %+v", got)
	}
	// Перезапуск API (синхронизация банка из кода) не стирает калибровку, пока априорные параметры те же.
	if err := svc.q.UpsertItemFamily(ctx, sqlcgen.UpsertItemFamilyParams{ID: famID, Spec: "backend", Topic: "sql", Grade: "middle", Format: "numeric", A: 1.2, B: 0, C: 0}); err != nil {
		t.Fatal(err)
	}
	fams, _ = svc.q.ListItemFamilies(ctx)
	for _, f := range fams {
		if f.ID == famID && math.Abs(f.B-got.B) > 1e-9 {
			t.Fatalf("sync must keep calibrated b %.3f, got %.3f", got.B, f.B)
		}
	}
}
