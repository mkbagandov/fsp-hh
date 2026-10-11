package irt

import "math"

// Observation — ответ кандидата с его оценкой способности (для калибровки).
type Observation struct {
	Theta   float64
	Correct bool
}

// CalibrateDifficulty обновляет сложность b байесовски: b ← (κ·b_prior + n·b_emp)/(κ+n),
// где b_emp решает уравнение mean P(θᵢ; a, b, c) = p̂ по наблюдениям (docs/04-testing.md, 7).
// При n < minObs или вырожденной доле верных ответов возвращает prior.B.
func CalibrateDifficulty(prior Params, obs []Observation, kappa float64, minObs int) float64 {
	n := len(obs)
	if n < minObs || n == 0 {
		return prior.B
	}
	correct := 0
	for _, o := range obs {
		if o.Correct {
			correct++
		}
	}
	pHat := float64(correct) / float64(n)
	if pHat <= prior.C+0.02 || pHat >= 0.98 {
		return prior.B
	}
	meanP := func(b float64) float64 {
		s := 0.0
		for _, o := range obs {
			s += Prob(o.Theta, Params{A: prior.A, B: b, C: prior.C})
		}
		return s / float64(n)
	}
	// mean P убывает по b: бисекция на [-4, 4].
	lo, hi := -4.0, 4.0
	for i := 0; i < 60; i++ {
		mid := (lo + hi) / 2
		if meanP(mid) > pHat {
			lo = mid
		} else {
			hi = mid
		}
	}
	bEmp := (lo + hi) / 2
	return (kappa*prior.B + float64(n)*bEmp) / (kappa + float64(n))
}

// PointBiserial — корреляция между правильностью ответа и θ (дискриминативность задания).
func PointBiserial(obs []Observation) float64 {
	n := float64(len(obs))
	if n < 2 {
		return 0
	}
	var sumT, sumT2 float64
	var n1 float64
	var sum1 float64
	for _, o := range obs {
		sumT += o.Theta
		sumT2 += o.Theta * o.Theta
		if o.Correct {
			n1++
			sum1 += o.Theta
		}
	}
	n0 := n - n1
	if n1 == 0 || n0 == 0 {
		return 0
	}
	mean := sumT / n
	sd := math.Sqrt(sumT2/n - mean*mean)
	if sd == 0 {
		return 0
	}
	m1 := sum1 / n1
	m0 := (sumT - sum1) / n0
	return (m1 - m0) / sd * math.Sqrt(n1*n0/(n*n))
}

// CalibrateDiscrimination корректирует a по точечно-бисериальной корреляции:
// a ← clamp(a_prior × r_pb / 0.3, 0.5, 2.0) при n ≥ minObs.
func CalibrateDiscrimination(prior Params, obs []Observation, minObs int) float64 {
	if len(obs) < minObs {
		return prior.A
	}
	r := PointBiserial(obs)
	if r <= 0 {
		return 0.5
	}
	a := prior.A * r / 0.3
	return math.Max(0.5, math.Min(2.0, a))
}
