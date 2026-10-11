// Package refdata — справочники домена: специализации, грейды, темы заданий,
// дисциплины ФСП, причины отказа, форматы работы и матрица релевантности.
//
// Справочники заданы кодом, а не таблицами: они меняются вместе с логикой
// (темы тестов, веса), а не данными. Навыки — исключение, см. skills.go.
package refdata

import "sort"

// Specialization — направление деятельности кандидата.
type Specialization struct {
	Code       string
	Name       string
	Short      string
	Selectable bool // common выбрать нельзя: это общий блок заданий
}

// Specializations — упорядоченный список специализаций.
var Specializations = []Specialization{
	{Code: "backend", Name: "Backend-разработка", Short: "Backend", Selectable: true},
	{Code: "frontend", Name: "Frontend-разработка", Short: "Frontend", Selectable: true},
	{Code: "data_ml", Name: "Data Science / ML", Short: "Data/ML", Selectable: true},
	{Code: "devops", Name: "DevOps / SRE", Short: "DevOps", Selectable: true},
	{Code: "qa", Name: "QA / автоматизация тестирования", Short: "QA", Selectable: true},
	{Code: "security", Name: "Информационная безопасность", Short: "ИБ", Selectable: true},
	{Code: "common", Name: "Общий блок", Short: "Общий", Selectable: false},
}

// Grade — уровень навыков. Order задаёт строгий порядок сравнения.
type Grade struct {
	Code  string
	Name  string
	Order int
	// PriorTheta — стартовая точка адаптивного теста для заявленного грейда.
	PriorTheta float64
	// LowerTheta — нижняя граница полосы грейда на шкале θ (−∞ для junior).
	LowerTheta float64
	// MinConfirmTheta — минимальный уровень для подтверждения грейда, если он выше границы полосы.
	// Нужен нижнему грейду: полоса Junior не ограничена снизу, но подтверждать Junior тому, кто
	// почти ничего не решает (в том числе угадывающему), нельзя. 0 — без отдельного порога.
	MinConfirmTheta float64
}

// Grades — упорядоченный список грейдов.
var Grades = []Grade{
	{Code: "junior", Name: "Junior", Order: 1, PriorTheta: -0.8, LowerTheta: -4.0, MinConfirmTheta: -1.5},
	{Code: "middle", Name: "Middle", Order: 2, PriorTheta: 0.1, LowerTheta: -0.5},
	{Code: "senior", Name: "Senior", Order: 3, PriorTheta: 0.9, LowerTheta: 0.7},
}

// Topic — тема заданий внутри специализации.
type Topic struct {
	Code string
	Name string
}

// Topics — темы по специализации. Общий блок common доступен всем специализациям.
var Topics = map[string][]Topic{
	"common": {
		{"complexity", "Алгоритмическая сложность"}, {"data_structures", "Структуры данных"},
		{"sql", "SQL"}, {"git", "Git"}, {"http", "HTTP"}, {"os_processes", "ОС и процессы"},
		{"networking", "Сети"},
	},
	"backend": {
		{"sql", "SQL и запросы"}, {"transactions", "Транзакции и изоляция"}, {"http_rest", "HTTP и REST"},
		{"concurrency", "Конкурентность"}, {"caching", "Кэширование"}, {"auth_security", "Аутентификация и безопасность"},
		{"testing", "Тестирование"}, {"system_design", "Проектирование систем"}, {"errors_logging", "Ошибки и логирование"},
	},
	"frontend": {
		{"js_core", "JavaScript"}, {"ts_types", "TypeScript и типы"}, {"react_state", "React и состояние"},
		{"rendering_perf", "Рендеринг и производительность"}, {"css_layout", "CSS и раскладка"},
		{"browser_api", "Браузерные API"}, {"accessibility", "Доступность"}, {"bundling", "Сборка"},
	},
	"data_ml": {
		{"python_core", "Python"}, {"pandas_sql", "pandas и SQL"}, {"statistics", "Статистика"},
		{"metrics_validation", "Метрики и валидация"}, {"feature_engineering", "Признаки"},
		{"models", "Модели"}, {"experiment_design", "Дизайн экспериментов"},
	},
	"devops": {
		{"linux_shell", "Linux и shell"}, {"docker", "Docker"}, {"kubernetes", "Kubernetes"},
		{"ci_cd", "CI/CD"}, {"observability", "Наблюдаемость"}, {"networking", "Сети"},
		{"iac", "Инфраструктура как код"}, {"incident_response", "Инциденты"},
	},
	"qa": {
		{"test_design", "Тест-дизайн"}, {"automation_api", "Автоматизация API"}, {"automation_ui", "Автоматизация UI"},
		{"test_data", "Тестовые данные"}, {"ci_integration", "Интеграция в CI"}, {"bug_analysis", "Анализ дефектов"},
	},
	"security": {
		{"web_vulns", "Веб-уязвимости"}, {"crypto_basics", "Основы криптографии"}, {"auth_protocols", "Протоколы аутентификации"},
		{"network_security", "Сетевая безопасность"}, {"secure_code_review", "Безопасный код"}, {"incident_forensics", "Расследование инцидентов"},
	},
}

// WorkFormat — формат работы.
type WorkFormat struct {
	Code string
	Name string
}

// WorkFormats — список форматов.
var WorkFormats = []WorkFormat{
	{"office", "Офис"}, {"hybrid", "Гибрид"}, {"remote", "Удалённо"},
}

// Discipline — дисциплина спортивного программирования ФСП.
type Discipline struct {
	Code string
	Name string
}

// Disciplines — пять дисциплин ФСП.
var Disciplines = []Discipline{
	{"algorithmic", "Алгоритмическое программирование"},
	{"product", "Продуктовое программирование"},
	{"security", "Программирование систем информационной безопасности"},
	{"robotics", "Программирование робототехники"},
	{"uav", "Программирование беспилотных авиационных систем"},
}

// Level — уровень соревнования и его вес в бусте.
type Level struct {
	Code   string
	Name   string
	Weight float64
}

// Levels — уровни соревнований ФСП.
var Levels = []Level{
	{"regional", "Региональный", 0.3},
	{"national", "Всероссийский", 0.6},
	{"international", "Международный", 1.0},
}

// ResultWeight возвращает вес результата: призовое место, финалист/призёр, участие.
func ResultWeight(kind string, place int) float64 {
	switch kind {
	case "place":
		if place >= 1 && place <= 3 {
			return 1.0
		}
		if place > 3 && place <= 10 {
			return 0.6
		}
		return 0.3
	case "rank":
		return 1.0 // разряд или звание присваиваются только за результат
	case "rating":
		return 0.6
	default:
		return 0.3
	}
}

// DeclineReason — причина отказа от приглашения.
type DeclineReason struct {
	Code string
	Name string
}

// DeclineReasons — справочник причин отказа.
var DeclineReasons = []DeclineReason{
	{"salary_below", "Вилка ниже ожиданий"},
	{"not_looking", "Сейчас не ищу работу"},
	{"role_mismatch", "Не моя роль или стек"},
	{"format_mismatch", "Не подходит формат или локация"},
	{"company", "Не подходит компания"},
	{"other", "Другое"},
}

// relevance — матрица «дисциплина ФСП → специализация».
var relevance = map[string]map[string]float64{
	"algorithmic": {"backend": 1.0, "frontend": 0.7, "data_ml": 0.9, "devops": 0.5, "qa": 0.6, "security": 0.6},
	"product":     {"backend": 0.9, "frontend": 1.0, "data_ml": 0.6, "devops": 0.7, "qa": 0.7, "security": 0.5},
	"security":    {"backend": 0.6, "frontend": 0.3, "data_ml": 0.3, "devops": 0.7, "qa": 0.5, "security": 1.0},
	"robotics":    {"backend": 0.5, "frontend": 0.2, "data_ml": 0.5, "devops": 0.4, "qa": 0.3, "security": 0.2},
	"uav":         {"backend": 0.4, "frontend": 0.2, "data_ml": 0.6, "devops": 0.4, "qa": 0.3, "security": 0.2},
}

// MinRelevance — минимальная релевантность: участие в соревнованиях — сигнал мотивации.
const MinRelevance = 0.2

// Relevance возвращает релевантность дисциплины специализации (не ниже MinRelevance).
func Relevance(discipline, spec string) float64 {
	if row, ok := relevance[discipline]; ok {
		if v, ok := row[spec]; ok && v > MinRelevance {
			return v
		}
	}
	return MinRelevance
}

// IsSpecialization сообщает, существует ли код специализации (включая common).
func IsSpecialization(code string) bool {
	_, ok := specializationByCode[code]
	return ok
}

// IsSelectableSpecialization — можно ли выбрать специализацию при регистрации.
func IsSelectableSpecialization(code string) bool {
	s, ok := specializationByCode[code]
	return ok && s.Selectable
}

// SpecializationByCode возвращает специализацию по коду.
func SpecializationByCode(code string) (Specialization, bool) {
	s, ok := specializationByCode[code]
	return s, ok
}

// IsGrade сообщает, существует ли код грейда.
func IsGrade(code string) bool {
	_, ok := gradeByCode[code]
	return ok
}

// GradeByCode возвращает грейд по коду.
func GradeByCode(code string) (Grade, bool) {
	g, ok := gradeByCode[code]
	return g, ok
}

// GradeOrder возвращает порядковый номер грейда (0 для неизвестного).
func GradeOrder(code string) int {
	return gradeByCode[code].Order
}

// NextGrade возвращает следующий грейд, если он есть.
func NextGrade(code string) (Grade, bool) {
	g, ok := gradeByCode[code]
	if !ok {
		return Grade{}, false
	}
	for _, c := range Grades {
		if c.Order == g.Order+1 {
			return c, true
		}
	}
	return Grade{}, false
}

// PrevGrade возвращает предыдущий грейд, если он есть.
func PrevGrade(code string) (Grade, bool) {
	g, ok := gradeByCode[code]
	if !ok {
		return Grade{}, false
	}
	for _, c := range Grades {
		if c.Order == g.Order-1 {
			return c, true
		}
	}
	return Grade{}, false
}

// IsTopic сообщает, существует ли тема в специализации или в общем блоке.
func IsTopic(spec, topic string) bool {
	for _, t := range Topics[spec] {
		if t.Code == topic {
			return true
		}
	}
	for _, t := range Topics["common"] {
		if t.Code == topic {
			return true
		}
	}
	return false
}

// AllTopicCodes возвращает отсортированный список всех кодов тем.
func AllTopicCodes() []string {
	seen := map[string]struct{}{}
	for _, ts := range Topics {
		for _, t := range ts {
			seen[t.Code] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// IsDiscipline, IsLevel, IsDeclineReason, IsWorkFormat — проверки кодов справочников.
func IsDiscipline(code string) bool {
	return indexOf(code, Disciplines, func(d Discipline) string { return d.Code })
}
func IsLevel(code string) bool { return indexOf(code, Levels, func(l Level) string { return l.Code }) }
func IsDeclineReason(code string) bool {
	return indexOf(code, DeclineReasons, func(r DeclineReason) string { return r.Code })
}
func IsWorkFormat(code string) bool {
	return indexOf(code, WorkFormats, func(w WorkFormat) string { return w.Code })
}

// LevelWeight возвращает вес уровня соревнования (0 для неизвестного).
func LevelWeight(code string) float64 {
	for _, l := range Levels {
		if l.Code == code {
			return l.Weight
		}
	}
	return 0
}

func indexOf[T any](code string, xs []T, key func(T) string) bool {
	for _, x := range xs {
		if key(x) == code {
			return true
		}
	}
	return false
}

var (
	specializationByCode = func() map[string]Specialization {
		m := make(map[string]Specialization, len(Specializations))
		for _, s := range Specializations {
			m[s.Code] = s
		}
		return m
	}()
	gradeByCode = func() map[string]Grade {
		m := make(map[string]Grade, len(Grades))
		for _, g := range Grades {
			m[g.Code] = g
		}
		return m
	}()
)

// DeclineReasonName — название причины отказа по коду (неизвестный код возвращается как есть).
func DeclineReasonName(code string) string {
	for _, r := range DeclineReasons {
		if r.Code == code {
			return r.Name
		}
	}
	return code
}
