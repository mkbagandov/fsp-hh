package validation

import (
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"testing"

	_ "fsp/pkg/itembank/families/all"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestConfusionKappaWithin(t *testing.T) {
	var perfect Confusion
	for _, b := range Bands {
		for i := 0; i < 10; i++ {
			perfect.Add(b, b)
		}
	}
	if perfect.Accuracy() != 1 || perfect.Within1() != 1 || !near(perfect.Kappa(), 1) {
		t.Fatalf("perfect agreement: %v %v %v", perfect.Accuracy(), perfect.Within1(), perfect.Kappa())
	}
	// Классический пример: po = 0,70, pe = 0,50 → κ = 0,40.
	var c Confusion
	c[0][0], c[0][1], c[1][0], c[1][1] = 20, 5, 10, 15
	if !near(c.Kappa(), 0.4) || !near(c.Accuracy(), 0.7) {
		t.Fatalf("kappa: %v, accuracy: %v", c.Kappa(), c.Accuracy())
	}
	var far Confusion
	far.Add("junior", "senior")
	far.Add("junior", "middle")
	if far.Within1() != 0.5 || far.Accuracy() != 0 {
		t.Fatalf("within1: %v", far.Within1())
	}
	far.Add("lead", "junior") // неизвестный грейд игнорируется
	if far[0][2] != 1 {
		t.Fatal("unknown band must be ignored")
	}
	var empty Confusion
	if empty.Accuracy() != 0 || empty.Kappa() != 0 || empty.Within1() != 0 {
		t.Fatal("empty matrix")
	}
}

func TestCorrelationsAndErrors(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	if !near(Pearson(x, []float64{2, 4, 6, 8, 10}), 1) || !near(Pearson(x, []float64{5, 4, 3, 2, 1}), -1) {
		t.Fatal("pearson ±1")
	}
	if Pearson(x, []float64{1, 1, 1, 1, 1}) != 0 || Pearson(x[:1], x[:1]) != 0 {
		t.Fatal("degenerate pearson")
	}
	if !near(RMSE([]float64{1, 3}, []float64{0, 0}), math.Sqrt(5)) || !near(MAE([]float64{1, -3}, []float64{0, 0}), 2) {
		t.Fatal("rmse/mae")
	}
	if Median([]float64{3, 1, 2}) != 2 || Quantile([]float64{1, 2, 3, 4, 5}, 0.75) != 4 || Median(nil) != 0 {
		t.Fatal("median/quantile")
	}
}

func TestRankingMetrics(t *testing.T) {
	rel := []float64{3, 2, 1, 0}
	if !near(NDCG([]int{0, 1, 2, 3}, rel, 4), 1) {
		t.Fatal("ideal order has NDCG 1")
	}
	worst := NDCG([]int{3, 2, 1, 0}, rel, 4)
	if worst >= 1 || worst <= 0 {
		t.Fatalf("reversed order: %v", worst)
	}
	if NDCG([]int{0}, []float64{0}, 1) != 0 {
		t.Fatal("zero relevance")
	}
	good := map[int]bool{0: true, 1: true}
	if PrecisionAt([]int{0, 2, 1, 3}, good, 2) != 0.5 || PrecisionAt([]int{0, 1}, good, 10) != 1 || PrecisionAt(nil, good, 3) != 0 {
		t.Fatal("precision@k")
	}
}

// Процедура целиком на маленькой популяции: метрики в разумных пределах, отчёт и графики пишутся.
func TestProcedureSmoke(t *testing.T) {
	pools := PoolsFromBank(8)
	if len(pools) == 0 {
		t.Skip("банк заданий не зарегистрирован в этом тестовом бинарнике")
	}
	p := pools[0]
	people := Population(rand.New(rand.NewPCG(1, 2)), 40, 0.3)
	acc := Accuracy(p, people, 7)
	if acc.Within1 < 0.8 || acc.MeanItems < 12 || acc.MeanItems > 20 {
		t.Fatalf("accuracy: %+v", acc)
	}
	rt := Retest(p, people[:20], 7)
	if rt.PearsonTheta < 0.5 {
		t.Fatalf("retest correlation too low: %v", rt.PearsonTheta)
	}
	rb := Robustness(p, 7, 20)
	if rb.GuessConfirmed["senior"] > 0.2 {
		t.Fatalf("guesser must not pass senior: %v", rb.GuessConfirmed)
	}
	rk := Ranking(7, 60, 3)
	if rk.Rows[0].NDCG10 <= rk.Rows[len(rk.Rows)-1].NDCG10 {
		t.Fatalf("our ranking must beat random: %+v", rk.Rows)
	}
	dir := t.TempDir()
	res := Results{Seed: 7, N: 40, Accuracy: []AccuracyResult{acc}, Retest: []RetestResult{rt}, Robustness: []RobustnessResult{rb},
		Baselines: []BaselineResult{Baselines(p, people[:10], 7, 20)}, Calibration: []CalibrationResult{Calibration(p, 7, 60)}, Info: []InfoCurve{Information(p)}, Ranking: rk}
	if err := WriteReport(dir, res); err != nil {
		t.Fatal(err)
	}
	report := readFile(t, dir+"/REPORT.md")
	for _, want := range []string{"## 1. Методика", "## 3. Test-retest", "![Утечка](leak.svg)", "## 10. Выводы"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report must contain %q", want)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
