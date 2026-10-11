// Package frontend — семейства заданий для frontend-разработки: JavaScript,
// TypeScript, React, CSS, браузерные API, производительность, доступность.
package frontend

import (
	"fmt"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(closureLoop{})
	itembank.Register(eventLoopOrder{})
	itembank.Register(cssSpecificity{})
}

// closureLoop — var/let в цикле с setTimeout.
type closureLoop struct{}

func (closureLoop) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.js_core.closure-loop.v1", Spec: itembank.SpecFrontend, Topic: "js_core",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.3, B: -0.5, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"javascript"},
	}
}

func (f closureLoop) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF001)
	n := itembank.IntBetween(r, 2, 5)
	start := itembank.IntBetween(r, 0, 3)
	useLet := r.IntN(2) == 0
	kw := "var"
	if useLet {
		kw = "let"
	}
	v := itembank.Pick(r, []string{"i", "k", "idx", "step"})
	delay := itembank.IntBetween(r, 0, 2) * 10
	code := fmt.Sprintf("for (%s %s = %d; %s < %d; %s++) {\n  setTimeout(() => console.log(%s), %d);\n}", kw, v, start, v, start+n, v, v, delay)
	var lines []string
	for i := 0; i < n; i++ {
		if useLet {
			lines = append(lines, fmt.Sprintf("%d", start+i))
		} else {
			lines = append(lines, fmt.Sprintf("%d", start+n))
		}
	}
	explain := fmt.Sprintf("`let` создаёт новую привязку переменной на каждую итерацию, поэтому каждый колбэк видит своё значение: %s.",
		strings.Join(lines, ", "))
	if !useLet {
		explain = fmt.Sprintf("`var` объявляет одну переменную на всю функцию; к моменту выполнения колбэков цикл завершился и %s = %d, "+
			"поэтому все %d вызовов печатают %d.", v, start+n, n, start+n)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        "Что выведет код в консоль (по одному значению на строку)?",
		Code:        &itembank.CodeBlock{Lang: "javascript", Source: code},
		Answer:      itembank.Answer{Text: strings.Join(lines, "\n")},
		Explanation: explain + " Колбэки setTimeout выполняются после завершения синхронного кода в порядке постановки.",
	}, nil
}

// eventLoopOrder — порядок вывода: синхронный код, микрозадачи, макрозадачи.
type eventLoopOrder struct{}

func (eventLoopOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.js_core.event-loop-order.v1", Spec: itembank.SpecFrontend, Topic: "browser_api",
		Grade: itembank.GradeMiddle, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.4, B: 0.4, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"javascript", "event-loop"},
	}
}

type stmtKind int

const (
	kindSync stmtKind = iota
	kindMicro
	kindMacro
)

func (f eventLoopOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF002)
	n := itembank.IntBetween(r, 4, 5)
	labels := []string{"A", "B", "C", "D", "E", "F", "G"}
	type stmt struct {
		label string
		kind  stmtKind
		code  string
	}
	var stmts []stmt
	kinds := []stmtKind{kindSync, kindMicro, kindMacro}
	for i := 0; i < n; i++ {
		k := kinds[r.IntN(3)]
		if i == 0 {
			k = kindSync
		}
		lbl := labels[i]
		var code string
		switch k {
		case kindSync:
			code = fmt.Sprintf("console.log('%s');", lbl)
		case kindMicro:
			if r.IntN(2) == 0 {
				code = fmt.Sprintf("Promise.resolve().then(() => console.log('%s'));", lbl)
			} else {
				code = fmt.Sprintf("queueMicrotask(() => console.log('%s'));", lbl)
			}
		default:
			code = fmt.Sprintf("setTimeout(() => console.log('%s'), 0);", lbl)
		}
		stmts = append(stmts, stmt{lbl, k, code})
	}
	// Гарантируем, что есть хотя бы одна микро- и одна макрозадача, иначе задание тривиально.
	hasMicro, hasMacro := false, false
	for _, s := range stmts {
		hasMicro = hasMicro || s.kind == kindMicro
		hasMacro = hasMacro || s.kind == kindMacro
	}
	if !hasMicro {
		stmts[1] = stmt{labels[1], kindMicro, fmt.Sprintf("Promise.resolve().then(() => console.log('%s'));", labels[1])}
	}
	if !hasMacro {
		stmts[n-1] = stmt{labels[n-1], kindMacro, fmt.Sprintf("setTimeout(() => console.log('%s'), 0);", labels[n-1])}
	}
	var order []string
	for _, k := range kinds {
		for _, s := range stmts {
			if s.kind == k {
				order = append(order, s.label)
			}
		}
	}
	var src []string
	for _, s := range stmts {
		src = append(src, s.code)
	}
	options, orderKeys := itembank.OrderingOptions(r, order)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    "В каком порядке строки появятся в консоли? Расположите метки от первой к последней.",
		Code:    &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(src, "\n")},
		Options: options,
		Answer:  itembank.Answer{Order: orderKeys},
		Explanation: "Сначала выполняется весь синхронный код, затем очередь микрозадач (промисы и queueMicrotask) в порядке постановки, " +
			"и только потом макрозадачи (setTimeout), даже с задержкой 0. Порядок: " + strings.Join(order, " → ") + ".",
	}, nil
}

// cssSpecificity — какой селектор победит.
type cssSpecificity struct{}

func (cssSpecificity) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.css_layout.specificity.v1", Spec: itembank.SpecFrontend, Topic: "css_layout",
		Grade: itembank.GradeJunior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.1, B: -0.4, C: 0.33}, TimeLimit: 90 * time.Second, Tags: []string{"css"},
	}
}

type selectorPart struct {
	text    string
	ids     int
	classes int
	tags    int
}

func (f cssSpecificity) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF003)
	tags := []string{"div", "p", "a", "li", "span", "button"}
	classes := []string{".card", ".active", ".title", ".btn", ".item", ".nav"}
	ids := []string{"#main", "#header", "#menu", "#footer"}
	attrs := []string{"[disabled]", "[type=text]", ":hover", ":first-child"}
	build := func() selectorPart {
		var parts []string
		sp := selectorPart{}
		shape := r.IntN(6)
		switch shape {
		case 0: // tag
			parts = append(parts, itembank.Pick(r, tags))
			sp.tags = 1
		case 1: // tag.class
			parts = append(parts, itembank.Pick(r, tags)+itembank.Pick(r, classes))
			sp.tags, sp.classes = 1, 1
		case 2: // .class .class
			c := itembank.PickN(r, classes, 2)
			parts = append(parts, c[0]+" "+c[1])
			sp.classes = 2
		case 3: // #id tag
			parts = append(parts, itembank.Pick(r, ids)+" "+itembank.Pick(r, tags))
			sp.ids, sp.tags = 1, 1
		case 4: // tag.class:pseudo
			parts = append(parts, itembank.Pick(r, tags)+itembank.Pick(r, classes)+itembank.Pick(r, attrs))
			sp.tags, sp.classes = 1, 2
		default: // tag tag.class
			t := itembank.PickN(r, tags, 2)
			parts = append(parts, t[0]+" "+t[1]+itembank.Pick(r, classes))
			sp.tags, sp.classes = 2, 1
		}
		sp.text = strings.Join(parts, " ")
		return sp
	}
	weight := func(s selectorPart) int { return s.ids*100 + s.classes*10 + s.tags }
	var sels []selectorPart
	for len(sels) < 3 {
		s := build()
		dup := false
		for _, e := range sels {
			if e.text == s.text || weight(e) == weight(s) {
				dup = true
			}
		}
		if !dup {
			sels = append(sels, s)
		}
	}
	best := sels[0]
	for _, s := range sels[1:] {
		if weight(s) > weight(best) {
			best = s
		}
	}
	rules := make([]string, 0, 3)
	colors := itembank.PickN(r, []string{"red", "blue", "green", "orange", "purple"}, 3)
	for i, s := range sels {
		rules = append(rules, fmt.Sprintf("%s { color: %s; }", s.text, colors[i]))
	}
	var distractors []string
	for _, s := range sels {
		if s.text != best.text {
			distractors = append(distractors, s.text)
		}
	}
	options, key := itembank.ShuffledOptions(r, best.text, distractors...)
	var spec []string
	for _, s := range sels {
		spec = append(spec, fmt.Sprintf("`%s` → (%d,%d,%d)", s.text, s.ids, s.classes, s.tags))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    "Все три правила применимы к одному элементу. Правило какого селектора определит цвет текста?",
		Code:    &itembank.CodeBlock{Lang: "css", Source: strings.Join(rules, "\n")},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: "Специфичность считается как тройка (id, классы/атрибуты/псевдоклассы, элементы), сравнение поразрядное: " +
			strings.Join(spec, "; ") + ". Побеждает " + best.text + ". Порядок в файле важен только при равной специфичности.",
	}, nil
}
