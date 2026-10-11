package frontend

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(promiseChainOutput{})
	itembank.Register(asyncAwaitOrder{})
	itembank.Register(eventPropagation{})
	itembank.Register(debounceThrottleCount{})
}

// promiseChainOutput — что выведет цепочка then/catch: ошибка пропускает then
// до ближайшего catch, а catch без ошибки передаёт значение дальше.
type promiseChainOutput struct{}

func (promiseChainOutput) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.js_core.promise-chain-output.v1", Spec: itembank.SpecFrontend, Topic: "js_core",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.2, B: -0.5, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"javascript", "promises"},
	}
}

type chainStepKind int

const (
	chainAdd chainStepKind = iota
	chainMul
	chainThrow
	chainCatch
)

type chainStep struct {
	kind chainStepKind
	k    int
}

func (f promiseChainOutput) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF101)
	start := itembank.IntBetween(r, 1, 9)
	n := itembank.IntBetween(r, 3, 5)
	v := itembank.Pick(r, []string{"x", "v", "n", "value"})
	steps := make([]chainStep, n)
	for i := range steps {
		if r.IntN(2) == 0 {
			steps[i] = chainStep{chainAdd, itembank.IntBetween(r, 1, 6)}
		} else {
			steps[i] = chainStep{chainMul, itembank.IntBetween(r, 2, 3)}
		}
	}
	if r.IntN(10) < 7 {
		// Ошибка и catch через 1–2 позиции: промежуточный then должен быть пропущен.
		throwPos := itembank.IntBetween(r, 1, n-2)
		catchPos := throwPos + 1
		if catchPos < n-1 && r.IntN(2) == 0 {
			catchPos++
		}
		steps[throwPos] = chainStep{chainThrow, 0}
		steps[catchPos] = chainStep{chainCatch, itembank.IntBetween(r, 1, 9)}
	} else if r.IntN(2) == 0 {
		// Catch без ошибки — пропускает значение дальше, не подменяя его.
		steps[itembank.IntBetween(r, 1, n-1)] = chainStep{chainCatch, itembank.IntBetween(r, 1, 9)}
	}
	lines := []string{fmt.Sprintf("Promise.resolve(%d)", start)}
	val, rejected := start, false
	var trace []string
	for _, s := range steps {
		switch s.kind {
		case chainAdd:
			lines = append(lines, fmt.Sprintf("  .then(%s => %s + %d)", v, v, s.k))
			if !rejected {
				val += s.k
				trace = append(trace, fmt.Sprintf("+%d → %d", s.k, val))
			} else {
				trace = append(trace, "then пропущен")
			}
		case chainMul:
			lines = append(lines, fmt.Sprintf("  .then(%s => %s * %d)", v, v, s.k))
			if !rejected {
				val *= s.k
				trace = append(trace, fmt.Sprintf("×%d → %d", s.k, val))
			} else {
				trace = append(trace, "then пропущен")
			}
		case chainThrow:
			lines = append(lines, "  .then(() => { throw new Error('fail'); })")
			rejected = true
			trace = append(trace, "ошибка")
		case chainCatch:
			lines = append(lines, fmt.Sprintf("  .catch(() => %d)", s.k))
			if rejected {
				rejected = false
				val = s.k
				trace = append(trace, fmt.Sprintf("catch → %d", val))
			} else {
				trace = append(trace, "catch пропущен")
			}
		}
	}
	lines = append(lines, fmt.Sprintf("  .then(%s => console.log(%s));", v, v))
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   "Что выведет код в консоль?",
		Code:   &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Text: fmt.Sprintf("%d", val)},
		Explanation: fmt.Sprintf("Цепочка: %d, %s. Отклонённый промис пропускает все then до ближайшего catch, "+
			"а catch на успешном промисе не вызывается и передаёт значение дальше без изменений. Итог: %d.",
			start, strings.Join(trace, ", "), val),
	}, nil
}

// asyncAwaitOrder — порядок вывода при смешении async/await, then и setTimeout:
// продолжение после await ставится в очередь микрозадач в момент самого await.
type asyncAwaitOrder struct{}

func (asyncAwaitOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.js_core.async-await-order.v1", Spec: itembank.SpecFrontend, Topic: "js_core",
		Grade: itembank.GradeSenior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.4, B: 0.8, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"javascript", "event-loop", "async"},
	}
}

type topStmtKind int

const (
	topCall topStmtKind = iota
	topSync
	topThen
	topTimeout
)

func (f asyncAwaitOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF102)
	fn := itembank.Pick(r, []string{"run", "main", "load", "init", "start"})
	awaits := itembank.IntBetween(r, 1, 2)
	labels := []string{"A", "B", "C", "D", "E", "F"}
	next := 0
	label := func() string { l := labels[next]; next++; return l }

	// Тело async-функции: сегменты между await.
	segs := make([]string, awaits+1)
	for i := range segs {
		segs[i] = label()
	}
	body := []string{fmt.Sprintf("async function %s() {", fn)}
	for i, s := range segs {
		if i > 0 {
			body = append(body, "  await null;")
		}
		body = append(body, fmt.Sprintf("  console.log('%s');", s))
	}
	body = append(body, "}")

	// Верхний уровень: вызов функции плюс 2–3 других оператора в случайном порядке.
	others := itembank.PickN(r, []topStmtKind{topSync, topThen, topTimeout}, itembank.IntBetween(r, 2, 3))
	kinds := append([]topStmtKind{topCall}, others...)
	kinds = itembank.Shuffle(r, kinds)

	var out []string
	var micro []func()
	var macro []string
	var runSeg func(i int)
	runSeg = func(i int) {
		out = append(out, segs[i])
		if i+1 < len(segs) {
			micro = append(micro, func() { runSeg(i + 1) })
		}
	}
	var top []string
	for _, k := range kinds {
		switch k {
		case topCall:
			top = append(top, fn+"();")
			runSeg(0)
		case topSync:
			l := label()
			top = append(top, fmt.Sprintf("console.log('%s');", l))
			out = append(out, l)
		case topThen:
			l := label()
			top = append(top, fmt.Sprintf("Promise.resolve().then(() => console.log('%s'));", l))
			micro = append(micro, func() { out = append(out, l) })
		case topTimeout:
			l := label()
			top = append(top, fmt.Sprintf("setTimeout(() => console.log('%s'), 0);", l))
			macro = append(macro, l)
		}
	}
	for len(micro) > 0 {
		t := micro[0]
		micro = micro[1:]
		t()
	}
	out = append(out, macro...)

	code := strings.Join(append(body, top...), "\n")
	options, order := itembank.OrderingOptions(r, out)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    "В каком порядке строки появятся в консоли? Расположите метки от первой к последней.",
		Code:    &itembank.CodeBlock{Lang: "javascript", Source: code},
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("Тело `%s` выполняется синхронно до первого `await`; продолжение после каждого `await` ставится в очередь "+
			"микрозадач в момент самого await, поэтому оно соревнуется с `then` по времени постановки, а не по месту в файле. "+
			"Второе продолжение попадает в очередь только после выполнения первого. setTimeout выполняется после всех микрозадач. "+
			"Порядок: %s.", fn, strings.Join(out, " → ")),
	}, nil
}

// eventPropagation — какие обработчики сработают при клике с учётом фазы перехвата,
// всплытия и stopPropagation.
type eventPropagation struct{}

func (eventPropagation) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.browser_api.event-propagation.v1", Spec: itembank.SpecFrontend, Topic: "browser_api",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.2, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"dom", "events"},
	}
}

type listenerSlot struct {
	el      int // 0 — внешний, 1 — средний, 2 — цель
	capture bool
}

// Порядок диспетчеризации: перехват сверху вниз, на цели сначала capture-обработчики, затем всплытие снизу вверх.
var dispatchOrder = []listenerSlot{{0, true}, {1, true}, {2, true}, {2, false}, {1, false}, {0, false}}

func (f eventPropagation) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF103)
	names := itembank.Pick(r, [][]string{
		{"outer", "middle", "inner"}, {"page", "card", "btn"}, {"list", "item", "link"}, {"app", "panel", "save"},
	})
	var chosen []int
	stopAt := -1
	for stopAt < 0 {
		idx := itembank.PickN(r, []int{0, 1, 2, 3, 4, 5}, itembank.IntBetween(r, 4, 5))
		sort.Ints(idx)
		var candidates []int
		for pos, i := range idx {
			// stopPropagation в capture-обработчике цели — спорный край спецификации, исключаем.
			if pos >= 1 && pos <= len(idx)-2 && i != 2 {
				candidates = append(candidates, pos)
			}
		}
		if len(candidates) > 0 {
			chosen = idx
			stopAt = itembank.Pick(r, candidates)
		}
	}
	labels := []string{"A", "B", "C", "D", "E"}
	phase := func(s listenerSlot) string {
		if s.capture {
			return "перехват"
		}
		return "всплытие"
	}
	var lines, correct, wrong, firedDesc []string
	for pos, i := range chosen {
		s := dispatchOrder[i]
		opt := fmt.Sprintf("%s — %s, %s", labels[pos], names[s.el], phase(s))
		handler := fmt.Sprintf("() => log('%s')", labels[pos])
		if pos == stopAt {
			handler = fmt.Sprintf("(e) => { log('%s'); e.stopPropagation(); }", labels[pos])
		}
		capArg := ""
		if s.capture {
			capArg = itembank.Pick(r, []string{", true", ", { capture: true }"})
		}
		lines = append(lines, fmt.Sprintf("%s.addEventListener('click', %s%s);", names[s.el], handler, capArg))
		if pos <= stopAt {
			correct = append(correct, opt)
			firedDesc = append(firedDesc, labels[pos])
		} else {
			wrong = append(wrong, opt)
		}
	}
	lines = itembank.Shuffle(r, lines)
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Разметка: `<div id=\"%s\"><div id=\"%s\"><button id=\"%s\">…</button></div></div>`. "+
			"Пользователь кликает по кнопке `%s`. Какие обработчики выполнятся (функция `log`)? Выберите все.",
			names[0], names[1], names[2], names[2]),
		Code:    &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(lines, "\n")},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: fmt.Sprintf("Событие идёт сверху вниз в фазе перехвата (третий аргумент `true`/`{ capture: true }`), затем на цели "+
			"срабатывают сначала capture-, потом обычные обработчики, после чего всплывает снизу вверх. `stopPropagation()` "+
			"останавливает переход к следующему элементу. Сработают: %s; остальные не дождутся события.", strings.Join(firedDesc, ", ")),
	}, nil
}

// debounceThrottleCount — сколько раз вызовется обёрнутая функция при серии событий.
type debounceThrottleCount struct{}

func (debounceThrottleCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.browser_api.debounce-throttle-count.v1", Spec: itembank.SpecFrontend, Topic: "browser_api",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.1, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"javascript", "timers"},
	}
}

func (f debounceThrottleCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF104)
	wait := itembank.Pick(r, []int{200, 300, 400, 500})
	n := itembank.IntBetween(r, 6, 8)
	small := []int{50, 100, 150, wait - 50}
	large := []int{wait + 100, wait + 250, wait * 2}
	times := []int{0}
	for i := 1; i < n; i++ {
		var gap int
		if r.IntN(10) < 6 {
			gap = itembank.Pick(r, small)
		} else {
			gap = itembank.Pick(r, large)
		}
		times = append(times, times[i-1]+gap)
	}
	inner := itembank.Pick(r, []string{"search", "save", "resize", "track", "sync"})
	event := itembank.Pick(r, []string{"input", "scroll", "resize", "mousemove", "keyup"})
	debounce := r.IntN(2) == 0
	var code string
	var fired []int
	if debounce {
		code = fmt.Sprintf("function debounce(fn, wait) {\n  let timer;\n  return (...args) => {\n    clearTimeout(timer);\n"+
			"    timer = setTimeout(() => fn(...args), wait);\n  };\n}\nconst handler = debounce(%s, %d);", inner, wait)
		for i, t := range times {
			if i == n-1 || times[i+1]-t > wait {
				fired = append(fired, t+wait)
			}
		}
	} else {
		code = fmt.Sprintf("function throttle(fn, wait) {\n  let last = -Infinity;\n  return (...args) => {\n    const now = Date.now();\n"+
			"    if (now - last >= wait) {\n      last = now;\n      fn(...args);\n    }\n  };\n}\nconst handler = throttle(%s, %d);", inner, wait)
		last := -1 << 30
		for _, t := range times {
			if t-last >= wait {
				last = t
				fired = append(fired, t)
			}
		}
	}
	var ts []string
	for _, t := range times {
		ts = append(ts, fmt.Sprintf("%d", t))
	}
	var fs []string
	for _, t := range fired {
		fs = append(fs, fmt.Sprintf("%d", t))
	}
	explain := fmt.Sprintf("Throttle с ведущим вызовом принимает событие, если с последнего принятого прошло не меньше %d мс, "+
		"остальные отбрасывает. Принятые моменты: %s мс — всего %d.", wait, strings.Join(fs, ", "), len(fired))
	if debounce {
		explain = fmt.Sprintf("Debounce сбрасывает таймер при каждом событии и вызывает функцию только через %d мс тишины, "+
			"поэтому серия близких событий даёт один вызов. Вызовы произойдут в моменты %s мс — всего %d.", wait, strings.Join(fs, ", "), len(fired))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("`handler` подписан на событие `%s`. События приходят в моменты (мс от начала): %s; после последнего "+
			"события ничего не происходит. Сколько раз будет вызвана функция `%s`?", event, strings.Join(ts, ", "), inner),
		Code:        &itembank.CodeBlock{Lang: "javascript", Source: code},
		Answer:      itembank.Answer{Number: float64(len(fired)), Tolerance: 0},
		Explanation: explain,
	}, nil
}
