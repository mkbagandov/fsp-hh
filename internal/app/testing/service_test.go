package testing

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	stdtesting "testing"
	"time"

	"github.com/google/uuid"

	"fsp/internal/domain"
	"fsp/internal/storage/postgres/pgtest"
	"fsp/internal/storage/postgres/sqlcgen"
	"fsp/pkg/itembank"
	_ "fsp/pkg/itembank/families/all"
)

func newService(t *stdtesting.T) (*Service, uuid.UUID) {
	t.Helper()
	pool := pgtest.Pool(t)
	ctx := context.Background()
	if _, err := SyncFamilies(ctx, pool); err != nil {
		t.Fatalf("sync families: %v", err)
	}
	email := "test-" + time.Now().Format("150405.000000") + "@test.local"
	u, err := sqlcgen.New(pool).CreateUser(ctx, sqlcgen.CreateUserParams{Email: email, Role: sqlcgen.UserRoleCandidate})
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlcgen.New(pool).EnsureCandidateProfile(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := Config{MinBankFamilies: 5}
	return New(cfg, pool, log), u.ID
}

// answerAs отвечает на текущее задание: верно (cheat через сохранённый экземпляр) или неверно.
func answerAs(t *stdtesting.T, svc *Service, uid, sessionID uuid.UUID, item *ItemView, correct bool) AnswerResult {
	t.Helper()
	ctx := context.Background()
	row, err := svc.q.GetTestItem(ctx, sqlcgen.GetTestItemParams{ID: item.ID, SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	var inst itembank.Instance
	_ = json.Unmarshal(row.Instance, &inst)
	n := inst.Answer.Number
	sub := itembank.Submission{Keys: inst.Answer.Keys, Number: &n, Text: inst.Answer.Text, Order: inst.Answer.Order, Rows: inst.Answer.Rows}
	if !correct {
		wrong := n + 1000
		sub = itembank.Submission{Keys: []string{"zz"}, Number: &wrong, Text: "неверно", Order: []string{"zz"}, Rows: [][]string{{"x"}}}
	}
	res, err := svc.Answer(ctx, uid, sessionID, item.ID, sub)
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	return res
}

func runSession(t *stdtesting.T, svc *Service, uid uuid.UUID, spec, grade string, correctness func(i int) bool) SessionView {
	t.Helper()
	ctx := context.Background()
	view, item, err := svc.StartTest(ctx, uid, StartInput{Specialization: spec, TargetGrade: grade})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if item == nil {
		t.Fatal("first item must be served")
	}
	for i := 0; item != nil && i < 40; i++ {
		res := answerAs(t, svc, uid, view.ID, item, correctness(i))
		if res.Finished {
			return *res.Session
		}
		item, err = svc.NextItem(ctx, uid, view.ID)
		if err != nil {
			t.Fatalf("next: %v", err)
		}
	}
	final, err := svc.GetSession(ctx, uid, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	return final
}

func TestSurveyAndStrongCandidate(t *stdtesting.T) {
	svc, uid := newService(t)
	ctx := context.Background()
	if _, _, err := svc.StartTest(ctx, uid, StartInput{Specialization: "backend", TargetGrade: "middle"}); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("test without survey must conflict, got %v", err)
	}
	sv, err := svc.SubmitSurvey(ctx, uid, SurveyInput{Specialization: "backend", Industry: "финтех", PrimaryStack: []string{"go", "postgresql"}, ClaimedGrade: "middle"})
	if err != nil || sv.ClaimedGrade != "middle" {
		t.Fatalf("survey: %+v %v", sv, err)
	}
	final := runSession(t, svc, uid, "backend", "middle", func(int) bool { return true })
	if final.Status != "completed" || final.Result == nil {
		t.Fatalf("session: %+v", final)
	}
	if final.Result.Decision != "confirmed" && final.Result.Decision != "upgrade_suggested" {
		t.Fatalf("all-correct candidate must confirm middle: %+v", final.Result)
	}
	if len(final.Review) == 0 || final.Review[0].Explanation == "" {
		t.Fatal("review with explanations must be available after completion")
	}
	cats, hist, err := svc.Categories(ctx, uid)
	if err != nil || len(cats) != 1 || cats[0].Grade != "middle" || !cats[0].IsPrimary || len(hist) != 1 {
		t.Fatalf("categories: %+v %+v %v", cats, hist, err)
	}
	// Повторный тест на тот же грейд запрещён (уже подтверждён); на выше — только по cooldown или предложению.
	if _, _, err := svc.StartTest(ctx, uid, StartInput{Specialization: "backend", TargetGrade: "middle"}); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("same grade again must conflict: %v", err)
	}
	if final.Result.Decision == "confirmed" {
		if _, _, err := svc.StartTest(ctx, uid, StartInput{Specialization: "backend", TargetGrade: "senior"}); !domain.Is(err, domain.KindCooldown) {
			t.Fatalf("higher grade before cooldown must be blocked: %v", err)
		}
	}
}

func TestWeakCandidateNotConfirmedThenLower(t *stdtesting.T) {
	svc, uid := newService(t)
	ctx := context.Background()
	if _, err := svc.SubmitSurvey(ctx, uid, SurveyInput{Specialization: "backend", ClaimedGrade: "senior"}); err != nil {
		t.Fatal(err)
	}
	final := runSession(t, svc, uid, "backend", "senior", func(int) bool { return false })
	if final.Result == nil || final.Result.Decision != "not_confirmed" {
		t.Fatalf("all-wrong candidate must not confirm senior: %+v", final.Result)
	}
	cats, _, _ := svc.Categories(ctx, uid)
	if len(cats) != 1 || cats[0].Status != "not_confirmed" || !cats[0].CanTestLower {
		t.Fatalf("category after failure: %+v", cats)
	}
	// Выше/тот же — cooldown; ниже — сразу.
	if _, _, err := svc.StartTest(ctx, uid, StartInput{Specialization: "backend", TargetGrade: "senior"}); !domain.Is(err, domain.KindCooldown) {
		t.Fatalf("retry same grade must hit cooldown: %v", err)
	}
	final2 := runSession(t, svc, uid, "backend", "middle", func(i int) bool { return i%4 != 0 })
	if final2.Result == nil {
		t.Fatal("second session must finish")
	}
	cats, hist, _ := svc.Categories(ctx, uid)
	if len(hist) != 2 {
		t.Fatalf("history must have 2 entries, got %d", len(hist))
	}
	t.Logf("second session decision=%s theta=%.2f; category=%+v", final2.Result.Decision, final2.Result.Theta, cats[0])
}

func TestAbandonAndEvents(t *stdtesting.T) {
	svc, uid := newService(t)
	ctx := context.Background()
	if _, err := svc.SubmitSurvey(ctx, uid, SurveyInput{Specialization: "frontend", ClaimedGrade: "junior"}); err != nil {
		t.Fatal(err)
	}
	view, item, err := svc.StartTest(ctx, uid, StartInput{Specialization: "frontend", TargetGrade: "junior"})
	if err != nil || item == nil {
		t.Fatalf("start: %v", err)
	}
	// Повторный старт возвращает ту же активную сессию и то же задание.
	view2, item2, err := svc.StartTest(ctx, uid, StartInput{Specialization: "frontend", TargetGrade: "junior"})
	if err != nil || view2.ID != view.ID || item2.ID != item.ID {
		t.Fatalf("resume: %v %v %v", err, view2.ID == view.ID, item2.ID == item.ID)
	}
	if err := svc.ReportEvent(ctx, uid, view.ID, "blur"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReportEvent(ctx, uid, view.ID, "hack"); !domain.Is(err, domain.KindValidation) {
		t.Fatal("unknown event kind")
	}
	// Чужой пользователь не видит сессию.
	if _, err := svc.GetSession(ctx, uuid.New(), view.ID); !domain.Is(err, domain.KindNotFound) {
		t.Fatalf("foreign session must be not found: %v", err)
	}
	if err := svc.Abandon(ctx, uid, view.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NextItem(ctx, uid, view.ID); !domain.Is(err, domain.KindConflict) {
		t.Fatalf("next after abandon: %v", err)
	}
	cats, _, _ := svc.Categories(ctx, uid)
	if len(cats) != 0 {
		t.Fatal("abandoned session must not create a category")
	}
	// После прерывания можно начать заново.
	if _, item3, err := svc.StartTest(ctx, uid, StartInput{Specialization: "frontend", TargetGrade: "junior"}); err != nil || item3 == nil {
		t.Fatalf("restart after abandon: %v", err)
	}
}
