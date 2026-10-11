package irt

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Candidate — семейство, доступное для выбора.
type Candidate struct {
	ID       string
	Topic    string
	Params   Params
	Exposure float64 // доля сессий за окно, где семейство показывалось (0..1)
}

// Config — параметры адаптивного теста.
type Config struct {
	MinItems      int     // минимум заданий до остановки по SE (12)
	MaxItems      int     // максимум заданий (20)
	TargetSE      float64 // остановка при SE ≤ (0.30)
	TopK          int     // randomesque: случайный выбор из K самых информативных (5)
	MaxExposure   float64 // семейства с экспозицией выше исключаются (0.20)
	MaxTopicShare float64 // доля заданий одной темы в сессии (0.30)
	BalanceUntil  int     // до этого номера задания предпочитаются новые темы (8)
	MinTopics     int     // сколько тем нужно покрыть в первых BalanceUntil (5)
	PriorSigma    float64 // σ априорного распределения (1.0)
}

// DefaultConfig — значения из docs/04-testing.md.
func DefaultConfig() Config {
	return Config{MinItems: 12, MaxItems: 20, TargetSE: 0.30, TopK: 5, MaxExposure: 0.20, MaxTopicShare: 0.30, BalanceUntil: 8, MinTopics: 5, PriorSigma: 1.0}
}

// State — состояние сессии (хранится приложением между запросами).
type State struct {
	PriorMu   float64
	Responses []Response
	AskedIDs  []string
	Topics    map[string]int
	Theta     float64
	SE        float64
}

// NewState создаёт состояние со стартовой оценкой из приора грейда.
func NewState(priorMu float64, priorSigma float64) State {
	return State{PriorMu: priorMu, Topics: map[string]int{}, Theta: priorMu, SE: priorSigma}
}

// Engine — адаптивный выбор заданий.
type Engine struct {
	Cfg  Config
	Grid Grid
}

// NewEngine создаёт движок с сеткой по умолчанию.
func NewEngine(cfg Config) *Engine {
	return &Engine{Cfg: cfg, Grid: DefaultGrid()}
}

// Update пересчитывает θ и SE после ответа.
func (e *Engine) Update(st *State, id, topic string, p Params, correct bool) {
	st.Responses = append(st.Responses, Response{Params: p, Correct: correct})
	st.AskedIDs = append(st.AskedIDs, id)
	if st.Topics == nil {
		st.Topics = map[string]int{}
	}
	st.Topics[topic]++
	_, st.Theta, st.SE = EAP(e.Grid, st.PriorMu, e.Cfg.PriorSigma, st.Responses)
}

// Posterior возвращает текущее апостериорное распределение.
func (e *Engine) Posterior(st *State) Posterior {
	post, _, _ := EAP(e.Grid, st.PriorMu, e.Cfg.PriorSigma, st.Responses)
	return post
}

// ShouldStop — правило остановки.
func (e *Engine) ShouldStop(st *State) bool {
	n := len(st.Responses)
	if n >= e.Cfg.MaxItems {
		return true
	}
	return n >= e.Cfg.MinItems && st.SE <= e.Cfg.TargetSE
}

// Next выбирает следующее семейство. rng — генератор сессии (детерминирован по seed сессии).
// Возвращает false, если подходящих семейств не осталось.
func (e *Engine) Next(st *State, pool []Candidate, rng *rand.Rand) (Candidate, bool) {
	asked := map[string]bool{}
	for _, id := range st.AskedIDs {
		asked[id] = true
	}
	n := len(st.Responses)
	topicCap := int(math.Ceil(e.Cfg.MaxTopicShare * float64(e.Cfg.MaxItems)))
	if topicCap < 1 {
		topicCap = 1
	}
	eligible := make([]Candidate, 0, len(pool))
	for _, c := range pool {
		if asked[c.ID] {
			continue
		}
		if e.Cfg.MaxExposure > 0 && c.Exposure > e.Cfg.MaxExposure {
			continue
		}
		if st.Topics[c.Topic] >= topicCap {
			continue
		}
		eligible = append(eligible, c)
	}
	if len(eligible) == 0 {
		// Экспозиция не должна блокировать тест: снимаем ограничение.
		for _, c := range pool {
			if !asked[c.ID] && st.Topics[c.Topic] < topicCap {
				eligible = append(eligible, c)
			}
		}
	}
	if len(eligible) == 0 {
		return Candidate{}, false
	}
	// Баланс тем: в начале теста предпочитаем ещё не встречавшиеся темы.
	if n < e.Cfg.BalanceUntil && len(st.Topics) < e.Cfg.MinTopics {
		fresh := make([]Candidate, 0, len(eligible))
		for _, c := range eligible {
			if st.Topics[c.Topic] == 0 {
				fresh = append(fresh, c)
			}
		}
		if len(fresh) > 0 {
			eligible = fresh
		}
	}
	type scored struct {
		c    Candidate
		info float64
	}
	sc := make([]scored, 0, len(eligible))
	for _, c := range eligible {
		sc = append(sc, scored{c, Info(st.Theta, c.Params)})
	}
	sort.Slice(sc, func(i, j int) bool {
		if sc[i].info != sc[j].info {
			return sc[i].info > sc[j].info
		}
		return sc[i].c.ID < sc[j].c.ID
	})
	k := e.Cfg.TopK
	if k <= 0 {
		k = 1
	}
	if k > len(sc) {
		k = len(sc)
	}
	return sc[rng.IntN(k)].c, true
}
