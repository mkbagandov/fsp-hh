// Package dataml — семейства заданий для Data Science / ML: статистика, метрики,
// валидация, признаки, модели, дизайн экспериментов.
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
	itembank.Register(meanMedian{})
	itembank.Register(precisionRecall{})
	itembank.Register(dataLeakage{})
}

// meanMedian — описательные статистики маленькой выборки.
type meanMedian struct{}

func (meanMedian) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.statistics.mean-median.v1", Spec: itembank.SpecDataML, Topic: "statistics",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -1.0, C: 0}, TimeLimit: 90 * time.Second,
	}
}

func (f meanMedian) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A01)
	n := itembank.IntBetween(r, 7, 9)
	xs := make([]int, n)
	for i := range xs {
		xs[i] = itembank.IntBetween(r, 1, 30)
	}
	sorted := append([]int{}, xs...)
	sort.Ints(sorted)
	strs := make([]string, n)
	for i, v := range xs {
		strs[i] = strconv.Itoa(v)
	}
	sum := 0
	for _, v := range xs {
		sum += v
	}
	kind := r.IntN(3)
	var answer float64
	var what, explain string
	switch kind {
	case 0:
		answer = math.Round(float64(sum)/float64(n)*100) / 100
		what = "среднее арифметическое (округлите до сотых)"
		explain = fmt.Sprintf("Сумма %d делится на число элементов %d: %.2f.", sum, n, float64(sum)/float64(n))
	case 1:
		answer = float64(sorted[n/2])
		what = "медиану"
		explain = fmt.Sprintf("После сортировки %v средний (%d-й) элемент — %d. Медиана устойчива к выбросам, в отличие от среднего.", sorted, n/2+1, sorted[n/2])
	default:
		answer = float64(sorted[n-1] - sorted[0])
		what = "размах (разность максимума и минимума)"
		explain = fmt.Sprintf("Максимум %d, минимум %d, размах %d.", sorted[n-1], sorted[0], sorted[n-1]-sorted[0])
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        fmt.Sprintf("Дана выборка: %s. Найдите %s.", strings.Join(strs, ", "), what),
		Answer:      itembank.Answer{Number: answer, Tolerance: 0.011},
		Explanation: explain + " Типичная ошибка — путать медиану со средним или брать средний элемент без сортировки.",
	}, nil
}

// precisionRecall — метрики по матрице ошибок.
type precisionRecall struct{}

func (precisionRecall) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.metrics_validation.precision-recall.v1", Spec: itembank.SpecDataML, Topic: "metrics_validation",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: -0.1, C: 0}, TimeLimit: 120 * time.Second,
	}
}

func (f precisionRecall) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A02)
	tp := itembank.IntBetween(r, 20, 90)
	fp := itembank.IntBetween(r, 5, 60)
	fn := itembank.IntBetween(r, 5, 60)
	tn := itembank.IntBetween(r, 50, 300)
	precision := float64(tp) / float64(tp+fp)
	recall := float64(tp) / float64(tp+fn)
	f1 := 2 * precision * recall / (precision + recall)
	accuracy := float64(tp+tn) / float64(tp+fp+fn+tn)
	var answer float64
	var what, explain string
	switch r.IntN(4) {
	case 0:
		answer, what = precision, "precision"
		explain = fmt.Sprintf("Precision = TP / (TP + FP) = %d / %d = %.3f.", tp, tp+fp, precision)
	case 1:
		answer, what = recall, "recall"
		explain = fmt.Sprintf("Recall = TP / (TP + FN) = %d / %d = %.3f.", tp, tp+fn, recall)
	case 2:
		answer, what = f1, "F1"
		explain = fmt.Sprintf("Precision = %.3f, recall = %.3f, F1 = 2PR/(P+R) = %.3f.", precision, recall, f1)
	default:
		answer, what = accuracy, "accuracy"
		explain = fmt.Sprintf("Accuracy = (TP + TN) / всего = %d / %d = %.3f.", tp+tn, tp+fp+fn+tn, accuracy)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Бинарный классификатор на тестовой выборке дал TP = %d, FP = %d, FN = %d, TN = %d. Вычислите %s (округлите до сотых).",
			tp, fp, fn, tn, what),
		Answer:      itembank.Answer{Number: math.Round(answer*100) / 100, Tolerance: 0.011},
		Explanation: explain + " Типичная ошибка — делить TP на общее число объектов или путать FP и FN в знаменателе.",
	}, nil
}

// dataLeakage — какие действия приводят к утечке данных.
type dataLeakage struct{}

func (dataLeakage) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.metrics_validation.data-leakage.v1", Spec: itembank.SpecDataML, Topic: "metrics_validation",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.4, C: 0}, TimeLimit: 120 * time.Second,
	}
}

var leakActions = []string{
	"нормализовать признаки по среднему и дисперсии всего датасета до разбиения на train/test",
	"подбирать гиперпараметры по качеству на тестовой выборке",
	"заполнять пропуски медианой, посчитанной по всему датасету, до разбиения",
	"строить целевое кодирование категорий (target encoding) на всех данных, включая тест",
	"отбирать признаки по корреляции с целевой переменной на всём датасете",
	"использовать признак, который становится известен только после наступления целевого события",
}

var safeActions = []string{
	"нормализовать признаки по статистикам, посчитанным только на train",
	"подбирать гиперпараметры кросс-валидацией внутри train",
	"делать стратифицированное разбиение по классам",
	"держать отложенную тестовую выборку и смотреть на неё один раз в конце",
	"выполнять отбор признаков внутри каждого фолда кросс-валидации",
	"разбивать временные ряды по времени, а не случайно",
}

func (f dataLeakage) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A03)
	nLeak := itembank.IntBetween(r, 2, 3)
	leaks := itembank.PickN(r, leakActions, nLeak)
	safes := itembank.PickN(r, safeActions, 5-nLeak)
	options, keys := itembank.ShuffledMulti(r, leaks, safes...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    "Какие из перечисленных действий при подготовке данных приводят к утечке (data leakage) и завышают оценку модели? Выберите все подходящие.",
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: "Утечка возникает, когда информация из тестовых объектов или из будущего попадает в обучение: статистики по всему датасету, " +
			"подбор по тесту, кодирование и отбор признаков с участием теста. Безопасные практики ограничивают любые вычисления только train или " +
			"внутренними фолдами.",
	}, nil
}
