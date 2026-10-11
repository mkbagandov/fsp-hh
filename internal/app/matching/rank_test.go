package matching

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"fsp/internal/domain"
)

func baseNeed() Need {
	return Need{
		Specialization: "backend", GradeMin: "middle", GradeMax: "senior",
		MustSkills: []int32{1, 2, 3}, NiceSkills: []int32{4},
		Salary: domain.SalaryRange{From: 200000, To: 300000}, WorkFormats: []string{"remote", "hybrid"}, City: "Москва", RemoteOK: true,
	}
}

func candidate(name string, theta, pct, se float64, status string, skills map[int32]string) Candidate {
	return Candidate{
		UserID: uuid.New(), FirstName: name, LastName: "Тест", Grade: "middle", GradeStatus: status,
		Theta: theta, SE: se, Percentile: pct, HasTheta: true, CategoryUpdatedAt: time.Now().Add(-10 * 24 * time.Hour),
		SalaryExpected: 250000, WorkFormats: []string{"remote"}, Visibility: "open", City: "Москва",
		Skills: skills, SkillNames: map[int32]string{1: "Go", 2: "PostgreSQL", 3: "Kafka", 4: "Kubernetes"}, FSPLinked: false,
	}
}

func TestRankOrderAndExplanations(t *testing.T) {
	now := time.Now()
	need := baseNeed()
	strong := candidate("Сильный", 1.2, 0.9, 0.3, "confirmed", map[int32]string{1: "test_confirmed", 2: "test_confirmed", 3: "declared", 4: "declared"})
	weakSkills := candidate("Без навыков", 1.2, 0.9, 0.3, "confirmed", map[int32]string{})
	unconfirmed := candidate("Неподтверждённый", 1.5, 0.95, 0.3, "not_confirmed", map[int32]string{1: "declared", 2: "declared", 3: "declared"})
	expensive := candidate("Дорогой", 1.0, 0.85, 0.3, "confirmed", map[int32]string{1: "declared", 2: "declared", 3: "declared"})
	expensive.SalaryMin = 400000
	expensive.SalaryExpected = 450000
	withFSP := candidate("С ФСП", 0.8, 0.7, 0.3, "confirmed", map[int32]string{1: "declared", 2: "declared", 3: "declared"})
	withFSP.FSPLinked = true
	withFSP.Achievements = []Achievement{{Discipline: "algorithmic", Level: "national", ResultKind: "place", Place: 2, EventDate: now.AddDate(0, -6, 0), EventName: "Кубок"}}
	noFSPSame := candidate("Без ФСП", 0.8, 0.7, 0.3, "confirmed", map[int32]string{1: "declared", 2: "declared", 3: "declared"})

	ranked := Rank(need, []Candidate{unconfirmed, weakSkills, expensive, strong, noFSPSame, withFSP}, DefaultWeights(), now)
	if ranked[0].Candidate.FirstName != "Сильный" {
		t.Fatalf("strong candidate must be first, got %s", ranked[0].Candidate.FirstName)
	}
	if ranked[len(ranked)-1].Candidate.FirstName != "Неподтверждённый" {
		t.Fatalf("unconfirmed must be last, got %s", ranked[len(ranked)-1].Candidate.FirstName)
	}
	pos := map[string]int{}
	for i, r := range ranked {
		pos[r.Candidate.FirstName] = i
	}
	if pos["С ФСП"] > pos["Без ФСП"] {
		t.Fatal("FSP achievement must boost within equal profiles")
	}
	if pos["Дорогой"] < pos["Без ФСП"] {
		t.Fatal("candidate above the salary range must rank lower than a fitting one")
	}
	// Объяснения: факты по каждому компоненту и штраф за вилку ниже минимума.
	for _, r := range ranked {
		kinds := map[string]bool{}
		for _, f := range r.Facts {
			kinds[f.Kind] = true
		}
		for _, k := range []string{"test", "fsp", "fresh", "fit", "accept"} {
			if !kinds[k] {
				t.Fatalf("%s: missing fact %s", r.Candidate.FirstName, k)
			}
		}
	}
	exp := ranked[pos["Дорогой"]]
	if exp.PAccept != 0.05 {
		t.Fatalf("salary below candidate minimum must set p_accept to 0.05, got %v", exp.PAccept)
	}
	// Без ФСП — нейтральный факт, не штраф.
	noFSP := ranked[pos["Без ФСП"]]
	if noFSP.Components["s_fsp"] != 0 {
		t.Fatal("no fsp history must give zero boost, not penalty")
	}
	for _, f := range noFSP.Facts {
		if f.Kind == "fsp" && f.Polarity == "-" {
			t.Fatal("no fsp history must not be negative")
		}
	}
}

func TestCategoryMapAndFilters(t *testing.T) {
	now := time.Now()
	need := baseNeed()
	var cands []Candidate
	for i := 0; i < 20; i++ {
		c := candidate("К", 0.5, 0.5, 0.3, "confirmed", map[int32]string{1: "declared"})
		c.SalaryExpected = int64(200000 + i*10000)
		if i%4 == 0 {
			c.GradeStatus = "not_confirmed"
		}
		if i%5 == 0 {
			c.Achievements = []Achievement{{Discipline: "product", Level: "regional", ResultKind: "participation", EventDate: now}}
			c.FSPLinked = true
		}
		if i >= 15 {
			c.Grade = "senior"
		}
		cands = append(cands, c)
	}
	m := CategoryMap(need, cands)
	if len(m) != 3 {
		t.Fatalf("map must have 3 grades, got %d", len(m))
	}
	var middle CategoryStat
	for _, s := range m {
		if s.Grade == "middle" {
			middle = s
		}
	}
	if middle.Total != 15 || !middle.InRange || middle.Coverage <= 0 || middle.Coverage >= 1 || middle.SalaryP50 == 0 {
		t.Fatalf("middle stats: %+v", middle)
	}
	if len(middle.Hints) == 0 {
		t.Fatal("hints expected when median exceeds the range")
	}

	ranked := Rank(need, cands, DefaultWeights(), now)
	yes := true
	filtered := Apply(ranked, SelectionFilters{Grades: []string{"middle"}, GradeStatuses: []string{"confirmed"}, HasFSP: &yes, Limit: 10}, nil, nil)
	for _, s := range filtered {
		if s.Candidate.Grade != "middle" || s.Candidate.GradeStatus != "confirmed" || len(s.Candidate.Achievements) == 0 {
			t.Fatalf("filter violated: %+v", s.Candidate)
		}
	}
	if len(filtered) == 0 {
		t.Fatal("filtered list must not be empty")
	}
	bySkill := Apply(ranked, SelectionFilters{}, []int32{99}, nil)
	if len(bySkill) != 0 {
		t.Fatal("skill filter with unknown skill must exclude everyone")
	}
	invited := map[uuid.UUID]bool{cands[0].UserID: true}
	hidden := Apply(ranked, SelectionFilters{HideInvited: true, Limit: 100}, nil, invited)
	if len(hidden) != len(ranked)-1 {
		t.Fatalf("hide invited: %d vs %d", len(hidden), len(ranked))
	}

	prev := []uuid.UUID{cands[0].UserID, cands[1].UserID, cands[2].UserID}
	cur := []uuid.UUID{cands[1].UserID, cands[0].UserID, cands[3].UserID}
	d := DiffVersions(prev, cur)
	if len(d.Added) != 1 || len(d.Removed) != 1 || d.Moved != 2 {
		t.Fatalf("diff: %+v", d)
	}
}

func TestFreshnessAndGrades(t *testing.T) {
	if freshness(10) != 1 || freshness(400) != 0.3 || freshness(227.5) < 0.6 || freshness(227.5) > 0.7 {
		t.Fatal("freshness curve")
	}
	n := Need{GradeMin: "senior", GradeMax: "junior"}
	if g := n.Grades(); len(g) != 3 {
		t.Fatalf("swapped grades must still cover the range: %v", g)
	}
}
