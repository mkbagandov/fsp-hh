package frontend

import (
	"fmt"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(closureCounter{})
	itembank.Register(nestedEventLoop{})
	itembank.Register(tsConditionalDistribution{})
	itembank.Register(tsUtilityAssignable{})
	itembank.Register(batchedRenderCount{})
}

// closureCounter — вывод фабрики счётчиков: независимые замыкания, псевдоним одной функции
// и общая внешняя переменная дают разные результаты при одинаковой последовательности вызовов.
type closureCounter struct{}

func (closureCounter) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.js_core.closure-counter.v1", Spec: itembank.SpecFrontend, Topic: "js_core",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.5, B: -0.8, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"javascript", "closures"},
	}
}

func (f closureCounter) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF501)
	factory := itembank.Pick(r, []string{"makeCounter", "createCounter", "counterFrom", "makeStepper"})
	names := itembank.Pick(r, [][]string{{"a", "b"}, {"x", "y"}, {"first", "second"}, {"inc", "tick"}})
	variant := r.IntN(3) // 0 — независимые счётчики, 1 — псевдоним, 2 — общая внешняя переменная
	start1, start2 := itembank.IntBetween(r, 0, 10), itembank.IntBetween(r, 0, 10)
	step1 := itembank.IntBetween(r, 1, 5)
	step2 := step1
	for step2 == step1 {
		step2 = itembank.IntBetween(r, 1, 5)
	}
	calls := make([]int, itembank.IntBetween(r, 2, 4))
	for i := range calls {
		calls[i] = r.IntN(2)
	}

	var lines []string
	if variant == 2 {
		lines = append(lines, fmt.Sprintf("let value = %d;", start1), fmt.Sprintf("function %s(step) {", factory))
	} else {
		lines = append(lines, fmt.Sprintf("function %s(start, step) {", factory), "  let value = start;")
	}
	lines = append(lines, "  return () => {", "    value += step;", "    return value;", "  };", "}")
	switch variant {
	case 0:
		lines = append(lines,
			fmt.Sprintf("const %s = %s(%d, %d);", names[0], factory, start1, step1),
			fmt.Sprintf("const %s = %s(%d, %d);", names[1], factory, start2, step2))
	case 1:
		lines = append(lines,
			fmt.Sprintf("const %s = %s(%d, %d);", names[0], factory, start1, step1),
			fmt.Sprintf("const %s = %s;", names[1], names[0]))
	default:
		lines = append(lines,
			fmt.Sprintf("const %s = %s(%d);", names[0], factory, step1),
			fmt.Sprintf("const %s = %s(%d);", names[1], factory, step2))
	}

	// Модель: у каждого счётчика своя ячейка состояния и свой шаг; псевдоним и общая переменная делят ячейку.
	cells := []int{start1, start2}
	cellOf := []int{0, 1}
	steps := []int{step1, step2}
	if variant != 0 {
		cellOf = []int{0, 0}
	}
	if variant == 1 {
		steps = []int{step1, step1}
	}
	call := func(i int) int {
		cells[cellOf[i]] += steps[i]
		return cells[cellOf[i]]
	}
	var trace []string
	for _, c := range calls {
		lines = append(lines, fmt.Sprintf("%s();", names[c]))
		trace = append(trace, fmt.Sprintf("`%s()` → %d", names[c], call(c)))
	}
	out1 := call(0)
	out2 := call(1)
	lines = append(lines, fmt.Sprintf("console.log(%s(), %s());", names[0], names[1]))
	trace = append(trace, fmt.Sprintf("`%s()` → %d", names[0], out1), fmt.Sprintf("`%s()` → %d", names[1], out2))

	var why string
	switch variant {
	case 0:
		why = fmt.Sprintf("Каждый вызов `%s` создаёт новое замыкание со своей переменной `value`, поэтому счётчики независимы "+
			"и не влияют друг на друга.", factory)
	case 1:
		why = fmt.Sprintf("`const %s = %s` не создаёт новый счётчик: обе переменные ссылаются на одну функцию и одно замыкание, "+
			"так что вызовы через любое имя увеличивают одно и то же `value` с шагом %d.", names[1], names[0], step1)
	default:
		why = fmt.Sprintf("`value` объявлена вне `%s`, поэтому оба счётчика разделяют одну переменную, и шаги %d и %d "+
			"складываются в общий итог.", factory, step1, step2)
	}
	stem := itembank.Pick(r, []string{
		"Что выведет код в консоль?",
		"Какая строка появится в консоли после выполнения кода?",
		"Что напечатает вызов `console.log` в последней строке?",
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Text: fmt.Sprintf("%d %d", out1, out2)},
		Explanation: why + " Последовательность: " + strings.Join(trace, ", ") +
			fmt.Sprintf(". Аргументы `console.log` вычисляются слева направо, поэтому вывод — `%d %d`.", out1, out2),
	}, nil
}

// nestedEventLoop — порядок вывода, когда микрозадачи ставятся из макрозадач и наоборот:
// после каждой макрозадачи очередь микрозадач опустошается заново.
type nestedEventLoop struct{}

func (nestedEventLoop) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.js_core.nested-event-loop.v1", Spec: itembank.SpecFrontend, Topic: "js_core",
		Grade: itembank.GradeSenior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.6, B: 1.1, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"javascript", "event-loop", "promises"},
	}
}

type elStmt struct {
	label  string
	kind   stmtKind
	viaQM  bool    // queueMicrotask вместо Promise.resolve().then
	nested *elStmt // задача, которую этот оператор ставит при выполнении
}

// elLeaf — однострочная форма оператора без вложенности.
func elLeaf(s elStmt) string {
	switch s.kind {
	case kindMicro:
		if s.viaQM {
			return fmt.Sprintf("queueMicrotask(() => console.log('%s'));", s.label)
		}
		return fmt.Sprintf("Promise.resolve().then(() => console.log('%s'));", s.label)
	case kindMacro:
		return fmt.Sprintf("setTimeout(() => console.log('%s'), 0);", s.label)
	default:
		return fmt.Sprintf("console.log('%s');", s.label)
	}
}

func elRender(s elStmt) []string {
	if s.nested == nil {
		return []string{elLeaf(s)}
	}
	open, closing := "setTimeout(() => {", "}, 0);"
	if s.kind == kindMicro {
		open, closing = "Promise.resolve().then(() => {", "});"
		if s.viaQM {
			open = "queueMicrotask(() => {"
		}
	}
	return []string{open, fmt.Sprintf("  console.log('%s');", s.label), "  " + elLeaf(*s.nested), closing}
}

func (f nestedEventLoop) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF502)
	labels := []string{"A", "B", "C", "D", "E", "F"}
	kinds := []stmtKind{kindSync, kindMicro, kindMacro}
	var stmts []elStmt
	// Нужны: 5–6 меток, микро- и макрозадачи, хотя бы одна вложенность другого вида (микро в макро или наоборот).
	for ok := false; !ok; {
		stmts = nil
		next := 0
		n := itembank.IntBetween(r, 3, 4)
		hasMicro, hasMacro, cross, topNonMacro := false, false, false, false
		for i := 0; i < n && next < len(labels); i++ {
			s := elStmt{label: labels[next], kind: kinds[r.IntN(3)], viaQM: r.IntN(3) == 0}
			next++
			topNonMacro = topNonMacro || s.kind != kindMacro
			if s.kind != kindSync && next < len(labels) && r.IntN(2) == 0 {
				nk := kinds[1+r.IntN(2)]
				s.nested = &elStmt{label: labels[next], kind: nk, viaQM: r.IntN(3) == 0}
				next++
				cross = cross || nk != s.kind
			}
			for _, x := range []*elStmt{&s, s.nested} {
				if x == nil {
					continue
				}
				hasMicro = hasMicro || x.kind == kindMicro
				hasMacro = hasMacro || x.kind == kindMacro
			}
			stmts = append(stmts, s)
		}
		ok = len(stmts) == n && hasMicro && hasMacro && cross && topNonMacro && next >= 5
	}

	// Честная модель цикла событий: синхронный код → все микрозадачи → по одной макрозадаче с опустошением микрозадач после каждой.
	var out []string
	var microQ, macroQ []*elStmt
	var run func(s *elStmt)
	schedule := func(s *elStmt) {
		switch s.kind {
		case kindMicro:
			microQ = append(microQ, s)
		case kindMacro:
			macroQ = append(macroQ, s)
		default:
			run(s)
		}
	}
	run = func(s *elStmt) {
		out = append(out, s.label)
		if s.nested != nil {
			schedule(s.nested)
		}
	}
	drainMicro := func() {
		for len(microQ) > 0 {
			s := microQ[0]
			microQ = microQ[1:]
			run(s)
		}
	}
	for i := range stmts {
		schedule(&stmts[i])
	}
	drainMicro()
	for len(macroQ) > 0 {
		s := macroQ[0]
		macroQ = macroQ[1:]
		run(s)
		drainMicro()
	}

	var src []string
	for _, s := range stmts {
		src = append(src, elRender(s)...)
	}
	options, order := itembank.OrderingOptions(r, out)
	stem := itembank.Pick(r, []string{
		"В каком порядке строки появятся в консоли? Расположите метки от первой к последней.",
		"Код выполняется в браузере. Расположите метки в порядке их вывода в консоль (сверху — первая).",
		"В какой последовательности будут напечатаны метки? Расположите их от первой к последней.",
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    stem,
		Code:    &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(src, "\n")},
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: "Сначала выполняется весь синхронный код, затем очередь микрозадач опустошается целиком: микрозадача, " +
			"поставленная из микрозадачи, дописывается в конец той же очереди и всё равно выполняется раньше любого setTimeout. " +
			"Макрозадачи выполняются по одной, и после каждой очередь микрозадач опустошается снова — поэтому промис из первого " +
			"setTimeout срабатывает раньше второго setTimeout, а setTimeout, поставленный из микрозадачи, встаёт в хвост очереди " +
			"макрозадач. Порядок: " + strings.Join(out, " → ") + ".",
	}, nil
}

// tsConditionalDistribution — результат условного типа над объединением: распределение по членам,
// обёртка в кортеж, отключающая распределение, и особый случай never.
type tsConditionalDistribution struct{}

func (tsConditionalDistribution) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.ts_types.conditional-distribution.v1", Spec: itembank.SpecFrontend, Topic: "ts_types",
		Grade: itembank.GradeSenior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.4, B: 0.9, C: 0.25}, TimeLimit: 120 * time.Second, Tags: []string{"typescript", "conditional-types"},
	}
}

type tsCheck struct {
	expr   string
	accept map[string]bool
	note   string
}

var tsChecks = []tsCheck{
	{"string", map[string]bool{"string": true}, "только `string`"},
	{"number", map[string]bool{"number": true}, "только `number`"},
	{"string | number", map[string]bool{"string": true, "number": true}, "`string` и `number`"},
	{"null | undefined", map[string]bool{"null": true, "undefined": true}, "`null` и `undefined`"},
	{"unknown[]", map[string]bool{"string[]": true}, "только массивы"},
	{"object", map[string]bool{"string[]": true, "Date": true}, "массивы и `Date`, но не `null` — в strict-режиме он к `object` не относится"},
}

func (f tsConditionalDistribution) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF503)
	alias := itembank.Pick(r, []string{"Check", "Flag", "Classify", "Shape"})
	inputName := itembank.Pick(r, []string{"Input", "Value", "Payload", "Source"})
	resName := itembank.Pick(r, []string{"Result", "Out", "Final"})
	lits := itembank.Pick(r, [][]string{{"yes", "no"}, {"scalar", "other"}, {"ok", "fail"}, {"text", "rest"}, {"on", "off"}})
	l1, l2 := "'"+lits[0]+"'", "'"+lits[1]+"'"
	form := r.IntN(4) // 0 — литералы, 1 — T : never (Extract), 2 — never : T (Exclude), 3 — [T] extends [C]
	neverInput := (form == 0 || form == 3) && r.IntN(6) == 0
	check := itembank.Pick(r, tsChecks)
	var members, acc, rej []string
	for ok := neverInput; !ok; {
		check = itembank.Pick(r, tsChecks)
		picked := itembank.PickN(r, tsMembers, itembank.IntBetween(r, 2, 3))
		members, acc, rej = nil, nil, nil
		for _, m := range tsMembers { // канонический порядок членов
			for _, p := range picked {
				if p == m {
					members = append(members, m)
				}
			}
		}
		for _, m := range members {
			if check.accept[m] {
				acc = append(acc, m)
			} else {
				rej = append(rej, m)
			}
		}
		// Распределительные формы интересны только на смешанном объединении; кортежная — когда есть подходящие члены.
		ok = len(acc) > 0 && (form == 3 || len(rej) > 0)
	}
	union := func(xs []string) string {
		if len(xs) == 0 {
			return "never"
		}
		return strings.Join(xs, " | ")
	}

	var cond string
	switch form {
	case 0:
		cond = fmt.Sprintf("T extends %s ? %s : %s", check.expr, l1, l2)
	case 1:
		cond = fmt.Sprintf("T extends %s ? T : never", check.expr)
	case 2:
		cond = fmt.Sprintf("T extends %s ? never : T", check.expr)
	default:
		cond = fmt.Sprintf("[T] extends [%s] ? %s : %s", check.expr, l1, l2)
	}
	code := []string{fmt.Sprintf("type %s<T> = %s;", alias, cond)}
	if neverInput {
		code = append(code, fmt.Sprintf("type %s = %s<never>;", resName, alias))
	} else {
		code = append(code, fmt.Sprintf("type %s = %s;", inputName, union(members)), fmt.Sprintf("type %s = %s<%s>;", resName, alias, inputName))
	}

	// Результат при распределении по членам (для пояснения и дистракторов).
	var perMember []string
	var distParts []string
	for _, m := range members {
		res := l2
		switch {
		case form == 1 && check.accept[m]:
			res = m
		case form == 1:
			res = "never"
		case form == 2 && check.accept[m]:
			res = "never"
		case form == 2:
			res = m
		case check.accept[m]:
			res = l1
		}
		perMember = append(perMember, fmt.Sprintf("`%s` → `%s`", m, res))
	}
	if len(acc) > 0 {
		distParts = append(distParts, l1)
	}
	if len(rej) > 0 {
		distParts = append(distParts, l2)
	}
	both := l1 + " | " + l2

	var correct, explain string
	var candidates []string
	switch {
	case neverInput && form == 0:
		correct = "never"
		candidates = []string{l1, l2, both, "unknown"}
		explain = fmt.Sprintf("`never` — пустое объединение. Распределительный условный тип применяется к каждому члену, а членов нет, "+
			"поэтому `%s` = `never`. Ветка %s не выбирается: условие не вычисляется ни для одного члена.", resName, l2)
	case neverInput:
		correct = l1
		candidates = []string{"never", l2, both, "unknown"}
		explain = fmt.Sprintf("Обёртка `[T] extends [%s]` отключает распределение: сравнивается весь тип целиком, а `never` присваивается "+
			"любому типу, поэтому `[never] extends [%s]` истинно и результат — `%s`. Без обёртки получилось бы `never`.",
			check.expr, check.expr, l1)
	case form == 0:
		correct = union(distParts)
		candidates = []string{l2, l1, "never", union(members), "unknown"}
		explain = fmt.Sprintf("Параметр `T` стоит в условии «голым», поэтому условный тип распределяется по каждому члену `%s` "+
			"(условию `%s` соответствуют %s): %s. Объединение результатов — `%s`. Ловушка — проверять всё объединение целиком: "+
			"`%s` не присваивается `%s`, и получилось бы `%s`.",
			inputName, check.expr, check.note, strings.Join(perMember, ", "), correct, union(members), check.expr, l2)
	case form == 1:
		correct = union(acc)
		candidates = []string{union(rej), union(members), "never", "unknown"}
		explain = fmt.Sprintf("Это распределительный условный тип (как встроенный `Extract`): каждый член `%s` проверяется отдельно "+
			"(условию `%s` соответствуют %s): %s. Остаются только подходящие члены: `%s`. Если бы объединение сравнивалось целиком, "+
			"`%s` не прошло бы проверку и результатом стал `never`.",
			inputName, check.expr, check.note, strings.Join(perMember, ", "), correct, union(members))
	case form == 2:
		correct = union(rej)
		candidates = []string{union(acc), union(members), "never", "unknown"}
		explain = fmt.Sprintf("Это распределительный условный тип (как встроенный `Exclude`): каждый член `%s` проверяется отдельно "+
			"(условию `%s` соответствуют %s): %s. Подходящие члены заменяются на `never` и исчезают из объединения, остаётся `%s`. "+
			"При сравнении объединения целиком оно не прошло бы проверку и вернулось бы без изменений: `%s`.",
			inputName, check.expr, check.note, strings.Join(perMember, ", "), correct, union(members))
	default:
		verdict := "да, каждый член подходит"
		correct = l1
		if len(rej) > 0 {
			correct = l2
			verdict = fmt.Sprintf("нет, члены `%s` не подходят", union(rej))
		}
		candidates = []string{union(distParts), l1, l2, "never", union(members)}
		explain = fmt.Sprintf("Обёртка в кортеж `[T] extends [%s]` отключает распределение: проверяется, присваивается ли всё объединение "+
			"`%s` типу `%s` целиком — %s, поэтому результат `%s`. При распределении по членам получилось бы `%s` (%s).",
			check.expr, union(members), check.expr, verdict, correct, union(distParts), strings.Join(perMember, ", "))
	}
	var distractors []string
	seen := map[string]bool{correct: true}
	for _, c := range candidates {
		if !seen[c] && len(distractors) < 3 {
			seen[c] = true
			distractors = append(distractors, c)
		}
	}
	options, key := itembank.ShuffledOptions(r, correct, distractors...)
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Чему равен тип `%s`?", resName),
		fmt.Sprintf("Какой тип получит `%s` (strict-режим TypeScript)?", resName),
		fmt.Sprintf("Что выведет TypeScript в качестве типа `%s`?", resName),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Code:        &itembank.CodeBlock{Lang: "typescript", Source: strings.Join(code, "\n")},
		Options:     options,
		Answer:      itembank.Answer{Keys: []string{key}},
		Explanation: explain,
	}, nil
}

// tsUtilityAssignable — какие объектные литералы присваиваются типу, собранному из Pick/Omit/Partial/Required
// и пересечений: нужны все обязательные поля и ни одного лишнего.
type tsUtilityAssignable struct{}

func (tsUtilityAssignable) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.ts_types.utility-assignable.v1", Spec: itembank.SpecFrontend, Topic: "ts_types",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.5, B: 0.1, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"typescript", "utility-types"},
	}
}

type tsProp struct {
	name, typ, sample string
}

type tsField struct {
	present, required bool
}

var tsShapes = []struct {
	name  string
	props []tsProp
}{
	{"User", []tsProp{{"id", "number", "1"}, {"name", "string", `"Ann"`}, {"email", "string", `"ann@example.com"`}, {"age", "number", "30"}}},
	{"Product", []tsProp{{"id", "number", "7"}, {"title", "string", `"Lamp"`}, {"price", "number", "1200"}, {"stock", "number", "4"}}},
	{"Task", []tsProp{{"id", "number", "3"}, {"title", "string", `"Deploy"`}, {"done", "boolean", "false"}, {"due", "string", `"2026-05-01"`}}},
	{"Order", []tsProp{{"id", "number", "42"}, {"total", "number", "990"}, {"status", "string", `"paid"`}, {"comment", "string", `"asap"`}}},
}

func (f tsUtilityAssignable) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF504)
	shape := itembank.Pick(r, tsShapes)
	props := shape.props
	n := len(props)
	alias := itembank.Pick(r, []string{"Draft", "Patch", "Input", "Payload"})
	optional := make([]bool, n)
	fields := make([]tsField, n)
	var expr string
	var form int
	assignable := func(mask int) bool {
		for i := range props {
			has := mask>>i&1 == 1
			if has && !fields[i].present || !has && fields[i].required {
				return false
			}
		}
		return true
	}
	var correct, wrong []int
	// Подбираем форму типа, при которой подходят ≥ 2 подмножеств и не подходят ≥ 3 (иначе вариантов не собрать).
	for len(correct) < 2 || len(wrong) < 3 {
		for i := range optional {
			optional[i] = false
		}
		if r.IntN(2) == 0 {
			optional[r.IntN(n)] = true
		}
		ab := itembank.PickN(r, []int{0, 1, 2, 3}, 2)
		if ab[0] > ab[1] {
			ab[0], ab[1] = ab[1], ab[0]
		}
		a, b := ab[0], ab[1]
		keysAB := fmt.Sprintf("'%s' | '%s'", props[a].name, props[b].name)
		keyA := "'" + props[a].name + "'"
		form = r.IntN(8)
		for i := range fields {
			declaredReq := !optional[i]
			inAB := i == a || i == b
			switch form {
			case 0:
				fields[i] = tsField{present: inAB, required: false}
			case 1:
				fields[i] = tsField{present: inAB, required: inAB}
			case 2:
				fields[i] = tsField{present: inAB, required: inAB && declaredReq}
			case 3:
				fields[i] = tsField{present: i != a, required: i != a && declaredReq}
			case 4:
				fields[i] = tsField{present: i != a, required: false}
			case 5:
				fields[i] = tsField{present: i != a, required: i != a}
			case 6:
				fields[i] = tsField{present: true, required: i == a && declaredReq}
			default:
				fields[i] = tsField{present: true, required: i != a && declaredReq}
			}
		}
		switch form {
		case 0:
			expr = fmt.Sprintf("Partial<Pick<%s, %s>>", shape.name, keysAB)
		case 1:
			expr = fmt.Sprintf("Required<Pick<%s, %s>>", shape.name, keysAB)
		case 2:
			expr = fmt.Sprintf("Pick<%s, %s>", shape.name, keysAB)
		case 3:
			expr = fmt.Sprintf("Omit<%s, %s>", shape.name, keyA)
		case 4:
			expr = fmt.Sprintf("Partial<Omit<%s, %s>>", shape.name, keyA)
		case 5:
			expr = fmt.Sprintf("Required<Omit<%s, %s>>", shape.name, keyA)
		case 6:
			expr = fmt.Sprintf("Partial<%s> & Pick<%s, %s>", shape.name, shape.name, keyA)
		default:
			expr = fmt.Sprintf("Omit<%s, %s> & Partial<Pick<%s, %s>>", shape.name, keyA, shape.name, keyA)
		}
		correct, wrong = nil, nil
		for mask := 0; mask < 1<<n; mask++ {
			if assignable(mask) {
				correct = append(correct, mask)
			} else {
				wrong = append(wrong, mask)
			}
		}
	}
	nCorrect := 2
	if len(correct) >= 3 && r.IntN(2) == 0 {
		nCorrect = 3
	}
	literal := func(mask int) string {
		var parts []string
		for i, p := range props {
			if mask>>i&1 == 1 {
				parts = append(parts, p.name+": "+p.sample)
			}
		}
		if len(parts) == 0 {
			return "`{}`"
		}
		return "`{ " + strings.Join(parts, ", ") + " }`"
	}
	var correctTexts, wrongTexts []string
	for _, m := range itembank.PickN(r, correct, nCorrect) {
		correctTexts = append(correctTexts, literal(m))
	}
	for _, m := range itembank.PickN(r, wrong, 5-nCorrect) {
		wrongTexts = append(wrongTexts, literal(m))
	}
	options, keys := itembank.ShuffledMulti(r, correctTexts, wrongTexts...)

	code := []string{fmt.Sprintf("interface %s {", shape.name)}
	for i, p := range props {
		q := ""
		if optional[i] {
			q = "?"
		}
		code = append(code, fmt.Sprintf("  %s%s: %s;", p.name, q, p.typ))
	}
	code = append(code, "}", fmt.Sprintf("type %s = %s;", alias, expr))

	var shapeParts, req, opt, absent []string
	for i, p := range props {
		switch {
		case !fields[i].present:
			absent = append(absent, "`"+p.name+"`")
		case fields[i].required:
			shapeParts = append(shapeParts, p.name+": "+p.typ)
			req = append(req, "`"+p.name+"`")
		default:
			shapeParts = append(shapeParts, p.name+"?: "+p.typ)
			opt = append(opt, "`"+p.name+"`")
		}
	}
	listOr := func(xs []string) string {
		if len(xs) == 0 {
			return "нет"
		}
		return strings.Join(xs, ", ")
	}
	intersectNote := ""
	if form >= 6 {
		intersectNote = " В пересечении поле обязательно, если оно обязательно хотя бы в одном из операндов."
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Какие из объектных литералов можно присвоить переменной `const v: %s = …` без ошибки компиляции "+
			"(strict-режим, литерал присваивается напрямую)? Выберите все подходящие.", alias),
		fmt.Sprintf("Переменная объявлена как `let v: %s`. Какие из литералов TypeScript примет в присваивании `v = …` "+
			"без ошибок (strict-режим)? Выберите все.", alias),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    stem,
		Code:    &itembank.CodeBlock{Lang: "typescript", Source: strings.Join(code, "\n")},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: fmt.Sprintf("`%s` раскрывается в `{ %s }`: обязательные поля — %s, необязательные — %s, отсутствующие в типе — %s.%s "+
			"Свежий объектный литерал присваивается, только если в нём есть все обязательные поля и нет ни одного поля вне типа: "+
			"лишнее поле TypeScript отвергает (excess property check), даже когда остальные совпадают. Подходят: %s.",
			alias, strings.Join(shapeParts, "; "), listOr(req), listOr(opt), listOr(absent), intersectNote, strings.Join(correctTexts, ", ")),
	}, nil
}

// batchedRenderCount — сколько рендеров даст обработчик с несколькими setState, разделёнными await,
// setTimeout и flushSync: автоматический батчинг React 18 объединяет обновления одного синхронного участка.
type batchedRenderCount struct{}

func (batchedRenderCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.react_state.batched-render-count.v1", Spec: itembank.SpecFrontend, Topic: "react_state",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.3, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"react", "hooks", "batching"},
	}
}

func setterOf(state string) string { return "set" + strings.ToUpper(state[:1]) + state[1:] }

// ruPlural — число с существительным в нужной форме: 1 рендер, 2 рендера, 5 рендеров.
func ruPlural(n int, one, few, many string) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return fmt.Sprintf("%d %s", n, one)
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf("%d %s", n, few)
	default:
		return fmt.Sprintf("%d %s", n, many)
	}
}

func (f batchedRenderCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF505)
	flag := itembank.Pick(r, []string{"saving", "loading", "busy", "pending"})
	num := itembank.Pick(r, []string{"count", "attempts", "total", "clicks"})
	setFlag, setNum := setterOf(flag), setterOf(num)
	comp := itembank.Pick(r, []string{"SaveButton", "SyncPanel", "RetryCard", "UploadForm"})
	api := itembank.Pick(r, []string{"api.save()", "api.sync()", "fetchStatus()", "client.retry()"})
	segKinds := []int{0} // 0 — синхронная часть, 1 — после await, 2 — колбэк setTimeout
	for extra := itembank.Pick(r, []int{0, 1, 1, 2}); extra > 0; extra-- {
		segKinds = append(segKinds, 1+r.IntN(2))
	}
	useFlush := r.IntN(3) == 0
	flushSeg := r.IntN(len(segKinds))
	segNames := []string{"синхронная часть обработчика", "код после `await`", "колбэк `setTimeout`"}

	flagVal := false
	total, updatesTotal := 1, 0
	hasAwait := false
	var body, notes []string
	for si, k := range segKinds {
		updates := make([]string, 0, 3)
		flagUsed := false
		// В первом участке и там, где стоит flushSync, обновлений не меньше двух — иначе батчинг не на чём показать.
		minUpd := 1
		if si == 0 || (useFlush && si == flushSeg) {
			minUpd = 2
		}
		for j := itembank.IntBetween(r, minUpd, 3); j > 0; j-- {
			// Флаг переключается не чаще одного раза на участок: каждое обновление должно менять состояние.
			if !flagUsed && r.IntN(2) == 0 {
				flagVal = !flagVal
				flagUsed = true
				updates = append(updates, fmt.Sprintf("%s(%t);", setFlag, flagVal))
			} else {
				updates = append(updates, fmt.Sprintf("%s(c => c + %d);", setNum, itembank.IntBetween(r, 1, 3)))
			}
		}
		flushes := 0
		if useFlush && si == flushSeg {
			updates[0] = fmt.Sprintf("flushSync(() => %s);", strings.TrimSuffix(updates[0], ";"))
			flushes = 1
		}
		renders := flushes
		if len(updates) > flushes {
			renders++
		}
		total += renders
		updatesTotal += len(updates)
		indent := "    "
		switch k {
		case 1:
			hasAwait = true
			body = append(body, fmt.Sprintf("    await %s;", api))
		case 2:
			body = append(body, "    setTimeout(() => {")
			indent = "      "
		}
		for _, u := range updates {
			body = append(body, indent+u)
		}
		if k == 2 {
			body = append(body, "    }, 0);")
		}
		note := fmt.Sprintf("%s: %s → %s", segNames[k], ruPlural(len(updates), "обновление", "обновления", "обновлений"),
			ruPlural(renders, "рендер", "рендера", "рендеров"))
		if flushes > 0 {
			note += " (`flushSync` выполняет свой рендер сразу, остальные обновления участка объединяются)"
		}
		notes = append(notes, note)
	}
	fnKw := "function"
	if hasAwait {
		fnKw = "async function"
	}
	lines := []string{
		fmt.Sprintf("function %s() {", comp),
		fmt.Sprintf("  const [%s, %s] = useState(false);", flag, setFlag),
		fmt.Sprintf("  const [%s, %s] = useState(0);", num, setNum),
		"  console.log('render');",
		fmt.Sprintf("  %s handleClick() {", fnKw),
	}
	lines = append(lines, body...)
	lines = append(lines, "  }", fmt.Sprintf("  return <button onClick={handleClick}>{%s ? '…' : %s}</button>;", flag, num), "}")

	apiNote := ""
	if hasAwait {
		apiNote = fmt.Sprintf(" `%s` возвращает промис, который успешно завершается.", api)
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("React 18 (`createRoot`), StrictMode выключен.%s Пользователь один раз нажимает кнопку. "+
			"Сколько раз в консоли появится `render`, считая первый рендер при монтировании?", apiNote),
		fmt.Sprintf("Приложение на React 18 (`createRoot`), StrictMode выключен.%s После монтирования пользователь один раз кликает "+
			"по кнопке. Сколько всего раз выполнится тело компонента `%s`, включая монтирование?", apiNote, comp),
	})
	naive := ""
	if 1+updatesTotal != total {
		naive = fmt.Sprintf(" Ошибка — считать каждый вызов setState отдельным рендером (получилось бы %d) или полагать, как в React 17, "+
			"что после await и в setTimeout батчинга нет.", 1+updatesTotal)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "jsx", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Number: float64(total), Tolerance: 0},
		Explanation: fmt.Sprintf("Монтирование — 1 рендер. В React 18 все обновления состояния внутри одного синхронного участка кода "+
			"объединяются в один рендер: автоматический батчинг работает и в колбэке setTimeout, и в коде после await, но участки, "+
			"разделённые await или таймером, выполняются в разных тактах и рендерятся отдельно; `flushSync` принудительно рендерит "+
			"немедленно. Участки: %s. Итого %d.%s", strings.Join(notes, "; "), total, naive),
	}, nil
}
