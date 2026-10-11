package qa

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(locatorCount{})
	itembank.Register(autowaitOutcome{})
	itembank.Register(pytestSelection{})
	itembank.Register(gitlabStagesRun{})
}

// locatorCount — что напечатает Playwright-код, считающий элементы по локаторам с фильтрами.
type locatorCount struct{}

func (locatorCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.automation_ui.locator-count.v1", Spec: itembank.SpecQA, Topic: "automation_ui",
		Grade: itembank.GradeMiddle, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.5, B: 0.0, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"playwright", "javascript", "locators"},
	}
}

type uiRow struct {
	title, status, button string
	hidden                bool
}

var (
	uiStatuses    = []string{"Оплачен", "Не оплачен", "Отменён", "В доставке", "Доставлен", "Возврат оформлен"}
	uiButtons     = []string{"", "", "Удалить", "Повторить", "Повторить заказ", "Отменить", "Отменить возврат"}
	uiTextNeedles = []string{"оплачен", "Оплачен", "отмен", "достав", "возврат", "Доставлен"}
	uiBtnNeedles  = []string{"Повторить", "повторить", "Отменить", "Удалить", "Повторить заказ", "Отменить возврат", "повторить заказ"}
)

func (f locatorCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A09)
	n := itembank.IntBetween(r, 5, 7)
	pool := make([]int, 0, 60)
	for i := 1001; i <= 1060; i++ {
		pool = append(pool, i)
	}
	nums := itembank.PickN(r, pool, n)
	sort.Ints(nums)
	rows := make([]uiRow, n)
	for i := range rows {
		rows[i] = uiRow{title: fmt.Sprintf("Заказ #%d", nums[i]), status: itembank.Pick(r, uiStatuses), button: itembank.Pick(r, uiButtons)}
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	for _, i := range itembank.PickN(r, idx, itembank.IntBetween(r, 1, 2)) {
		rows[i].hidden = true
	}
	var html strings.Builder
	html.WriteString("<ul id=\"orders\">\n")
	for _, row := range rows {
		attr := ""
		if row.hidden {
			attr = " hidden"
		}
		fmt.Fprintf(&html, "  <li data-testid=\"row\"%s><span class=\"title\">%s</span> <span class=\"status\">%s</span>", attr, row.title, row.status)
		if row.button != "" {
			fmt.Fprintf(&html, " <button>%s</button>", row.button)
		}
		html.WriteString("</li>\n")
	}
	html.WriteString("</ul>")

	rowText := func(row uiRow) string {
		t := row.title + " " + row.status
		if row.button != "" {
			t += " " + row.button
		}
		return t
	}
	hasText := func(row uiRow, needle string) bool {
		return strings.Contains(strings.ToLower(rowText(row)), strings.ToLower(needle))
	}
	btnMatch := func(row uiRow, needle string, exact bool) bool {
		if row.hidden || row.button == "" {
			return false
		}
		if exact {
			return row.button == needle
		}
		return strings.Contains(strings.ToLower(row.button), strings.ToLower(needle))
	}
	variant := r.IntN(5)
	matcher := func(needle string) func(uiRow) bool {
		switch variant {
		case 0:
			return func(row uiRow) bool { return hasText(row, needle) }
		case 1:
			return func(row uiRow) bool { return !hasText(row, needle) }
		case 3:
			return func(row uiRow) bool { return btnMatch(row, needle, true) }
		default:
			return func(row uiRow) bool { return btnMatch(row, needle, false) }
		}
	}
	// Подстрока выбирается так, чтобы выборка по возможности была непустой: ответ «0» слишком удобен для угадывания.
	needles := uiTextNeedles
	if variant >= 2 {
		needles = uiBtnNeedles
	}
	var needle string
	for _, cand := range itembank.Shuffle(r, needles) {
		needle = cand
		hits := 0
		for _, row := range rows {
			if matcher(cand)(row) {
				hits++
			}
		}
		if hits > 0 {
			break
		}
	}
	match := matcher(needle)
	var code, rule string
	switch variant {
	case 0:
		code = fmt.Sprintf("const rows = page.getByTestId('row');\nconsole.log(await rows.filter({ hasText: '%s' }).count());", needle)
		rule = "`hasText` со строкой ищет подстроку без учёта регистра по всему тексту элемента (включая текст кнопки), поэтому «оплачен» находит и «Не оплачен»; " +
			"`count()` не проверяет видимость и считает скрытые (`hidden`) элементы тоже."
	case 1:
		code = fmt.Sprintf("const rows = page.getByTestId('row');\nconsole.log(await rows.filter({ hasNotText: '%s' }).count());", needle)
		rule = "`hasNotText` отбрасывает элементы, чей текст содержит подстроку без учёта регистра (учитывается весь текст, включая кнопку); " +
			"`count()` считает и скрытые (`hidden`) элементы, видимость он не проверяет."
	case 2:
		code = fmt.Sprintf("console.log(await page.getByRole('button', { name: '%s' }).count());", needle)
		rule = "`getByRole` ищет в дереве доступности: кнопки внутри `hidden`-элементов в него не попадают. " +
			"`name` без `exact` сравнивается как подстрока без учёта регистра, поэтому «Повторить» находит и «Повторить заказ»."
	case 3:
		code = fmt.Sprintf("console.log(await page.getByRole('button', { name: '%s', exact: true }).count());", needle)
		rule = "`getByRole` ищет в дереве доступности: кнопки внутри `hidden`-элементов не учитываются. " +
			"С `exact: true` имя сравнивается целиком и с учётом регистра — «Повторить» не совпадает ни с «Повторить заказ», ни с «повторить»."
	default:
		code = fmt.Sprintf("const rows = page.getByTestId('row');\nconsole.log(await rows.filter({ has: page.getByRole('button', { name: '%s' }) }).count());", needle)
		rule = "`filter({ has })` оставляет строки, внутри которых внутренний локатор что-то находит. `getByTestId` возвращает и скрытые строки, " +
			"но `getByRole` не видит кнопки внутри `hidden`, поэтому такие строки отпадают; `name` без `exact` — подстрока без учёта регистра."
	}
	count := 0
	var trace []string
	for i, row := range rows {
		mark := "—"
		if match(row) {
			count++
			mark = "✓"
		}
		hiddenNote := ""
		if row.hidden {
			hiddenNote = ", hidden"
		}
		trace = append(trace, fmt.Sprintf("№%d «%s»%s: %s", i+1, rowText(row), hiddenNote, mark))
	}
	stems := []string{
		fmt.Sprintf("На странице такой фрагмент DOM (атрибут `hidden` делает элемент невидимым):\n\n```html\n%s\n```\n\nЧто выведет в консоль фрагмент Playwright-теста ниже?", html.String()),
		fmt.Sprintf("Фрагмент разметки страницы заказов (`hidden` скрывает элемент):\n\n```html\n%s\n```\n\nЧто напечатает `console.log` в коде Playwright ниже?", html.String()),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        itembank.Pick(r, stems),
		Code:        &itembank.CodeBlock{Lang: "javascript", Source: code},
		Answer:      itembank.Answer{Text: fmt.Sprint(count)},
		Explanation: fmt.Sprintf("%s Проверка строк (✓ — попадает в выборку): %s. Итого %d.", rule, strings.Join(trace, "; "), count),
	}, nil
}

// autowaitOutcome — чем завершится Playwright-тест при заданных таймаутах и задержках приложения.
type autowaitOutcome struct{}

func (autowaitOutcome) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.automation_ui.autowait-outcome.v1", Spec: itembank.SpecQA, Topic: "automation_ui",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.4, B: 0.3, C: 0.25}, TimeLimit: 150 * time.Second, Tags: []string{"playwright", "typescript", "timeouts"},
	}
}

type uiScenario struct {
	name, path, button, toast string
}

var uiScenarios = []uiScenario{
	{"оплата заказа", "/checkout", "Оплатить", "Заказ оплачен"},
	{"сохранение профиля", "/profile", "Сохранить", "Профиль обновлён"},
	{"отправка заявки", "/apply", "Отправить", "Заявка отправлена"},
	{"создание проекта", "/projects/new", "Создать", "Проект создан"},
	{"публикация отзыва", "/reviews/new", "Опубликовать", "Отзыв опубликован"},
}

var autowaitOutcomes = []string{
	"Тест проходит успешно",
	"Тест падает на `click()`: истёк actionTimeout — кнопка не стала активной",
	"Тест падает на `expect(...).toBeVisible()`: истёк expect.timeout — сообщение не появилось",
	"Тест падает по общему лимиту теста: «Test timeout exceeded»",
}

func (f autowaitOutcome) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A0A)
	sc := itembank.Pick(r, uiScenarios)
	actionT := itembank.Pick(r, []int{2000, 3000, 5000})
	expectT := itembank.Pick(r, []int{3000, 5000, 7000})
	testT := itembank.Pick(r, []int{10000, 15000, 20000})
	gotoMs := itembank.Pick(r, []int{500, 800, 1000, 1500})
	var enabledAfter, toastAfter int
	switch r.IntN(4) {
	case 0: // проходит
		enabledAfter = 100 * itembank.IntBetween(r, 2, actionT/100-1)
		toastAfter = 100 * itembank.IntBetween(r, 2, expectT/100-1)
	case 1: // actionTimeout
		enabledAfter = actionT + 100*itembank.IntBetween(r, 5, 60)
		toastAfter = 100 * itembank.IntBetween(r, 2, expectT/100-1)
	case 2: // expect.timeout
		enabledAfter = 100 * itembank.IntBetween(r, 2, actionT/100-1)
		toastAfter = expectT + 100*itembank.IntBetween(r, 5, 60)
	default: // общий лимит теста
		testT, actionT, expectT = 10000, 5000, 7000
		gotoMs = itembank.Pick(r, []int{1000, 1500})
		enabledAfter = 100 * itembank.IntBetween(r, 35, 49)
		toastAfter = 100 * itembank.IntBetween(r, 60, 100)
	}
	// Убираем совпадения с лимитом, чтобы не спорить о знаке «≤».
	for gotoMs+enabledAfter+toastAfter == testT || gotoMs+actionT == testT || gotoMs+enabledAfter+expectT == testT {
		gotoMs += 100
	}
	// Ответ вычисляется симуляцией хронологии независимо от сценария генерации.
	outcome := 0
	t := gotoMs
	timeline := []string{fmt.Sprintf("goto завершается на %d мс", t)}
	switch {
	case enabledAfter > actionT:
		if t+actionT > testT {
			outcome = 3
			timeline = append(timeline, fmt.Sprintf("click ждал бы до %d мс (actionTimeout), но общий лимит %d мс истекает раньше", t+actionT, testT))
		} else {
			outcome = 1
			timeline = append(timeline, fmt.Sprintf("кнопка станет активной только на %d мс, а click ждёт до %d мс (actionTimeout %d) — ошибка действия", t+enabledAfter, t+actionT, actionT))
		}
	default:
		t += enabledAfter
		switch {
		case t > testT:
			outcome = 3
			timeline = append(timeline, fmt.Sprintf("кнопка активна на %d мс — это уже позже общего лимита %d мс", t, testT))
		case toastAfter > expectT:
			if t+expectT > testT {
				outcome = 3
				timeline = append(timeline, fmt.Sprintf("click выполнен на %d мс; expect ждал бы до %d мс, но общий лимит %d мс истекает раньше", t, t+expectT, testT))
			} else {
				outcome = 2
				timeline = append(timeline, fmt.Sprintf("click выполнен на %d мс; сообщение появится на %d мс, а expect ждёт до %d мс (expect.timeout %d) — ошибка ожидания", t, t+toastAfter, t+expectT, expectT))
			}
		default:
			timeline = append(timeline, fmt.Sprintf("click выполнен на %d мс (кнопка активна через %d ≤ %d)", t, enabledAfter, actionT))
			t += toastAfter
			if t > testT {
				outcome = 3
				timeline = append(timeline, fmt.Sprintf("сообщение появляется на %d мс — позже общего лимита %d мс", t, testT))
			} else {
				timeline = append(timeline, fmt.Sprintf("сообщение появляется на %d мс (через %d ≤ %d), общее время %d ≤ %d — тест зелёный", t, toastAfter, expectT, t, testT))
			}
		}
	}
	var distractors []string
	for i, o := range autowaitOutcomes {
		if i != outcome {
			distractors = append(distractors, o)
		}
	}
	options, key := itembank.ShuffledOptions(r, autowaitOutcomes[outcome], distractors...)
	config := fmt.Sprintf("// playwright.config.ts\nexport default defineConfig({\n  timeout: %d,\n  expect: { timeout: %d },\n  use: { actionTimeout: %d },\n});", testT, expectT, actionT)
	code := fmt.Sprintf("test('%s', async ({ page }) => {\n  await page.goto('%s');\n  await page.getByRole('button', { name: '%s' }).click();\n  await expect(page.getByText('%s')).toBeVisible();\n});", sc.name, sc.path, sc.button, sc.toast)
	behavior := fmt.Sprintf("Поведение приложения: `goto` завершается за %d мс; кнопка «%s» отрисована сразу, но заблокирована (`disabled`) и становится активной через %d мс после загрузки; после нажатия сообщение «%s» появляется через %d мс. "+
		"`click()` ждёт, пока элемент станет доступным для действия (видимый и активный), не дольше actionTimeout; `expect` ждёт не дольше expect.timeout; общий лимит теста — `timeout`.",
		gotoMs, sc.button, enabledAfter, sc.toast, toastAfter)
	stems := []string{
		fmt.Sprintf("Конфигурация Playwright:\n\n```ts\n%s\n```\n\n%s Чем завершится тест ниже?", config, behavior),
		fmt.Sprintf("В проекте такой `playwright.config.ts`:\n\n```ts\n%s\n```\n\n%s Какой будет результат прогона теста из блока кода?", config, behavior),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "typescript", Source: code},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("Хронология: %s. Итог: %s. "+
			"Типичная ошибка — сравнивать задержки только с «своим» таймаутом, забывая, что общий лимит теста обрывает ожидание раньше, "+
			"или считать, что click не ждёт снятия `disabled` и падает сразу.",
			strings.Join(timeline, "; "), autowaitOutcomes[outcome]),
	}, nil
}

// pytestSelection — сколько тестов отберут `-k` и `-m` для набора тестов с маркерами.
type pytestSelection struct{}

func (pytestSelection) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.ci_integration.pytest-selection.v1", Spec: itembank.SpecQA, Topic: "ci_integration",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: -0.5, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"pytest", "python", "ci"},
	}
}

type pyTest struct {
	file, name string
}

var pyTests = []pyTest{
	{"test_api.py", "test_login_ok"}, {"test_api.py", "test_login_bad_password"}, {"test_api.py", "test_orders_list"},
	{"test_api.py", "test_order_create"}, {"test_api.py", "test_payment_webhook"},
	{"test_ui.py", "test_login_form"}, {"test_ui.py", "test_cart_add_item"}, {"test_ui.py", "test_cart_remove_item"}, {"test_ui.py", "test_checkout_button"},
	{"test_payments.py", "test_refund_full"}, {"test_payments.py", "test_refund_partial"}, {"test_payments.py", "test_payment_declined"},
}

var (
	pyMarkers = []string{"slow", "smoke", "regression"}
	pyKTerms  = []string{"login", "api", "ui", "cart", "order", "payment", "refund", "checkout"}
)

type pyExpr struct {
	form int // 0 — a; 1 — not a; 2 — a and b; 3 — a or b; 4 — a and not b
	a, b string
}

func (e pyExpr) String() string {
	switch e.form {
	case 1:
		return "not " + e.a
	case 2:
		return e.a + " and " + e.b
	case 3:
		return e.a + " or " + e.b
	case 4:
		return e.a + " and not " + e.b
	default:
		return e.a
	}
}

func (e pyExpr) eval(match func(string) bool) bool {
	switch e.form {
	case 1:
		return !match(e.a)
	case 2:
		return match(e.a) && match(e.b)
	case 3:
		return match(e.a) || match(e.b)
	case 4:
		return match(e.a) && !match(e.b)
	default:
		return match(e.a)
	}
}

func (f pytestSelection) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A0B)
	tests := itembank.PickN(r, pyTests, itembank.IntBetween(r, 8, 10))
	sort.Slice(tests, func(i, j int) bool {
		if tests[i].file != tests[j].file {
			return tests[i].file < tests[j].file
		}
		return tests[i].name < tests[j].name
	})
	marks := make([]string, len(tests))
	for i := range marks {
		if r.IntN(5) < 2 {
			marks[i] = itembank.Pick(r, pyMarkers)
		}
	}
	mode := r.IntN(3) // 0 — только -k, 1 — только -m, 2 — оба
	newExpr := func(terms []string, allowBinary bool) pyExpr {
		ab := itembank.PickN(r, terms, 2)
		form := r.IntN(2)
		if allowBinary {
			form = r.IntN(5)
		}
		return pyExpr{form: form, a: ab[0], b: ab[1]}
	}
	kMatch := func(i int) func(string) bool {
		return func(term string) bool {
			term = strings.ToLower(term)
			for _, name := range []string{tests[i].file, tests[i].name, marks[i]} {
				if name != "" && strings.Contains(strings.ToLower(name), term) {
					return true
				}
			}
			return false
		}
	}
	mMatch := func(i int) func(string) bool {
		return func(term string) bool { return marks[i] == term }
	}
	var kExpr, mExpr pyExpr
	var selected []int
	for attempt := 0; attempt < 30; attempt++ {
		if mode != 1 {
			kExpr = newExpr(pyKTerms, true)
		}
		if mode != 0 {
			mExpr = newExpr(pyMarkers, mode == 1)
		}
		selected = selected[:0]
		for i := range tests {
			ok := true
			if mode != 1 {
				ok = kExpr.eval(kMatch(i))
			}
			if ok && mode != 0 {
				ok = mExpr.eval(mMatch(i))
			}
			if ok {
				selected = append(selected, i)
			}
		}
		if len(selected) >= 1 && len(selected) < len(tests) {
			break
		}
	}
	cmd := "pytest"
	if mode != 1 {
		cmd += fmt.Sprintf(" -k \"%s\"", kExpr)
	}
	if mode != 0 {
		cmd += fmt.Sprintf(" -m \"%s\"", mExpr)
	}
	var table strings.Builder
	table.WriteString("| Тест | Маркеры |\n|---|---|\n")
	for i, t := range tests {
		m := "—"
		if marks[i] != "" {
			m = marks[i]
		}
		fmt.Fprintf(&table, "| `%s::%s` | %s |\n", t.file, t.name, m)
	}
	var chosen []string
	for _, i := range selected {
		chosen = append(chosen, fmt.Sprintf("`%s::%s`", tests[i].file, tests[i].name))
	}
	chosenText := "ни один тест"
	if len(chosen) > 0 {
		chosenText = strings.Join(chosen, ", ")
	}
	rules := "`-k` сравнивает подстроки без учёта регистра с именем теста, именем файла-модуля и именами маркеров; `-m` — логическое выражение над маркерами; при обоих флагах тест должен удовлетворять каждому."
	stems := []string{
		fmt.Sprintf("В проекте собраны тесты из таблицы (вывод `pytest --collect-only -q`, маркеры указаны рядом). %s Сколько тестов запустит команда `%s`?\n\n%s", rules, cmd, table.String()),
		fmt.Sprintf("В CI-шаге выполняется команда `%s`. Набор тестов и их маркеры — в таблице. %s Укажите число тестов, которые будут запущены.\n\n%s", cmd, rules, table.String()),
		fmt.Sprintf("Инженер настраивает выборку тестов для быстрого прогона: `%s`. %s Сколько тестов из таблицы попадёт в прогон?\n\n%s", cmd, rules, table.String()),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: float64(len(selected))},
		Explanation: fmt.Sprintf("Отобраны: %s — итого %d. Ловушки: `-k ui` совпадает со всеми тестами файла `test_ui.py`, а `-k api` — со всем `test_api.py`, потому что имя модуля тоже участвует в сравнении; "+
			"`-k slow` найдёт и тесты с маркером slow; `-m` сравнивает маркеры целиком, а не как подстроки.",
			chosenText, len(selected)),
	}, nil
}

// gitlabStagesRun — сколько job запустится, пропустится или упадёт в пайплайне GitLab CI при заданных падениях.
type gitlabStagesRun struct{}

func (gitlabStagesRun) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.ci_integration.gitlab-stages-run.v1", Spec: itembank.SpecQA, Topic: "ci_integration",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.8, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"gitlab-ci", "yaml", "pipeline"},
	}
}

type ciJob struct {
	name, stage, script, when string
	allowFailure, fails       bool
}

var (
	ciTestPool = []ciJob{
		{name: "unit", stage: "test", script: "make unit"},
		{name: "lint", stage: "test", script: "make lint"},
		{name: "e2e", stage: "test", script: "npx playwright test"},
		{name: "contract", stage: "test", script: "make contract-tests"},
		{name: "security_scan", stage: "test", script: "make scan"},
	}
	ciNotifyPool = []ciJob{
		{name: "notify_slack", stage: "notify", script: "./notify.sh", when: "on_failure"},
		{name: "publish_report", stage: "notify", script: "make report", when: "always"},
	}
)

func (f gitlabStagesRun) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A0C)
	build := ciJob{name: itembank.Pick(r, []string{"build", "compile", "build_image"}), stage: "build", script: "make build"}
	tests := itembank.PickN(r, ciTestPool, itembank.IntBetween(r, 2, 3))
	for i := range tests {
		tests[i].allowFailure = r.IntN(3) == 0
	}
	deploy := ciJob{name: "deploy_staging", stage: "deploy", script: "make deploy ENV=staging"}
	if r.IntN(4) == 0 {
		deploy.when = "manual"
	}
	notifies := itembank.PickN(r, ciNotifyPool, itembank.IntBetween(r, 1, 2))
	jobs := []ciJob{build}
	jobs = append(jobs, tests...)
	jobs = append(jobs, deploy)
	jobs = append(jobs, notifies...)
	// Падения выбираются среди build и test; ответ вычисляется симуляцией стадий.
	candidates := make([]int, 0, 1+len(tests))
	for i := 0; i <= len(tests); i++ {
		candidates = append(candidates, i)
	}
	var failingNames []string
	for _, i := range itembank.PickN(r, candidates, itembank.Pick(r, []int{0, 1, 1, 2})) {
		jobs[i].fails = true
	}
	for _, j := range jobs {
		if j.fails {
			failingNames = append(failingNames, "`"+j.name+"`")
		}
	}
	failedEarlier := false
	executed, skipped, failedJobs := 0, 0, 0
	blocking, warnings := false, false
	var trace []string
	for _, stage := range []string{"build", "test", "deploy", "notify"} {
		stageFailed := false
		for _, j := range jobs {
			if j.stage != stage {
				continue
			}
			run, reason := false, ""
			switch j.when {
			case "manual":
				reason = "when: manual — ждёт ручного запуска, который никто не делает"
			case "always":
				run = true
			case "on_failure":
				run = failedEarlier
				reason = "when: on_failure — блокирующих падений в предыдущих стадиях не было"
			default:
				run = !failedEarlier
				reason = "предыдущая стадия завершилась блокирующим падением"
			}
			if !run {
				skipped++
				trace = append(trace, fmt.Sprintf("`%s` не запускается (%s)", j.name, reason))
				continue
			}
			executed++
			switch {
			case j.fails && j.allowFailure:
				failedJobs++
				warnings = true
				trace = append(trace, fmt.Sprintf("`%s` запускается и падает, но allow_failure — следующие стадии не блокирует", j.name))
			case j.fails:
				failedJobs++
				stageFailed = true
				blocking = true
				trace = append(trace, fmt.Sprintf("`%s` запускается и падает — блокирует следующие стадии", j.name))
			default:
				trace = append(trace, fmt.Sprintf("`%s` запускается и проходит", j.name))
			}
		}
		if stageFailed {
			failedEarlier = true
		}
	}
	status := "passed"
	switch {
	case blocking:
		status = "failed"
	case warnings:
		status = "passed with warnings"
	}
	var yaml strings.Builder
	yaml.WriteString("stages: [build, test, deploy, notify]\n")
	for _, j := range jobs {
		fmt.Fprintf(&yaml, "\n%s:\n  stage: %s\n  script: %s\n", j.name, j.stage, j.script)
		if j.allowFailure {
			yaml.WriteString("  allow_failure: true\n")
		}
		if j.when != "" {
			fmt.Fprintf(&yaml, "  when: %s\n", j.when)
		}
	}
	failText := "При запуске все job завершаются успешно."
	switch len(failingNames) {
	case 1:
		failText = fmt.Sprintf("Известно: при запуске job %s завершается с ненулевым кодом; остальные job, если запустятся, успешны.", failingNames[0])
	case 2:
		failText = fmt.Sprintf("Известно: при запуске job %s завершаются с ненулевым кодом; остальные job, если запустятся, успешны.", strings.Join(failingNames, " и "))
	}
	type question struct {
		text   string
		answer int
	}
	q := itembank.Pick(r, []question{
		{"будет запущено раннером (независимо от результата)", executed},
		{"не будет запущено вовсе (пропущено или оставлено ждать ручного запуска)", skipped},
		{"завершатся с ошибкой (включая падения с allow_failure)", failedJobs},
	})
	rules := "Стадии выполняются по порядку, job одной стадии — параллельно; падение в стадии не останавливает её соседей."
	stems := []string{
		fmt.Sprintf("Ниже — `.gitlab-ci.yml` проекта. %s %s Сколько job %s?", rules, failText, q.text),
		fmt.Sprintf("Пайплайн GitLab CI описан в блоке кода. %s %s Укажите, сколько job %s.", failText, rules, q.text),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "yaml", Source: strings.TrimRight(yaml.String(), "\n")},
		Answer: itembank.Answer{Number: float64(q.answer)},
		Explanation: fmt.Sprintf("По стадиям: %s. Запущено %d, не запущено %d, с ошибкой %d; статус пайплайна — %s. "+
			"Ловушки: падение с allow_failure не блокирует следующие стадии и не считается «падением» для when: on_failure; "+
			"when: manual не запускается сам и не блокирует пайплайн; when: always выполняется в любом случае.",
			strings.Join(trace, "; "), executed, skipped, failedJobs, status),
	}, nil
}
