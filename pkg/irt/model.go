// Package irt — трёхпараметрическая логистическая модель теории ответов на задания,
// оценка способности (EAP/MLE), адаптивный выбор заданий (CAT), решение по грейду
// и калибровка параметров (docs/04-testing.md, разделы 4–7).
//
// Пакет не зависит от приложения: входы — параметры заданий и ответы, выходы — числа.
package irt

import "math"

// Params — параметры задания (семейства).
type Params struct {
	A float64 // дискриминативность
	B float64 // сложность
	C float64 // угадывание
}

// Response — ответ на задание с его параметрами.
type Response struct {
	Params  Params
	Correct bool
}

// Prob — вероятность верного ответа при способности theta (3PL).
func Prob(theta float64, p Params) float64 {
	return p.C + (1-p.C)/(1+math.Exp(-p.A*(theta-p.B)))
}

// Info — информация Фишера задания в точке theta.
func Info(theta float64, p Params) float64 {
	pr := Prob(theta, p)
	if pr <= 0 || pr >= 1 {
		return 0
	}
	q := 1 - pr
	d := (pr - p.C) / (1 - p.C)
	return p.A * p.A * d * d * q / pr
}

// Grid — сетка θ для численного интегрирования.
type Grid struct {
	Points []float64
}

// DefaultGrid — θ ∈ [−4, 4] с шагом 0.05.
func DefaultGrid() Grid {
	const lo, hi, step = -4.0, 4.0, 0.05
	n := int(math.Round((hi-lo)/step)) + 1
	pts := make([]float64, n)
	for i := range pts {
		pts[i] = lo + float64(i)*step
	}
	return Grid{Points: pts}
}

// Posterior — апостериорное распределение θ на сетке.
type Posterior struct {
	Grid    Grid
	Weights []float64 // нормированы, Σ = 1
}

// EAP считает апостериорное распределение при нормальном априоре N(mu, sigma²)
// и возвращает его вместе с точечной оценкой (среднее) и стандартной ошибкой.
func EAP(grid Grid, mu, sigma float64, responses []Response) (post Posterior, theta, se float64) {
	w := make([]float64, len(grid.Points))
	// Логарифмы для устойчивости при 20+ ответах.
	maxLog := math.Inf(-1)
	for i, th := range grid.Points {
		z := (th - mu) / sigma
		logw := -0.5 * z * z
		for _, r := range responses {
			p := Prob(th, r.Params)
			if r.Correct {
				logw += math.Log(p)
			} else {
				logw += math.Log(1 - p)
			}
		}
		w[i] = logw
		if logw > maxLog {
			maxLog = logw
		}
	}
	sum := 0.0
	for i := range w {
		w[i] = math.Exp(w[i] - maxLog)
		sum += w[i]
	}
	mean := 0.0
	for i, th := range grid.Points {
		w[i] /= sum
		mean += th * w[i]
	}
	variance := 0.0
	for i, th := range grid.Points {
		d := th - mean
		variance += d * d * w[i]
	}
	return Posterior{Grid: grid, Weights: w}, mean, math.Sqrt(variance)
}

// ProbAtLeast — апостериорная вероятность того, что θ ≥ x.
func (p Posterior) ProbAtLeast(x float64) float64 {
	s := 0.0
	for i, th := range p.Grid.Points {
		if th >= x {
			s += p.Weights[i]
		}
	}
	return s
}

// Mean — апостериорное среднее.
func (p Posterior) Mean() float64 {
	m := 0.0
	for i, th := range p.Grid.Points {
		m += th * p.Weights[i]
	}
	return m
}

// MLE — оценка максимального правдоподобия по сетке (для сравнения с EAP);
// при всех верных/неверных ответах уходит к краю сетки.
func MLE(grid Grid, responses []Response) float64 {
	best, bestLL := 0.0, math.Inf(-1)
	for _, th := range grid.Points {
		ll := 0.0
		for _, r := range responses {
			p := Prob(th, r.Params)
			if r.Correct {
				ll += math.Log(p)
			} else {
				ll += math.Log(1 - p)
			}
		}
		if ll > bestLL {
			best, bestLL = th, ll
		}
	}
	return best
}

// TestInfo — суммарная информация набора заданий в точке θ.
func TestInfo(theta float64, params []Params) float64 {
	s := 0.0
	for _, p := range params {
		s += Info(theta, p)
	}
	return s
}
