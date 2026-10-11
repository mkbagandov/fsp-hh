package dataml

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(knnRegression{})
	itembank.Register(logregProbability{})
	itembank.Register(giniBestSplit{})
	itembank.Register(abSampleSize{})
}

// fmtNum печатает число без лишних нулей: 4.5, 12, 0.044.
func fmtNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// knnRegression — предсказание kNN-регрессии: ранжирование по расстоянию и среднее
// таргета по k соседям.
type knnRegression struct{}

func (knnRegression) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.models.knn-regression.v1", Spec: itembank.SpecDataML, Topic: "models",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: -0.6, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"knn"},
	}
}

type knnPoint struct{ x1, x2, y int }

func (f knnRegression) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A60)
	k := itembank.Pick(r, []int{1, 2, 3})
	manhattan := r.IntN(3) == 0
	feats := itembank.Pick(r, [][2]string{{"x1", "x2"}, {"area", "rooms"}, {"age", "income"}, {"speed", "load"}})
	target := itembank.Pick(r, []string{"y", "price", "score", "time"})
	n := itembank.IntBetween(r, 6, 7)
	dist := func(p knnPoint, qx, qy int) int {
		dx, dy := p.x1-qx, p.x2-qy
		if manhattan {
			return absInt(dx) + absInt(dy)
		}
		return dx*dx + dy*dy
	}
	var pts []knnPoint
	var qx, qy int
	var order []int
	for {
		pts = make([]knnPoint, 0, n)
		used := map[[2]int]bool{}
		for len(pts) < n {
			p := knnPoint{x1: itembank.IntBetween(r, 0, 9), x2: itembank.IntBetween(r, 0, 9), y: itembank.IntBetween(r, 2, 18) * 5}
			if used[[2]int{p.x1, p.x2}] {
				continue
			}
			used[[2]int{p.x1, p.x2}] = true
			pts = append(pts, p)
		}
		qx, qy = itembank.IntBetween(r, 0, 9), itembank.IntBetween(r, 0, 9)
		if used[[2]int{qx, qy}] {
			continue
		}
		order = make([]int, n)
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return dist(pts[order[a]], qx, qy) < dist(pts[order[b]], qx, qy) })
		// k-й и (k+1)-й соседи не должны быть на одном расстоянии: иначе ответ неоднозначен.
		if dist(pts[order[k-1]], qx, qy) < dist(pts[order[k]], qx, qy) {
			break
		}
	}
	sum := 0
	neighbors := make([]string, 0, k)
	for _, i := range order[:k] {
		sum += pts[i].y
		neighbors = append(neighbors, fmt.Sprintf("№%d (%s = %d)", i+1, target, pts[i].y))
	}
	answer := float64(sum) / float64(k)
	var md strings.Builder
	fmt.Fprintf(&md, "| № | %s | %s | %s |\n|---|---|---|---|\n", feats[0], feats[1], target)
	for i, p := range pts {
		fmt.Fprintf(&md, "| %d | %d | %d | %d |\n", i+1, p.x1, p.x2, p.y)
	}
	dists := make([]string, 0, n)
	for i, p := range pts {
		dists = append(dists, fmt.Sprintf("№%d — %d", i+1, dist(p, qx, qy)))
	}
	metric, distKind := "евклидово расстояние", "квадраты евклидовых расстояний (для ранжирования корень не нужен)"
	if manhattan {
		metric, distKind = "манхэттенское расстояние (L1)", "расстояния L1"
	}
	query := fmt.Sprintf("%s = %d, %s = %d", feats[0], qx, feats[1], qy)
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Обучающая выборка kNN-регрессии (k = %d, %s; предсказание — среднее `%s` по k ближайшим соседям):\n\n%s\nКакое значение `%s` модель предскажет для объекта (%s)? Округлите до сотых.",
			k, metric, target, md.String(), target, query),
		fmt.Sprintf("Метод k ближайших соседей для регрессии: k = %d, %s, ответ — среднее `%s` по соседям. Обучающие объекты:\n\n%s\nЧему равно предсказание для нового объекта (%s)? Округлите до сотых.",
			k, metric, target, md.String(), query),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Answer: itembank.Answer{Number: round2(answer), Tolerance: 0.011},
		Explanation: fmt.Sprintf("Считаем %s до объекта (%s): %s. Ближайшие k = %d: %s; предсказание — среднее %d/%d = %.2f. "+
			"Типичные ошибки — складывать таргеты соседей вместо усреднения, считать расстояние по одному признаку или брать соседей по другой метрике.",
			distKind, query, strings.Join(dists, ", "), k, strings.Join(neighbors, ", "), sum, k, answer),
	}, nil
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// logregProbability — вероятность и шансы по весам логистической регрессии.
type logregProbability struct{}

func (logregProbability) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.models.logreg-probability.v1", Spec: itembank.SpecDataML, Topic: "models",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.1, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"logistic-regression"},
	}
}

var subscripts = []string{"₁", "₂", "₃"}

// tenths печатает число, заданное в десятых долях: 8 → 0.8, -5 → -0.5.
func tenths(v int) string { return fmtNum(float64(v) / 10) }

func (f logregProbability) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A61)
	d := itembank.IntBetween(r, 2, 3)
	w := make([]int, d)
	x := make([]int, d)
	var b, zT int
	for {
		for i := range w {
			w[i] = itembank.IntBetween(r, -15, 15)
			if w[i] == 0 {
				w[i] = 7
			}
			x[i] = itembank.IntBetween(r, 0, 4)
		}
		b = itembank.IntBetween(r, -10, 10)
		zT = b
		for i := range w {
			zT += w[i] * x[i]
		}
		if a := absInt(zT); a >= 2 && a <= 35 {
			break
		}
	}
	z := float64(zT) / 10
	p1 := 1 / (1 + math.Exp(-z))
	kind := r.IntN(4)
	j := r.IntN(d)
	wParts := make([]string, d)
	xParts := make([]string, d)
	zParts := make([]string, d)
	for i := range w {
		wParts[i] = fmt.Sprintf("w%s = %s", subscripts[i], tenths(w[i]))
		xParts[i] = fmt.Sprintf("x%s = %d", subscripts[i], x[i])
		zParts[i] = fmt.Sprintf("(%s)·%d", tenths(w[i]), x[i])
	}
	setup := fmt.Sprintf("Логистическая регрессия: P(y = 1 | x) = σ(w·x + b), где σ(z) = 1/(1 + e^(−z)). Признаки стандартизованы. Веса %s, смещение b = %s. Объект: %s.",
		strings.Join(wParts, ", "), tenths(b), strings.Join(xParts, ", "))
	var question string
	var answer float64
	switch kind {
	case 0:
		question = "Какую вероятность класса 1 выдаст модель для этого объекта?"
		answer = p1
	case 1:
		question = "Чему равна предсказанная вероятность P(y = 1 | x)?"
		answer = p1
	case 2:
		question = "Какую вероятность класса 0 (P(y = 0 | x)) выдаст модель для этого объекта?"
		answer = 1 - p1
	default:
		question = fmt.Sprintf("Во сколько раз изменятся шансы (odds) класса 1, если x%s увеличить на единицу при прочих равных?", subscripts[j])
		answer = math.Exp(float64(w[j]) / 10)
	}
	negZ := fmtNum(-z) // показатель экспоненты −z без двойного знака
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   setup + " " + question + " Округлите до сотых.",
		Answer: itembank.Answer{Number: round2(answer), Tolerance: 0.011},
		Explanation: fmt.Sprintf("z = w·x + b = %s + (%s) = %s; σ(z) = 1/(1 + e^(%s)) = %.2f — вероятность класса 1, вероятность класса 0 равна 1 − %.2f = %.2f. "+
			"Шансы класса 1 равны e^z = %.2f; при увеличении x%s на единицу z меняется на w%s = %s, а шансы умножаются на e^(w%s) = %.2f. "+
			"Типичные ошибки — выдавать z вместо вероятности, забывать смещение, терять знак в экспоненте и путать вероятность с шансами.",
			strings.Join(zParts, " + "), tenths(b), fmtNum(z), negZ, p1, p1, 1-p1, math.Exp(z), subscripts[j], subscripts[j], tenths(w[j]), subscripts[j], math.Exp(float64(w[j])/10)),
	}, nil
}

// giniBestSplit — лучший порог первого разбиения дерева по критерию Джини на маленькой
// одномерной выборке.
type giniBestSplit struct{}

func (giniBestSplit) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.models.gini-best-split.v1", Spec: itembank.SpecDataML, Topic: "models",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.6, B: 1.0, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"decision-tree"},
	}
}

// gini — примесь Джини узла с n объектами, из них pos положительных.
func gini(pos, n int) float64 {
	if n == 0 {
		return 0
	}
	p := float64(pos) / float64(n)
	return 1 - p*p - (1-p)*(1-p)
}

// weightedGini — взвешенная примесь после разбиения labels[:left] | labels[left:].
func weightedGini(labels []int, left int) float64 {
	n := len(labels)
	posL, posR := sumInts(labels[:left]), sumInts(labels[left:])
	return (float64(left)*gini(posL, left) + float64(n-left)*gini(posR, n-left)) / float64(n)
}

func (f giniBestSplit) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A62)
	feature := itembank.Pick(r, []string{"age", "income", "tenure", "score", "visits"})
	label := itembank.Pick(r, []string{"churn", "default", "fraud", "y"})
	n := itembank.IntBetween(r, 6, 8)
	grid := make([]int, 12)
	for i := range grid {
		grid[i] = i + 1
	}
	var xs, labels []int
	var ginis []float64
	best := 0
	for {
		xs = itembank.PickN(r, grid, n)
		sort.Ints(xs)
		labels = make([]int, n)
		for i := range labels {
			labels[i] = r.IntN(2)
		}
		pos := sumInts(labels)
		if pos == 0 || pos == n {
			continue
		}
		ginis = make([]float64, n-1)
		best = 0
		for i := range ginis {
			ginis[i] = weightedGini(labels, i+1)
			if ginis[i] < ginis[best] {
				best = i
			}
		}
		second := math.Inf(1)
		for i, g := range ginis {
			if i != best && g < second {
				second = g
			}
		}
		// Минимум единственный с запасом, и после разбиения остаётся примесь (задача не вырождена).
		if second-ginis[best] >= 0.02 && ginis[best] > 0.01 {
			break
		}
	}
	threshold := float64(xs[best]+xs[best+1]) / 2
	var md strings.Builder
	fmt.Fprintf(&md, "| %s | %s |\n|---|---|\n", feature, label)
	for i := range xs {
		fmt.Fprintf(&md, "| %d | %d |\n", xs[i], labels[i])
	}
	rule := fmt.Sprintf("Дерево решений выбирает первое разбиение по признаку `%s` с критерием Джини: перебираются пороги вида `%s ≤ t`, где t — середина между соседними значениями признака, "+
		"и берётся порог с минимальной взвешенной (по долям объектов) примесью Джини двух дочерних узлов.", feature, feature)
	var question string
	var answer, tol float64
	if r.IntN(2) == 0 {
		question, answer, tol = "Какой порог t будет выбран?", threshold, 0.01
	} else {
		question, answer, tol = "Чему равна взвешенная примесь Джини дочерних узлов после лучшего разбиения? Округлите до сотых.", round2(ginis[best]), 0.011
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("%s Обучающая выборка:\n\n%s\n%s", rule, md.String(), question),
		fmt.Sprintf("Дана выборка с бинарной меткой `%s`:\n\n%s\n%s %s", label, md.String(), rule, question),
	})
	notes := make([]string, 0, n-1)
	for i, g := range ginis {
		notes = append(notes, fmt.Sprintf("t = %s → %.2f", fmtNum(float64(xs[i]+xs[i+1])/2), g))
	}
	left := best + 1
	posL, posR := sumInts(labels[:left]), sumInts(labels[left:])
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Answer: itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: fmt.Sprintf("Взвешенная примесь по порогам: %s. Минимум при t = %s: слева объектов %d, из них класса 1 — %d, G = 1 − (%d/%d)² − (%d/%d)² = %.2f; "+
			"справа объектов %d, из них класса 1 — %d, G = %.2f; взвешенная (%d·%.2f + %d·%.2f)/%d = %.2f. Типичные ошибки — брать порог равным значению признака вместо середины, "+
			"не взвешивать примеси долями объектов, искать максимум вместо минимума или подменять Джини энтропией.",
			strings.Join(notes, ", "), fmtNum(threshold), left, posL, posL, left, left-posL, left, gini(posL, left),
			n-left, posR, gini(posR, n-left), left, gini(posL, left), n-left, gini(posR, n-left), n, ginis[best]),
	}, nil
}

// abSampleSize — размер группы A/B-теста по формуле для двух долей: двусторонний квантиль,
// мощность, относительный против абсолютного эффекта, группа против всего теста.
type abSampleSize struct{}

func (abSampleSize) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.experiment_design.ab-sample-size.v1", Spec: itembank.SpecDataML, Topic: "experiment_design",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.4, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"ab-test"},
	}
}

var abMetrics = []string{
	"конверсия в покупку", "доля кликов по баннеру (CTR)", "доля пользователей, завершивших регистрацию",
	"retention 7-го дня", "доля открытий письма", "доля продлений подписки",
}

func (f abSampleSize) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A63)
	metric := itembank.Pick(r, abMetrics)
	p1pct := float64(itembank.Pick(r, []int{2, 3, 4, 5, 6, 8, 10, 12, 15, 20, 25}))
	p1 := p1pct / 100
	var p2 float64
	var effect, effectNote string
	if r.IntN(2) == 0 {
		uplift := itembank.Pick(r, []int{5, 10, 15, 20, 25, 30})
		p2 = p1 * (1 + float64(uplift)/100)
		effect = fmt.Sprintf("относительный прирост на %d %%", uplift)
		effectNote = fmt.Sprintf("относительный прирост: p₂ = p₁·(1 + %d/100) = %s, а не p₁ + 0.%02d", uplift, fmtNum(round4(p2)), uplift)
	} else {
		pp := itembank.Pick(r, []float64{0.5, 1, 1.5, 2, 3})
		p2 = (p1pct + pp) / 100
		effect = fmt.Sprintf("абсолютный прирост на %s п.п.", fmtNum(pp))
		effectNote = fmt.Sprintf("абсолютный прирост: p₂ = %s %% = %s", fmtNum(p1pct+pp), fmtNum(round4(p2)))
	}
	alpha, zA, zOne := 0.05, 1.96, 1.645
	if r.IntN(3) == 0 {
		alpha, zA, zOne = 0.01, 2.576, 2.326
	}
	power, zB := 80, 0.842
	if r.IntN(2) == 0 {
		power, zB = 90, 1.282
	}
	variance := p1*(1-p1) + p2*(1-p2)
	delta := p2 - p1
	nExact := (zA + zB) * (zA + zB) * variance / (delta * delta)
	perGroup := math.Ceil(nExact)
	answer, scope, scopeNote := perGroup, "в каждой группе", "это размер одной группы"
	if r.IntN(3) == 0 {
		answer, scope, scopeNote = 2*perGroup, "суммарно в обеих группах (одинакового размера)", "на обе группы вдвое больше"
	}
	tol := math.Max(2, math.Round(answer*0.01))
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("В A/B-тесте %s в контроле равна p₁ = %s %%. Нужно обнаружить %s при двустороннем уровне значимости α = %s и мощности %d %%.", metric, fmtNum(p1pct), effect, fmtNum(alpha), power),
		fmt.Sprintf("Планируется A/B-тест: базовое значение метрики «%s» p₁ = %s %%, минимальный детектируемый эффект — %s, двусторонний критерий с α = %s, мощность %d %%.", metric, fmtNum(p1pct), effect, fmtNum(alpha), power),
	}) + fmt.Sprintf(" Размер группы оценивают по формуле n = (z₁₋α/₂ + z₁₋β)²·(p₁(1 − p₁) + p₂(1 − p₂))/(p₂ − p₁)². "+
		"Квантили стандартного нормального распределения: z₀.₈ = 0.842, z₀.₉ = 1.282, z₀.₉₅ = 1.645, z₀.₉₇₅ = 1.96, z₀.₉₉ = 2.326, z₀.₉₉₅ = 2.576. "+
		"Сколько пользователей нужно %s? Округлите вверх до целого.", scope)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Answer: itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: fmt.Sprintf("Критерий двусторонний, поэтому берём z₁₋α/₂ = %s (а не z₁₋α = %s); для мощности %d %% z₁₋β = %s. Эффект — %s. "+
			"p₁(1 − p₁) + p₂(1 − p₂) = %.5f, (p₂ − p₁)² = %.7f, n = %s²·%.5f/%.7f = %.1f → %d на группу, %s: %d. "+
			"Типичные ошибки — брать односторонний квантиль, путать относительный и абсолютный прирост, подставлять проценты вместо долей и выдавать размер одной группы за размер всего теста (или наоборот).",
			fmtNum(zA), fmtNum(zOne), power, fmtNum(zB), effectNote, variance, delta*delta, fmtNum(zA+zB), variance, delta*delta, nExact, int(perGroup), scopeNote, int(answer)),
	}, nil
}

// round4 округляет до десятитысячных.
func round4(x float64) float64 { return math.Round(x*10000) / 10000 }
