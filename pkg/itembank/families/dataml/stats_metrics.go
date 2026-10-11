package dataml

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(confidenceInterval{})
	itembank.Register(binomialPValue{})
	itembank.Register(pipelineOrder{})
	itembank.Register(multipleTesting{})
}

// confidenceInterval — доверительный интервал для среднего по нормальному приближению:
// стандартная ошибка s/√n, двусторонний квантиль, граница против полуширины.
type confidenceInterval struct{}

func (confidenceInterval) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.statistics.confidence-interval.v1", Spec: itembank.SpecDataML, Topic: "statistics",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.4, B: -0.5, C: 0}, TimeLimit: 120 * time.Second,
	}
}

func (f confidenceInterval) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A70)
	ctx := itembank.Pick(r, testContexts)
	xbar := itembank.Pick(r, ctx.mu0) + itembank.IntBetween(r, -9, 9)
	s := int(math.Round(float64(xbar) * itembank.Pick(r, []float64{0.05, 0.08, 0.1, 0.12, 0.15, 0.2})))
	if s < 1 {
		s = 1
	}
	n := itembank.Pick(r, []int{16, 25, 36, 49, 64, 100, 144, 225, 400})
	sqrtN := int(math.Sqrt(float64(n)))
	conf, z := 95, 1.96
	switch r.IntN(3) {
	case 0:
		conf, z = 90, 1.645
	case 1:
		conf, z = 99, 2.576
	}
	se := float64(s) / float64(sqrtN)
	half := z * se
	lo, hi := float64(xbar)-half, float64(xbar)+half
	var what string
	var answer float64
	switch r.IntN(4) {
	case 0:
		what, answer = "нижнюю границу интервала", lo
	case 1:
		what, answer = "верхнюю границу интервала", hi
	case 2:
		what, answer = "полуширину интервала (z·s/√n)", half
	default:
		what, answer = "ширину интервала (разность верхней и нижней границ)", 2*half
	}
	zTable := "z₀.₉₅ = 1.645, z₀.₉₇₅ = 1.96, z₀.₉₉₅ = 2.576"
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("По выборке из n = %d измерений %s составило x̄ = %d %s при выборочном стандартном отклонении s = %d %s. Постройте %d%%-й доверительный интервал для среднего "+
			"по нормальному приближению x̄ ± z·s/√n (квантили: %s). Укажите %s. Округлите до сотых.", n, ctx.what, xbar, ctx.unit, s, ctx.unit, conf, zTable, what),
		fmt.Sprintf("Измерено %s: n = %d, выборочное среднее x̄ = %d %s, стандартное отклонение s = %d %s. Для %d%%-го двустороннего доверительного интервала среднего "+
			"x̄ ± z·s/√n (квантили нормального распределения: %s) найдите %s. Округлите до сотых.", ctx.what, n, xbar, ctx.unit, s, ctx.unit, conf, zTable, what),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Answer: itembank.Answer{Number: round2(answer), Tolerance: 0.011},
		Explanation: fmt.Sprintf("Стандартная ошибка среднего s/√n = %d/%d = %.2f. Интервал двусторонний, поэтому для %d %% берём квантиль уровня %s: z = %s; полуширина z·s/√n = %.2f, "+
			"интервал [%.2f; %.2f], ширина %.2f. Типичные ошибки — делить s на n вместо √n или не делить вовсе, брать квантиль уровня %s (одностороннего) и путать полуширину с шириной.",
			s, sqrtN, se, conf, fmtNum(1-(1-float64(conf)/100)/2), fmtNum(z), half, lo, hi, 2*half, fmtNum(float64(conf)/100)),
	}, nil
}

// binomialPValue — точный биномиальный критерий при p = 0.5 на маленьких n: одностороннее
// и двустороннее p-value как сумма вероятностей хвоста.
type binomialPValue struct{}

func (binomialPValue) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.statistics.binomial-pvalue.v1", Spec: itembank.SpecDataML, Topic: "statistics",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.6, B: 0.8, C: 0}, TimeLimit: 180 * time.Second,
	}
}

// binomScenario — сюжет для биномиального критерия; story принимает имя, n и k.
type binomScenario struct {
	names []string
	story string // %s — имя, %d — n, %d — k
	h0    string
	h1    string
}

var binomScenarios = []binomScenario{
	{[]string{"Анна", "Борис", "Вера", "Глеб", "Дарья", "Егор"},
		"Дегустатор %s в слепом тесте из %d пар правильно отличил новый рецепт от старого в %d случаях",
		"дегустатор угадывает (p = 0.5)", "дегустатор различает рецепты (p > 0.5)"},
	{[]string{"gbm-v2", "ranker-x", "cnn-small", "bert-tiny", "xgb-tuned"},
		"Модель %s на %d независимых тестовых наборах оказалась точнее базовой модели в %d случаях",
		"модели равносильны (p = 0.5)", "новая модель точнее (p > 0.5)"},
	{[]string{"Север", "Орбита", "Вектор", "Квант", "Импульс"},
		"Команда «%s» из %d матчей с равными по рейтингу соперниками выиграла %d",
		"шансы равны (p = 0.5)", "команда сильнее соперников (p > 0.5)"},
	{[]string{"Ольга", "Пётр", "Кира", "Лев", "Мария", "Никита"},
		"Аналитик %s проверяет монету: из %d бросков орёл выпал %d раз",
		"монета симметрична (p = 0.5)", "орёл выпадает чаще (p > 0.5)"},
	{[]string{"корзина", "чекаут", "онбординг", "поиск", "профиль"},
		"В парном сравнении экрана «%s» из %d респондентов новый дизайн предпочли %d",
		"предпочтения равновероятны (p = 0.5)", "новый дизайн предпочитают чаще (p > 0.5)"},
}

// binom — биномиальный коэффициент C(n, k).
func binom(n, k int) int {
	res := 1
	for i := 1; i <= k; i++ {
		res = res * (n - k + i) / i
	}
	return res
}

func (f binomialPValue) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A71)
	sc := itembank.Pick(r, binomScenarios)
	name := itembank.Pick(r, sc.names)
	n := itembank.IntBetween(r, 6, 10)
	k := itembank.IntBetween(r, n/2+2, n-1) // строго больше половины и не все n: хвост нетривиален
	twoSided := r.IntN(2) == 0
	tail := 0
	terms := make([]string, 0, n-k+1)
	for i := k; i <= n; i++ {
		tail += binom(n, i)
		terms = append(terms, fmt.Sprintf("C(%d,%d) = %d", n, i, binom(n, i)))
	}
	total := 1 << n
	oneSided := float64(tail) / float64(total)
	answer := oneSided
	side := "одностороннее"
	alt := sc.h1
	if twoSided {
		answer = 2 * oneSided
		side = "двустороннее"
		alt = strings.Replace(sc.h1, "p > 0.5", "p ≠ 0.5", 1)
	}
	story := fmt.Sprintf(sc.story, name, n, k)
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("%s. Проверяется H0: %s против H1: %s точным биномиальным критерием. Чему равно %s p-value? Округлите до тысячных.", story, sc.h0, alt, side),
		fmt.Sprintf("%s. Нулевая гипотеза — %s, альтернатива — %s. Вычислите %s p-value точного биномиального критерия (без нормального приближения). Округлите до тысячных.", story, sc.h0, alt, side),
	})
	doubling := "Для односторонней альтернативы p-value = P(X ≥ k) и равно этой сумме."
	if twoSided {
		doubling = fmt.Sprintf("Для двусторонней альтернативы при симметричном p = 0.5 p-value удваивается: 2·%.4f = %.4f.", oneSided, answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Answer: itembank.Answer{Number: math.Round(answer*1000) / 1000, Tolerance: 0.0006},
		Explanation: fmt.Sprintf("При H0 число успехов X ~ Bin(%d, 0.5), всего 2^%d = %d равновероятных исходов. Хвост «не менее %d успехов»: %s, сумма %d, P(X ≥ %d) = %d/%d = %.4f. %s "+
			"Типичные ошибки — брать только P(X = k), считать P(X > k) без самого k, забывать удвоение для двусторонней альтернативы (или удваивать одностороннее) и сравнивать долю k/n с 0.5 вместо расчёта вероятности.",
			n, n, total, k, strings.Join(terms, ", "), tail, k, tail, total, oneSided, doubling),
	}, nil
}

// pipelineOrder — порядок шагов честной оценки на отложенной выборке: разбиение раньше fit
// преобразователя, test только через transform, метрика последней.
type pipelineOrder struct{}

func (pipelineOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.metrics_validation.pipeline-order.v1", Spec: itembank.SpecDataML, Topic: "metrics_validation",
		Grade: itembank.GradeJunior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.4, B: -0.7, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"sklearn", "python"},
	}
}

type preprocessor struct{ class, what string }

var preprocessors = []preprocessor{
	{"StandardScaler", "среднее и дисперсия признаков"},
	{"MinMaxScaler", "минимум и максимум признаков"},
	{"SimpleImputer(strategy=\"median\")", "медианы для заполнения пропусков"},
	{"OneHotEncoder(handle_unknown=\"ignore\")", "словарь категорий"},
	{"TfidfVectorizer", "словарь и idf-веса"},
}

type modelMetric struct{ model, metric string }

var modelMetrics = []modelMetric{
	{"LogisticRegression", "ROC-AUC"}, {"RandomForestClassifier", "F1"}, {"GradientBoostingClassifier", "ROC-AUC"},
	{"KNeighborsClassifier", "accuracy"}, {"Ridge", "RMSE"}, {"RandomForestRegressor", "MAE"}, {"LinearSVC", "F1"},
}

func (f pipelineOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A72)
	pre := itembank.Pick(r, preprocessors)
	mm := itembank.Pick(r, modelMetrics)
	preVar := itembank.Pick(r, []string{"prep", "scaler", "transformer", "enc"})
	modelVar := itembank.Pick(r, []string{"model", "clf", "estimator"})
	merged := r.IntN(2) == 0
	steps := []string{}
	if merged && r.IntN(2) == 0 {
		steps = append(steps, itembank.Pick(r, []string{
			"Перемешать строки датасета с фиксированным random_state",
			"Загрузить датасет и отделить матрицу признаков X от целевой переменной y",
		}))
	}
	steps = append(steps, fmt.Sprintf("Разбить данные на train и test: `train_test_split(X, y, test_size=%s)`", itembank.Pick(r, []string{"0.2", "0.25", "0.3"})))
	if merged {
		steps = append(steps, fmt.Sprintf("Выполнить `X_train = %s.fit_transform(X_train)` для `%s = %s()`", preVar, preVar, pre.class))
	} else {
		steps = append(steps,
			fmt.Sprintf("Обучить преобразователь `%s = %s()` на train: `%s.fit(X_train)`", preVar, pre.class, preVar),
			fmt.Sprintf("Преобразовать train: `X_train = %s.transform(X_train)`", preVar))
	}
	steps = append(steps,
		fmt.Sprintf("Обучить модель `%s = %s()` на преобразованном train: `%s.fit(X_train, y_train)`", modelVar, mm.model, modelVar),
		fmt.Sprintf("Преобразовать test тем же обученным `%s.transform(X_test)` и получить `%s.predict(...)`", preVar, modelVar),
		fmt.Sprintf("Посчитать %s по предсказаниям на test", mm.metric))
	options, order := itembank.OrderingOptions(r, steps)
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Оценивается модель %s на отложенной выборке; признаки предобрабатываются `%s`. Расставьте шаги в порядке, при котором оценка %s на test будет честной — без утечки информации из test в обучение.",
			mm.model, pre.class, mm.metric),
		fmt.Sprintf("Нужно получить несмещённую оценку %s для %s с предобработкой `%s`. Расположите шаги в правильной последовательности.",
			mm.metric, mm.model, pre.class),
		fmt.Sprintf("Расставьте шаги подготовки данных и оценки %s так, чтобы `%s` не «увидел» test при обучении (holdout-оценка метрики %s).",
			mm.model, pre.class, mm.metric),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    stem,
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("`%s` при fit запоминает статистики (%s), и они должны вычисляться только по train — значит, разбиение идёт раньше fit. "+
			"Модель обучается на уже преобразованном train, test преобразуется тем же обученным объектом через `transform` (не `fit_transform`), и только после этого считается %s. "+
			"Если обучить преобразователь на всех данных до разбиения, информация о test просочится в обучение и метрика окажется завышенной. "+
			"Типичные ошибки — `fit_transform` на всём датасете перед `train_test_split` и повторный `fit` преобразователя на test.",
			pre.class, pre.what, mm.metric),
	}, nil
}

// multipleTesting — какие гипотезы отвергаются после поправки Бонферрони или Холма на
// множественное сравнение.
type multipleTesting struct{}

func (multipleTesting) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.experiment_design.multiple-testing.v1", Spec: itembank.SpecDataML, Topic: "experiment_design",
		Grade: itembank.GradeSenior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.6, B: 0.7, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"ab-test"},
	}
}

var abTestMetrics = []string{"конверсия", "средний чек", "retention", "CTR", "время сессии", "доля отказов", "ARPU", "число заказов"}

// rejectedSet возвращает множество индексов отвергнутых гипотез (в тысячных долях p-value).
func rejectedSet(pMilli []int, alphaMilli int, holm bool) map[int]bool {
	m := len(pMilli)
	out := map[int]bool{}
	if !holm {
		for i, p := range pMilli {
			if p*m <= alphaMilli {
				out[i] = true
			}
		}
		return out
	}
	idx := make([]int, m)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return pMilli[idx[a]] < pMilli[idx[b]] })
	for j, i := range idx {
		// Сравнение p_(j) ≤ α/(m − j) в целых числах: p·(m − j) ≤ α.
		if pMilli[i]*(m-j) > alphaMilli {
			break
		}
		out[i] = true
	}
	return out
}

func sameKeySet(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func (f multipleTesting) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A73)
	m := itembank.IntBetween(r, 5, 6)
	alphaMilli := itembank.Pick(r, []int{50, 100})
	holm := r.IntN(5) < 3
	wantDiff := holm && r.IntN(10) < 7
	var pMilli []int
	var rejected map[int]bool
	for {
		pMilli = make([]int, 0, m)
		seen := map[int]bool{}
		for len(pMilli) < m {
			var p int
			switch r.IntN(3) {
			case 0:
				p = itembank.IntBetween(r, 1, 9)
			case 1:
				p = itembank.IntBetween(r, 10, 49)
			default:
				p = itembank.IntBetween(r, 5, 30) * 10
			}
			if !seen[p] {
				seen[p] = true
				pMilli = append(pMilli, p)
			}
		}
		rejected = rejectedSet(pMilli, alphaMilli, holm)
		if len(rejected) < 2 || len(rejected) > 3 || len(rejected) > m-2 {
			continue
		}
		naive := map[int]bool{}
		for i, p := range pMilli {
			if p <= alphaMilli {
				naive[i] = true
			}
		}
		if sameKeySet(naive, rejected) {
			continue
		}
		if wantDiff && sameKeySet(rejectedSet(pMilli, alphaMilli, false), rejected) {
			continue
		}
		break
	}
	metrics := itembank.PickN(r, abTestMetrics, m)
	texts := make([]string, m)
	var correct, wrong []string
	for i := range pMilli {
		texts[i] = fmt.Sprintf("«%s» (p = %s)", metrics[i], fmtNum(float64(pMilli[i])/1000))
		if rejected[i] {
			correct = append(correct, texts[i])
		} else {
			wrong = append(wrong, texts[i])
		}
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	alpha := fmtNum(float64(alphaMilli) / 1000)
	method := fmt.Sprintf("Бонферрони: каждое p-value сравнивается с α/m = %s/%d", alpha, m)
	if holm {
		method = fmt.Sprintf("Холма (Холма — Бонферрони): p-value упорядочивают по возрастанию, j-е по порядку сравнивают с α/(m − j + 1) при α = %s, m = %d, "+
			"и останавливаются на первом неотвергнутом — оно и все большие не отвергаются", alpha, m)
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("В A/B-тесте одновременно проверяли %d метрик; p-value каждой приведено в вариантах. Чтобы удержать вероятность хотя бы одного ложного открытия на уровне α = %s, "+
			"применяют поправку %s. Какие гипотезы будут отвергнуты? Выберите все подходящие.", m, alpha, method),
		fmt.Sprintf("По итогам эксперимента получены p-value для %d метрик (см. варианты). Групповой уровень значимости α = %s, поправка на множественные сравнения — %s. "+
			"Отметьте все метрики, по которым нулевая гипотеза отвергается.", m, alpha, method),
	})
	// Пояснение: пошаговая проверка в порядке возрастания p-value.
	idx := make([]int, m)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return pMilli[idx[a]] < pMilli[idx[b]] })
	stepsNote := make([]string, 0, m)
	for j, i := range idx {
		thr := float64(alphaMilli) / 1000 / float64(m)
		if holm {
			thr = float64(alphaMilli) / 1000 / float64(m-j)
		}
		verdict := "не отвергается"
		if rejected[i] {
			verdict = "отвергается"
		}
		stepsNote = append(stepsNote, fmt.Sprintf("%s: %s %s %.4f → %s", texts[i], fmtNum(float64(pMilli[i])/1000), cmpSign(float64(pMilli[i])/1000, thr), thr, verdict))
	}
	naiveCount := 0
	for _, p := range pMilli {
		if p <= alphaMilli {
			naiveCount++
		}
	}
	contrast := fmt.Sprintf("Поправка Холма отвергла бы %d гипотез", len(rejectedSet(pMilli, alphaMilli, true)))
	if holm {
		contrast = fmt.Sprintf("Поправка Бонферрони (все сравнения с α/m) отвергла бы %d гипотез", len(rejectedSet(pMilli, alphaMilli, false)))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    stem,
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: fmt.Sprintf("Проверяем в порядке возрастания p-value: %s. Без поправки (сравнение с α = %s) отверглись бы %d гипотез — это и есть инфляция ложных открытий. %s. "+
			"Типичные ошибки — сравнивать с α без поправки, применять α/m ко всем гипотезам там, где процедура Холма ступенчатая, или продолжать отвергать после первого неотвергнутого p-value.",
			strings.Join(stepsNote, "; "), alpha, naiveCount, contrast),
	}, nil
}

// cmpSign — знак сравнения для пояснения: ≤ или >.
func cmpSign(a, b float64) string {
	if a <= b+1e-12 {
		return "≤"
	}
	return ">"
}
