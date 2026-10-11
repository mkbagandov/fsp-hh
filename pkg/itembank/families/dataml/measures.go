package dataml

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(varianceStd{})
	itembank.Register(pearsonCorrelation{})
	itembank.Register(zTestConclusion{})
	itembank.Register(maeRmse{})
	itembank.Register(rocAucPairs{})
	itembank.Register(cvScheme{})
}

// intsCSV форматирует числа через запятую: "3, 8, 1".
func intsCSV(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ", ")
}

// meanInts — среднее арифметическое.
func meanInts(xs []int) float64 {
	return float64(sumInts(xs)) / float64(len(xs))
}

// sumSqDev — сумма квадратов отклонений от среднего.
func sumSqDev(xs []int) float64 {
	m := meanInts(xs)
	ss := 0.0
	for _, x := range xs {
		d := float64(x) - m
		ss += d * d
	}
	return ss
}

// varianceStd — дисперсия и стандартное отклонение: делитель n против n − 1.
type varianceStd struct{}

func (varianceStd) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.statistics.variance-std.v1", Spec: itembank.SpecDataML, Topic: "statistics",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -0.6, C: 0}, TimeLimit: 120 * time.Second,
	}
}

func (f varianceStd) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A30)
	n := itembank.IntBetween(r, 5, 6)
	xs := make([]int, n)
	ss := 0.0
	for ss == 0 {
		for i := range xs {
			xs[i] = itembank.IntBetween(r, 1, 20)
		}
		ss = sumSqDev(xs)
	}
	varS, varP := ss/float64(n-1), ss/float64(n)
	var answer float64
	var what string
	switch r.IntN(4) {
	case 0:
		answer, what = varS, "несмещённую выборочную дисперсию (делитель n − 1)"
	case 1:
		answer, what = varP, "дисперсию как среднее квадратов отклонений от среднего (делитель n)"
	case 2:
		answer, what = math.Sqrt(varS), "выборочное стандартное отклонение (корень из дисперсии с делителем n − 1)"
	default:
		answer, what = math.Sqrt(varP), "стандартное отклонение с делителем n (как `np.std` по умолчанию)"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   fmt.Sprintf("Дана выборка: %s. Найдите %s. Округлите до сотых.", intsCSV(xs), what),
		Answer: itembank.Answer{Number: round2(answer), Tolerance: 0.011},
		Explanation: fmt.Sprintf("Среднее %.2f, сумма квадратов отклонений %.2f. С делителем n − 1 = %d дисперсия равна %.2f (σ ≈ %.2f), "+
			"с делителем n = %d — %.2f (σ ≈ %.2f); ответ %.2f. Типичная ошибка — взять другой делитель (pandas `.var()`/`.std()` по умолчанию "+
			"используют n − 1, numpy — n) или забыть извлечь корень для стандартного отклонения.",
			meanInts(xs), ss, n-1, varS, math.Sqrt(varS), n, varP, math.Sqrt(varP), answer),
	}, nil
}

// pearsonCorrelation — коэффициент корреляции Пирсона или выборочная ковариация по 5–6 точкам.
type pearsonCorrelation struct{}

func (pearsonCorrelation) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.statistics.pearson-correlation.v1", Spec: itembank.SpecDataML, Topic: "statistics",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.8, C: 0}, TimeLimit: 180 * time.Second,
	}
}

// linearPoints строит n точек вокруг прямой y = a·x + b с целым шумом из [−noise, noise].
func linearPoints(r *rand.Rand, n, a, b, noise int) (xs, ys []int) {
	grid := make([]int, 12)
	for i := range grid {
		grid[i] = i + 1
	}
	xs = itembank.PickN(r, grid, n)
	sort.Ints(xs)
	ys = make([]int, n)
	for i, x := range xs {
		ys[i] = a*x + b + itembank.IntBetween(r, -noise, noise)
	}
	return xs, ys
}

func pairsText(xs, ys []int) string {
	parts := make([]string, len(xs))
	for i := range xs {
		parts[i] = fmt.Sprintf("(%d, %d)", xs[i], ys[i])
	}
	return strings.Join(parts, ", ")
}

// crossSums возвращает Sxx, Syy, Sxy.
func crossSums(xs, ys []int) (sxx, syy, sxy float64) {
	mx, my := meanInts(xs), meanInts(ys)
	for i := range xs {
		dx, dy := float64(xs[i])-mx, float64(ys[i])-my
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	return sxx, syy, sxy
}

func (f pearsonCorrelation) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A31)
	n := itembank.IntBetween(r, 5, 6)
	a := itembank.Pick(r, []int{-2, -1, 1, 2})
	b := itembank.IntBetween(r, 2, 15)
	if a < 0 {
		b = itembank.IntBetween(r, 30, 45)
	}
	var xs, ys []int
	var sxx, syy, sxy float64
	for syy == 0 {
		xs, ys = linearPoints(r, n, a, b, 2)
		sxx, syy, sxy = crossSums(xs, ys)
	}
	pearson := sxy / math.Sqrt(sxx*syy)
	cov := sxy / float64(n-1)
	answer, what := pearson, "коэффициент корреляции Пирсона"
	if r.IntN(3) == 0 {
		answer, what = cov, "выборочную ковариацию (делитель n − 1)"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   fmt.Sprintf("Даны наблюдения (x, y): %s. Найдите %s. Округлите до сотых.", pairsText(xs, ys), what),
		Answer: itembank.Answer{Number: round2(answer), Tolerance: 0.011},
		Explanation: fmt.Sprintf("x̄ = %.2f, ȳ = %.2f; Sxy = Σ(x − x̄)(y − ȳ) = %.2f, Sxx = %.2f, Syy = %.2f. "+
			"r = Sxy / √(Sxx·Syy) = %.2f, ковариация = Sxy / (n − 1) = %.2f. Типичная ошибка — делить на произведение дисперсий вместо "+
			"стандартных отклонений, смешивать делители n и n − 1 или терять знак при отрицательной зависимости.",
			meanInts(xs), meanInts(ys), sxy, sxx, syy, pearson, cov),
	}, nil
}

// zTestConclusion — корректный вывод по z-критерию: статистика, критическое значение,
// интерпретация.
type zTestConclusion struct{}

func (zTestConclusion) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.statistics.z-test-conclusion.v1", Spec: itembank.SpecDataML, Topic: "statistics",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.3, C: 0.25}, TimeLimit: 150 * time.Second,
	}
}

type testContext struct {
	what string
	unit string
	mu0  []int
}

var testContexts = []testContext{
	{"среднее время ответа сервиса", "мс", []int{120, 200, 250, 300}},
	{"средняя масса упаковки", "г", []int{250, 500, 1000}},
	{"среднее время доставки", "мин", []int{30, 45, 60, 90}},
	{"средний чек", "руб.", []int{900, 1200, 1500, 2000}},
}

func (f zTestConclusion) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A32)
	ctx := itembank.Pick(r, testContexts)
	mu0 := itembank.Pick(r, ctx.mu0)
	s := int(math.Round(float64(mu0) * itembank.Pick(r, []float64{0.05, 0.08, 0.1, 0.12, 0.15})))
	if s < 1 {
		s = 1
	}
	n := itembank.Pick(r, []int{25, 36, 49, 64, 100, 144})
	reject := r.IntN(2) == 0
	sign := float64(itembank.Pick(r, []int{-1, 1}))
	se := float64(s) / math.Sqrt(float64(n))
	var xbar, z float64
	for {
		target := 0.5 + r.Float64()*1.4
		if reject {
			target = 2.1 + r.Float64()*1.1
		}
		xbar = math.Round((float64(mu0)+sign*target*se)*10) / 10
		z = (xbar - float64(mu0)) / se
		if (reject && math.Abs(z) >= 2.0) || (!reject && math.Abs(z) <= 1.9) {
			break
		}
	}
	zw := (xbar - float64(mu0)) / float64(s)
	var correct string
	var distractors []string
	if reject {
		correct = fmt.Sprintf("z ≈ %.2f, |z| > 1.96: H0 отвергается на уровне 5 %%, 95%%-й доверительный интервал для μ не содержит %d", z, mu0)
		distractors = []string{
			fmt.Sprintf("z ≈ %.2f, |z| < 1.96: H0 не отвергается, данных недостаточно для вывода о сдвиге среднего", zw),
			"H0 отвергается, следовательно вероятность того, что H0 верна, меньше 5 %",
			fmt.Sprintf("Различие значимо, значит с вероятностью 95 %% истинное среднее равно %.1f", xbar),
		}
	} else {
		correct = fmt.Sprintf("z ≈ %.2f, |z| < 1.96: H0 не отвергается, 95%%-й доверительный интервал для μ содержит %d", z, mu0)
		oneSided := fmt.Sprintf("z ≈ %.2f, x̄ ≠ %d: H0 отвергается, так как выборочное среднее отличается от заявленного", z, mu0)
		if math.Abs(z) > 1.645 {
			oneSided = fmt.Sprintf("z ≈ %.2f, |z| > 1.645: H0 отвергается на уровне 5 %%", z)
		}
		distractors = []string{
			oneSided,
			fmt.Sprintf("H0 не отвергается, следовательно доказано, что μ = %d", mu0),
			"H0 не отвергается, значит вероятность того, что H0 верна, равна 95 %",
		}
	}
	options, key := itembank.ShuffledOptions(r, correct, distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Заявлено, что %s равно %d %s. По выборке из n = %d измерений получили x̄ = %.1f %s при стандартном отклонении s = %d %s. "+
			"Проверяется H0: μ = %d против двусторонней альтернативы на уровне α = 0.05 (z-критерий, критическое значение 1.96). Какой вывод корректен?",
			ctx.what, mu0, ctx.unit, n, xbar, ctx.unit, s, ctx.unit, mu0),
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("z = (x̄ − μ0) / (s / √n) = (%.1f − %d) / (%d / %d) = %.2f; сравниваем |z| с 1.96. %s "+
			"Типичные ошибки — делить на s без √n (получилось бы %.2f), применять одностороннее критическое значение 1.645 к двусторонней гипотезе "+
			"и трактовать уровень значимости или p-value как вероятность истинности H0: критерий говорит лишь о совместимости данных с H0.",
			xbar, mu0, s, int(math.Sqrt(float64(n))), z, zTestVerdict(reject, mu0), zw),
	}, nil
}

func zTestVerdict(reject bool, mu0 int) string {
	if reject {
		return fmt.Sprintf("|z| > 1.96, поэтому H0 отвергается, а 95%%-й интервал x̄ ± 1.96·s/√n не накрывает %d.", mu0)
	}
	return fmt.Sprintf("|z| < 1.96, поэтому оснований отвергнуть H0 нет, интервал x̄ ± 1.96·s/√n накрывает %d; это не доказывает равенство μ = %d.", mu0, mu0)
}

// maeRmse — MAE, MSE и RMSE по 5–6 парам «истина — прогноз».
type maeRmse struct{}

func (maeRmse) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.metrics_validation.mae-rmse.v1", Spec: itembank.SpecDataML, Topic: "metrics_validation",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.7, C: 0}, TimeLimit: 120 * time.Second,
	}
}

func (f maeRmse) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A40)
	n := itembank.IntBetween(r, 5, 6)
	y := make([]int, n)
	yhat := make([]int, n)
	errs := make([]int, n)
	absSum, sqSum := 0, 0
	for absSum == 0 {
		absSum, sqSum = 0, 0
		for i := range y {
			y[i] = itembank.IntBetween(r, 10, 100)
			errs[i] = itembank.IntBetween(r, -12, 12)
			yhat[i] = y[i] + errs[i]
			if errs[i] < 0 {
				absSum -= errs[i]
			} else {
				absSum += errs[i]
			}
			sqSum += errs[i] * errs[i]
		}
	}
	mae := float64(absSum) / float64(n)
	mse := float64(sqSum) / float64(n)
	rmse := math.Sqrt(mse)
	var answer float64
	var what string
	switch r.IntN(3) {
	case 0:
		answer, what = mae, "MAE (среднюю абсолютную ошибку)"
	case 1:
		answer, what = mse, "MSE (среднеквадратичную ошибку)"
	default:
		answer, what = rmse, "RMSE (корень из среднеквадратичной ошибки)"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Истинные значения y = [%s], предсказания модели ŷ = [%s]. Вычислите %s. Округлите до сотых.",
			intsCSV(y), intsCSV(yhat), what),
		Answer: itembank.Answer{Number: round2(answer), Tolerance: 0.011},
		Explanation: fmt.Sprintf("Ошибки ŷ − y: [%s]. Сумма модулей %d, сумма квадратов %d, n = %d: MAE = %.2f, MSE = %.2f, RMSE = √MSE = %.2f. "+
			"Типичные ошибки — усреднять ошибки со знаком (они частично гасят друг друга), забывать извлечь корень для RMSE или делить на n − 1.",
			intsCSV(errs), absSum, sqSum, n, mae, mse, rmse),
	}, nil
}

// rocAucPairs — ROC-AUC как доля верно упорядоченных пар «положительный — отрицательный».
type rocAucPairs struct{}

func (rocAucPairs) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.metrics_validation.roc-auc-pairs.v1", Spec: itembank.SpecDataML, Topic: "metrics_validation",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.4, B: 0.9, C: 0}, TimeLimit: 150 * time.Second,
	}
}

func (f rocAucPairs) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A41)
	n := itembank.IntBetween(r, 5, 6)
	nPos := itembank.IntBetween(r, 2, 3)
	labels := make([]int, n)
	for i := 0; i < nPos; i++ {
		labels[i] = 1
	}
	labels = itembank.Shuffle(r, labels)
	grid := make([]int, 19)
	for i := range grid {
		grid[i] = (i + 1) * 5 // 0.05 … 0.95 в сотых
	}
	scores := itembank.PickN(r, grid, n)
	withTie := r.IntN(10) < 4
	if withTie {
		posIdx, negIdx := -1, -1
		for i, l := range labels {
			if l == 1 && posIdx < 0 {
				posIdx = i
			}
			if l == 0 && negIdx < 0 {
				negIdx = i
			}
		}
		scores[negIdx] = scores[posIdx]
	}
	concordant, ties, total := 0, 0, 0
	for i, li := range labels {
		if li != 1 {
			continue
		}
		for j, lj := range labels {
			if lj != 0 {
				continue
			}
			total++
			switch {
			case scores[i] > scores[j]:
				concordant++
			case scores[i] == scores[j]:
				ties++
			}
		}
	}
	auc := (float64(concordant) + 0.5*float64(ties)) / float64(total)
	var md strings.Builder
	md.WriteString("| объект | score | y |\n|---|---|---|\n")
	for i := range labels {
		fmt.Fprintf(&md, "| %d | %.2f | %d |\n", i+1, float64(scores[i])/100, labels[i])
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Бинарный классификатор выдал оценки принадлежности к классу 1 (score) для объектов тестовой выборки с истинными метками y:\n\n%s\n"+
			"Вычислите ROC-AUC. Округлите до сотых.", md.String()),
		Answer: itembank.Answer{Number: round2(auc), Tolerance: 0.011},
		Explanation: fmt.Sprintf("ROC-AUC равна вероятности того, что случайный положительный объект получит score выше случайного отрицательного. "+
			"Пар (положительный, отрицательный) всего %d·%d = %d; верно упорядочено %d, с равными score %d (каждая даёт 0.5). AUC = (%d + 0.5·%d) / %d = %.2f. "+
			"Типичная ошибка — считать accuracy при пороге 0.5 или делить на число всех пар объектов.",
			nPos, n-nPos, total, concordant, ties, concordant, ties, total, auc),
	}, nil
}

// cvScheme — выбор схемы кросс-валидации по структуре данных.
type cvScheme struct{}

func (cvScheme) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.metrics_validation.cv-scheme.v1", Spec: itembank.SpecDataML, Topic: "metrics_validation",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.0, C: 0.25}, TimeLimit: 90 * time.Second,
	}
}

var cvOptions = []string{
	"KFold с перемешиванием (shuffle=True)",
	"StratifiedKFold (сохраняет доли классов в каждом фолде)",
	"GroupKFold (все записи одной сущности попадают в один фолд)",
	"TimeSeriesSplit (обучающие фолды строго раньше валидационного)",
}

type groupEntity struct{ dat, records string }

var groupEntities = []groupEntity{
	{"пациентам", "визитов"}, {"клиентам", "сессий"}, {"устройствам", "измерений"}, {"водителям", "поездок"},
}

func (f cvScheme) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A42)
	kind := r.IntN(4)
	var scenario, explain string
	switch kind {
	case 0:
		scenario = fmt.Sprintf("Регрессия %s: %d независимых объектов, собранных одномоментно, без повторных измерений одного и того же объекта и без временной структуры.",
			itembank.Pick(r, []string{"стоимости квартир по характеристикам", "расхода топлива по параметрам автомобиля", "времени выполнения задачи по её описанию", "урожайности по составу почвы"}),
			itembank.IntBetween(r, 8, 60)*100)
		explain = "Объекты независимы, нет групп, классов и времени — достаточно обычного KFold с перемешиванием; стратификация и группы здесь не нужны, а TimeSeriesSplit лишь выбросит часть данных из обучения."
	case 1:
		scenario = fmt.Sprintf("Бинарная классификация %s: из %d независимых объектов положительный класс составляет %d %%; записи не связаны между собой и не упорядочены по времени.",
			itembank.Pick(r, []string{"мошеннических транзакций", "оттока клиентов", "дефектов на производстве", "отклика на рассылку"}),
			itembank.IntBetween(r, 10, 80)*100, itembank.Pick(r, []int{1, 2, 3, 5, 8}))
		explain = "При сильном дисбалансе классов обычный KFold может оставить фолд почти без положительных объектов, и метрики по фолдам будут скакать; StratifiedKFold сохраняет долю классов в каждом фолде."
	case 2:
		e := itembank.Pick(r, groupEntities)
		scenario = fmt.Sprintf("Датасет из %d записей по %d %s: на каждого приходится несколько %s; модель будет применяться к новым %s, которых нет в обучении.",
			itembank.IntBetween(r, 20, 90)*100, itembank.IntBetween(r, 2, 9)*100, e.dat, e.records, e.dat)
		explain = "Записи одной сущности похожи друг на друга: если они попадут и в train, и в validation, оценка будет завышена (утечка через группу). GroupKFold держит все записи сущности в одном фолде и имитирует применение к новым сущностям."
	default:
		scenario = fmt.Sprintf("Нужно прогнозировать %s на следующую неделю по данным за %d недель: каждая строка — один день, строки упорядочены по времени, признаки включают лаги целевой переменной.",
			itembank.Pick(r, []string{"спрос на товары", "нагрузку на сервис", "продажи магазина", "число обращений в поддержку"}), itembank.IntBetween(r, 40, 160))
		explain = "Во временных рядах случайное разбиение обучает модель на будущем и проверяет на прошлом — утечка через время и лаговые признаки. TimeSeriesSplit всегда валидирует на отрезке позже обучающего."
	}
	correct := cvOptions[kind]
	var distractors []string
	for i, o := range cvOptions {
		if i != kind {
			distractors = append(distractors, o)
		}
	}
	options, key := itembank.ShuffledOptions(r, correct, distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        scenario + " Какую схему кросс-валидации следует выбрать для честной оценки качества?",
		Options:     options,
		Answer:      itembank.Answer{Keys: []string{key}},
		Explanation: explain + " Типичная ошибка — применять одну схему «по умолчанию», не глядя на структуру данных.",
	}, nil
}
