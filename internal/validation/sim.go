package validation

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"

	"fsp/internal/domain/refdata"
	"fsp/pkg/irt"
	"fsp/pkg/itembank"
)

// GradeBands и Priors — те же, что в сервисе тестирования: полосы и порог подтверждения Junior
// берутся из справочника грейдов (docs/04-testing.md, раздел 6).
var GradeBands = func() []irt.Band {
	out := make([]irt.Band, 0, len(refdata.Grades))
	for _, g := range refdata.Grades {
		out = append(out, irt.Band{Code: g.Code, Lower: g.LowerTheta, MinConfirm: g.MinConfirmTheta})
	}
	return out
}()

// JuniorFloor — минимальный уровень для подтверждения Junior (из справочника).
var JuniorFloor = GradeBands[0].MinConfirm
var Priors = map[string]float64{"junior": -0.8, "middle": 0.1, "senior": 0.9}

const (
	confirmMinP = 0.70
	upgradeMinP = 0.80
)

// Pool — банк одной специализации (семейства специализации + общие), как его видит сервис тестирования.
type Pool struct {
	Spec    string
	Items   []irt.Candidate
	Formats map[string]itembank.Format
	TrueB   map[string]float64 // «истинная» сложность в эксперименте калибровки (по умолчанию = априорной)
}

// PoolsFromBank собирает пулы по специализациям с минимумом семейств.
func PoolsFromBank(minFamilies int) []Pool {
	common := itembank.BySpec(itembank.SpecCommon)
	var out []Pool
	// Специализации без собственного банка (меньше minFamilies семейств) в валидацию не попадают — как и в тест.
	for _, spec := range []itembank.Spec{itembank.SpecBackend, itembank.SpecFrontend, itembank.SpecDataML, itembank.SpecDevOps, itembank.SpecQA, itembank.SpecSecurity} {
		fams := itembank.BySpec(spec)
		if len(fams) < minFamilies {
			continue
		}
		p := Pool{Spec: string(spec), Formats: map[string]itembank.Format{}, TrueB: map[string]float64{}}
		seen := map[string]bool{}
		for _, f := range append(append([]itembank.Family{}, fams...), common...) {
			m := f.Meta()
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			p.Items = append(p.Items, irt.Candidate{ID: m.ID, Topic: m.Topic, Params: irt.Params{A: m.IRT.A, B: m.IRT.B, C: m.IRT.C}})
			p.Formats[m.ID] = m.Format
			p.TrueB[m.ID] = m.IRT.B
		}
		out = append(out, p)
	}
	return out
}

// Person — синтетический кандидат с известной способностью.
type Person struct {
	Theta    float64
	TrueBand string
	Claimed  string
	Inflated bool
}

// Population генерирует n кандидатов: θ ~ N(0, 1.1), доля overclaim завышает грейд на ступень.
func Population(rng *rand.Rand, n int, overclaim float64) []Person {
	out := make([]Person, 0, n)
	for i := 0; i < n; i++ {
		theta := rng.NormFloat64() * 1.1
		tb := irt.BandOf(theta, GradeBands)
		p := Person{Theta: theta, TrueBand: tb, Claimed: tb}
		if rng.Float64() < overclaim {
			if next := nextBand(tb); next != "" {
				p.Claimed, p.Inflated = next, true
			}
		}
		out = append(out, p)
	}
	return out
}

func nextBand(code string) string {
	i := bandIndex(code)
	if i >= 0 && i+1 < len(Bands) {
		return Bands[i+1]
	}
	return ""
}

// AnswerPolicy решает, верен ли ответ на семейство при данной θ.
type AnswerPolicy func(rng *rand.Rand, item irt.Candidate, theta float64) bool

// Honest — ответ по 3PL с истинной сложностью пула.
func (p Pool) Honest() AnswerPolicy {
	return func(rng *rand.Rand, item irt.Candidate, theta float64) bool {
		params := item.Params
		if b, ok := p.TrueB[item.ID]; ok {
			params.B = b
		}
		return rng.Float64() < irt.Prob(theta, params)
	}
}

// Guesser — случайные ответы без знаний: вероятность по формату задания.
func (p Pool) Guesser() AnswerPolicy {
	return func(rng *rand.Rand, item irt.Candidate, _ float64) bool {
		pr := 0.02
		switch p.Formats[item.ID] {
		case itembank.FormatSingleChoice:
			pr = 0.25
		case itembank.FormatMultiChoice:
			pr = 0.08
		case itembank.FormatOrdering:
			pr = 0.05
		}
		return rng.Float64() < pr
	}
}

// Leaker — знает верные ответы на утёкшие семейства, остальное — честно по 3PL.
func (p Pool) Leaker(known map[string]bool) AnswerPolicy {
	honest := p.Honest()
	return func(rng *rand.Rand, item irt.Candidate, theta float64) bool {
		if known[item.ID] {
			return true
		}
		return honest(rng, item, theta)
	}
}

// Observation — выданное задание в сессии (для статистики семейств и калибровки).
type Observation struct {
	Family      string
	Correct     bool
	ThetaBefore float64
}

// Session — результат одной смоделированной сессии.
type Session struct {
	Theta    float64
	SE       float64
	Items    int
	Decision irt.Decision
	Obs      []Observation
}

// RunCAT проходит адаптивный тест по движку приложения.
func (p Pool) RunCAT(rng *rand.Rand, person Person, cfg irt.Config, answer AnswerPolicy) Session {
	eng := irt.NewEngine(cfg)
	st := irt.NewState(Priors[person.Claimed], cfg.PriorSigma)
	var obs []Observation
	for !eng.ShouldStop(&st) {
		c, ok := eng.Next(&st, p.Items, rng)
		if !ok {
			break
		}
		before := st.Theta
		correct := answer(rng, c, person.Theta)
		eng.Update(&st, c.ID, c.Topic, c.Params, correct)
		obs = append(obs, Observation{Family: c.ID, Correct: correct, ThetaBefore: before})
	}
	return Session{Theta: st.Theta, SE: st.SE, Items: len(st.Responses), Decision: irt.Decide(eng.Posterior(&st), person.Claimed, GradeBands, confirmMinP, upgradeMinP), Obs: obs}
}

// RunFixed — линейный тест из заранее выбранных семейств (одинаковых для всех).
func (p Pool) RunFixed(rng *rand.Rand, person Person, items []irt.Candidate, answer AnswerPolicy) Session {
	grid := irt.DefaultGrid()
	var resp []irt.Response
	var obs []Observation
	for _, c := range items {
		correct := answer(rng, c, person.Theta)
		resp = append(resp, irt.Response{Params: c.Params, Correct: correct})
		obs = append(obs, Observation{Family: c.ID, Correct: correct})
	}
	post, theta, se := irt.EAP(grid, Priors[person.Claimed], 1, resp)
	return Session{Theta: theta, SE: se, Items: len(items), Decision: irt.Decide(post, person.Claimed, GradeBands, confirmMinP, upgradeMinP), Obs: obs}
}

// FixedForm — n семейств, равномерно покрывающих диапазон сложности (как «бумажный» тест).
func (p Pool) FixedForm(n int) []irt.Candidate {
	items := append([]irt.Candidate(nil), p.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].Params.B < items[j].Params.B })
	if n >= len(items) {
		return items
	}
	out := make([]irt.Candidate, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, items[i*(len(items)-1)/(n-1)])
	}
	return out
}

// RandomForm — n случайных семейств.
func (p Pool) RandomForm(rng *rand.Rand, n int) []irt.Candidate {
	perm := rng.Perm(len(p.Items))
	if n > len(perm) {
		n = len(perm)
	}
	out := make([]irt.Candidate, 0, n)
	for _, i := range perm[:n] {
		out = append(out, p.Items[i])
	}
	return out
}

// AccuracyResult — точность оценки и решения.
type AccuracyResult struct {
	Spec               string
	N                  int
	Families           int
	MAE, RMSE          float64
	MedianSE           float64
	MeanItems          float64
	Confusion          Confusion
	BandAccuracy       float64
	Within1            float64
	HonestConfirmed    float64 // доля честных заявок, подтверждённых (или повышенных)
	BelowFloor         int     // заявили Junior, но истинный уровень ниже минимума Junior
	BelowFloorRejected float64 // доля таких заявок, верно не подтверждённых
	InflatedRejected   float64 // доля завышенных заявок, отклонённых
	InflatedConfirmed  float64 // ложные подтверждения завышенных заявок
	UpgradeSuggested   float64 // доля честных, кому предложили повышение
	ItemsHist          map[int]int
	FamilyStats        map[string]*FamilyStat
}

// FamilyStat — статистика семейства по симуляции.
type FamilyStat struct {
	ID       string
	Topic    string
	A, B, C  float64
	Served   int
	Correct  int
	Biserial float64
	obs      []irt.Observation
}

// Accuracy запускает основной эксперимент: точность θ̂, матрица грейдов, решения.
func Accuracy(p Pool, people []Person, seed uint64) AccuracyResult {
	rng := rand.New(rand.NewPCG(seed, 11))
	res := AccuracyResult{Spec: p.Spec, N: len(people), Families: len(p.Items), ItemsHist: map[int]int{}, FamilyStats: map[string]*FamilyStat{}}
	for _, it := range p.Items {
		res.FamilyStats[it.ID] = &FamilyStat{ID: it.ID, Topic: it.Topic, A: it.Params.A, B: it.Params.B, C: it.Params.C}
	}
	var est, truth, ses []float64
	honest, inflated, hc, up, ir, ic, below, br := 0, 0, 0, 0, 0, 0, 0, 0
	var items float64
	answer := p.Honest()
	for _, person := range people {
		s := p.RunCAT(rng, person, irt.DefaultConfig(), answer)
		est = append(est, s.Theta)
		truth = append(truth, person.Theta)
		ses = append(ses, s.SE)
		items += float64(s.Items)
		res.ItemsHist[s.Items]++
		res.Confusion.Add(person.TrueBand, irt.BandOf(s.Theta, GradeBands))
		for _, o := range s.Obs {
			fs := res.FamilyStats[o.Family]
			fs.Served++
			if o.Correct {
				fs.Correct++
			}
			fs.obs = append(fs.obs, irt.Observation{Theta: s.Theta, Correct: o.Correct})
		}
		if !person.Inflated && person.Claimed == "junior" && person.Theta < JuniorFloor {
			// Ниже минимального уровня Junior верное решение — «не подтверждён».
			below++
			if s.Decision.Status == "not_confirmed" {
				br++
			}
			continue
		}
		if person.Inflated {
			inflated++
			switch s.Decision.Status {
			case "not_confirmed":
				ir++
			default:
				ic++
			}
		} else {
			honest++
			switch s.Decision.Status {
			case "confirmed":
				hc++
			case "upgrade_suggested":
				hc++
				up++
			}
		}
	}
	res.MAE, res.RMSE, res.MedianSE = MAE(est, truth), RMSE(est, truth), Median(ses)
	res.MeanItems = items / float64(len(people))
	res.BandAccuracy, res.Within1 = res.Confusion.Accuracy(), res.Confusion.Within1()
	if honest > 0 {
		res.HonestConfirmed = float64(hc) / float64(honest)
		res.UpgradeSuggested = float64(up) / float64(honest)
	}
	res.BelowFloor = below
	if below > 0 {
		res.BelowFloorRejected = float64(br) / float64(below)
	}
	if inflated > 0 {
		res.InflatedRejected = float64(ir) / float64(inflated)
		res.InflatedConfirmed = float64(ic) / float64(inflated)
	}
	for _, fs := range res.FamilyStats {
		fs.Biserial = irt.PointBiserial(fs.obs)
	}
	return res
}

// RetestResult — согласованность повторного прохождения.
type RetestResult struct {
	Spec         string
	N            int
	Kappa        float64
	PearsonTheta float64
	MeanAbsDiff  float64
	SameDecision float64
	SameBand     float64
	Agreement    Confusion
}

// Retest проходит тест дважды независимыми сессиями одной и той же θ.
func Retest(p Pool, people []Person, seed uint64) RetestResult {
	r1 := rand.New(rand.NewPCG(seed, 21))
	r2 := rand.New(rand.NewPCG(seed, 22))
	res := RetestResult{Spec: p.Spec, N: len(people)}
	var t1, t2 []float64
	same, sameBand := 0, 0
	answer := p.Honest()
	for _, person := range people {
		a := p.RunCAT(r1, person, irt.DefaultConfig(), answer)
		b := p.RunCAT(r2, person, irt.DefaultConfig(), answer)
		t1 = append(t1, a.Theta)
		t2 = append(t2, b.Theta)
		ba, bb := irt.BandOf(a.Theta, GradeBands), irt.BandOf(b.Theta, GradeBands)
		res.Agreement.Add(ba, bb)
		if ba == bb {
			sameBand++
		}
		if a.Decision.Status == b.Decision.Status {
			same++
		}
		res.MeanAbsDiff += math.Abs(a.Theta - b.Theta)
	}
	res.Kappa = res.Agreement.Kappa()
	res.PearsonTheta = Pearson(t1, t2)
	res.MeanAbsDiff /= float64(len(people))
	res.SameDecision = float64(same) / float64(len(people))
	res.SameBand = float64(sameBand) / float64(len(people))
	return res
}

// BaselineResult — сравнение CAT с линейными формами той же длины.
type BaselineResult struct {
	Spec  string
	Rows  []BaselineRow
	Items int
}

// BaselineRow — метрики одного метода.
type BaselineRow struct {
	Method       string
	RMSE         float64
	BandAccuracy float64
	Within1      float64
	MeanItems    float64
}

// Baselines сравнивает CAT (штатная остановка), CAT фиксированной длины, равномерную и случайную формы.
func Baselines(p Pool, people []Person, seed uint64, length int) BaselineResult {
	res := BaselineResult{Spec: p.Spec, Items: length}
	run := func(name string, f func(rng *rand.Rand, person Person) Session) {
		rng := rand.New(rand.NewPCG(seed, 31))
		var est, truth []float64
		var c Confusion
		var items float64
		for _, person := range people {
			s := f(rng, person)
			est = append(est, s.Theta)
			truth = append(truth, person.Theta)
			items += float64(s.Items)
			c.Add(person.TrueBand, irt.BandOf(s.Theta, GradeBands))
		}
		res.Rows = append(res.Rows, BaselineRow{Method: name, RMSE: RMSE(est, truth), BandAccuracy: c.Accuracy(), Within1: c.Within1(), MeanItems: items / float64(len(people))})
	}
	honest := p.Honest()
	run("CAT (остановка по SE ≤ 0,30)", func(rng *rand.Rand, person Person) Session { return p.RunCAT(rng, person, irt.DefaultConfig(), honest) })
	fixedCfg := irt.DefaultConfig()
	fixedCfg.MinItems, fixedCfg.MaxItems, fixedCfg.TargetSE = length, length, 0
	run("CAT фиксированной длины", func(rng *rand.Rand, person Person) Session { return p.RunCAT(rng, person, fixedCfg, honest) })
	form := p.FixedForm(length)
	run("Линейный тест (равномерно по сложности)", func(rng *rand.Rand, person Person) Session { return p.RunFixed(rng, person, form, honest) })
	run("Случайные задания", func(rng *rand.Rand, person Person) Session {
		return p.RunFixed(rng, person, p.RandomForm(rng, length), honest)
	})
	return res
}

// RobustnessResult — угадывание и утечка банка.
type RobustnessResult struct {
	Spec             string
	GuessConfirmed   map[string]float64 // claimed → доля подтверждений угадывающего
	GuessThetaMean   float64
	LeakRows         []LeakRow
	HonestMiddleRate float64 // для сравнения: честный middle подтверждается
}

// LeakRow — результат при K утёкших семействах.
type LeakRow struct {
	Known         int
	ShareOfBank   float64
	ConfirmedRate float64
	MeanTheta     float64
	GainTheta     float64
}

// Robustness: угадывающий с нулевыми знаниями и атакующий с утечкой K семейств (θ = −1,0, заявка middle).
func Robustness(p Pool, seed uint64, n int) RobustnessResult {
	res := RobustnessResult{Spec: p.Spec, GuessConfirmed: map[string]float64{}}
	for _, claimed := range Bands {
		rng := rand.New(rand.NewPCG(seed, 41))
		conf := 0
		var sum float64
		for i := 0; i < n; i++ {
			s := p.RunCAT(rng, Person{Theta: -3, TrueBand: "junior", Claimed: claimed}, irt.DefaultConfig(), p.Guesser())
			if s.Decision.Status != "not_confirmed" {
				conf++
			}
			sum += s.Theta
		}
		res.GuessConfirmed[claimed] = float64(conf) / float64(n)
		if claimed == "junior" {
			res.GuessThetaMean = sum / float64(n)
		}
	}
	// Честный middle (θ = 0,1) — ориентир.
	{
		rng := rand.New(rand.NewPCG(seed, 42))
		conf := 0
		for i := 0; i < n; i++ {
			if s := p.RunCAT(rng, Person{Theta: 0.1, TrueBand: "middle", Claimed: "middle"}, irt.DefaultConfig(), p.Honest()); s.Decision.Status != "not_confirmed" {
				conf++
			}
		}
		res.HonestMiddleRate = float64(conf) / float64(n)
	}
	var base float64
	for _, k := range []int{0, 5, 10, 20, 40} {
		rng := rand.New(rand.NewPCG(seed, 43+uint64(k))) //nolint:gosec // k — малое неотрицательное
		conf := 0
		var sum float64
		for i := 0; i < n; i++ {
			known := map[string]bool{}
			for _, idx := range rng.Perm(len(p.Items))[:min(k, len(p.Items))] {
				known[p.Items[idx].ID] = true
			}
			s := p.RunCAT(rng, Person{Theta: -1.0, TrueBand: "junior", Claimed: "middle"}, irt.DefaultConfig(), p.Leaker(known))
			if s.Decision.Status != "not_confirmed" {
				conf++
			}
			sum += s.Theta
		}
		row := LeakRow{Known: min(k, len(p.Items)), ShareOfBank: float64(min(k, len(p.Items))) / float64(len(p.Items)), ConfirmedRate: float64(conf) / float64(n), MeanTheta: sum / float64(n)}
		if k == 0 {
			base = row.MeanTheta
		}
		row.GainTheta = row.MeanTheta - base
		res.LeakRows = append(res.LeakRows, row)
	}
	return res
}

// CalibrationResult — восстановление сложности по накопленным ответам.
type CalibrationResult struct {
	Spec            string
	Sessions        int
	FamiliesUsed    int
	MAEPrior        float64 // |b_prior − b_true| до калибровки
	MAECalibrated   float64 // |b_calibrated − b_true| после
	CorrShift       float64 // корреляция оценённого сдвига с истинным
	MedianObsPerFam float64
}

// Calibration возмущает истинную сложность (b_true = b_prior + N(0, 0,4)), копит ответы и калибрует.
func Calibration(p Pool, seed uint64, sessions int) CalibrationResult {
	rng := rand.New(rand.NewPCG(seed, 51))
	perturbed := Pool{Spec: p.Spec, Items: p.Items, Formats: p.Formats, TrueB: map[string]float64{}}
	for _, it := range p.Items {
		perturbed.TrueB[it.ID] = it.Params.B + rng.NormFloat64()*0.4
	}
	obs := map[string][]irt.Observation{}
	people := Population(rng, sessions, 0.2)
	answer := perturbed.Honest()
	for _, person := range people {
		s := perturbed.RunCAT(rng, person, irt.DefaultConfig(), answer)
		for _, o := range s.Obs {
			obs[o.Family] = append(obs[o.Family], irt.Observation{Theta: s.Theta, Correct: o.Correct})
		}
	}
	res := CalibrationResult{Spec: p.Spec, Sessions: sessions}
	var priorErr, calErr, shiftTrue, shiftEst []float64
	var counts []float64
	for _, it := range p.Items {
		o := obs[it.ID]
		counts = append(counts, float64(len(o)))
		if len(o) < 30 {
			continue
		}
		bTrue := perturbed.TrueB[it.ID]
		bCal := irt.CalibrateDifficulty(it.Params, o, 20, 30)
		priorErr = append(priorErr, math.Abs(it.Params.B-bTrue))
		calErr = append(calErr, math.Abs(bCal-bTrue))
		shiftTrue = append(shiftTrue, bTrue-it.Params.B)
		shiftEst = append(shiftEst, bCal-it.Params.B)
	}
	res.FamiliesUsed = len(priorErr)
	if len(priorErr) > 0 {
		var a, b float64
		for i := range priorErr {
			a += priorErr[i]
			b += calErr[i]
		}
		res.MAEPrior, res.MAECalibrated = a/float64(len(priorErr)), b/float64(len(priorErr))
		res.CorrShift = Pearson(shiftTrue, shiftEst)
	}
	res.MedianObsPerFam = Median(counts)
	return res
}

// InfoCurve — тестовая информационная функция по сетке θ и слабые семейства.
type InfoCurve struct {
	Spec   string
	Thetas []float64
	TIF    []float64
	SEMin  []float64 // 1/√TIF
	Weak   []WeakFamily
}

// WeakFamily — семейство с низкой информативностью.
type WeakFamily struct {
	ID      string
	A, B, C float64
	MaxInfo float64
	Reason  string
}

// Information считает TIF и выделяет семейства с max info < 0,25 или a < 0,8.
func Information(p Pool) InfoCurve {
	ic := InfoCurve{Spec: p.Spec}
	for t := -3.0; t <= 3.0001; t += 0.25 {
		ic.Thetas = append(ic.Thetas, t)
		var tif float64
		for _, it := range p.Items {
			tif += irt.Info(t, it.Params)
		}
		ic.TIF = append(ic.TIF, tif)
		if tif > 0 {
			ic.SEMin = append(ic.SEMin, 1/math.Sqrt(tif))
		} else {
			ic.SEMin = append(ic.SEMin, 0)
		}
	}
	for _, it := range p.Items {
		maxInfo := 0.0
		for t := -4.0; t <= 4.0; t += 0.1 {
			if v := irt.Info(t, it.Params); v > maxInfo {
				maxInfo = v
			}
		}
		switch {
		case it.Params.A < 0.8:
			ic.Weak = append(ic.Weak, WeakFamily{ID: it.ID, A: it.Params.A, B: it.Params.B, C: it.Params.C, MaxInfo: maxInfo, Reason: "низкая дискриминативность a < 0,8"})
		case maxInfo < 0.25:
			ic.Weak = append(ic.Weak, WeakFamily{ID: it.ID, A: it.Params.A, B: it.Params.B, C: it.Params.C, MaxInfo: maxInfo, Reason: fmt.Sprintf("максимум информации %.2f < 0,25 (угадывание c = %.2f съедает информативность)", maxInfo, it.Params.C)})
		}
	}
	return ic
}
