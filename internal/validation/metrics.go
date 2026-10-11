// Package validation — воспроизводимая процедура валидации механик тестирования и подбора
// на синтетической популяции с известной «истиной» (docs/validation/REPORT.md).
package validation

import (
	"math"
	"sort"
)

// Bands — порядок грейдов для матриц и каппы.
var Bands = []string{"junior", "middle", "senior"}

func bandIndex(code string) int {
	for i, b := range Bands {
		if b == code {
			return i
		}
	}
	return -1
}

// Confusion — матрица ошибок 3×3: строки — истинный грейд, столбцы — предсказанный.
type Confusion [3][3]int

// Add учитывает пару (истина, предсказание).
func (c *Confusion) Add(trueBand, pred string) {
	i, j := bandIndex(trueBand), bandIndex(pred)
	if i >= 0 && j >= 0 {
		c[i][j]++
	}
}

// Accuracy — доля точных совпадений.
func (c Confusion) Accuracy() float64 {
	total, diag := 0, 0
	for i := range c {
		for j := range c[i] {
			total += c[i][j]
			if i == j {
				diag += c[i][j]
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(diag) / float64(total)
}

// Within1 — доля предсказаний не дальше одной ступени от истины.
func (c Confusion) Within1() float64 {
	total, ok := 0, 0
	for i := range c {
		for j := range c[i] {
			total += c[i][j]
			if i-j <= 1 && j-i <= 1 {
				ok += c[i][j]
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(ok) / float64(total)
}

// Kappa — каппа Коэна по матрице согласия (строки — первая оценка, столбцы — вторая).
func (c Confusion) Kappa() float64 {
	total := 0
	var row, col [3]int
	diag := 0
	for i := range c {
		for j := range c[i] {
			total += c[i][j]
			row[i] += c[i][j]
			col[j] += c[i][j]
			if i == j {
				diag += c[i][j]
			}
		}
	}
	if total == 0 {
		return 0
	}
	po := float64(diag) / float64(total)
	pe := 0.0
	for i := range row {
		pe += float64(row[i]) * float64(col[i]) / float64(total) / float64(total)
	}
	if pe >= 1 {
		return 1
	}
	return (po - pe) / (1 - pe)
}

// Pearson — коэффициент корреляции.
func Pearson(x, y []float64) float64 {
	n := float64(len(x))
	if n < 2 || len(x) != len(y) {
		return 0
	}
	var sx, sy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
	}
	mx, my := sx/n, sy/n
	var sxy, sxx, syy float64
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

// RMSE и MAE по парам (оценка, истина).
func RMSE(est, truth []float64) float64 {
	if len(est) == 0 {
		return 0
	}
	var s float64
	for i := range est {
		d := est[i] - truth[i]
		s += d * d
	}
	return math.Sqrt(s / float64(len(est)))
}

// MAE — средняя абсолютная ошибка.
func MAE(est, truth []float64) float64 {
	if len(est) == 0 {
		return 0
	}
	var s float64
	for i := range est {
		s += math.Abs(est[i] - truth[i])
	}
	return s / float64(len(est))
}

// Median — медиана (копирует срез).
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	return c[len(c)/2]
}

// Quantile — квантиль q ∈ [0,1] по отсортированной копии.
func Quantile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	i := int(math.Round(q * float64(len(c)-1)))
	return c[i]
}

// NDCG считает нормированный DCG@k для ранжирования rank (индексы кандидатов) по релевантности rel.
func NDCG(rank []int, rel []float64, k int) float64 {
	if k > len(rank) {
		k = len(rank)
	}
	dcg := 0.0
	for i := 0; i < k; i++ {
		dcg += rel[rank[i]] / math.Log2(float64(i)+2)
	}
	ideal := append([]float64(nil), rel...)
	sort.Sort(sort.Reverse(sort.Float64Slice(ideal)))
	idcg := 0.0
	for i := 0; i < k && i < len(ideal); i++ {
		idcg += ideal[i] / math.Log2(float64(i)+2)
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

// PrecisionAt — доля первых k позиций ранжирования, попавших в множество «хороших».
func PrecisionAt(rank []int, good map[int]bool, k int) float64 {
	if k > len(rank) {
		k = len(rank)
	}
	if k == 0 {
		return 0
	}
	hit := 0
	for i := 0; i < k; i++ {
		if good[rank[i]] {
			hit++
		}
	}
	return float64(hit) / float64(k)
}
