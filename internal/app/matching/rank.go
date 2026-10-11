// Package matching — подбор кандидатов под потребность работодателя: отбор по категориям,
// объяснимое ранжирование (релевантность × вероятность отклика), карта категорий
// и версии подборок (docs/03-matching.md). Чистые функции этого файла не зависят от БД.
package matching

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
)

// Weights — веса компонентов релевантности (сумма 1).
type Weights struct {
	Test   float64 `json:"test"`
	Skills float64 `json:"skills"`
	FSP    float64 `json:"fsp"`
	Fresh  float64 `json:"fresh"`
	Fit    float64 `json:"fit"`
}

// DefaultWeights — значения из docs/03-matching.md, раздел 4.2.
func DefaultWeights() Weights {
	return Weights{Test: 0.35, Skills: 0.25, FSP: 0.15, Fresh: 0.10, Fit: 0.15}
}

// Need — потребность в терминах подбора.
type Need struct {
	CompanyID      uuid.UUID
	Specialization string
	GradeMin       string
	GradeMax       string
	MustSkills     []int32
	NiceSkills     []int32
	Salary         domain.SalaryRange
	WorkFormats    []string
	City           string
	RemoteOK       bool
}

// Grades возвращает коды грейдов диапазона потребности.
func (n Need) Grades() []string {
	lo, hi := refdata.GradeOrder(n.GradeMin), refdata.GradeOrder(n.GradeMax)
	if lo > hi {
		lo, hi = hi, lo
	}
	var out []string
	for _, g := range refdata.Grades {
		if g.Order >= lo && g.Order <= hi {
			out = append(out, g.Code)
		}
	}
	return out
}

// Achievement — достижение ФСП для буста.
type Achievement struct {
	Discipline string
	Level      string
	ResultKind string
	Place      int
	EventDate  time.Time
	EventName  string
}

// Candidate — кандидат с данными, нужными для ранжирования (загружает сервис).
type Candidate struct {
	UserID         uuid.UUID
	FirstName      string
	LastName       string
	Headline       string
	City           string
	ShowFullName   bool
	ShowCity       bool
	ShowSalary     bool
	ShowFSP        bool
	ShowExperience bool
	// SalaryHidden — ожидание указано, но скрыто кандидатом. Скрытые поля (город, ожидание, стаж)
	// приходят пустыми: в оценке они считаются неизвестными, в объяснениях и фильтрах не участвуют.
	SalaryHidden      bool
	Grade             string
	GradeStatus       string
	Theta             float64
	SE                float64
	Percentile        float64
	HasTheta          bool
	CategoryUpdatedAt time.Time
	SalaryMin         int64
	SalaryExpected    int64
	WorkFormats       []string
	Relocation        bool
	Visibility        string
	ExperienceYears   float64
	Skills            map[int32]string // skill id → source (declared | resume | test_confirmed)
	SkillNames        map[int32]string
	Achievements      []Achievement
	FSPLinked         bool
	DeclinedCompany   int // отклонял приглашения этой компании
	AcceptedAny       int // принимал приглашения
}

// ExplanationFact — элемент объяснения.
type ExplanationFact struct {
	Kind     string   `json:"kind" enum:"test,skill,fsp,fresh,fit,accept"`
	Polarity string   `json:"polarity" enum:"+,-,0"`
	Text     string   `json:"text"`
	Contrib  *float64 `json:"contrib,omitempty" doc:"Вклад в релевантность (вес × значение)"`
}

// Scored — результат ранжирования одного кандидата.
type Scored struct {
	Candidate  Candidate
	Components map[string]float64 `json:"components"`
	Relevance  float64            `json:"relevance"`
	PAccept    float64            `json:"p_accept"`
	Score      float64            `json:"score"`
	Facts      []ExplanationFact  `json:"facts"`
	Confirmed  bool               `json:"confirmed"`
}

// Score считает компоненты, релевантность, вероятность отклика и объяснение.
func Score(need Need, c Candidate, w Weights, now time.Time) Scored {
	comp := map[string]float64{}
	var facts []ExplanationFact
	ptr := func(v float64) *float64 { return &v }

	// s_test: процентиль × уверенность.
	sTest := 0.0
	if c.HasTheta {
		conf := math.Max(0.5, math.Min(1, 1-c.SE/1.5))
		sTest = c.Percentile * conf
		confText := "высокая"
		if conf < 0.8 {
			confText = "средняя"
		}
		facts = append(facts, ExplanationFact{Kind: "test", Polarity: polarity(sTest, 0.5), Contrib: ptr(w.Test * sTest),
			Text: fmt.Sprintf("Тест: %d-й процентиль в категории, уверенность оценки %s", int(c.Percentile*100), confText)})
	} else {
		facts = append(facts, ExplanationFact{Kind: "test", Polarity: "-", Contrib: ptr(0), Text: "Тест не пройден: результат не подтверждён"})
	}
	comp["s_test"] = sTest

	// s_skills: покрытие обязательных и желательных навыков.
	mustCov, mustFound, mustMissing := coverage(need.MustSkills, c)
	niceCov, niceFound, _ := coverage(need.NiceSkills, c)
	sSkills := 0.7*mustCov + 0.3*niceCov
	if len(need.MustSkills) == 0 {
		sSkills = 0.7 + 0.3*niceCov
	}
	comp["s_skills"] = sSkills
	if len(need.MustSkills) > 0 {
		confirmed := 0
		for _, id := range mustFound {
			if c.Skills[id] == string(domain.SkillTestConfirmed) {
				confirmed++
			}
		}
		text := fmt.Sprintf("Навыки: %d из %d обязательных", len(mustFound), len(need.MustSkills))
		if confirmed > 0 {
			text += fmt.Sprintf(", %d подтверждены тестом", confirmed)
		}
		facts = append(facts, ExplanationFact{Kind: "skill", Polarity: polarity(mustCov, 0.6), Contrib: ptr(w.Skills * 0.7 * mustCov), Text: text})
		if len(mustMissing) > 0 {
			facts = append(facts, ExplanationFact{Kind: "skill", Polarity: "-", Text: "Нет в профиле: " + strings.Join(names(c, mustMissing), ", ")})
		}
	}
	if len(need.NiceSkills) > 0 && len(niceFound) > 0 {
		facts = append(facts, ExplanationFact{Kind: "skill", Polarity: "+", Contrib: ptr(w.Skills * 0.3 * niceCov),
			Text: "Желательные навыки: " + strings.Join(names(c, niceFound), ", ")})
	}

	// s_fsp: буст достижений с релевантностью и затуханием.
	sFSP, bestText := fspBoost(need.Specialization, c.Achievements, now)
	comp["s_fsp"] = sFSP
	switch {
	case !c.FSPLinked:
		facts = append(facts, ExplanationFact{Kind: "fsp", Polarity: "0", Text: "Нет данных ФСП — ранжирование по тесту и профилю", Contrib: ptr(0)})
	case len(c.Achievements) == 0:
		facts = append(facts, ExplanationFact{Kind: "fsp", Polarity: "0", Text: "ФСП ID привязан, достижений в реестре пока нет", Contrib: ptr(0)})
	default:
		facts = append(facts, ExplanationFact{Kind: "fsp", Polarity: "+", Contrib: ptr(w.FSP * sFSP), Text: "ФСП: " + bestText})
	}

	// s_fresh: давность теста.
	days := now.Sub(c.CategoryUpdatedAt).Hours() / 24
	sFresh := freshness(days)
	comp["s_fresh"] = sFresh
	facts = append(facts, ExplanationFact{Kind: "fresh", Polarity: polarity(sFresh, 0.7), Contrib: ptr(w.Fresh * sFresh),
		Text: fmt.Sprintf("Активность: тест %s", humanDays(days))})

	// s_fit: зарплата, формат, локация.
	salaryFit := need.Salary.FitScore(c.SalaryExpected)
	formatFit := formatOverlap(need.WorkFormats, c.WorkFormats)
	locFit := locationFit(need, c)
	sFit := (salaryFit + formatFit + locFit) / 3
	comp["s_fit"] = sFit
	facts = append(facts, ExplanationFact{Kind: "fit", Polarity: polarity(sFit, 0.7), Contrib: ptr(w.Fit * sFit), Text: fitText(need, c, salaryFit, formatFit, locFit)})

	relevance := w.Test*sTest + w.Skills*sSkills + w.FSP*sFSP + w.Fresh*sFresh + w.Fit*sFit

	// p_accept — эвристика вероятности отклика.
	pAccept, acceptText, acceptPol := acceptance(need, c, formatFit, locFit)
	comp["p_accept"] = pAccept
	facts = append(facts, ExplanationFact{Kind: "accept", Polarity: acceptPol, Text: acceptText})

	score := relevance * (0.6 + 0.4*pAccept)
	confirmed := c.GradeStatus == string(domain.GradeConfirmed) || c.GradeStatus == string(domain.GradeUpgradeSuggested)
	return Scored{Candidate: c, Components: comp, Relevance: round3(relevance), PAccept: round3(pAccept), Score: round3(score), Facts: facts, Confirmed: confirmed}
}

// Rank ранжирует кандидатов: подтверждённые выше неподтверждённых, внутри — по score.
func Rank(need Need, cands []Candidate, w Weights, now time.Time) []Scored {
	out := make([]Scored, 0, len(cands))
	for _, c := range cands {
		out = append(out, Score(need, c, w, now))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confirmed != out[j].Confirmed {
			return out[i].Confirmed
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Components["s_test"] != out[j].Components["s_test"] {
			return out[i].Components["s_test"] > out[j].Components["s_test"]
		}
		return out[i].Candidate.UserID.String() < out[j].Candidate.UserID.String()
	})
	return out
}

func coverage(required []int32, c Candidate) (cov float64, found, missing []int32) {
	if len(required) == 0 {
		return 1, nil, nil
	}
	var sum float64
	for _, id := range required {
		src, ok := c.Skills[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		found = append(found, id)
		sum += domain.SkillSource(src).Weight()
	}
	return sum / float64(len(required)), found, missing
}

func names(c Candidate, ids []int32) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := c.SkillNames[id]; ok && n != "" {
			out = append(out, n)
		} else {
			out = append(out, fmt.Sprintf("#%d", id))
		}
	}
	return out
}

// fspBoost — Σ level × result × relevance × 0.5^(лет/3), потолок 1; возвращает текст лучшего достижения.
func fspBoost(spec string, achs []Achievement, now time.Time) (float64, string) {
	total, best, bestText := 0.0, 0.0, ""
	for _, a := range achs {
		years := math.Max(0, now.Sub(a.EventDate).Hours()/24/365.25)
		v := refdata.LevelWeight(a.Level) * refdata.ResultWeight(a.ResultKind, a.Place) * refdata.Relevance(a.Discipline, spec) * math.Pow(0.5, years/3)
		total += v
		if v > best {
			best = v
			rel := "релевантно"
			if refdata.Relevance(a.Discipline, spec) <= refdata.MinRelevance {
				rel = "другая дисциплина"
			}
			bestText = fmt.Sprintf("%s (%d, %s)", a.EventName, a.EventDate.Year(), rel)
		}
	}
	return math.Min(1, total), bestText
}

func freshness(days float64) float64 {
	switch {
	case days <= 90:
		return 1
	case days >= 365:
		return 0.3
	default:
		return 1 - 0.7*(days-90)/275
	}
}

func formatOverlap(need, cand []string) float64 {
	if len(need) == 0 || len(cand) == 0 {
		return 1
	}
	for _, a := range need {
		for _, b := range cand {
			if a == b {
				return 1
			}
		}
	}
	return 0
}

func locationFit(need Need, c Candidate) float64 {
	if need.City == "" {
		return 1
	}
	if c.City != "" && strings.EqualFold(c.City, need.City) {
		return 1
	}
	if need.RemoteOK {
		for _, f := range c.WorkFormats {
			if f == "remote" {
				return 1
			}
		}
	}
	if c.Relocation {
		return 1
	}
	if c.City == "" {
		return 0.7
	}
	return 0
}

func fitText(need Need, c Candidate, salary, format, loc float64) string {
	var parts []string
	switch {
	case c.SalaryHidden:
		parts = append(parts, "ожидание по зарплате скрыто кандидатом")
	case c.SalaryExpected == 0:
		parts = append(parts, "ожидание по зарплате не указано")
	case salary >= 1:
		parts = append(parts, fmt.Sprintf("ожидание %s ₽ внутри вилки", money(c.SalaryExpected)))
	case salary > 0:
		parts = append(parts, fmt.Sprintf("ожидание %s ₽ чуть выше вилки", money(c.SalaryExpected)))
	default:
		parts = append(parts, fmt.Sprintf("ожидание %s ₽ выше вилки", money(c.SalaryExpected)))
	}
	if format >= 1 {
		parts = append(parts, "формат совпадает")
	} else {
		parts = append(parts, "формат не совпадает")
	}
	if need.City != "" {
		switch {
		case loc >= 1:
			parts = append(parts, "локация подходит")
		case c.City == "":
			parts = append(parts, "город не указан или скрыт")
		default:
			parts = append(parts, "локация не совпадает")
		}
	}
	return "Условия: " + strings.Join(parts, "; ")
}

func acceptance(need Need, c Candidate, formatFit, locFit float64) (p float64, text, pol string) {
	p = 0.5
	var reasons []string
	switch c.Visibility {
	case string(domain.VisibilityOpen):
		p += 0.2
		reasons = append(reasons, "открыт к предложениям")
	case string(domain.VisibilityPassive):
		p -= 0.1
		reasons = append(reasons, "рассматривает предложения пассивно")
	}
	if c.SalaryMin > 0 && need.Salary.To < c.SalaryMin {
		p = 0.05
		// Минимум кандидата приватен: называем только факт, без суммы.
		reasons = append(reasons, "вилка ниже минимальных ожиданий кандидата")
	}
	if formatFit < 1 {
		p -= 0.2
		reasons = append(reasons, "формат не совпадает")
	}
	if locFit < 1 {
		p -= 0.2
		if c.City == "" {
			reasons = append(reasons, "город не указан или скрыт")
		} else {
			reasons = append(reasons, "локация не совпадает")
		}
	}
	if c.DeclinedCompany > 0 {
		p -= 0.15
		reasons = append(reasons, "ранее отклонял приглашения компании")
	}
	if c.AcceptedAny > 0 {
		p += 0.1
		reasons = append(reasons, "принимал приглашения на платформе")
	}
	p = math.Max(0.05, math.Min(0.95, p))
	switch {
	case p >= 0.6:
		pol = "+"
	case p < 0.4:
		pol = "-"
	default:
		pol = "0"
	}
	return p, fmt.Sprintf("Вероятность отклика %d%%: %s", int(p*100), strings.Join(reasons, ", ")), pol
}

func polarity(v, threshold float64) string {
	switch {
	case v >= threshold:
		return "+"
	case v < threshold/2:
		return "-"
	default:
		return "0"
	}
}

func humanDays(days float64) string {
	switch {
	case days < 1:
		return "сегодня"
	case days < 30:
		return fmt.Sprintf("%d дн. назад", int(days))
	case days < 365:
		return fmt.Sprintf("%d мес. назад", int(days/30))
	default:
		return fmt.Sprintf("%.1f г. назад", days/365)
	}
}

func money(v int64) string {
	s := fmt.Sprintf("%d", v)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, c)
	}
	return string(out)
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// --- карта категорий ---

// CategoryStat — агрегат по категории.
type CategoryStat struct {
	Specialization string   `json:"specialization"`
	Grade          string   `json:"grade"`
	InRange        bool     `json:"in_range" doc:"Грейд входит в диапазон потребности"`
	Total          int      `json:"total"`
	Confirmed      int      `json:"confirmed"`
	WithFSP        int      `json:"with_fsp"`
	ConfirmedShare float64  `json:"confirmed_share"`
	FSPShare       float64  `json:"fsp_share"`
	SalaryP25      int64    `json:"salary_p25"`
	SalaryP50      int64    `json:"salary_p50"`
	SalaryP75      int64    `json:"salary_p75"`
	Coverage       float64  `json:"coverage" doc:"Доля кандидатов с ожиданием не выше верхней границы вилки"`
	Hints          []string `json:"hints"`
}

// CategoryMap строит агрегаты по грейдам специализации.
func CategoryMap(need Need, cands []Candidate) []CategoryStat {
	inRange := map[string]bool{}
	for _, g := range need.Grades() {
		inRange[g] = true
	}
	byGrade := map[string][]Candidate{}
	for _, c := range cands {
		byGrade[c.Grade] = append(byGrade[c.Grade], c)
	}
	out := make([]CategoryStat, 0, len(refdata.Grades))
	for _, g := range refdata.Grades {
		cs := byGrade[g.Code]
		st := CategoryStat{Specialization: need.Specialization, Grade: g.Code, InRange: inRange[g.Code], Total: len(cs), Hints: []string{}}
		var salaries []int64
		covered := 0
		for _, c := range cs {
			if c.GradeStatus == string(domain.GradeConfirmed) || c.GradeStatus == string(domain.GradeUpgradeSuggested) {
				st.Confirmed++
			}
			if len(c.Achievements) > 0 {
				st.WithFSP++
			}
			if c.SalaryExpected > 0 {
				salaries = append(salaries, c.SalaryExpected)
				if c.SalaryExpected <= need.Salary.To {
					covered++
				}
			}
		}
		if st.Total > 0 {
			st.ConfirmedShare = round3(float64(st.Confirmed) / float64(st.Total))
			st.FSPShare = round3(float64(st.WithFSP) / float64(st.Total))
		}
		if len(salaries) > 0 {
			sort.Slice(salaries, func(i, j int) bool { return salaries[i] < salaries[j] })
			st.SalaryP25, st.SalaryP50, st.SalaryP75 = quantile(salaries, 0.25), quantile(salaries, 0.5), quantile(salaries, 0.75)
			st.Coverage = round3(float64(covered) / float64(len(salaries)))
			for _, q := range []struct {
				label string
				value int64
			}{{"медианы", st.SalaryP50}, {"75-го процентиля", st.SalaryP75}} {
				if q.value > need.Salary.To {
					n := 0
					for _, s := range salaries {
						if s <= q.value {
							n++
						}
					}
					st.Hints = append(st.Hints, fmt.Sprintf("при вилке до %s ₽ (уровень %s) покрытие вырастет до %d%%", money(q.value), q.label, n*100/len(salaries)))
				}
			}
		} else {
			st.Coverage = 1
		}
		out = append(out, st)
	}
	return out
}

func quantile(sorted []int64, q float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Round(q * float64(len(sorted)-1)))
	return sorted[idx]
}

// --- фильтры версии подборки ---

// SelectionFilters — уточнение подборки; каждое изменение создаёт новую версию.
type SelectionFilters struct {
	Grades        []string `json:"grades,omitempty"`
	GradeStatuses []string `json:"grade_statuses,omitempty" doc:"confirmed, upgrade_suggested, not_confirmed; по умолчанию все"`
	SkillCodes    []string `json:"skills,omitempty" doc:"Коды навыков, которые обязаны быть в профиле"`
	City          string   `json:"city,omitempty"`
	Formats       []string `json:"formats,omitempty"`
	HasFSP        *bool    `json:"has_fsp,omitempty"`
	MinScore      float64  `json:"min_score,omitempty"`
	HideInvited   bool     `json:"hide_invited,omitempty"`
	Limit         int      `json:"limit,omitempty" doc:"По умолчанию 50"`
}

// MaxLimit — наибольший размер выдачи подбора и поиска.
const MaxLimit = 200

// Apply применяет фильтры к ранжированному списку. skillIDs — коды фильтра, переведённые в id.
func Apply(scored []Scored, f SelectionFilters, skillIDs []int32, invited map[uuid.UUID]bool) []Scored {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	grades := toSet(f.Grades)
	statuses := toSet(f.GradeStatuses)
	formats := toSet(f.Formats)
	out := make([]Scored, 0, len(scored))
	for _, s := range scored {
		c := s.Candidate
		if len(grades) > 0 && !grades[c.Grade] {
			continue
		}
		if len(statuses) > 0 && !statuses[c.GradeStatus] {
			continue
		}
		if f.City != "" && !strings.EqualFold(c.City, f.City) {
			continue
		}
		if len(formats) > 0 && formatOverlap(f.Formats, c.WorkFormats) < 1 {
			continue
		}
		if f.HasFSP != nil && *f.HasFSP != (len(c.Achievements) > 0) {
			continue
		}
		if s.Score < f.MinScore {
			continue
		}
		if f.HideInvited && invited[c.UserID] {
			continue
		}
		ok := true
		for _, id := range skillIDs {
			if _, has := c.Skills[id]; !has {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func toSet(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		if x != "" {
			m[x] = true
		}
	}
	return m
}

// SelectionDiff — изменения между версиями подборки.
type SelectionDiff struct {
	Added   []uuid.UUID `json:"added"`
	Removed []uuid.UUID `json:"removed"`
	Moved   int         `json:"moved" doc:"Сколько кандидатов сменили позицию"`
}

// DiffVersions сравнивает порядок кандидатов двух версий.
func DiffVersions(prev, cur []uuid.UUID) SelectionDiff {
	d := SelectionDiff{Added: []uuid.UUID{}, Removed: []uuid.UUID{}}
	prevPos := map[uuid.UUID]int{}
	for i, id := range prev {
		prevPos[id] = i
	}
	curPos := map[uuid.UUID]int{}
	for i, id := range cur {
		curPos[id] = i
		if p, ok := prevPos[id]; !ok {
			d.Added = append(d.Added, id)
		} else if p != i {
			d.Moved++
		}
	}
	for _, id := range prev {
		if _, ok := curPos[id]; !ok {
			d.Removed = append(d.Removed, id)
		}
	}
	return d
}

// reAmount — денежная сумма в тексте объяснения («260 000 ₽»).
var reAmount = regexp.MustCompile(`\d[\d\x{00A0}\x{202F} ]*\s?₽`)

// redactFacts убирает суммы из сохранённых объяснений по текущим настройкам приватности:
// минимум кандидата («accept») не показывается никогда, ожидание («fit») — если кандидат его скрыл.
// Нужна для версий подборки, построенных до изменения настроек (снимки хранятся как текст).
func redactFacts(facts []ExplanationFact, c Candidate) []ExplanationFact {
	for i := range facts {
		if facts[i].Kind == "accept" || facts[i].Kind == "fit" && !c.ShowSalary {
			facts[i].Text = reAmount.ReplaceAllString(facts[i].Text, "[скрыто]")
		}
	}
	return facts
}
