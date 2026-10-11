// Package qa — семейства заданий для специализации QA / автоматизация тестирования.
package qa

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(boundaryValues{})
	itembank.Register(pairwiseGaps{})
	itembank.Register(triageSeverity{})
	itembank.Register(bisectCulprit{})
}

// boundaryValues — какие значения вокруг границ валидатор примет или отклонит
// при разных формулировках границ (включительно / строго).
type boundaryValues struct{}

func (boundaryValues) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.test_design.boundary-values.v1", Spec: itembank.SpecQA, Topic: "test_design",
		Grade: itembank.GradeJunior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.4, B: -0.8, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"bva", "validation"},
	}
}

type bvField struct {
	name                       string
	loMin, loMax, hiMin, hiMax int
}

var bvFields = []bvField{
	{"возраст пользователя (лет)", 14, 21, 60, 75},
	{"количество единиц товара в заказе", 1, 5, 20, 99},
	{"длина пароля (символов)", 6, 12, 24, 64},
	{"размер скидки (%)", 0, 5, 30, 70},
	{"вес посылки (кг)", 1, 3, 15, 40},
	{"размер вложения (МБ)", 1, 2, 10, 50},
	{"число участников встречи", 2, 3, 8, 50},
	{"срок аренды (дней)", 1, 3, 30, 90},
}

type bvRule struct {
	loIncl, hiIncl bool
	texts          []string
}

var bvRules = []bvRule{
	{true, true, []string{"от %d до %d включительно", "не менее %d и не более %d", "в диапазоне [%d; %d]"}},
	{false, false, []string{"строго больше %d и строго меньше %d", "в открытом интервале (%d; %d)"}},
	{true, false, []string{"не менее %d и меньше %d", "от %d включительно до %d не включая", "в полуинтервале [%d; %d)"}},
	{false, true, []string{"больше %d и не более %d", "от %d не включая до %d включительно", "в полуинтервале (%d; %d]"}},
}

func inclText(incl bool) string {
	if incl {
		return "входит в допустимый диапазон"
	}
	return "не входит в допустимый диапазон"
}

func (f boundaryValues) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A01)
	fld := itembank.Pick(r, bvFields)
	lo := itembank.IntBetween(r, fld.loMin, fld.loMax)
	hi := itembank.IntBetween(r, fld.hiMin, fld.hiMax)
	rule := itembank.Pick(r, bvRules)
	ruleText := fmt.Sprintf(itembank.Pick(r, rule.texts), lo, hi)
	accept := func(v int) bool {
		okLo := v > lo
		if rule.loIncl {
			okLo = v >= lo
		}
		okHi := v < hi
		if rule.hiIncl {
			okHi = v <= hi
		}
		return okLo && okHi
	}
	candidates := []int{lo - 1, lo, lo + 1, hi - 1, hi, hi + 1}
	var accepted, rejected, verdicts []string
	for _, v := range candidates {
		s := strconv.Itoa(v)
		if accept(v) {
			accepted = append(accepted, s)
			verdicts = append(verdicts, s+" — принять")
		} else {
			rejected = append(rejected, s)
			verdicts = append(verdicts, s+" — отклонить")
		}
	}
	// Вопрос ставится так, чтобы верных вариантов было 2–3 из 6.
	askAccepted := len(accepted) >= 2 && len(accepted) <= 3
	if askAccepted && len(rejected) >= 2 && len(rejected) <= 3 {
		askAccepted = r.IntN(2) == 0
	}
	correct, rest := rejected, accepted
	verbs := [3]string{"отклонить", "отклонит", "должны быть отклонены"}
	if askAccepted {
		correct, rest = accepted, rejected
		verbs = [3]string{"принять", "примет", "должны быть приняты"}
	}
	options, keys := itembank.ShuffledMulti(r, correct, rest...)
	stems := []string{
		fmt.Sprintf("Поле «%s» валидно, если значение %s. Тестировщик подобрал значения вокруг границ. Отметьте все значения из списка, которые валидатор должен %s.", fld.name, ruleText, verbs[0]),
		fmt.Sprintf("По требованию ТЗ %s — %s. Какие из перечисленных тестовых значений валидатор %s? Выберите все подходящие.", fld.name, ruleText, verbs[1]),
		fmt.Sprintf("Правило валидации: %s — %s. Ниже перечислены кандидаты в граничные значения. Выберите все, которые %s.", fld.name, ruleText, verbs[2]),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: fmt.Sprintf("Нижняя граница %d %s, верхняя граница %d %s. Проверка каждого значения: %s. "+
			"Типичная ошибка — считать границу всегда допустимой (или всегда недопустимой), не различая формулировки "+
			"«включительно»/«не менее» и «строго больше»/«не включая».",
			lo, inclText(rule.loIncl), hi, inclText(rule.hiIncl), strings.Join(verdicts, ", ")),
	}, nil
}

// pairwiseGaps — сколько пар значений параметров не покрыто (или покрыто) набором тест-кейсов.
type pairwiseGaps struct{}

func (pairwiseGaps) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.test_design.pairwise-gaps.v1", Spec: itembank.SpecQA, Topic: "test_design",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.2, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"pairwise", "combinatorial"},
	}
}

type pwParam struct {
	name string
	vals []string
}

var pwParams = []pwParam{
	{"Браузер", []string{"Chrome", "Firefox", "Safari"}},
	{"ОС", []string{"Windows", "macOS", "Linux"}},
	{"Язык", []string{"ru", "en", "de"}},
	{"Роль", []string{"admin", "manager", "guest"}},
	{"Оплата", []string{"карта", "СБП", "баланс"}},
	{"Устройство", []string{"desktop", "tablet", "mobile"}},
	{"Тариф", []string{"free", "pro", "team"}},
	{"Валюта", []string{"RUB", "USD", "EUR"}},
}

var pwFeatures = []string{"оформления заказа", "экрана входа", "страницы оплаты", "экспорта отчёта", "настроек уведомлений", "поиска по каталогу"}

func (f pairwiseGaps) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A02)
	feature := itembank.Pick(r, pwFeatures)
	params := itembank.PickN(r, pwParams, 3)
	counts := itembank.Shuffle(r, []int{3, itembank.IntBetween(r, 2, 3), itembank.IntBetween(r, 2, 3)})
	vals := make([][]string, 3)
	for i, p := range params {
		vals[i] = itembank.PickN(r, p.vals, counts[i])
	}
	n := itembank.IntBetween(r, 4, 6)
	var rows [][3]int
	var uncovered []string
	total := 0
	// Набор строк перегенерируется, если он случайно покрыл все пары: тогда вопрос вырожден.
	for attempt := 0; attempt < 6; attempt++ {
		rows = rows[:0]
		seen := map[[3]int]bool{}
		for len(rows) < n {
			row := [3]int{r.IntN(len(vals[0])), r.IntN(len(vals[1])), r.IntN(len(vals[2]))}
			if seen[row] {
				continue
			}
			seen[row] = true
			rows = append(rows, row)
		}
		total = 0
		uncovered = uncovered[:0]
		for p1 := 0; p1 < 3; p1++ {
			for p2 := p1 + 1; p2 < 3; p2++ {
				for a := range vals[p1] {
					for b := range vals[p2] {
						total++
						found := false
						for _, row := range rows {
							if row[p1] == a && row[p2] == b {
								found = true
								break
							}
						}
						if !found {
							uncovered = append(uncovered, fmt.Sprintf("%s=%s / %s=%s", params[p1].name, vals[p1][a], params[p2].name, vals[p2][b]))
						}
					}
				}
			}
		}
		if len(uncovered) > 0 {
			break
		}
	}
	covered := total - len(uncovered)
	askUncovered := r.IntN(3) != 0
	answer := len(uncovered)
	if !askUncovered {
		answer = covered
	}
	var paramParts []string
	for i, p := range params {
		paramParts = append(paramParts, fmt.Sprintf("%s (%s)", p.name, strings.Join(vals[i], ", ")))
	}
	var table strings.Builder
	fmt.Fprintf(&table, "| № | %s | %s | %s |\n|---|---|---|---|\n", params[0].name, params[1].name, params[2].name)
	for i, row := range rows {
		fmt.Fprintf(&table, "| %d | %s | %s | %s |\n", i+1, vals[0][row[0]], vals[1][row[1]], vals[2][row[2]])
	}
	verbs := [3]string{"ещё не покрыто", "непокрытых", "ещё не покрывает"}
	if !askUncovered {
		verbs = [3]string{"уже покрыто", "покрытых", "уже покрывает"}
	}
	stems := []string{
		fmt.Sprintf("Для проверки %s выбраны параметры: %s. Команда написала тест-кейсы из таблицы ниже. Сколько пар «значение одного параметра — значение другого» (по всем трём парам параметров) %s этим набором?\n\n%s", feature, strings.Join(paramParts, "; "), verbs[0], table.String()),
		fmt.Sprintf("Тест-аналитик оценивает полноту pairwise-покрытия %s. Параметры и их значения: %s. Текущий набор тест-кейсов — в таблице. Укажите число %s пар значений.\n\n%s", feature, strings.Join(paramParts, "; "), verbs[1], table.String()),
		fmt.Sprintf("Техника pairwise требует, чтобы каждая пара значений любых двух параметров встретилась хотя бы в одном тест-кейсе. Параметры %s: %s. Сколько пар значений %s набор из таблицы?\n\n%s", feature, strings.Join(paramParts, "; "), verbs[2], table.String()),
	}
	a, b, c := len(vals[0]), len(vals[1]), len(vals[2])
	uncoveredText := "непокрытых пар нет"
	if len(uncovered) > 0 {
		uncoveredText = "непокрытые пары: " + strings.Join(uncovered, "; ")
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Всего пар: %d·%d + %d·%d + %d·%d = %d. Каждый тест-кейс покрывает ровно три пары, поэтому %d кейсов покрывают не больше %d пар; "+
			"здесь покрыто %d (повторы одной пары не считаются), %s. Типичная ошибка — считать полные комбинации (%d·%d·%d = %d) "+
			"или учитывать только пары с одним «главным» параметром.",
			a, b, a, c, b, c, total, n, 3*n, covered, uncoveredText, a, b, c, a*b*c),
	}, nil
}

// triageSeverity — сколько дефектов получат заданный уровень серьёзности по матрице триажа.
type triageSeverity struct{}

func (triageSeverity) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.bug_analysis.triage-severity.v1", Spec: itembank.SpecQA, Topic: "bug_analysis",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: -0.7, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"triage", "severity"},
	}
}

type triDefect struct {
	text       string
	impact     int // 0 — основной сценарий, 1 — второстепенная функция, 2 — косметика
	workaround bool
}

var triDefects = []triDefect{
	{"Регистрация нового пользователя завершается ошибкой 500 у всех; другого способа создать аккаунт нет", 0, false},
	{"Кнопка «Оплатить» не реагирует на нажатие ни в одном браузере; оплатить заказ невозможно", 0, false},
	{"Поиск по каталогу возвращает пустой список по любому запросу; навигация по категориям отключена", 0, false},
	{"Вход по логину и паролю отклоняет верные данные у всех пользователей; других способов входа нет", 0, false},
	{"Товар не добавляется в корзину ни со страницы списка, ни со страницы товара", 0, false},
	{"Кнопка «Оплатить» в корзине не реагирует; оплата из раздела «Мои заказы» проходит", 0, true},
	{"Поиск по каталогу падает с ошибкой, но товары доступны через навигацию по категориям", 0, true},
	{"Вход через соцсети не работает; вход по логину и паролю доступен", 0, true},
	{"Добавление товара в корзину со страницы списка не срабатывает; со страницы товара — работает", 0, true},
	{"Экспорт истории заказов и в PDF, и в Excel завершается ошибкой; другого способа выгрузить историю нет", 1, false},
	{"Отзывы о товаре не публикуются ни с сайта, ни из мобильного приложения", 1, false},
	{"Раздел «Избранное» не открывается ни в одном браузере", 1, false},
	{"Push-уведомления о статусе заказа не приходят; e-mail-уведомления в продукте не предусмотрены", 1, false},
	{"Экспорт истории заказов в PDF даёт пустой файл; экспорт в Excel работает", 1, true},
	{"Push-уведомления о статусе заказа не приходят; e-mail-уведомления приходят", 1, true},
	{"Фильтр по цене в избранном не применяется; сортировка по цене работает", 1, true},
	{"Смена темы оформления из профиля не сохраняется; переключатель в шапке работает", 1, true},
	{"Логотип в подвале смещён на 3 px вправо относительно макета", 2, false},
	{"Опечатка в подписи кнопки: «Оформитть заказ» (функция работает)", 2, false},
	{"Цвет ссылки в письме-подтверждении отличается от фирменного", 2, false},
	{"Иконка корзины в шапке на 2 px больше, чем в макете", 2, false},
}

func (f triageSeverity) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A03)
	defects := itembank.PickN(r, triDefects, itembank.IntBetween(r, 6, 7))
	secNo := itembank.Pick(r, []string{"S2", "S3"})
	severity := func(d triDefect) string {
		switch d.impact {
		case 2:
			return "S4"
		case 0:
			if d.workaround {
				return "S2"
			}
			return "S1"
		default:
			if d.workaround {
				return "S3"
			}
			return secNo
		}
	}
	counts := map[string]int{}
	var list, trace []string
	for i, d := range defects {
		s := severity(d)
		counts[s]++
		list = append(list, fmt.Sprintf("%d. %s", i+1, d.text))
		var why string
		switch d.impact {
		case 2:
			why = "косметика"
		case 0:
			why = "основной сценарий, " + map[bool]string{true: "обходной путь есть", false: "обходного пути нет"}[d.workaround]
		default:
			why = "второстепенная функция, " + map[bool]string{true: "обходной путь есть", false: "обходного пути нет"}[d.workaround]
		}
		trace = append(trace, fmt.Sprintf("№%d — %s → %s", i+1, why, s))
	}
	type question struct {
		text  string
		count int
	}
	candidates := []question{
		{"уровень S1", counts["S1"]},
		{"уровень S2", counts["S2"]},
		{"уровень S3", counts["S3"]},
		{"уровень S4", counts["S4"]},
		{"уровень не ниже S2 (то есть S1 или S2)", counts["S1"] + counts["S2"]},
	}
	var pool []question
	for _, q := range candidates {
		if q.count > 0 {
			pool = append(pool, q)
		}
	}
	q := itembank.Pick(r, pool)
	matrix := fmt.Sprintf("| Влияние | Обходного пути нет | Обходной путь есть |\n|---|---|---|\n"+
		"| Блокирует основной сценарий | S1 | S2 |\n| Ломает второстепенную функцию | %s | S3 |\n| Косметический дефект | S4 | S4 |", secNo)
	context := "Основные сценарии продукта: регистрация и вход, поиск товара, корзина и оплата; всё остальное — второстепенные функции."
	stems := []string{
		fmt.Sprintf("Команда классифицирует дефекты интернет-магазина по матрице серьёзности (S1 — максимальная):\n\n%s\n\n%s Дефекты из отчёта о тестировании:\n\n%s\n\nСколько дефектов получат %s?", matrix, context, strings.Join(list, "\n"), q.text),
		fmt.Sprintf("Тест-лид интернет-магазина проводит триаж по матрице ниже (S1 — самый серьёзный уровень).\n\n%s\n\n%s Найденные дефекты:\n\n%s\n\nУкажите число дефектов, которым будет присвоен %s.", matrix, context, strings.Join(list, "\n"), q.text),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: float64(q.count)},
		Explanation: fmt.Sprintf("Классификация: %s. Итого S1 — %d, S2 — %d, S3 — %d, S4 — %d. "+
			"Типичная ошибка — повышать серьёзность по «заметности» дефекта (опечатка на видном месте) или не учитывать, что обходной путь понижает уровень на один шаг.",
			strings.Join(trace, "; "), counts["S1"], counts["S2"], counts["S3"], counts["S4"]),
	}, nil
}

// bisectCulprit — какой коммит внёс регрессию по результатам проверок отдельных коммитов.
type bisectCulprit struct{}

func (bisectCulprit) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.bug_analysis.bisect-culprit.v1", Spec: itembank.SpecQA, Topic: "bug_analysis",
		Grade: itembank.GradeSenior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.5, B: 0.9, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"git", "regression"},
	}
}

var bisectMsgs = []string{
	"fix: typo in README", "feat: discount codes in cart", "refactor: extract price calculator", "chore: bump dependencies",
	"feat: new checkout button", "fix: rounding in order totals", "perf: cache product list", "test: add e2e for login",
	"feat: dark theme", "fix: null check in cart", "ci: speed up pipeline", "docs: update API guide",
	"feat: export orders to PDF", "refactor: rename OrderService", "fix: timezone in reports", "feat: phone validation",
	"feat: promo banner on main page", "fix: retry on payment timeout",
}

func (f bisectCulprit) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A04)
	n := itembank.IntBetween(r, 8, 12)
	msgs := itembank.PickN(r, bisectMsgs, n)
	hashes := make([]string, 0, n)
	seen := map[string]bool{}
	for len(hashes) < n {
		h := fmt.Sprintf("%07x", r.Uint32()&0xfffffff)
		if seen[h] {
			continue
		}
		seen[h] = true
		hashes = append(hashes, h)
	}
	tag := fmt.Sprintf("v%d.%d.0", itembank.IntBetween(r, 1, 4), itembank.IntBetween(r, 0, 9))
	culprit := itembank.IntBetween(r, 1, n-1)
	type check struct {
		idx  int
		good bool
	}
	var checks []check
	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if mid >= culprit {
			hi = mid
			checks = append(checks, check{mid, false})
		} else {
			lo = mid
			checks = append(checks, check{mid, true})
		}
	}
	checks = itembank.Shuffle(r, checks)
	var log []string
	for i := n - 1; i >= 0; i-- {
		switch i {
		case n - 1:
			log = append(log, fmt.Sprintf("%s (HEAD -> main) %s", hashes[i], msgs[i]))
		case 0:
			log = append(log, fmt.Sprintf("%s (tag: %s) %s", hashes[i], tag, msgs[i]))
		default:
			log = append(log, hashes[i]+" "+msgs[i])
		}
	}
	var results, failing []string
	for _, c := range checks {
		verdict := "тесты падают"
		if c.good {
			verdict = "тесты проходят"
		} else {
			failing = append(failing, "`"+hashes[c.idx]+"`")
		}
		results = append(results, fmt.Sprintf("- `%s` — %s", hashes[c.idx], verdict))
	}
	stems := []string{
		fmt.Sprintf("В ветке `main` e2e-тесты проходят на релизе `%s` (самый старый коммит в списке) и падают на `HEAD`. Ниже — вывод `git log --oneline` (новые коммиты сверху) и результаты уже выполненных прогонов тестов на отдельных коммитах:\n\n%s\n\nКакой коммит внёс регрессию? Введите его короткий хеш.", tag, strings.Join(results, "\n")),
		fmt.Sprintf("Инженер ищет коммит, сломавший e2e-тесты: на `%s` они зелёные, на `HEAD` — красные. История (`git log --oneline`, сверху — самые новые) — в блоке кода; результаты прогонов на отдельных коммитах:\n\n%s\n\nУкажите короткий хеш коммита, начиная с которого тесты падают.", tag, strings.Join(results, "\n")),
		fmt.Sprintf("Регрессия появилась между тегом `%s` (тесты проходят) и `HEAD` (тесты падают). Известны результаты прогонов на некоторых промежуточных коммитах:\n\n%s\n\nПо `git log --oneline` ниже (новые коммиты сверху) определите коммит-виновник и введите его короткий хеш.", tag, strings.Join(results, "\n")),
	}
	chrono := make([]string, 0, n)
	for i := 0; i < n; i++ {
		chrono = append(chrono, hashes[i])
	}
	var reasoning string
	if len(failing) == 0 {
		reasoning = fmt.Sprintf("все проверенные коммиты проходят, в том числе `%s` — непосредственный предшественник `HEAD`; значит, регрессию внёс сам `HEAD` (`%s`)", hashes[culprit-1], hashes[culprit])
	} else {
		reasoning = fmt.Sprintf("среди проверенных падают %s; самый ранний из них — `%s`, а предыдущий коммит `%s` проходит — значит, регрессию внёс `%s`",
			strings.Join(failing, ", "), hashes[culprit], hashes[culprit-1], hashes[culprit])
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "bash", Source: strings.Join(log, "\n")},
		Answer: itembank.Answer{Text: hashes[culprit]},
		Explanation: fmt.Sprintf("Хронологический порядок (от старого к новому): %s. Виновник — самый ранний падающий коммит, у которого предыдущий проходит: %s. "+
			"Типичные ошибки — назвать последний проходящий коммит или читать `git log` сверху вниз как хронологию, хотя новые коммиты в нём сверху.",
			strings.Join(chrono, " → "), reasoning),
	}, nil
}
