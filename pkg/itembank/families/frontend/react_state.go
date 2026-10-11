package frontend

import (
	"fmt"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(stateBatchingCount{})
	itembank.Register(effectOrder{})
	itembank.Register(listKeysInput{})
	itembank.Register(tsNarrowing{})
}

// stateBatchingCount — значение состояния после нескольких setState в одном обработчике:
// прямые обновления берут значение из замыкания, функциональные — из очереди.
type stateBatchingCount struct{}

func (stateBatchingCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.react_state.state-batching-count.v1", Spec: itembank.SpecFrontend, Topic: "react_state",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.4, B: 0.1, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"react", "hooks"},
	}
}

func (f stateBatchingCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF301)
	state := itembank.Pick(r, []string{"count", "total", "score", "step"})
	setter := "set" + strings.ToUpper(state[:1]) + state[1:]
	init := itembank.IntBetween(r, 0, 5)
	n := itembank.IntBetween(r, 3, 4)
	type upd struct {
		functional bool
		k          int
	}
	updates := make([]upd, n)
	for i := range updates {
		updates[i] = upd{r.IntN(2) == 0, itembank.IntBetween(r, 1, 5)}
	}
	// Хотя бы одно прямое и одно функциональное обновление, иначе ловушки нет.
	updates[0].functional = false
	updates[n-1].functional = true
	clicks := itembank.IntBetween(r, 1, 2)
	lines := []string{
		fmt.Sprintf("function Counter() {\n  const [%s, %s] = useState(%d);", state, setter, init),
		"  function handleClick() {",
	}
	for _, u := range updates {
		if u.functional {
			lines = append(lines, fmt.Sprintf("    %s(v => v + %d);", setter, u.k))
		} else {
			lines = append(lines, fmt.Sprintf("    %s(%s + %d);", setter, state, u.k))
		}
	}
	lines = append(lines, "  }", fmt.Sprintf("  return <button onClick={handleClick}>{%s}</button>;", state), "}")
	val := init
	var trace []string
	for c := 0; c < clicks; c++ {
		base := val
		for _, u := range updates {
			if u.functional {
				val += u.k
			} else {
				val = base + u.k
			}
			trace = append(trace, fmt.Sprintf("%d", val))
		}
	}
	clickWord := "одного клика"
	if clicks == 2 {
		clickWord = "двух кликов"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   fmt.Sprintf("Что покажет кнопка после %s (React 18, обновления батчатся)?", clickWord),
		Code:   &itembank.CodeBlock{Lang: "jsx", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Number: float64(val), Tolerance: 0},
		Explanation: fmt.Sprintf("Обновления применяются по очереди при следующем рендере. `%s(%s + k)` использует `%s` из замыкания "+
			"текущего рендера, то есть затирает всё, что было в очереди до него; `%s(v => v + k)` получает актуальное значение очереди. "+
			"Промежуточные значения: %s. Итог: %d.", setter, state, state, setter, strings.Join(trace, " → "), val),
	}, nil
}

// effectOrder — порядок логов рендера, useLayoutEffect, useEffect и их cleanup при монтировании и обновлении.
type effectOrder struct{}

func (effectOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.react_state.effect-order.v1", Spec: itembank.SpecFrontend, Topic: "react_state",
		Grade: itembank.GradeMiddle, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.3, B: 0.4, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"react", "hooks", "effects"},
	}
}

type effectCfg struct {
	hook    string // useLayoutEffect | useEffect
	label   string
	rerun   bool
	cleanup bool
	deps    string
}

func (f effectOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF302)
	state := itembank.Pick(r, []string{"count", "step", "tick", "n"})
	layout := effectCfg{hook: "useLayoutEffect", label: "layout"}
	passive := effectCfg{hook: "useEffect", label: "effect"}
	// Варианты: один из эффектов с [] (не перезапускается), либо оба перезапускаются без cleanup.
	switch r.IntN(3) {
	case 0:
		layout.rerun, passive.rerun = false, true
		passive.cleanup = r.IntN(2) == 0
		layout.cleanup = r.IntN(2) == 0
	case 1:
		layout.rerun, passive.rerun = true, false
		layout.cleanup = r.IntN(2) == 0
		passive.cleanup = r.IntN(2) == 0
	default:
		layout.rerun, passive.rerun = true, true
	}
	for _, e := range []*effectCfg{&layout, &passive} {
		switch {
		case !e.rerun:
			e.deps = ", []"
		case r.IntN(2) == 0:
			e.deps = fmt.Sprintf(", [%s]", state)
		default:
			e.deps = ""
		}
	}
	render := func(e effectCfg) string {
		body := fmt.Sprintf("    console.log('%s', %s);", e.label, state)
		if e.cleanup {
			body += fmt.Sprintf("\n    return () => console.log('%s cleanup', %s);", e.label, state)
		}
		return fmt.Sprintf("  %s(() => {\n%s\n  }%s);", e.hook, body, e.deps)
	}
	effects := []effectCfg{layout, passive}
	if r.IntN(2) == 0 {
		effects[0], effects[1] = effects[1], effects[0]
	}
	lines := []string{fmt.Sprintf("function Widget() {\n  const [%s, %s] = useState(0);", state, "set"+strings.ToUpper(state[:1])+state[1:])}
	for _, e := range effects {
		lines = append(lines, render(e))
	}
	lines = append(lines,
		fmt.Sprintf("  console.log('render', %s);", state),
		fmt.Sprintf("  return <button onClick={() => set%s(v => v + 1)}>{%s}</button>;", strings.ToUpper(state[:1])+state[1:], state),
		"}")

	// Монтирование, затем один клик: render → layout → effect; render → layout cleanup/layout → effect cleanup/effect.
	events := []string{"render 0", "layout 0", "effect 0", "render 1"}
	for _, e := range []effectCfg{layout, passive} {
		if !e.rerun {
			continue
		}
		if e.cleanup {
			events = append(events, e.label+" cleanup 0")
		}
		events = append(events, e.label+" 1")
	}
	options, order := itembank.OrderingOptions(r, events)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: "Компонент монтируется, затем пользователь один раз нажимает кнопку (StrictMode выключен). " +
			"Расположите строки консоли в порядке появления.",
		Code:    &itembank.CodeBlock{Lang: "jsx", Source: strings.Join(lines, "\n")},
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: "Тело компонента выполняется при каждом рендере первым; после коммита синхронно выполняются layout-эффекты, " +
			"затем асинхронно — обычные эффекты. При обновлении сначала вызывается cleanup предыдущего запуска (с значением из его замыкания, " +
			"то есть 0), затем новый эффект; эффект с `[]` при обновлении не трогается. Порядок: " + strings.Join(events, " → ") + ".",
	}, nil
}

// listKeysInput — куда «уедет» введённый в поле текст при изменении списка с key по индексу и по id.
type listKeysInput struct{}

func (listKeysInput) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.react_state.list-keys-input.v1", Spec: itembank.SpecFrontend, Topic: "react_state",
		Grade: itembank.GradeSenior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.4, B: 0.7, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"react", "reconciliation", "keys"},
	}
}

func (f listKeysInput) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF303)
	n := itembank.IntBetween(r, 4, 5)
	names := itembank.Idents(r, "name_lat", n+1)
	extra := names[n]
	names = names[:n]
	byIndex := r.IntN(2) == 0
	keyExpr := "p.id"
	if byIndex {
		keyExpr = "i"
	}
	code := fmt.Sprintf(`function People({ people, onChange }) {
  return (
    <ul>
      {people.map((p, i) => (
        <li key={%s}>
          <span>{p.name}</span>
          <input placeholder="note" />
        </li>
      ))}
    </ul>
  );
}`, keyExpr)
	q := itembank.IntBetween(r, 0, n-2) // поле, в которое ввели текст (не последнее — иначе при index-key узел исчезает)
	var action, answer, actionExplain string
	switch r.IntN(3) {
	case 0: // удаление элемента p ≠ q
		p := q
		for p == q {
			p = r.IntN(n)
		}
		action = fmt.Sprintf("родитель удаляет из массива `people` запись «%s»", names[p])
		if byIndex {
			if q < p {
				answer = names[q]
			} else {
				answer = names[q+1]
			}
			actionExplain = fmt.Sprintf("после удаления на позицию %d попадает «%s», а DOM-узел с полем остаётся на месте", q, answer)
		} else {
			answer = names[q]
			actionExplain = "узел с ключом записи переезжает вместе с ней"
		}
	case 1: // вставка в начало
		action = fmt.Sprintf("родитель добавляет запись «%s» в начало массива `people`", extra)
		if byIndex {
			if q == 0 {
				answer = extra
			} else {
				answer = names[q-1]
			}
			actionExplain = fmt.Sprintf("все записи сдвигаются на одну позицию вниз, и на позиции %d теперь «%s»", q, answer)
		} else {
			answer = names[q]
			actionExplain = "React вставляет новый узел в начало, а существующие узлы сопоставляет по ключу"
		}
	default: // разворот списка
		action = "родитель разворачивает массив `people` (`[...people].reverse()`)"
		if byIndex {
			answer = names[n-1-q]
			actionExplain = fmt.Sprintf("на позиции %d после разворота оказывается «%s»", q, answer)
		} else {
			answer = names[q]
			actionExplain = "узлы переставляются вместе со своими ключами"
		}
	}
	var explain string
	if byIndex {
		explain = fmt.Sprintf("При `key={i}` React сопоставляет элементы по позиции: DOM-узел `<li>` с неконтролируемым `<input>` остаётся "+
			"на позиции %d и получает только новый текст `<span>` — %s. Введённое значение живёт в DOM, а не в данных, поэтому оно "+
			"оказывается рядом с «%s».", q, actionExplain, answer)
	} else {
		explain = fmt.Sprintf("При `key={p.id}` узел привязан к записи, а не к позиции: %s, и поле с введённым текстом остаётся "+
			"у «%s». Ошибка — считать, что React переносит значение input вместе с позицией.", actionExplain, answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Список `people` (id уникальны): %s. Пользователь вводит текст в поле рядом с «%s», после чего %s. "+
			"Рядом с каким именем окажется введённый текст после перерисовки? Ответьте именем.",
			strings.Join(names, ", "), names[q], action),
		Code:        &itembank.CodeBlock{Lang: "jsx", Source: code},
		Answer:      itembank.Answer{Text: answer},
		Explanation: explain,
	}, nil
}

// tsNarrowing — какой тип у переменной внутри ветки после сужения union-типа.
type tsNarrowing struct{}

func (tsNarrowing) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.ts_types.union-narrowing.v1", Spec: itembank.SpecFrontend, Topic: "ts_types",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.2, C: 0.25}, TimeLimit: 90 * time.Second, Tags: []string{"typescript", "narrowing"},
	}
}

type tsGuard struct {
	cond  func(v string) string
	keeps map[string]bool
	note  string
}

var tsMembers = []string{"string", "number", "boolean", "null", "undefined", "string[]", "Date"}

var tsGuards = []tsGuard{
	{func(v string) string { return fmt.Sprintf("typeof %s === 'string'", v) }, map[string]bool{"string": true}, "typeof отделяет только строки"},
	{func(v string) string { return fmt.Sprintf("typeof %s === 'number'", v) }, map[string]bool{"number": true}, "typeof отделяет только числа"},
	{func(v string) string { return fmt.Sprintf("typeof %s === 'boolean'", v) }, map[string]bool{"boolean": true}, "typeof отделяет только boolean"},
	{func(v string) string { return fmt.Sprintf("%s == null", v) }, map[string]bool{"null": true, "undefined": true}, "нестрогое `== null` ловит и null, и undefined"},
	{func(v string) string { return fmt.Sprintf("%s === null", v) }, map[string]bool{"null": true}, "строгое `=== null` не затрагивает undefined"},
	{func(v string) string { return fmt.Sprintf("%s === undefined", v) }, map[string]bool{"undefined": true}, "строгое `=== undefined` не затрагивает null"},
	{func(v string) string { return fmt.Sprintf("Array.isArray(%s)", v) }, map[string]bool{"string[]": true}, "Array.isArray выделяет массивы"},
	{func(v string) string { return fmt.Sprintf("%s instanceof Date", v) }, map[string]bool{"Date": true}, "instanceof выделяет экземпляры класса"},
	{func(v string) string { return fmt.Sprintf("typeof %s === 'object'", v) }, map[string]bool{"null": true, "string[]": true, "Date": true}, "typeof null — тоже 'object', как и у массива и Date"},
}

func (f tsNarrowing) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF304)
	v := itembank.Pick(r, []string{"value", "input", "x", "data", "item"})
	fn := itembank.Pick(r, []string{"format", "normalize", "describe", "handle", "parse"})
	var members, inside, outside []string
	var guard tsGuard
	for len(inside) == 0 || len(outside) == 0 {
		members = itembank.PickN(r, tsMembers, itembank.IntBetween(r, 3, 4))
		// Сохраняем канонический порядок членов, чтобы варианты выглядели единообразно.
		var ordered []string
		for _, m := range tsMembers {
			for _, x := range members {
				if x == m {
					ordered = append(ordered, m)
				}
			}
		}
		members = ordered
		guard = itembank.Pick(r, tsGuards)
		inside, outside = nil, nil
		for _, m := range members {
			if guard.keeps[m] {
				inside = append(inside, m)
			} else {
				outside = append(outside, m)
			}
		}
	}
	askElse := r.IntN(2) == 0
	correctSet, otherSet := inside, outside
	point := "(1)"
	if askElse {
		correctSet, otherSet = outside, inside
		point = "(2)"
	}
	join := func(xs []string) string { return strings.Join(xs, " | ") }
	correct := join(correctSet)
	candidates := []string{join(otherSet), join(members)}
	// Ответ плюс один лишний член из другой ветки.
	for _, m := range members {
		if guard.keeps[m] == askElse { // член не из correctSet
			var plus []string
			for _, x := range members {
				if x == m || guard.keeps[x] != askElse {
					plus = append(plus, x)
				}
			}
			candidates = append(candidates, join(plus))
			break
		}
	}
	if len(correctSet) >= 2 {
		candidates = append(candidates, join(correctSet[1:]))
	}
	candidates = append(candidates, "never", "unknown")
	var distractors []string
	seen := map[string]bool{correct: true}
	for _, c := range candidates {
		if !seen[c] && len(distractors) < 3 {
			seen[c] = true
			distractors = append(distractors, c)
		}
	}
	code := fmt.Sprintf("function %s(%s: %s) {\n  if (%s) {\n    // (1)\n  } else {\n    // (2)\n  }\n}", fn, v, join(members), guard.cond(v))
	options, key := itembank.ShuffledOptions(r, correct, distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    fmt.Sprintf("Какой тип выведет TypeScript для `%s` в точке %s?", v, point),
		Code:    &itembank.CodeBlock{Lang: "typescript", Source: code},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("Сужение: %s, поэтому внутри `if` остаётся `%s`, а в `else` — всё остальное из объединения: `%s`. "+
			"Ветка %s даёт `%s`.", guard.note, join(inside), join(outside), point, correct),
	}, nil
}
