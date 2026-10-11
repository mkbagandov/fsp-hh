package validation

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/google/uuid"

	"fsp/internal/app/matching"
	"fsp/internal/domain"
)

// RankResult — качество подбора против скрытой «истинной» полезности.
type RankResult struct {
	Candidates int
	Needs      int
	Rows       []RankRow
}

// RankRow — метрики одного метода ранжирования.
type RankRow struct {
	Method      string
	NDCG10      float64
	Precision10 float64
	Accepted10  float64 // ожидаемое число принятых приглашений среди top-10
}

type hidden struct {
	theta     float64
	skills    map[int32]bool
	fsp       bool
	accept    float64 // истинная вероятность отклика
	utility   float64
	candidate matching.Candidate
}

// Ranking: синтетический пул, где известны истинная способность, истинные навыки и склонность к отклику;
// система видит оценку θ̂ с шумом, заявленные навыки с пропусками и ошибками. Сравниваем с базовыми методами.
func Ranking(seed uint64, candidates, needs int) RankResult {
	rng := rand.New(rand.NewPCG(seed, 61))
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC) // фиксированное «сейчас»: отчёт одинаков в любой день
	res := RankResult{Candidates: candidates, Needs: needs}
	methods := []string{"ФСП: relevance × (0,6 + 0,4·p_accept)", "Только релевантность (без p_accept)", "Только тест (процентиль)", "Только навыки", "Случайно"}
	sums := make([]RankRow, len(methods))
	for i := range sums {
		sums[i].Method = methods[i]
	}
	for n := 0; n < needs; n++ {
		need := matching.Need{Specialization: "backend", GradeMin: "middle", GradeMax: "senior", Salary: domain.SalaryRange{From: 200000, To: 300000}, WorkFormats: []string{"remote"}, RemoteOK: true, City: "Москва"}
		skillIDs := []int32{}
		for s := int32(1); s <= 12; s++ {
			skillIDs = append(skillIDs, s)
		}
		need.MustSkills = skillIDs[:4]
		need.NiceSkills = skillIDs[4:6]
		pool := make([]hidden, 0, candidates)
		var thetas []float64
		for i := 0; i < candidates; i++ {
			h := hidden{theta: rng.NormFloat64()*0.8 + 0.3, skills: map[int32]bool{}, fsp: rng.Float64() < 0.25}
			for _, s := range skillIDs {
				if rng.Float64() < 0.45 {
					h.skills[s] = true
				}
			}
			thetas = append(thetas, h.theta)
			pool = append(pool, h)
		}
		sort.Float64s(thetas)
		for i := range pool {
			h := &pool[i]
			// Наблюдаемое: θ̂ с шумом SE≈0,3, процентиль по пулу; заявленные навыки с пропусками (20 %) и ложными (10 %).
			est := h.theta + rng.NormFloat64()*0.3
			pct := float64(sort.SearchFloat64s(thetas, est)) / float64(len(thetas))
			declared := map[int32]string{}
			for _, s := range skillIDs { // по списку, а не по map: розыгрыш не зависит от порядка обхода
				if h.skills[s] && rng.Float64() >= 0.2 {
					declared[s] = "declared"
				}
			}
			for _, s := range skillIDs {
				if !h.skills[s] && rng.Float64() < 0.1 {
					declared[s] = "declared"
				}
			}
			grade := "middle"
			if est >= 0.7 {
				grade = "senior"
			}
			salary := int64(180000 + rng.IntN(180000))
			formats := []string{"remote"}
			if rng.Float64() < 0.3 {
				formats = []string{"office"}
			}
			vis := "open"
			if rng.Float64() < 0.3 {
				vis = "passive"
			}
			var achs []matching.Achievement
			if h.fsp {
				achs = []matching.Achievement{{Discipline: "algorithmic", Level: "national", ResultKind: "place", Place: 5, EventDate: now.AddDate(0, -6, 0), EventName: "Чемпионат"}}
			}
			h.candidate = matching.Candidate{
				UserID: uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "rank-%d-%d", n, i)), FirstName: "К", Grade: grade, GradeStatus: "confirmed", Theta: est, SE: 0.3, Percentile: pct, HasTheta: true,
				CategoryUpdatedAt: now.AddDate(0, 0, -rng.IntN(120)), SalaryExpected: salary, SalaryMin: salary * 85 / 100, WorkFormats: formats, Visibility: vis,
				Skills: declared, SkillNames: map[int32]string{}, Achievements: achs, FSPLinked: h.fsp,
			}
			// Истинная полезность: способность, реальное покрытие must-навыков, ФСП, условия.
			cov := 0.0
			for _, s := range need.MustSkills {
				if h.skills[s] {
					cov++
				}
			}
			cov /= float64(len(need.MustSkills))
			fit := need.Salary.FitScore(salary)
			if formats[0] != "remote" {
				fit *= 0.6
			}
			h.utility = 0.45*normCDF(h.theta) + 0.3*cov + 0.1*boolF(h.fsp) + 0.15*fit
			h.accept = 0.35 + 0.35*boolF(vis == "open") + 0.2*fit - 0.3*boolF(salary > need.Salary.To)
			h.accept = math.Max(0.05, math.Min(0.95, h.accept))
		}
		rel := make([]float64, len(pool))
		for i, h := range pool {
			rel[i] = h.utility
		}
		good := topShare(rel, 0.2)
		cands := make([]matching.Candidate, len(pool))
		index := map[uuid.UUID]int{}
		for i, h := range pool {
			cands[i] = h.candidate
			index[h.candidate.UserID] = i
		}
		ranked := matching.Rank(need, cands, matching.DefaultWeights(), now)
		ours := make([]int, 0, len(ranked))
		relOnly := make([]int, 0, len(ranked))
		for _, s := range ranked {
			ours = append(ours, index[s.Candidate.UserID])
		}
		byRel := append([]matching.Scored(nil), ranked...)
		sort.SliceStable(byRel, func(i, j int) bool { return byRel[i].Relevance > byRel[j].Relevance })
		for _, s := range byRel {
			relOnly = append(relOnly, index[s.Candidate.UserID])
		}
		byTest := orderBy(pool, func(h hidden) float64 { return h.candidate.Percentile })
		bySkills := orderBy(pool, func(h hidden) float64 {
			c := 0.0
			for _, s := range need.MustSkills {
				if _, ok := h.candidate.Skills[s]; ok {
					c++
				}
			}
			return c + 0.001*h.candidate.Percentile
		})
		random := rng.Perm(len(pool))
		for mi, order := range [][]int{ours, relOnly, byTest, bySkills, random} {
			sums[mi].NDCG10 += NDCG(order, rel, 10)
			sums[mi].Precision10 += PrecisionAt(order, good, 10)
			for _, idx := range order[:min(10, len(order))] {
				sums[mi].Accepted10 += pool[idx].accept
			}
		}
	}
	for i := range sums {
		sums[i].NDCG10 /= float64(needs)
		sums[i].Precision10 /= float64(needs)
		sums[i].Accepted10 /= float64(needs)
	}
	res.Rows = sums
	return res
}

func orderBy(pool []hidden, key func(hidden) float64) []int {
	idx := make([]int, len(pool))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return key(pool[idx[a]]) > key(pool[idx[b]]) })
	return idx
}

func topShare(rel []float64, share float64) map[int]bool {
	idx := make([]int, len(rel))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return rel[idx[a]] > rel[idx[b]] })
	k := int(math.Ceil(share * float64(len(rel))))
	good := map[int]bool{}
	for _, i := range idx[:k] {
		good[i] = true
	}
	return good
}

func normCDF(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

func boolF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
