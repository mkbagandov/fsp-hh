// Package seed — детерминированная синтетика для демо и валидации: кандидаты с профилями и
// реальными тест-сессиями (ответы по скрытой θ через 3PL), достижения ФСП, компании, потребности,
// вакансии, приглашения и отклики — всё через сервисы приложения, чтобы данные были «живыми».
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/app/applications"
	"fsp/internal/app/candidates"
	"fsp/internal/app/employers"
	"fsp/internal/app/invitations"
	"fsp/internal/app/matching"
	apptesting "fsp/internal/app/testing"
	"fsp/internal/app/vacancies"
	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/infrastructure/secrets"
	"fsp/internal/storage/postgres/sqlcgen"
	"fsp/pkg/itembank"
)

// Domain — домен всех сид-аккаунтов: по нему же выполняется сброс.
const Domain = "@seed.fsp.local"

// Password — общий пароль демо-аккаунтов (только локально).
const Password = "Demo1234!"

// Options — параметры генерации.
type Options struct {
	Candidates int
	Seed       uint64
	SkipTests  bool
}

// Reset удаляет всех сид-пользователей и их компании (каскадом — всё, что они создали).
func Reset(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if _, err := pool.Exec(ctx, "DELETE FROM companies WHERE id IN (SELECT eu.company_id FROM employer_users eu JOIN users u ON u.id = eu.user_id WHERE u.email LIKE $1)", "%"+Domain); err != nil {
		return 0, err
	}
	tag, err := pool.Exec(ctx, "DELETE FROM users WHERE email LIKE $1", "%"+Domain)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type runner struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	q     *sqlcgen.Queries
	log   *slog.Logger
	rnd   *rand.Rand
	hash  string
	cand  *candidates.Service
	test  *apptesting.Service
	emp   *employers.Service
	match *matching.Service
	inv   *invitations.Service
	vac   *vacancies.Service
	app   *applications.Service
	fams  map[string]sqlcgen.ItemFamily
	skill map[string][]refdata.Skill
	stack map[string][][]string
	now   time.Time
}

type candidate struct {
	id       uuid.UUID
	spec     string
	grade    string
	theta    float64
	consent  bool
	visible  bool
	tested   bool
	created  time.Time
	email    string
	salary   int64
	skills   []string
	withFSP  bool
	lastName string
}

// Run генерирует данные. Идемпотентность — через Reset перед запуском.
func Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, opt Options) error {
	if opt.Candidates <= 0 {
		opt.Candidates = 300
	}
	if opt.Seed == 0 {
		opt.Seed = 42
	}
	hash, err := secrets.HashPassword(Password)
	if err != nil {
		return err
	}
	mock, err := llm.New(llm.Config{Provider: "mock"}, log)
	if err != nil {
		return err
	}
	if _, err := apptesting.SyncFamilies(ctx, pool); err != nil {
		return fmt.Errorf("sync families: %w", err)
	}
	r := &runner{
		ctx: ctx, pool: pool, q: sqlcgen.New(pool), log: log, rnd: rand.New(rand.NewPCG(opt.Seed, opt.Seed^0x9E3779B97F4A7C15)), hash: hash,
		cand: candidates.New(pool, log), test: apptesting.New(apptesting.Config{}, pool, log), emp: employers.New(pool, mock, log),
		match: matching.New(pool, log), inv: invitations.New(invitations.Config{}, pool, log), vac: vacancies.New(pool, log), now: time.Now(),
	}
	r.app = applications.New(pool, r.inv, log)
	fams, err := r.q.ListItemFamilies(ctx)
	if err != nil {
		return err
	}
	r.fams = map[string]sqlcgen.ItemFamily{}
	for _, f := range fams {
		r.fams[f.ID] = *f
	}
	r.skill = map[string][]refdata.Skill{}
	idx := refdata.MustSkills()
	for _, s := range idx.Skills {
		for _, sp := range s.Specs {
			r.skill[sp] = append(r.skill[sp], s)
		}
	}
	// Связные стеки: оставляем только коды, которые есть в справочнике.
	r.stack = map[string][][]string{}
	for sp, sets := range stacks {
		for _, set := range sets {
			var ok []string
			for _, code := range set {
				if _, found := idx.ByCode(code); found {
					ok = append(ok, code)
				}
			}
			if len(ok) >= 3 {
				r.stack[sp] = append(r.stack[sp], ok)
			}
		}
	}

	// Кандидаты: демо + синтетика.
	cands := make([]candidate, 0, opt.Candidates+2)
	demo, err := r.candidate(candidateSpec{email: "demo-candidate" + Domain, first: "Артём", last: "Соколов", male: true, spec: "backend", grade: "middle", theta: 0.45, consent: true, visibility: "open", withFSP: true, test: !opt.SkipTests, fixedClaim: true, city: "Казань"})
	if err != nil {
		return fmt.Errorf("demo candidate: %w", err)
	}
	cands = append(cands, demo)
	fresh, err := r.candidate(candidateSpec{email: "demo-candidate-new" + Domain, first: "Полина", last: "Егорова", male: false, spec: "frontend", grade: "junior", theta: -1.0, consent: true, visibility: "open", test: false, city: "Санкт-Петербург"})
	if err != nil {
		return fmt.Errorf("demo candidate new: %w", err)
	}
	cands = append(cands, fresh)
	for i := 0; i < opt.Candidates; i++ {
		spec := r.candidateSpec(i)
		spec.test = spec.test && !opt.SkipTests
		c, err := r.candidate(spec)
		if err != nil {
			return fmt.Errorf("candidate %d: %w", i, err)
		}
		cands = append(cands, c)
		if (i+1)%50 == 0 {
			log.Info("seed: candidates", "done", i+1, "of", opt.Candidates)
		}
	}

	// Работодатели, потребности, вакансии.
	companies, err := r.companies()
	if err != nil {
		return err
	}
	needs, err := r.needs(companies)
	if err != nil {
		return err
	}
	vacs, err := r.vacancies(companies, needs)
	if err != nil {
		return err
	}
	if err := r.invitations(companies, needs, cands); err != nil {
		return err
	}
	if err := r.applications(companies, vacs, cands); err != nil {
		return err
	}
	// Админ.
	adminID, err := r.user("demo-admin"+Domain, sqlcgen.UserRoleAdmin)
	if err != nil {
		return err
	}
	_ = adminID
	log.Info("seed: done", "candidates", len(cands), "companies", len(companies), "needs", len(needs), "vacancies", len(vacs))
	return nil
}

func (r *runner) pick(xs []weighted) string {
	total := 0
	for _, x := range xs {
		total += x.w
	}
	n := r.rnd.IntN(total)
	for _, x := range xs {
		if n < x.w {
			return x.v
		}
		n -= x.w
	}
	return xs[len(xs)-1].v
}

func (r *runner) one(xs []string) string { return xs[r.rnd.IntN(len(xs))] }

func (r *runner) normal(mu, sigma float64) float64 { return mu + sigma*r.rnd.NormFloat64() }

func (r *runner) user(email string, role sqlcgen.UserRole) (uuid.UUID, error) {
	hash := r.hash
	u, err := r.q.CreateUser(r.ctx, sqlcgen.CreateUserParams{Email: email, PasswordHash: &hash, Role: role})
	if err != nil {
		return uuid.Nil, err
	}
	if err := r.q.SetEmailVerified(r.ctx, u.ID); err != nil {
		return uuid.Nil, err
	}
	if _, err := r.q.CreateConsent(r.ctx, sqlcgen.CreateConsentParams{UserID: u.ID, Kind: sqlcgen.ConsentKindProcessing, Version: "1"}); err != nil {
		return uuid.Nil, err
	}
	return u.ID, nil
}

type candidateSpec struct {
	email, first, last, spec, grade, city, visibility string
	male, consent, withFSP, test, fixedClaim          bool
	theta                                             float64
	idx                                               int
}

func (r *runner) candidateSpec(i int) candidateSpec {
	male := r.rnd.Float64() < 0.62
	first, last := r.one(femaleFirst), r.one(lastFemale)
	if male {
		first, last = r.one(maleFirst), r.one(lastMale)
	}
	spec := r.pick(specWeights)
	grade := r.pick([]weighted{{"junior", 35}, {"middle", 45}, {"senior", 20}})
	mu := map[string]float64{"junior": -1.2, "middle": 0.2, "senior": 1.1}[grade]
	vis := r.pick([]weighted{{"open", 70}, {"passive", 25}, {"hidden", 5}})
	return candidateSpec{
		email: fmt.Sprintf("cand%03d%s", i+1, Domain), first: first, last: last, male: male, spec: spec, grade: grade,
		theta: r.normal(mu, 0.5), city: r.pick(cities), visibility: vis, consent: r.rnd.Float64() < 0.9,
		withFSP: r.rnd.Float64() < 0.25, test: testable(spec) && r.rnd.Float64() < 0.85, idx: i,
	}
}

func (r *runner) candidate(sp candidateSpec) (candidate, error) {
	id, err := r.user(sp.email, sqlcgen.UserRoleCandidate)
	if err != nil {
		return candidate{}, err
	}
	created := r.now.Add(-time.Duration(r.rnd.IntN(45*24)) * time.Hour)
	if _, err := r.pool.Exec(r.ctx, "UPDATE users SET created_at = $2::timestamptz, last_login_at = $2::timestamptz + interval '1 hour' WHERE id = $1", id, created); err != nil {
		return candidate{}, err
	}
	if sp.consent {
		if _, err := r.q.CreateConsent(r.ctx, sqlcgen.CreateConsentParams{UserID: id, Kind: sqlcgen.ConsentKindPublication, Version: "1"}); err != nil {
			return candidate{}, err
		}
	}
	years := map[string][2]int{"junior": {0, 2}, "middle": {2, 6}, "senior": {5, 12}}[sp.grade]
	exp := float64(years[0]) + r.rnd.Float64()*float64(years[1]-years[0])
	base := map[string][2]int64{"junior": {90000, 150000}, "middle": {180000, 280000}, "senior": {300000, 450000}}[sp.grade]
	salary := base[0] + r.rnd.Int64N(base[1]-base[0]+1)
	if sp.spec == "data_ml" || sp.spec == "devops" {
		salary = salary * 11 / 10
	}
	salary = salary / 5000 * 5000
	salaryMin := salary * 85 / 100 / 5000 * 5000
	formats := r.pick([]weighted{{"remote", 45}, {"hybrid", 30}, {"office", 10}, {"remote,hybrid", 15}})
	headline := fmt.Sprintf(r.one(headlines[sp.spec]), int(math.Round(exp)))
	if exp < 1 {
		headline = strings.ReplaceAll(headline, "0 лет", "меньше года")
	}
	about := r.one(abouts)
	city := sp.city
	relocation := r.rnd.Float64() < 0.3
	specPtr := sp.spec
	if _, err := r.cand.Update(r.ctx, id, candidates.UpdateInput{
		FirstName: sp.first, LastName: sp.last, City: &city, WorkFormats: strings.Split(formats, ","), Relocation: relocation,
		SalaryMin: &salaryMin, SalaryExpected: &salary, Visibility: sp.visibility, Headline: &headline, About: &about, ExperienceYears: &exp,
		PrimarySpecialization: &specPtr, Links: map[string]string{"github": "https://github.com/" + strings.ToLower(translit(sp.last)) + fmt.Sprintf("%d", r.rnd.IntN(900)+100)},
	}); err != nil {
		return candidate{}, fmt.Errorf("profile: %w", err)
	}
	// Навыки: 5–12 из специализации + 1–3 общих.
	pool := r.skill[sp.spec]
	n := 5 + r.rnd.IntN(8)
	codes := map[string]bool{}
	var skills []candidates.SkillInput
	if sets := r.stack[sp.spec]; len(sets) > 0 {
		for _, code := range sets[r.rnd.IntN(len(sets))] {
			if !codes[code] && r.rnd.Float64() < 0.85 {
				codes[code] = true
				skills = append(skills, candidates.SkillInput{Code: code})
			}
		}
	}
	for len(skills) < n && len(pool) > 0 {
		s := pool[r.rnd.IntN(len(pool))]
		if !codes[s.Code] {
			codes[s.Code] = true
			skills = append(skills, candidates.SkillInput{Code: s.Code})
		}
	}
	for k, common := 0, r.skill["common"]; k < 1+r.rnd.IntN(3) && len(common) > 0; k++ {
		s := common[r.rnd.IntN(len(common))]
		if !codes[s.Code] {
			codes[s.Code] = true
			skills = append(skills, candidates.SkillInput{Code: s.Code})
		}
	}
	if _, err := r.cand.SetSkills(r.ctx, id, skills, domain.SkillDeclared); err != nil {
		return candidate{}, fmt.Errorf("skills: %w", err)
	}
	// Опыт и образование.
	var exps []candidates.ExperienceInput
	end := r.now.AddDate(0, -r.rnd.IntN(3), 0)
	for k := 0; k < 1+r.rnd.IntN(3) && end.Year() > 2012; k++ {
		months := 10 + r.rnd.IntN(30)
		start := end.AddDate(0, -months, 0)
		endStr := end.Format("2006-01-02")
		var endPtr *string
		if k > 0 {
			endPtr = &endStr
		}
		desc := "Разработка и поддержка сервисов, участие в планировании и код-ревью."
		exps = append(exps, candidates.ExperienceInput{CompanyName: r.one(companiesPast), Position: r.one(positions[sp.spec]), Specialization: &specPtr, StartedOn: start.Format("2006-01-02"), EndedOn: endPtr, Description: &desc})
		end = start.AddDate(0, -(1 + r.rnd.IntN(4)), 0)
	}
	if exp < 0.5 {
		exps = nil
	}
	if len(exps) > 0 {
		if _, err := r.cand.ReplaceExperiences(r.ctx, id, exps); err != nil {
			return candidate{}, fmt.Errorf("experiences: %w", err)
		}
	}
	uni := universities[r.rnd.IntN(len(universities))]
	deg := r.one(degrees)
	year := int32(r.now.Year() - int(exp) - 1 - r.rnd.IntN(3)) //nolint:gosec // малые числа
	if _, err := r.cand.ReplaceEducation(r.ctx, id, []candidates.EducationInput{{Institution: uni.name, Degree: &deg, Field: &uni.field, GraduatedYear: &year}}); err != nil {
		return candidate{}, fmt.Errorf("education: %w", err)
	}
	phone := fmt.Sprintf("+7 9%02d %03d-%02d-%02d", r.rnd.IntN(100), r.rnd.IntN(1000), r.rnd.IntN(100), r.rnd.IntN(100))
	tg := "@" + strings.ToLower(translit(sp.first)) + "_" + strings.ToLower(translit(sp.last))
	email := sp.email
	if _, err := r.cand.UpdateContacts(r.ctx, id, candidates.Contacts{Phone: &phone, Telegram: &tg, Email: &email, Other: map[string]string{}}); err != nil {
		return candidate{}, fmt.Errorf("contacts: %w", err)
	}
	c := candidate{id: id, spec: sp.spec, grade: sp.grade, theta: sp.theta, consent: sp.consent, visible: sp.visibility != "hidden", created: created, email: sp.email, salary: salary, withFSP: sp.withFSP, lastName: sp.last}
	for code := range codes {
		c.skills = append(c.skills, code)
	}
	if sp.withFSP {
		if err := r.fsp(id, sp); err != nil {
			return candidate{}, fmt.Errorf("fsp: %w", err)
		}
	}
	if sp.test {
		if err := r.runTest(c, sp); err != nil {
			return candidate{}, fmt.Errorf("test: %w", err)
		}
		c.tested = true
	}
	return c, nil
}

func (r *runner) fsp(id uuid.UUID, sp candidateSpec) error {
	if _, err := r.q.CreateFSPLink(r.ctx, sqlcgen.CreateFSPLinkParams{UserID: id, FspID: fmt.Sprintf("FSP-%06d", 100000+sp.idx), Issuer: "seed", Subject: uuid.NewString()}); err != nil {
		return err
	}
	n := 1 + r.rnd.IntN(3)
	for k := 0; k < n; k++ {
		ev := fspEvents[r.rnd.IntN(len(fspEvents))]
		if sp.spec == "security" && r.rnd.Float64() < 0.6 {
			ev = fspEvents[3]
		}
		date := r.now.AddDate(0, -r.rnd.IntN(48), -r.rnd.IntN(28))
		kind := sqlcgen.FspResultKindParticipation
		var place *int32
		if r.rnd.Float64() < 0.6 {
			kind = sqlcgen.FspResultKindPlace
			p := int32(1 + r.rnd.IntN(25)) //nolint:gosec // малые числа
			if sp.grade == "senior" && r.rnd.Float64() < 0.5 {
				p = int32(1 + r.rnd.IntN(3)) //nolint:gosec // малые числа
			}
			place = &p
		}
		if err := r.q.UpsertFSPAchievement(r.ctx, sqlcgen.UpsertFSPAchievementParams{
			UserID: id, ExternalID: fmt.Sprintf("seed-%d-%d-%d", sp.idx, k, date.Year()), Discipline: sqlcgen.FspDiscipline(ev.discipline), Level: sqlcgen.FspLevel(ev.level),
			EventName: fmt.Sprintf("%s %d", ev.name, date.Year()), EventDate: date, ResultKind: kind, Place: place,
		}); err != nil {
			return err
		}
	}
	return nil
}

// runTest проходит реальную адаптивную сессию: ответ верный с вероятностью 3PL по скрытой θ.
func (r *runner) runTest(c candidate, sp candidateSpec) error {
	claimed := sp.grade
	roll := r.rnd.Float64()
	if sp.fixedClaim {
		roll = 0.5
	}
	switch {
	case roll < 0.2 && claimed != "senior":
		claimed = map[string]string{"junior": "middle", "middle": "senior"}[claimed]
	case roll > 0.9 && claimed != "junior":
		claimed = map[string]string{"middle": "junior", "senior": "middle"}[claimed]
	}
	stack := c.skills
	if len(stack) > 5 {
		stack = stack[:5]
	}
	years := c.theta
	_ = years
	if _, err := r.test.SubmitSurvey(r.ctx, c.id, apptesting.SurveyInput{Specialization: c.spec, Industry: r.one(industries), PrimaryStack: stack, ClaimedGrade: claimed}); err != nil {
		return fmt.Errorf("survey: %w", err)
	}
	view, item, err := r.test.StartTest(r.ctx, c.id, apptesting.StartInput{Specialization: c.spec, TargetGrade: claimed})
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	for guard := 0; item != nil && guard < 40; guard++ {
		row, err := r.q.GetTestItem(r.ctx, sqlcgen.GetTestItemParams{ID: item.ID, SessionID: view.ID})
		if err != nil {
			return err
		}
		var inst itembank.Instance
		_ = json.Unmarshal(row.Instance, &inst)
		p := 0.2
		if f, ok := r.fams[row.FamilyID]; ok {
			p = f.C + (1-f.C)/(1+math.Exp(-f.A*(c.theta-f.B)))
		}
		correct := r.rnd.Float64() < p
		n := inst.Answer.Number
		sub := itembank.Submission{Keys: inst.Answer.Keys, Number: &n, Text: inst.Answer.Text, Order: inst.Answer.Order, Rows: inst.Answer.Rows}
		if !correct {
			wrong := n + 1000
			sub = itembank.Submission{Keys: []string{"zz"}, Number: &wrong, Text: "не знаю", Order: []string{"zz"}, Rows: [][]string{{"x"}}}
		}
		res, err := r.test.Answer(r.ctx, c.id, view.ID, item.ID, sub)
		if err != nil {
			return fmt.Errorf("answer: %w", err)
		}
		if res.Finished {
			break
		}
		item, err = r.test.NextItem(r.ctx, c.id, view.ID)
		if err != nil {
			return fmt.Errorf("next: %w", err)
		}
	}
	// Отодвигаем время сессии и категории к дате регистрации, чтобы графики и «свежесть» были правдоподобны.
	at := c.created.Add(time.Duration(1+r.rnd.IntN(72)) * time.Hour)
	if _, err := r.pool.Exec(r.ctx, "UPDATE test_sessions SET created_at = $2::timestamptz, started_at = $2::timestamptz, finished_at = $2::timestamptz + interval '18 minutes' WHERE id = $1", view.ID, at); err != nil {
		return err
	}
	if _, err := r.pool.Exec(r.ctx, "UPDATE candidate_categories SET updated_at = $2::timestamptz, confirmed_at = CASE WHEN confirmed_at IS NULL THEN NULL ELSE $2::timestamptz END WHERE user_id = $1 AND specialization = $3::specialization_code", c.id, at.Add(18*time.Minute), c.spec); err != nil {
		return err
	}
	return nil
}

type company struct {
	id    uuid.UUID
	owner uuid.UUID
	name  string
}

func (r *runner) companies() ([]company, error) {
	out := make([]company, 0, len(companyNames))
	for i, name := range companyNames {
		email := fmt.Sprintf("employer%02d%s", i+1, Domain)
		if i == 0 {
			email = "demo-employer" + Domain
		}
		uid, err := r.user(email, sqlcgen.UserRoleEmployer)
		if err != nil {
			return nil, err
		}
		city := r.pick(cities)
		c, err := r.emp.UpsertCompany(r.ctx, uid, employers.CompanyInput{
			Name: name, Industry: r.one(industries), City: city, Website: "https://" + strings.ToLower(translit(strings.Fields(strings.NewReplacer("«", "", "»", "").Replace(name))[0])) + ".example",
			Description:    "Продуктовая компания: собственная разработка, несколько команд, прозрачные вилки и понятные процессы найма.",
			PublicContacts: map[string]string{"email": fmt.Sprintf("hr@company%02d.example", i+1), "telegram": fmt.Sprintf("@hr_company%02d", i+1)}, Position: "HR-менеджер",
		})
		if err != nil {
			return nil, fmt.Errorf("company %s: %w", name, err)
		}
		out = append(out, company{id: c.ID, owner: uid, name: c.Name})
	}
	return out, nil
}

type need struct {
	id      uuid.UUID
	company company
	row     *sqlcgen.Need
	spec    string
}

func (r *runner) gradeRange() (string, string) {
	lo := r.pick([]weighted{{"junior", 25}, {"middle", 55}, {"senior", 20}})
	hi := lo
	if lo == "junior" && r.rnd.Float64() < 0.5 {
		hi = "middle"
	}
	if lo == "middle" && r.rnd.Float64() < 0.6 {
		hi = "senior"
	}
	return lo, hi
}

func (r *runner) salaryFor(lo, hi string) (int64, int64) {
	base := map[string][2]int64{"junior": {90000, 160000}, "middle": {180000, 300000}, "senior": {280000, 450000}}
	from := base[lo][0] + r.rnd.Int64N(40000)
	to := base[hi][1] - r.rnd.Int64N(40000)
	if to < from+30000 {
		to = from + 30000
	}
	return from / 5000 * 5000, to / 5000 * 5000
}

func (r *runner) needSkills(spec string, must, nice int) ([]string, []string) {
	seen := map[string]bool{}
	var ordered []string
	if sets := r.stack[spec]; len(sets) > 0 {
		set := sets[r.rnd.IntN(len(sets))]
		perm := r.rnd.Perm(len(set))
		for _, i := range perm {
			ordered = append(ordered, set[i])
		}
	}
	pool := r.skill[spec]
	take := func(n int) []string {
		var out []string
		for len(out) < n && len(ordered) > 0 {
			code := ordered[0]
			ordered = ordered[1:]
			if !seen[code] {
				seen[code] = true
				out = append(out, code)
			}
		}
		for len(out) < n && len(pool) > 0 {
			s := pool[r.rnd.IntN(len(pool))]
			if !seen[s.Code] {
				seen[s.Code] = true
				out = append(out, s.Code)
			}
		}
		return out
	}
	return take(must), take(nice)
}

func (r *runner) needs(companies []company) ([]need, error) {
	var out []need
	for i := 0; i < 15; i++ {
		co := companies[i%len(companies)]
		spec := r.pick(specWeights)
		lo, hi := r.gradeRange()
		from, to := r.salaryFor(lo, hi)
		must, nice := r.needSkills(spec, 2+r.rnd.IntN(3), r.rnd.IntN(3))
		title := needTitles[spec][(i/len(companies))%len(needTitles[spec])]
		text := fmt.Sprintf(r.one(needTexts[spec]), strings.ToLower(title), fmt.Sprintf("%d–%d тыс.", from/1000, to/1000))
		formats := r.pick([]weighted{{"remote", 50}, {"hybrid", 35}, {"office", 15}})
		var city string
		if formats != "remote" {
			city = r.pick(cities)
		}
		n, err := r.emp.CreateNeed(r.ctx, co.owner, employers.NeedInput{
			Title: title, Description: text, TeamDescription: "Команда из 5–8 инженеров, двухнедельные спринты, код-ревью обязателен.",
			Specialization: spec, GradeMin: lo, GradeMax: hi, MustSkills: must, NiceSkills: nice, SalaryFrom: from, SalaryTo: to,
			WorkFormats: []string{formats}, City: city, RemoteOK: formats == "remote",
		})
		if err != nil {
			return nil, fmt.Errorf("need %d: %w", i, err)
		}
		row, err := r.emp.OwnNeed(r.ctx, co.owner, n.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, need{id: n.ID, company: co, row: row, spec: spec})
	}
	return out, nil
}

type vacancy struct {
	id      uuid.UUID
	company company
	status  string
}

func (r *runner) vacancies(companies []company, needs []need) ([]vacancy, error) {
	var out []vacancy
	for i := 0; i < 20; i++ {
		co := companies[(i*3)%len(companies)]
		spec := r.pick(specWeights)
		lo, hi := r.gradeRange()
		from, to := r.salaryFor(lo, hi)
		must, nice := r.needSkills(spec, 2+r.rnd.IntN(3), 1+r.rnd.IntN(3))
		title := r.one(needTitles[spec])
		text := fmt.Sprintf(r.one(needTexts[spec]), strings.ToLower(title), fmt.Sprintf("%d–%d тыс.", from/1000, to/1000)) + "\n\nЧто предлагаем: ДМС, обучение, прозрачный рост грейдов, оплата участия в соревнованиях по программированию."
		var needID *uuid.UUID
		for _, n := range needs {
			if n.company.id == co.id && n.spec == spec {
				id := n.id
				needID = &id
				break
			}
		}
		formats := r.pick([]weighted{{"remote", 50}, {"hybrid", 35}, {"office", 15}})
		v, err := r.vac.Create(r.ctx, co.owner, co.id, vacancies.Input{
			Title: title, Description: text, Specialization: spec, GradeMin: lo, GradeMax: hi, MustSkills: must, NiceSkills: nice,
			SalaryFrom: from, SalaryTo: to, WorkFormats: []string{formats}, City: r.pick(cities), RemoteOK: formats == "remote", NeedID: needID,
		})
		if err != nil {
			return nil, fmt.Errorf("vacancy %d: %w", i, err)
		}
		status := "published"
		if i%7 == 6 {
			status = "draft"
		}
		if status == "published" {
			if _, err := r.vac.SetStatus(r.ctx, co.id, v.ID, "published"); err != nil {
				return nil, err
			}
			if i%9 == 8 {
				if _, err := r.vac.SetStatus(r.ctx, co.id, v.ID, "closed"); err != nil {
					return nil, err
				}
				status = "closed"
			}
		}
		out = append(out, vacancy{id: v.ID, company: co, status: status})
	}
	return out, nil
}

// reservedForDemo — пары, которые сид не связывает: демо-работодатель и демо-кандидат. Сценарий показа
// (приложение Б документации, ролики питча) начинается с закрытых контактов, а случайное приглашение
// или отклик из сида выдали бы компании грант заранее.
func reservedForDemo(co company, companies []company, c candidate) bool {
	return len(companies) > 0 && co.id == companies[0].id && c.email == "demo-candidate"+Domain
}

func (r *runner) invitations(companies []company, needs []need, cands []candidate) error {
	byID := map[uuid.UUID]candidate{}
	for _, c := range cands {
		byID[c.id] = c
	}
	total, accepted := 0, 0
	for _, n := range needs {
		v, err := r.match.BuildVersion(r.ctx, n.row, n.company.owner, matching.SelectionFilters{Limit: 30})
		if err != nil {
			return fmt.Errorf("selection: %w", err)
		}
		k := 0
		for _, it := range v.Items {
			if k >= 4+r.rnd.IntN(4) {
				break
			}
			c, ok := byID[it.Candidate.ID]
			if !ok || reservedForDemo(n.company, companies, c) {
				continue
			}
			from, to := n.row.SalaryFrom, n.row.SalaryTo
			if r.rnd.Float64() < 0.3 {
				from, to = from-20000, to-20000
			}
			inv, err := r.inv.Create(r.ctx, n.company.owner, n.company.id, invitations.CreateInput{
				CandidateID: c.id, NeedID: &n.id, Message: fmt.Sprintf("Здравствуйте! Увидели ваш профиль в категории и хотим обсудить роль «%s». Команда из 6 инженеров, можно удалённо.", n.row.Title),
				SalaryFrom: int64(from), SalaryTo: int64(to), ContactMethod: invitations.ContactMethod{Kind: "telegram", Value: fmt.Sprintf("@hr_%s", strings.ToLower(translit(strings.Fields(strings.NewReplacer("«", "", "»", "").Replace(n.company.name))[0])))},
			})
			if err != nil {
				if domain.Is(err, domain.KindConflict) || domain.Is(err, domain.KindRateLimited) {
					continue
				}
				return fmt.Errorf("invite: %w", err)
			}
			k++
			total++
			switch roll := r.rnd.Float64(); {
			case roll < 0.45:
				if _, err := r.inv.Accept(r.ctx, c.id, inv.ID); err != nil {
					return fmt.Errorf("accept: %w", err)
				}
				accepted++
			case roll < 0.72:
				reason := r.pick([]weighted{{"salary_below", 35}, {"not_looking", 25}, {"role_mismatch", 15}, {"format_mismatch", 15}, {"company", 5}, {"other", 5}})
				if _, err := r.inv.Decline(r.ctx, c.id, inv.ID, reason, r.one(declineComments)); err != nil {
					return fmt.Errorf("decline: %w", err)
				}
			case roll < 0.85:
				if _, err := r.inv.GetForCandidate(r.ctx, c.id, inv.ID); err != nil {
					return err
				}
			}
		}
	}
	r.log.Info("seed: invitations", "total", total, "accepted", accepted)
	return nil
}

func (r *runner) applications(companies []company, vacs []vacancy, cands []candidate) error {
	var pool []candidate
	for _, c := range cands {
		// «новый» демо-кандидат остаётся без откликов: на нём показывают онбординг
		if c.consent && c.email != "demo-candidate-new"+Domain {
			pool = append(pool, c)
		}
	}
	total := 0
	for _, v := range vacs {
		if v.status != "published" {
			continue
		}
		n := r.rnd.IntN(6)
		for k := 0; k < n && len(pool) > 0; k++ {
			c := pool[r.rnd.IntN(len(pool))]
			if reservedForDemo(v.company, companies, c) {
				continue
			}
			a, err := r.app.Apply(r.ctx, c.id, v.id, r.one(coverLetters))
			if err != nil {
				if domain.Is(err, domain.KindConflict) {
					continue
				}
				return fmt.Errorf("apply: %w", err)
			}
			total++
			switch roll := r.rnd.Float64(); {
			case roll < 0.25:
				if _, err := r.app.View(r.ctx, v.company.owner, v.company.id, a.ID); err != nil {
					return err
				}
			case roll < 0.45:
				if _, err := r.app.Reject(r.ctx, v.company.owner, v.company.id, a.ID, r.one([]string{"Не хватает опыта с нашим стеком", "Выбрали другого кандидата", "Ожидания по зарплате выше вилки"})); err != nil {
					return err
				}
			case roll < 0.6:
				vr, err := r.vac.Own(r.ctx, v.company.id, v.id)
				if err != nil {
					return err
				}
				if _, err := r.app.Invite(r.ctx, v.company.owner, v.company.id, a.ID, applications.InviteInput{Message: "Спасибо за отклик! Приглашаем на техническое интервью.", SalaryFrom: int64(vr.SalaryFrom), SalaryTo: int64(vr.SalaryTo), ContactMethod: invitations.ContactMethod{Kind: "email", Value: "hr@company.example"}}); err != nil {
					if !domain.Is(err, domain.KindConflict) && !domain.Is(err, domain.KindRateLimited) {
						return fmt.Errorf("invite from application: %w", err)
					}
				}
			}
		}
	}
	r.log.Info("seed: applications", "total", total)
	return nil
}

var translitMap = map[rune]string{'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya"}

func translit(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if t, ok := translitMap[r]; ok {
			b.WriteString(t)
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
