package irt

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestProbAndInfo(t *testing.T) {
	p := Params{A: 1, B: 0, C: 0}
	if got := Prob(0, p); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("P(0)=%v", got)
	}
	if Prob(2, p) <= Prob(1, p) || Prob(-2, p) >= Prob(-1, p) {
		t.Fatal("P must increase with theta")
	}
	pc := Params{A: 1, B: 0, C: 0.25}
	if Prob(-10, pc) < 0.249 {
		t.Fatal("guessing floor")
	}
	// Информация максимальна около b (для c=0 — ровно в b).
	if Info(0, p) < Info(1, p) || Info(0, p) < Info(-1, p) {
		t.Fatal("info peak")
	}
	if Info(0, Params{A: 2, B: 0, C: 0}) <= Info(0, p) {
		t.Fatal("higher a gives more info")
	}
}

// simulate отвечает как кандидат с истинной θ.
func simulate(rng *rand.Rand, theta float64, p Params) bool {
	return rng.Float64() < Prob(theta, p)
}

func TestEAPRecoversTheta(t *testing.T) {
	grid := DefaultGrid()
	rng := rand.New(rand.NewPCG(1, 2))
	for _, trueTheta := range []float64{-1.5, 0, 1.2} {
		var errs float64
		const trials = 40
		for trial := 0; trial < trials; trial++ {
			var rs []Response
			for i := 0; i < 20; i++ {
				p := Params{A: 1.0 + rng.Float64()*0.8, B: trueTheta + (rng.Float64()-0.5)*2, C: 0.1}
				rs = append(rs, Response{Params: p, Correct: simulate(rng, trueTheta, p)})
			}
			_, est, se := EAP(grid, 0, 1, rs)
			errs += math.Abs(est - trueTheta)
			if se <= 0 || se > 1 {
				t.Fatalf("se out of range: %v", se)
			}
		}
		if mean := errs / trials; mean > 0.45 {
			t.Fatalf("theta=%v: mean abs error %.2f too large", trueTheta, mean)
		}
	}
}

func TestEAPExtremes(t *testing.T) {
	grid := DefaultGrid()
	all := make([]Response, 0, 15)
	for i := 0; i < 15; i++ {
		all = append(all, Response{Params: Params{A: 1.2, B: float64(i-7) * 0.3, C: 0.1}, Correct: true})
	}
	_, hi, _ := EAP(grid, 0, 1, all)
	for i := range all {
		all[i].Correct = false
	}
	_, lo, _ := EAP(grid, 0, 1, all)
	if hi < 1.5 || lo > -1.5 {
		t.Fatalf("extremes: all correct %.2f, all wrong %.2f", hi, lo)
	}
	post, _, _ := EAP(grid, 0, 1, nil)
	if p := post.ProbAtLeast(0); math.Abs(p-0.5) > 0.03 {
		t.Fatalf("prior symmetric: P(theta>=0)=%v", p)
	}
}

func pool(n int) []Candidate {
	topics := []string{"sql", "http", "concurrency", "caching", "testing", "git", "complexity"}
	out := make([]Candidate, 0, n)
	rng := rand.New(rand.NewPCG(9, 9))
	for i := 0; i < n; i++ {
		out = append(out, Candidate{
			ID:     "f" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+i%7)),
			Topic:  topics[i%len(topics)],
			Params: Params{A: 0.8 + rng.Float64()*0.8, B: -2.5 + rng.Float64()*5, C: 0.2},
		})
	}
	return out
}

func runCAT(t *testing.T, trueTheta float64, seed uint64) (State, int) {
	t.Helper()
	eng := NewEngine(DefaultConfig())
	st := NewState(0, 1)
	rng := rand.New(rand.NewPCG(seed, 1))
	cands := pool(120)
	for !eng.ShouldStop(&st) {
		c, ok := eng.Next(&st, cands, rng)
		if !ok {
			t.Fatal("no candidates")
		}
		eng.Update(&st, c.ID, c.Topic, c.Params, simulate(rng, trueTheta, c.Params))
	}
	return st, len(st.Responses)
}

func TestCATConvergesAndBalances(t *testing.T) {
	for _, theta := range []float64{-1.2, 0.3, 1.5} {
		var total, items float64
		const trials = 30
		for i := 0; i < trials; i++ {
			st, n := runCAT(t, theta, uint64(i+1))
			total += math.Abs(st.Theta - theta)
			items += float64(n)
			if n < 12 || n > 20 {
				t.Fatalf("items %d out of [12,20]", n)
			}
			// Баланс тем: первые 8 заданий — не менее 5 тем; одна тема — не больше 6 заданий.
			if len(st.Topics) < 5 {
				t.Fatalf("only %d topics covered", len(st.Topics))
			}
			for tp, cnt := range st.Topics {
				if cnt > 6 {
					t.Fatalf("topic %s asked %d times", tp, cnt)
				}
			}
			seen := map[string]bool{}
			for _, id := range st.AskedIDs {
				if seen[id] {
					t.Fatalf("family %s asked twice", id)
				}
				seen[id] = true
			}
		}
		if mean := total / trials; mean > 0.5 {
			t.Fatalf("theta=%v: mean abs error %.2f", theta, mean)
		}
		t.Logf("theta=%v: mean abs error %.2f, mean items %.1f", theta, total/trials, items/trials)
	}
}

func TestCATRandomesqueDiffersBetweenSessions(t *testing.T) {
	a, _ := runCAT(t, 0.5, 11)
	b, _ := runCAT(t, 0.5, 12)
	same := 0
	for i := 0; i < len(a.AskedIDs) && i < len(b.AskedIDs); i++ {
		if a.AskedIDs[i] == b.AskedIDs[i] {
			same++
		}
	}
	if same == len(a.AskedIDs) {
		t.Fatal("two sessions with the same theta got identical item sequences")
	}
}

func TestDecide(t *testing.T) {
	bands := []Band{{Code: "junior", Lower: -4, MinConfirm: -1.5}, {Code: "middle", Lower: -0.5}, {Code: "senior", Lower: 0.7}}
	grid := DefaultGrid()
	strong := make([]Response, 0, 15)
	for i := 0; i < 15; i++ {
		strong = append(strong, Response{Params: Params{A: 1.3, B: 0.5 + float64(i%5)*0.3, C: 0.1}, Correct: i%6 != 0})
	}
	post, theta, _ := EAP(grid, 0.1, 1, strong)
	d := Decide(post, "middle", bands, 0.7, 0.8)
	if d.Status != "upgrade_suggested" && d.Status != "confirmed" {
		t.Fatalf("strong middle candidate (theta %.2f): %+v", theta, d)
	}
	weak := make([]Response, 0, 15)
	for i := 0; i < 15; i++ {
		weak = append(weak, Response{Params: Params{A: 1.3, B: -0.5 + float64(i%5)*0.3, C: 0.1}, Correct: i%4 == 0})
	}
	post2, theta2, _ := EAP(grid, 0.9, 1, weak)
	d2 := Decide(post2, "senior", bands, 0.7, 0.8)
	if d2.Status != "not_confirmed" {
		t.Fatalf("weak senior candidate (theta %.2f): %+v", theta2, d2)
	}
	if BandOf(-1, bands) != "junior" || BandOf(0, bands) != "middle" || BandOf(1, bands) != "senior" {
		t.Fatal("band of")
	}
	if Decide(post, "unknown", bands, 0.7, 0.8).Status != "not_confirmed" {
		t.Fatal("unknown grade")
	}
	// Junior: полоса не ограничена снизу, но угадывающий (все ответы неверны) не подтверждается,
	// а кандидат, решающий примерно половину заданий уровня Junior, подтверждается.
	var none, half []Response
	for i := 0; i < 16; i++ {
		p := Params{A: 1.2, B: -0.9 + float64(i%4)*0.2, C: 0}
		none = append(none, Response{Params: p, Correct: false})
		half = append(half, Response{Params: p, Correct: i%2 == 0})
	}
	pNone, thNone, _ := EAP(grid, -0.8, 1, none)
	if d := Decide(pNone, "junior", bands, 0.7, 0.8); d.Status != "not_confirmed" || BandOf(thNone, bands) != "junior" {
		t.Fatalf("guesser must not confirm junior (theta %.2f): %+v", thNone, d)
	}
	pHalf, thHalf, _ := EAP(grid, -0.8, 1, half)
	if d := Decide(pHalf, "junior", bands, 0.7, 0.8); d.Status == "not_confirmed" {
		t.Fatalf("half-correct junior must be confirmed (theta %.2f): %+v", thHalf, d)
	}
}

func TestCalibration(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	trueP := Params{A: 1.2, B: 0.8, C: 0.2}
	prior := Params{A: 1.2, B: -0.5, C: 0.2} // автор ошибся: задание сложнее, чем он думал
	var obs []Observation
	for i := 0; i < 3000; i++ {
		th := rng.NormFloat64()
		obs = append(obs, Observation{Theta: th, Correct: simulate(rng, th, trueP)})
	}
	// При 3000 наблюдениях ошибка p̂ ≈ 0.01, что соответствует ≈ 0.05 по b; априор почти не влияет.
	b := CalibrateDifficulty(prior, obs, 30, 30)
	if math.Abs(b-trueP.B) > 0.3 {
		t.Fatalf("calibrated b=%.2f, want ≈ %.2f", b, trueP.B)
	}
	if bPrior := CalibrateDifficulty(prior, obs[:40], 1000, 30); math.Abs(bPrior-prior.B) > 0.2 {
		t.Fatalf("strong prior (kappa=1000) must dominate few observations, got %.2f", bPrior)
	}
	if CalibrateDifficulty(prior, obs[:10], 30, 30) != prior.B {
		t.Fatal("too few observations must keep prior")
	}
	r := PointBiserial(obs)
	if r < 0.2 {
		t.Fatalf("point-biserial %.2f too low for a discriminating item", r)
	}
	a := CalibrateDiscrimination(prior, obs, 100)
	if a < 0.5 || a > 2.0 {
		t.Fatalf("a=%.2f out of range", a)
	}
	// Задание без связи с θ: нулевая дискриминативность.
	var noise []Observation
	for i := 0; i < 400; i++ {
		noise = append(noise, Observation{Theta: rng.NormFloat64(), Correct: rng.Float64() < 0.5})
	}
	if r := PointBiserial(noise); math.Abs(r) > 0.15 {
		t.Fatalf("noise item r=%.2f", r)
	}
}
