package llm

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"fsp/internal/domain/refdata"
)

// Mock — детерминированный эмулятор LLM: правила, регулярные выражения и справочник
// синонимов навыков. Даёт тот же контракт и те же JSON-структуры, что и реальная модель,
// поэтому переключение провайдера не меняет вызывающий код и интерфейс.
type Mock struct {
	idx    *refdata.SkillIndex
	maxN   int
	cities []string
}

// NewMock создаёт эмулятор на текущем справочнике навыков.
func NewMock() *Mock {
	idx := refdata.MustSkills()
	maxN := 1
	for _, a := range idx.Aliases() {
		if n := strings.Count(a, " ") + 1; n > maxN {
			maxN = n
		}
	}
	if maxN > 4 {
		maxN = 4
	}
	return &Mock{idx: idx, maxN: maxN, cities: knownCities}
}

// Name — имя провайдера.
func (m *Mock) Name() string { return "mock" }

var (
	reEmail    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	rePhone    = regexp.MustCompile(`(?:\+7|8)[\s\-(]*(\d{3})[\s\-)]*(\d{3})[\s\-]*(\d{2})[\s\-]*(\d{2})`)
	reTelegram = regexp.MustCompile(`(?i)(?:t\.me/|telegram[:\s]+|тг[:\s]+|tg[:\s]+)@?([A-Za-z0-9_]{4,32})`)
	reHandle   = regexp.MustCompile(`(?:^|[\s,;:(])@([A-Za-z0-9_]{4,32})\b`)
	reGitHub   = regexp.MustCompile(`(?i)github\.com/([A-Za-z0-9\-]+)`)
	reGitLab   = regexp.MustCompile(`(?i)gitlab\.com/([A-Za-z0-9\-]+)`)
	reLinkedIn = regexp.MustCompile(`(?i)linkedin\.com/in/([A-Za-z0-9\-_]+)`)
	reHH       = regexp.MustCompile(`(?i)hh\.ru/resume/([A-Za-z0-9]+)`)
	reYear     = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)
	reDuration = regexp.MustCompile(`(?i)^\s*\d+\s*(год|года|лет|месяц|месяца|месяцев|year|years|month|months)`)
	// Важно: \b в RE2 — граница ASCII-слова и не работает после кириллицы; границы задаём явно.
	reSalary   = regexp.MustCompile(`(?i)(\d{2,3})\s?(?:000|к(?:[^\p{L}]|$)|k(?:[^\p{L}]|$)|тыс)`)
	reSalaryFR = regexp.MustCompile(`(?i)(?:от|from)\s*(\d{2,3})\s?(?:000|к|k|тыс)?\s*(?:до|to|[-–—])?\s*(\d{2,3})?\s?(?:000|к|k|тыс)?`)
	reExpYears = regexp.MustCompile(`(?i)(?:опыт|experience)[^\d]{0,30}(\d{1,2})(?:[.,](\d))?\s*(?:\+\s*)?(?:год|года|лет|year|years|\+)`)
	reFromYrs  = regexp.MustCompile(`(?i)(?:от|from|не менее|минимум)\s*(\d{1,2})\s*(?:\+\s*)?(?:год|года|лет|years?)`)
)

const monthsPattern = `(?:январ[ья]|феврал[ья]|марта?|апрел[ья]|ма[йя]|июн[ья]|июл[ья]|августа?|сентябр[ья]|октябр[ья]|ноябр[ья]|декабр[ья]|янв|фев|мар|апр|июн|июл|авг|сен|сент|окт|ноя|дек|january|february|march|april|may|june|july|august|september|october|november|december|jan|feb|mar|apr|jun|jul|aug|sep|sept|oct|nov|dec)\.?`

var (
	datePattern = `(?:(?:0?[1-9]|1[0-2])[./](?:19|20)\d{2}|` + monthsPattern + `\s+(?:19|20)\d{2}|(?:19|20)\d{2})`
	reRange     = regexp.MustCompile(`(?i)(` + datePattern + `)\s*(?:—|–|-|−|to|по|до)\s*(` + datePattern + `|настоящее время|по настоящее время|н\.\s?в\.|present|now|сейчас|текущее время|по сей день)`)
	reDate      = regexp.MustCompile(`(?i)^(?:(0?[1-9]|1[0-2])[./]((?:19|20)\d{2})|(` + monthsPattern + `)\s+((?:19|20)\d{2})|((?:19|20)\d{2}))$`)
)

var monthIndex = map[string]int{
	"январ": 1, "янв": 1, "january": 1, "jan": 1, "феврал": 2, "фев": 2, "february": 2, "feb": 2,
	"март": 3, "мар": 3, "march": 3, "mar": 3, "апрел": 4, "апр": 4, "april": 4, "apr": 4,
	"май": 5, "мая": 5, "may": 5, "июн": 6, "june": 6, "jun": 6, "июл": 7, "july": 7, "jul": 7,
	"август": 8, "авг": 8, "august": 8, "aug": 8, "сентябр": 9, "сен": 9, "сент": 9, "september": 9, "sep": 9, "sept": 9,
	"октябр": 10, "окт": 10, "october": 10, "oct": 10, "ноябр": 11, "ноя": 11, "november": 11, "nov": 11,
	"декабр": 12, "дек": 12, "december": 12, "dec": 12,
}

type section int

const (
	secNone section = iota
	secExperience
	secEducation
	secSkills
	secAbout
	secOther
)

var sectionHeaders = []struct {
	re  *regexp.Regexp
	sec section
}{
	{regexp.MustCompile(`(?i)^(опыт работы|опыт|experience|work experience|employment history|карьера)(?:[^\p{L}]|$)`), secExperience},
	{regexp.MustCompile(`(?i)^(образование|education)(?:[^\p{L}]|$)`), secEducation},
	{regexp.MustCompile(`(?i)^(ключевые навыки|навыки|skills|технологии|стек|tech stack|technical skills|компетенции)(?:[^\p{L}]|$)`), secSkills},
	{regexp.MustCompile(`(?i)^(о себе|обо мне|about|summary|профиль|profile)(?:[^\p{L}]|$)`), secAbout},
	{regexp.MustCompile(`(?i)^(контакты|сертификаты|языки|languages|проекты|projects|достижения|дополнительная информация|рекомендации|хобби|certificates|courses|курсы|повышение квалификации)(?:[^\p{L}]|$)`), secOther},
}

var (
	reGradeSenior = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(senior|сеньор|синьор|ведущий|старший)(?:[^\p{L}]|$)`)
	reGradeMiddle = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(middle|мидл|миддл)(?:[^\p{L}]|$)`)
	reGradeJunior = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(junior|джун|джуниор|младший|стажер|стажёр|intern)(?:[^\p{L}]|$)`)
	reGradeRange  = regexp.MustCompile(`(?i)middle\s*\+|мидл\s*\+|middle\s*/\s*senior|мидл\s*/\s*сеньор`)
)

// specKeywords — подсказки специализации в заголовке; границы слов заданы явно,
// чтобы «иб» не находилось в «гибрид», а «дата» — в «кандидата».
var specKeywords = []struct {
	re   *regexp.Regexp
	spec string
}{
	{regexp.MustCompile(`(?i)(?:^|[^\p{L}])(frontend|фронтенд|front-end|фронт|react|verstal|верстальщик)(?:[^\p{L}]|$)`), "frontend"},
	{regexp.MustCompile(`(?i)(?:^|[^\p{L}])(backend|бэкенд|бекенд|back-end|серверн\p{L}*)(?:[^\p{L}]|$)`), "backend"},
	{regexp.MustCompile(`(?i)(?:^|[^\p{L}])(devops|sre|инфраструктур\p{L}*|системный администратор|platform engineer)(?:[^\p{L}]|$)`), "devops"},
	{regexp.MustCompile(`(?i)(?:^|[^\p{L}])(data scientist|data science|ml|machine learning|ml-инженер|аналитик данных|data analyst|data engineer|дата-сайентист)(?:[^\p{L}]|$)`), "data_ml"},
	{regexp.MustCompile(`(?i)(?:^|[^\p{L}])(qa|тестировщик|тестирован\p{L}*|sdet|автотест\p{L}*)(?:[^\p{L}]|$)`), "qa"},
	{regexp.MustCompile(`(?i)(?:^|[^\p{L}])(security|безопасност\p{L}*|пентест\p{L}*|appsec|ciso|информационной безопасности)(?:[^\p{L}]|$)`), "security"},
}

var roleKeywords = regexp.MustCompile(`(?i)(разработчик|developer|engineer|инженер|программист|аналитик|analyst|тестировщик|qa|devops|sre|data scientist|scientist|архитектор|architect|тимлид|team ?lead|lead|руководитель|специалист|администратор|administrator)`)

var nameStop = regexp.MustCompile(`(?i)(резюме|resume|cv|разработчик|developer|engineer|инженер|программист|специалист|аналитик|менеджер|тестировщик|мужчина|женщина|лет|год|москва|санкт|опыт|experience|желаемая|должность|телефон|email|почта)`)

// ExtractResume разбирает текст резюме по правилам.
func (m *Mock) ExtractResume(_ context.Context, text string) (ResumeDraft, Usage, error) {
	start := time.Now()
	d := ResumeDraft{Skills: []DraftSkill{}, Experiences: []DraftExperience{}, Education: []DraftEducation{}, Links: map[string]string{}}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := splitLines(text)
	if len(strings.TrimSpace(text)) < 40 {
		d.Warnings = append(d.Warnings, "Текст резюме слишком короткий или не извлёкся из PDF: заполните профиль вручную")
		return d, m.usage(start), nil
	}

	// Контакты и ссылки — по всему тексту.
	if em := reEmail.FindString(text); em != "" {
		d.Email = strings.ToLower(em)
	}
	if ph := rePhone.FindStringSubmatch(text); ph != nil {
		d.Phone = "+7" + ph[1] + ph[2] + ph[3] + ph[4]
	}
	if tg := reTelegram.FindStringSubmatch(text); tg != nil {
		d.Telegram = tg[1]
	} else if h := reHandle.FindStringSubmatch(text); h != nil {
		d.Telegram = h[1]
	}
	if g := reGitHub.FindStringSubmatch(text); g != nil {
		d.Links["github"] = "https://github.com/" + g[1]
	}
	if g := reGitLab.FindStringSubmatch(text); g != nil {
		d.Links["gitlab"] = "https://gitlab.com/" + g[1]
	}
	if l := reLinkedIn.FindStringSubmatch(text); l != nil {
		d.Links["linkedin"] = "https://linkedin.com/in/" + l[1]
	}
	if h := reHH.FindStringSubmatch(text); h != nil {
		d.Links["hh"] = "https://hh.ru/resume/" + h[1]
	}

	// ФИО, город, заголовок — в начале документа.
	head := lines
	if len(head) > 25 {
		head = head[:25]
	}
	first, last, middle := detectName(head)
	d.FirstName, d.LastName, d.MiddleName = first, last, middle
	d.City = m.detectCity(head)
	if d.City == "" {
		d.City = m.detectCity(lines)
	}
	for _, l := range head {
		if roleKeywords.MatchString(l) && len([]rune(l)) <= 120 && !reRange.MatchString(l) && !strings.Contains(strings.ToLower(l), "опыт работы") {
			d.Headline = strings.TrimSpace(l)
			break
		}
	}
	if y := reExpYears.FindStringSubmatch(text); y != nil {
		v, _ := strconv.ParseFloat(y[1], 64)
		if y[2] != "" {
			frac, _ := strconv.ParseFloat(y[2], 64)
			v += frac / 10
		}
		d.ExperienceYears = v
	}

	// Секции.
	secs := splitSections(lines)
	d.About = clip(strings.Join(secs[secAbout], " "), 1000)
	d.Experiences = m.parseExperiences(secs[secExperience])
	d.Education = parseEducation(secs[secEducation])
	if d.ExperienceYears == 0 {
		d.ExperienceYears = totalYears(d.Experiences)
	}

	// Навыки — по всему тексту, но секция навыков даёт больший вес.
	found := m.findSkills(text)
	inSkills := m.findSkills(strings.Join(secs[secSkills], "\n"))
	inSkillSet := map[string]bool{}
	for _, s := range inSkills {
		inSkillSet[s.Code] = true
	}
	for i := range found {
		if inSkillSet[found[i].Code] {
			found[i].Confidence = 0.95
		}
	}
	d.Skills = found
	d.PrimarySpecialization = m.guessSpecialization(d.Headline, d.Skills)

	if d.FirstName == "" && d.LastName == "" {
		d.Warnings = append(d.Warnings, "Не удалось распознать ФИО")
	}
	if d.Email == "" && d.Phone == "" {
		d.Warnings = append(d.Warnings, "Контакты не найдены")
	}
	if len(d.Experiences) == 0 {
		d.Warnings = append(d.Warnings, "Опыт работы с датами не распознан")
	}
	if len(d.Skills) == 0 {
		d.Warnings = append(d.Warnings, "Навыки из справочника не найдены")
	}
	return m.sanitizeResume(d), m.usage(start), nil
}

// ExtractNeed разбирает описание потребности работодателя по правилам.
func (m *Mock) ExtractNeed(_ context.Context, text string) (NeedDraft, Usage, error) {
	start := time.Now()
	d := NeedDraft{MustSkills: []string{}, NiceSkills: []string{}, RemoteOK: false}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lower := strings.ToLower(strings.ReplaceAll(text, "ё", "е"))

	// Разбиваем на предложения/строки: «желательно/плюсом» переводит навыки в nice.
	nicePattern := regexp.MustCompile(`(?i)(желательно|будет плюсом|плюсом|приветствуется|nice to have|как плюс|бонусом|опционально)`)
	seen := map[string]bool{}
	for _, chunk := range regexp.MustCompile(`[.\n;•·]+`).Split(text, -1) {
		isNice := nicePattern.MatchString(chunk)
		for _, s := range m.findSkills(chunk) {
			if seen[s.Code] {
				continue
			}
			seen[s.Code] = true
			if isNice {
				d.NiceSkills = append(d.NiceSkills, s.Code)
			} else {
				d.MustSkills = append(d.MustSkills, s.Code)
			}
		}
	}

	// Грейды.
	gradeMin, gradeMax := "", ""
	switch {
	case reGradeSenior.MatchString(text):
		gradeMin, gradeMax = "senior", "senior"
	case reGradeMiddle.MatchString(text):
		gradeMin, gradeMax = "middle", "middle"
	case reGradeJunior.MatchString(text):
		gradeMin, gradeMax = "junior", "junior"
	}
	if reGradeRange.MatchString(text) {
		gradeMin, gradeMax = "middle", "senior"
	}
	if gradeMin == "" {
		if y := reFromYrs.FindStringSubmatch(text); y != nil {
			n, _ := strconv.Atoi(y[1])
			switch {
			case n >= 5:
				gradeMin, gradeMax = "senior", "senior"
			case n >= 2:
				gradeMin, gradeMax = "middle", "senior"
			default:
				gradeMin, gradeMax = "junior", "middle"
			}
		}
	}
	d.GradeMin, d.GradeMax = gradeMin, gradeMax

	// Зарплата: «от 250 до 350 тыс», «250–350к», «250 000 — 350 000 ₽».
	if mr := reSalaryFR.FindStringSubmatch(text); mr != nil && mr[1] != "" {
		d.SalaryFrom = toRubles(mr[1])
		if mr[2] != "" {
			d.SalaryTo = toRubles(mr[2])
		}
	}
	if d.SalaryFrom == 0 {
		all := reSalary.FindAllStringSubmatch(text, -1)
		if len(all) >= 2 {
			d.SalaryFrom, d.SalaryTo = toRubles(all[0][1]), toRubles(all[1][1])
		} else if len(all) == 1 {
			d.SalaryFrom = toRubles(all[0][1])
		}
	}
	if d.SalaryFrom > 0 && d.SalaryTo == 0 {
		d.SalaryTo = d.SalaryFrom
	}
	if d.SalaryFrom > d.SalaryTo && d.SalaryTo > 0 {
		d.SalaryFrom, d.SalaryTo = d.SalaryTo, d.SalaryFrom
	}
	if d.SalaryFrom == 0 {
		d.Warnings = append(d.Warnings, "Зарплатная вилка не найдена: укажите её вручную (обязательна)")
	}

	// Формат и город.
	if strings.Contains(lower, "удален") || strings.Contains(lower, "remote") {
		d.WorkFormats = append(d.WorkFormats, "remote")
		d.RemoteOK = true
	}
	if strings.Contains(lower, "гибрид") || strings.Contains(lower, "hybrid") {
		d.WorkFormats = append(d.WorkFormats, "hybrid")
	}
	if strings.Contains(lower, "офис") || strings.Contains(lower, "office") {
		d.WorkFormats = append(d.WorkFormats, "office")
	}
	d.City = m.detectCity(splitLines(text))

	// Заголовок и специализация.
	for _, l := range splitLines(text) {
		if roleKeywords.MatchString(l) && len([]rune(l)) <= 90 {
			d.Title = strings.TrimSpace(l)
			break
		}
	}
	skills := make([]DraftSkill, 0, len(d.MustSkills)+len(d.NiceSkills))
	for _, c := range append(append([]string{}, d.MustSkills...), d.NiceSkills...) {
		skills = append(skills, DraftSkill{Code: c})
	}
	d.Specialization = m.guessSpecialization(d.Title+" "+clip(text, 300), skills)
	return m.sanitizeNeed(d), m.usage(start), nil
}

// Complete — детерминированная заглушка генерации: возвращает усечённый вход.
func (m *Mock) Complete(_ context.Context, _ string, input string) (string, Usage, error) {
	start := time.Now()
	return clip(strings.TrimSpace(input), 400), m.usage(start), nil
}

func (m *Mock) usage(start time.Time) Usage {
	return Usage{Provider: "mock", Duration: time.Since(start)}
}

func (m *Mock) skillCodes() string {
	codes := make([]string, 0, m.idx.Len())
	for _, s := range m.idx.Skills {
		codes = append(codes, s.Code)
	}
	return strings.Join(codes, ", ")
}

// sanitizeResume отбрасывает неизвестные коды навыков и приводит значения в допустимые границы.
func (m *Mock) sanitizeResume(d ResumeDraft) ResumeDraft {
	out := d
	out.Skills = make([]DraftSkill, 0, len(d.Skills))
	seen := map[string]bool{}
	for _, s := range d.Skills {
		sk, ok := m.idx.ByCode(s.Code)
		if !ok || seen[s.Code] {
			continue
		}
		seen[s.Code] = true
		c := s.Confidence
		if c <= 0 || c > 1 {
			c = 0.8
		}
		out.Skills = append(out.Skills, DraftSkill{Code: sk.Code, Name: sk.Name, Confidence: c})
	}
	sort.Slice(out.Skills, func(i, j int) bool {
		if out.Skills[i].Confidence != out.Skills[j].Confidence {
			return out.Skills[i].Confidence > out.Skills[j].Confidence
		}
		return out.Skills[i].Code < out.Skills[j].Code
	})
	if !refdata.IsSelectableSpecialization(out.PrimarySpecialization) {
		out.PrimarySpecialization = ""
	}
	if out.ExperienceYears < 0 || out.ExperienceYears > 50 {
		out.ExperienceYears = 0
	}
	for i := range out.Experiences {
		out.Experiences[i].StartedOn = normDate(out.Experiences[i].StartedOn)
		out.Experiences[i].EndedOn = normDate(out.Experiences[i].EndedOn)
		out.Experiences[i].Company = clip(out.Experiences[i].Company, 200)
		out.Experiences[i].Position = clip(out.Experiences[i].Position, 200)
		out.Experiences[i].Description = clip(out.Experiences[i].Description, 2000)
	}
	out.FirstName, out.LastName, out.MiddleName = clip(out.FirstName, 80), clip(out.LastName, 80), clip(out.MiddleName, 80)
	out.Headline, out.About, out.City = clip(out.Headline, 160), clip(out.About, 4000), clip(out.City, 80)
	if out.Experiences == nil {
		out.Experiences = []DraftExperience{}
	}
	if out.Education == nil {
		out.Education = []DraftEducation{}
	}
	return out
}

func (m *Mock) sanitizeNeed(d NeedDraft) NeedDraft {
	out := d
	out.MustSkills = m.knownCodes(d.MustSkills)
	out.NiceSkills = m.knownCodes(d.NiceSkills)
	if !refdata.IsSelectableSpecialization(out.Specialization) {
		out.Specialization = ""
	}
	if !refdata.IsGrade(out.GradeMin) {
		out.GradeMin = ""
	}
	if !refdata.IsGrade(out.GradeMax) {
		out.GradeMax = ""
	}
	formats := out.WorkFormats[:0]
	for _, f := range out.WorkFormats {
		if refdata.IsWorkFormat(f) {
			formats = append(formats, f)
		}
	}
	out.WorkFormats = formats
	out.Title = clip(out.Title, 120)
	return out
}

func (m *Mock) knownCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	seen := map[string]bool{}
	for _, c := range codes {
		if _, ok := m.idx.ByCode(c); ok && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// findSkills ищет навыки по n-граммам нормализованных токенов.
func (m *Mock) findSkills(text string) []DraftSkill {
	toks := tokens(text)
	counts := map[string]int{}
	order := []string{}
	for i := 0; i < len(toks); {
		matched := 0
		for n := m.maxN; n >= 1; n-- {
			if i+n > len(toks) {
				continue
			}
			gram := strings.Join(toks[i:i+n], " ")
			if s, ok := m.idx.Lookup(gram); ok {
				if counts[s.Code] == 0 {
					order = append(order, s.Code)
				}
				counts[s.Code]++
				matched = n
				break
			}
		}
		if matched == 0 {
			matched = 1
		}
		i += matched
	}
	out := make([]DraftSkill, 0, len(order))
	for _, code := range order {
		s, _ := m.idx.ByCode(code)
		conf := 0.75
		if counts[code] > 1 {
			conf = 0.9
		}
		out = append(out, DraftSkill{Code: code, Name: s.Name, Confidence: conf})
	}
	return out
}

func (m *Mock) guessSpecialization(headline string, skills []DraftSkill) string {
	score := map[string]float64{}
	for _, kw := range specKeywords {
		if kw.re.MatchString(headline) {
			score[kw.spec] += 3
		}
	}
	for _, s := range skills {
		sk, ok := m.idx.ByCode(s.Code)
		if !ok {
			continue
		}
		for _, sp := range sk.Specs {
			if sp == "common" {
				continue
			}
			score[sp] += 1.0 / float64(len(sk.Specs))
		}
	}
	best, bestScore := "", 0.0
	for _, spec := range refdata.Specializations {
		if !spec.Selectable {
			continue
		}
		if v := score[spec.Code]; v > bestScore {
			best, bestScore = spec.Code, v
		}
	}
	return best
}

// detectCity ищет город по основе слова, чтобы находить косвенные падежи («в Москве», «из Казани»).
func (m *Mock) detectCity(lines []string) string {
	for _, l := range lines {
		ll := []rune(strings.ToLower(strings.ReplaceAll(l, "ё", "е")))
		for _, c := range m.cities {
			stem := []rune(cityStem(c))
			for idx := indexRunes(ll, stem, 0); idx >= 0; idx = indexRunes(ll, stem, idx+1) {
				before := idx == 0 || !unicode.IsLetter(ll[idx-1])
				// после основы допускаем до 2 букв окончания, затем не-буква или конец строки
				end := idx + len(stem)
				tail := 0
				for end+tail < len(ll) && unicode.IsLetter(ll[end+tail]) {
					tail++
				}
				if before && tail <= 2 {
					return c
				}
			}
		}
	}
	return ""
}

func cityStem(city string) string {
	r := []rune(strings.ToLower(strings.ReplaceAll(city, "ё", "е")))
	if len(r) > 4 {
		switch r[len(r)-1] {
		case 'а', 'ь', 'ы', 'я':
			return string(r[:len(r)-1])
		}
	}
	return string(r)
}

func indexRunes(s, sub []rune, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func (m *Mock) parseExperiences(lines []string) []DraftExperience {
	var out []DraftExperience
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		mr := reRange.FindStringSubmatch(l)
		if mr == nil {
			continue
		}
		exp := DraftExperience{StartedOn: parseDateStart(mr[1]), EndedOn: parseDateEnd(mr[2])}
		rest := strings.TrimSpace(reRange.ReplaceAllString(l, ""))
		rest = strings.Trim(rest, " ,;:()[]—–-")
		j := i + 1
		if rest != "" {
			// «Должность — Компания» на одной строке с датами.
			parts := regexp.MustCompile(`\s+(?:—|–|-|в|at|@)\s+|,\s+`).Split(rest, 2)
			exp.Position = strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				exp.Company = strings.TrimSpace(parts[1])
			}
		} else {
			// Формат hh.ru: даты, длительность, компания, должность.
			var picked []string
			for j < len(lines) && len(picked) < 2 {
				cand := strings.TrimSpace(lines[j])
				if cand == "" || reDuration.MatchString(cand) {
					j++
					continue
				}
				if reRange.MatchString(cand) {
					break
				}
				picked = append(picked, cand)
				j++
			}
			if len(picked) > 0 {
				exp.Company = picked[0]
			}
			if len(picked) > 1 {
				exp.Position = picked[1]
			}
		}
		var desc []string
		for ; j < len(lines); j++ {
			if reRange.MatchString(lines[j]) {
				break
			}
			desc = append(desc, strings.TrimSpace(lines[j]))
		}
		exp.Description = clip(strings.Join(desc, " "), 1200)
		for _, s := range m.findSkills(exp.Description) {
			exp.Stack = append(exp.Stack, s.Code)
		}
		if exp.Company == "" && exp.Position == "" && exp.Description == "" {
			continue
		}
		out = append(out, exp)
		i = j - 1
	}
	return out
}

func parseEducation(lines []string) []DraftEducation {
	eduKw := regexp.MustCompile(`(?i)(университет|институт|академия|колледж|техникум|school|university|college|мгу|мфти|вшэ|мгту|спбгу|итмо|мирэа|мисис|рудн|бауман|политех|мэи|мади|ргу|урфу|нгу|кфу|тгу|юфу|сфу)`)
	degreeKw := regexp.MustCompile(`(?i)(бакалавр|магистр|специалист|bachelor|master|phd|кандидат|аспирант|высшее|среднее)`)
	var out []DraftEducation
	for i, l := range lines {
		if !eduKw.MatchString(l) {
			continue
		}
		e := DraftEducation{Institution: clip(strings.Trim(reYear.ReplaceAllString(l, ""), " ,;:—–-"), 200)}
		if y := reYear.FindString(l); y != "" {
			e.GraduatedYear, _ = strconv.Atoi(y)
		} else if i > 0 {
			if y := reYear.FindString(lines[i-1]); y != "" {
				e.GraduatedYear, _ = strconv.Atoi(y)
			}
		}
		window := l
		if i > 0 {
			window = lines[i-1] + " " + window
		}
		if i+1 < len(lines) {
			window += " " + lines[i+1]
		}
		if dg := degreeKw.FindString(window); dg != "" {
			e.Degree = strings.ToLower(dg)
		}
		if parts := strings.SplitN(e.Institution, ",", 2); len(parts) == 2 {
			e.Institution, e.Field = strings.TrimSpace(parts[0]), clip(strings.TrimSpace(parts[1]), 200)
		}
		out = append(out, e)
	}
	return out
}

// --- вспомогательные функции ---

func splitLines(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func splitSections(lines []string) map[section][]string {
	out := map[section][]string{}
	cur := secNone
	for _, l := range lines {
		if len([]rune(l)) <= 45 {
			matched := false
			for _, h := range sectionHeaders {
				if h.re.MatchString(l) {
					cur = h.sec
					matched = true
					break
				}
			}
			if matched {
				continue
			}
		}
		if cur != secNone {
			out[cur] = append(out[cur], l)
		}
	}
	return out
}

func detectName(head []string) (first, last, middle string) {
	for _, l := range head {
		if nameStop.MatchString(l) || strings.ContainsAny(l, "@+0123456789:/") {
			continue
		}
		words := strings.Fields(l)
		if len(words) < 2 || len(words) > 3 || len([]rune(l)) > 60 {
			continue
		}
		ok := true
		for _, w := range words {
			r := []rune(w)
			if !unicode.IsUpper(r[0]) {
				ok = false
				break
			}
			for _, c := range r {
				if !unicode.IsLetter(c) && c != '-' {
					ok = false
					break
				}
			}
		}
		if !ok {
			continue
		}
		if len(words) == 3 && isPatronymic(words[2]) {
			return words[1], words[0], words[2]
		}
		if len(words) == 3 && isPatronymic(words[1]) {
			return words[0], words[2], words[1]
		}
		if len(words) == 3 {
			return words[1], words[0], words[2]
		}
		if looksLikeSurname(words[0]) && !looksLikeSurname(words[1]) {
			return words[1], words[0], ""
		}
		return words[0], words[1], ""
	}
	return "", "", ""
}

func isPatronymic(w string) bool {
	lw := strings.ToLower(w)
	for _, suf := range []string{"вич", "вна", "ична", "инична", "ович", "евич", "овна", "евна"} {
		if strings.HasSuffix(lw, suf) {
			return true
		}
	}
	return false
}

func looksLikeSurname(w string) bool {
	lw := strings.ToLower(w)
	for _, suf := range []string{"ов", "ев", "ёв", "ин", "ын", "ский", "цкий", "ова", "ева", "ёва", "ина", "ына", "ская", "цкая", "ых", "их", "енко", "ук", "юк", "дзе", "ян"} {
		if strings.HasSuffix(lw, suf) && len([]rune(lw)) > len([]rune(suf))+1 {
			return true
		}
	}
	return false
}

func tokens(text string) []string {
	norm := refdata.NormalizeTerm(text)
	raw := strings.Fields(norm)
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimRight(t, ".")
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n]))
}

func toRubles(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	if n < 1000 {
		return n * 1000
	}
	return n
}

// parseDateStart приводит начало диапазона к YYYY-MM-DD (день — первый).
func parseDateStart(s string) string {
	y, mo := parseYM(s)
	if y == 0 {
		return ""
	}
	if mo == 0 {
		mo = 1
	}
	return strconvDate(y, mo)
}

// parseDateEnd: «настоящее время» — пустая строка; год без месяца — декабрь.
func parseDateEnd(s string) string {
	y, mo := parseYM(s)
	if y == 0 {
		return ""
	}
	if mo == 0 {
		mo = 12
	}
	return strconvDate(y, mo)
}

func parseYM(s string) (int, int) {
	s = strings.TrimSpace(strings.ToLower(s))
	mr := reDate.FindStringSubmatch(s)
	if mr == nil {
		return 0, 0
	}
	switch {
	case mr[1] != "":
		m, _ := strconv.Atoi(mr[1])
		y, _ := strconv.Atoi(mr[2])
		return y, m
	case mr[3] != "":
		y, _ := strconv.Atoi(mr[4])
		mon := strings.TrimRight(mr[3], ".")
		for k, v := range monthIndex {
			if strings.HasPrefix(mon, k) {
				return y, v
			}
		}
		return y, 1
	default:
		y, _ := strconv.Atoi(mr[5])
		return y, 0
	}
}

func strconvDate(y, m int) string {
	return strconv.Itoa(y) + "-" + pad2(m) + "-01"
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func normDate(s string) string {
	if s == "" {
		return ""
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s
	}
	if d := parseDateStart(s); d != "" {
		return d
	}
	return ""
}

// totalYears суммирует интервалы занятости с объединением пересечений.
func totalYears(exps []DraftExperience) float64 {
	type iv struct{ a, b time.Time }
	var ivs []iv
	now := time.Now()
	for _, e := range exps {
		a, err := time.Parse("2006-01-02", e.StartedOn)
		if err != nil {
			continue
		}
		b := now
		if e.EndedOn != "" {
			if t, err := time.Parse("2006-01-02", e.EndedOn); err == nil {
				b = t
			}
		}
		if b.Before(a) {
			continue
		}
		ivs = append(ivs, iv{a, b})
	}
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].a.Before(ivs[j].a) })
	var total time.Duration
	for i := 0; i < len(ivs); i++ {
		cur := ivs[i]
		for i+1 < len(ivs) && !ivs[i+1].a.After(cur.b) {
			i++
			if ivs[i].b.After(cur.b) {
				cur.b = ivs[i].b
			}
		}
		total += cur.b.Sub(cur.a)
	}
	years := total.Hours() / 24 / 365.25
	return float64(int(years*2+0.5)) / 2 // шаг 0.5 года
}

var knownCities = []string{
	"Москва", "Санкт-Петербург", "Новосибирск", "Екатеринбург", "Казань", "Нижний Новгород", "Челябинск", "Самара",
	"Омск", "Ростов-на-Дону", "Уфа", "Красноярск", "Воронеж", "Пермь", "Волгоград", "Краснодар", "Саратов", "Тюмень",
	"Тольятти", "Ижевск", "Барнаул", "Ульяновск", "Иркутск", "Хабаровск", "Ярославль", "Владивосток", "Махачкала",
	"Томск", "Оренбург", "Кемерово", "Новокузнецк", "Рязань", "Астрахань", "Набережные Челны", "Пенза", "Киров",
	"Липецк", "Чебоксары", "Калининград", "Тула", "Курск", "Ставрополь", "Сочи", "Белгород", "Иваново", "Брянск",
	"Сургут", "Владимир", "Архангельск", "Калуга", "Смоленск", "Якутск", "Мурманск", "Севастополь", "Симферополь",
	"Минск", "Алматы", "Астана", "Ташкент", "Тбилиси", "Ереван", "Бишкек",
}
