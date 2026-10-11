package dataml

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(listSlicing{})
	itembank.Register(generatorExhaust{})
	itembank.Register(mutableDefault{})
}

// pyIntList форматирует срез как Python-список целых: [1, 2, 3].
func pyIntList(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// pyStrList форматирует строки как Python-список строковых литералов.
func pyStrList(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = `"` + x + `"`
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// pySlice воспроизводит семантику среза Python xs[start:stop:step]; nil — граница опущена.
func pySlice(xs []int, start, stop *int, step int) []int {
	n := len(xs)
	lo, hi := 0, n
	if step < 0 {
		lo, hi = n-1, -1
	}
	adjust := func(v int) int {
		if v < 0 {
			v += n
			if v < 0 {
				if step < 0 {
					return -1
				}
				return 0
			}
			return v
		}
		if v >= n {
			if step < 0 {
				return n - 1
			}
			return n
		}
		return v
	}
	if start != nil {
		lo = adjust(*start)
	}
	if stop != nil {
		hi = adjust(*stop)
	}
	out := []int{}
	if step > 0 {
		for i := lo; i < hi; i += step {
			out = append(out, xs[i])
		}
		return out
	}
	for i := lo; i > hi; i += step {
		out = append(out, xs[i])
	}
	return out
}

// listSlicing — что выведут срезы списка с границами по умолчанию, отрицательными
// индексами и отрицательным шагом.
type listSlicing struct{}

func (listSlicing) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.python_core.list-slicing.v1", Spec: itembank.SpecDataML, Topic: "python_core",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.1, B: -0.8, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"python"},
	}
}

// sliceExpr — индексное выражение и вывод print для него.
type sliceExpr struct {
	text   string
	result string
}

const sliceShapes = 11

func buildSlice(r *rand.Rand, xs []int, shape int) sliceExpr {
	n := len(xs)
	ptr := func(v int) *int { return &v }
	var start, stop *int
	step := 1
	var text string
	switch shape {
	case 0:
		i := itembank.IntBetween(r, 1, n-3)
		j := itembank.IntBetween(r, i+1, n-1)
		start, stop, text = ptr(i), ptr(j), fmt.Sprintf("%d:%d", i, j)
	case 1:
		i := itembank.IntBetween(r, 1, n-2)
		start, text = ptr(i), fmt.Sprintf("%d:", i)
	case 2:
		j := itembank.IntBetween(r, 2, n-1)
		stop, text = ptr(j), fmt.Sprintf(":%d", j)
	case 3:
		k := itembank.IntBetween(r, 1, 3)
		start, text = ptr(-k), fmt.Sprintf("-%d:", k)
	case 4:
		k := itembank.IntBetween(r, 1, 3)
		stop, text = ptr(-k), fmt.Sprintf(":-%d", k)
	case 5:
		step = itembank.IntBetween(r, 2, 3)
		text = fmt.Sprintf("::%d", step)
	case 6:
		i := itembank.IntBetween(r, 0, 2)
		j := itembank.IntBetween(r, i+3, n)
		step = 2
		start, stop, text = ptr(i), ptr(j), fmt.Sprintf("%d:%d:2", i, j)
	case 7:
		step = -itembank.IntBetween(r, 1, 2)
		text = fmt.Sprintf("::%d", step)
	case 8:
		j := itembank.IntBetween(r, 3, n-1)
		i := itembank.IntBetween(r, 0, j-2)
		step = -1
		start, stop, text = ptr(j), ptr(i), fmt.Sprintf("%d:%d:-1", j, i)
	case 9:
		i := itembank.IntBetween(r, 1, 2)
		k := itembank.IntBetween(r, 1, 2)
		start, stop, text = ptr(i), ptr(-k), fmt.Sprintf("%d:-%d", i, k)
	default:
		// Одиночный отрицательный индекс: печатается элемент, а не список.
		k := itembank.IntBetween(r, 1, 3)
		return sliceExpr{text: fmt.Sprintf("-%d", k), result: strconv.Itoa(xs[n-k])}
	}
	return sliceExpr{text: text, result: pyIntList(pySlice(xs, start, stop, step))}
}

func (f listSlicing) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A10)
	name := itembank.Pick(r, []string{"xs", "data", "vals", "arr", "nums"})
	n := itembank.IntBetween(r, 6, 8)
	xs := make([]int, 0, n)
	used := map[int]bool{}
	for len(xs) < n {
		v := itembank.IntBetween(r, 1, 30)
		if !used[v] {
			used[v] = true
			xs = append(xs, v)
		}
	}
	shapes := r.Perm(sliceShapes)[:2]
	exprs := []sliceExpr{buildSlice(r, xs, shapes[0]), buildSlice(r, xs, shapes[1])}
	var code strings.Builder
	fmt.Fprintf(&code, "%s = %s\n", name, pyIntList(xs))
	outs := make([]string, 0, len(exprs))
	notes := make([]string, 0, len(exprs))
	for _, e := range exprs {
		fmt.Fprintf(&code, "print(%s[%s])\n", name, e.text)
		outs = append(outs, e.result)
		notes = append(notes, fmt.Sprintf("`%s[%s]` → `%s`", name, e.text, e.result))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   "Что выведет код? Укажите вывод построчно — по одной строке на каждый `print`.",
		Code:   &itembank.CodeBlock{Lang: "python", Source: strings.TrimRight(code.String(), "\n")},
		Answer: itembank.Answer{Text: strings.Join(outs, "\n")},
		Explanation: "Срез `a[start:stop:step]` берёт элементы с индексами от start включительно до stop не включая, с шагом step. " +
			"Отрицательные индексы отсчитываются от конца (−1 — последний элемент), при отрицательном шаге обход идёт справа налево, " +
			"а границы по умолчанию меняются местами. Одиночный индекс возвращает элемент, а не список. Здесь: " +
			strings.Join(notes, "; ") + ". Типичная ошибка — включать правую границу или путать направление при отрицательном шаге.",
	}, nil
}

// generatorExhaust — генераторное выражение исчерпывается после первого обхода,
// в отличие от списка.
type generatorExhaust struct{}

func (generatorExhaust) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.python_core.generator-exhaust.v1", Spec: itembank.SpecDataML, Topic: "python_core",
		Grade: itembank.GradeMiddle, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.3, B: 0.2, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"python"},
	}
}

type pyExpr struct {
	text string
	fn   func(int) int
}

var genExprs = []pyExpr{
	{"x * 2", func(x int) int { return 2 * x }},
	{"x * x", func(x int) int { return x * x }},
	{"x - 1", func(x int) int { return x - 1 }},
	{"x + 10", func(x int) int { return x + 10 }},
}

// genOpSequences — последовательности операций над генератором: next нельзя вызывать
// после операции, которая исчерпала генератор.
var genOpSequences = [][]string{
	{"next", "sum"}, {"next", "list"}, {"sum", "sum"}, {"list", "list"},
	{"next", "next", "sum"}, {"list", "sum"}, {"sum", "list"},
}

func sumInts(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}

func (f generatorExhaust) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A11)
	src := itembank.Pick(r, []string{"nums", "data", "values", "items"})
	gname := itembank.Pick(r, []string{"gen", "g", "it", "stream"})
	expr := itembank.Pick(r, genExprs)
	condKind := r.IntN(4)
	var nums, seq []int
	var condText string
	for len(seq) < 3 || len(seq) == len(nums) {
		n := itembank.IntBetween(r, 6, 8)
		nums = make([]int, n)
		for i := range nums {
			nums[i] = itembank.IntBetween(r, 1, 20)
		}
		var cond func(int) bool
		switch condKind {
		case 0:
			condText, cond = "x % 2 == 0", func(x int) bool { return x%2 == 0 }
		case 1:
			condText, cond = "x % 2 == 1", func(x int) bool { return x%2 == 1 }
		case 2:
			t := itembank.IntBetween(r, 5, 12)
			condText, cond = fmt.Sprintf("x > %d", t), func(x int) bool { return x > t }
		default:
			condText, cond = "x % 3 == 0", func(x int) bool { return x%3 == 0 }
		}
		seq = seq[:0]
		for _, x := range nums {
			if cond(x) {
				seq = append(seq, expr.fn(x))
			}
		}
	}
	isList := r.IntN(4) == 0
	var code strings.Builder
	fmt.Fprintf(&code, "%s = %s\n", src, pyIntList(nums))
	var outs []string
	var explain string
	if isList {
		fmt.Fprintf(&code, "%s = [%s for x in %s if %s]\n", gname, expr.text, src, condText)
		switch r.IntN(3) {
		case 0:
			fmt.Fprintf(&code, "print(sum(%s))\nprint(sum(%s))\n", gname, gname)
			outs = []string{strconv.Itoa(sumInts(seq)), strconv.Itoa(sumInts(seq))}
		case 1:
			fmt.Fprintf(&code, "print(len(%s))\nprint(%s)\n", gname, gname)
			outs = []string{strconv.Itoa(len(seq)), pyIntList(seq)}
		default:
			fmt.Fprintf(&code, "print(%s)\nprint(sum(%s))\n", gname, gname)
			outs = []string{pyIntList(seq), strconv.Itoa(sumInts(seq))}
		}
		explain = fmt.Sprintf("Квадратные скобки создают списковое включение: список %s вычисляется сразу и целиком, его можно обходить сколько угодно раз, "+
			"поэтому повторные вызовы дают одинаковый результат. Ловушка — спутать его с генераторным выражением в круглых скобках, которое исчерпывается после первого обхода.",
			pyIntList(seq))
	} else {
		fmt.Fprintf(&code, "%s = (%s for x in %s if %s)\n", gname, expr.text, src, condText)
		rest := append([]int{}, seq...)
		for _, op := range itembank.Pick(r, genOpSequences) {
			switch op {
			case "next":
				fmt.Fprintf(&code, "print(next(%s))\n", gname)
				outs = append(outs, strconv.Itoa(rest[0]))
				rest = rest[1:]
			case "sum":
				fmt.Fprintf(&code, "print(sum(%s))\n", gname)
				outs = append(outs, strconv.Itoa(sumInts(rest)))
				rest = nil
			default:
				fmt.Fprintf(&code, "print(list(%s))\n", gname)
				outs = append(outs, pyIntList(rest))
				rest = nil
			}
		}
		explain = fmt.Sprintf("Генераторное выражение в круглых скобках — одноразовый итератор, который лениво выдаёт %s. "+
			"`next` забирает очередной элемент, `sum` и `list` дочитывают оставшиеся и исчерпывают генератор: после этого `sum` возвращает 0, а `list` — пустой список. "+
			"Типичная ошибка — считать, что генератор можно обойти повторно, как список.", pyIntList(seq))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        "Что выведет код? Укажите вывод построчно — по одной строке на каждый `print`.",
		Code:        &itembank.CodeBlock{Lang: "python", Source: strings.TrimRight(code.String(), "\n")},
		Answer:      itembank.Answer{Text: strings.Join(outs, "\n")},
		Explanation: explain,
	}, nil
}

// mutableDefault — изменяемое значение по умолчанию разделяется между вызовами.
type mutableDefault struct{}

func (mutableDefault) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.python_core.mutable-default.v1", Spec: itembank.SpecDataML, Topic: "python_core",
		Grade: itembank.GradeSenior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.4, B: 0.8, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"python"},
	}
}

var resultVarSets = [][]string{
	{"a", "b", "c", "d"}, {"r1", "r2", "r3", "r4"}, {"first", "second", "third", "fourth"},
}

func (f mutableDefault) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A12)
	fn := itembank.Pick(r, []string{"add", "collect", "push", "track", "register"})
	param := itembank.Pick(r, []string{"item", "value", "x", "elem"})
	acc := itembank.Pick(r, []string{"bucket", "acc", "items", "store"})
	fixed := r.IntN(3) == 0
	nCalls := itembank.IntBetween(r, 3, 4)
	explicitPos := r.IntN(nCalls)
	vars := itembank.Pick(r, resultVarSets)
	vals := make([]int, 0, nCalls)
	seen := map[int]bool{}
	for len(vals) < nCalls {
		v := itembank.IntBetween(r, 1, 99)
		if !seen[v] {
			seen[v] = true
			vals = append(vals, v)
		}
	}
	var code strings.Builder
	if fixed {
		fmt.Fprintf(&code, "def %s(%s, %s=None):\n    if %s is None:\n        %s = []\n    %s.append(%s)\n    return %s\n\n",
			fn, param, acc, acc, acc, acc, param, acc)
	} else {
		fmt.Fprintf(&code, "def %s(%s, %s=[]):\n    %s.append(%s)\n    return %s\n\n", fn, param, acc, acc, param, acc)
	}
	var shared []int
	var defaults []int // индексы вызовов без явного списка
	for i := 0; i < nCalls; i++ {
		if i == explicitPos {
			fmt.Fprintf(&code, "%s = %s(%d, [])\n", vars[i], fn, vals[i])
			continue
		}
		fmt.Fprintf(&code, "%s = %s(%d)\n", vars[i], fn, vals[i])
		shared = append(shared, vals[i])
		defaults = append(defaults, i)
	}
	d1, d2 := defaults[0], defaults[len(defaults)-1]
	show := func(i int) string {
		if fixed {
			return pyIntList([]int{vals[i]})
		}
		return pyIntList(shared)
	}
	outs := []string{show(d1), show(d2)}
	fmt.Fprintf(&code, "print(%s)\nprint(%s)\nprint(%s is %s)\n", vars[d1], vars[d2], vars[d1], vars[d2])
	if fixed {
		outs = append(outs, "False")
	} else {
		outs = append(outs, "True")
	}
	if r.IntN(2) == 0 {
		fmt.Fprintf(&code, "print(len(%s))\n", vars[explicitPos])
		outs = append(outs, "1")
	} else {
		fmt.Fprintf(&code, "print(%s)\n", vars[explicitPos])
		outs = append(outs, pyIntList([]int{vals[explicitPos]}))
	}
	explain := fmt.Sprintf("Значение по умолчанию вычисляется один раз при определении функции, поэтому все вызовы без второго аргумента "+
		"дописывают в один и тот же список. `%s` и `%s` — ссылки на этот объект, и `print` показывает его текущее содержимое %s, а не состояние на момент вызова; "+
		"`%s is %s` даёт True. Явно переданный `[]` — отдельный объект с одним элементом. Типичная ошибка — ожидать `%s` для первого вызова.",
		vars[d1], vars[d2], pyIntList(shared), vars[d1], vars[d2], pyIntList([]int{vals[d1]}))
	if fixed {
		explain = fmt.Sprintf("Идиома `%s=None` с созданием списка внутри функции даёт новый список на каждый вызов: `%s` содержит только %s, "+
			"`%s` — только %s, `%s is %s` даёт False. Так исправляют классическую ловушку изменяемого значения по умолчанию, при которой список накапливал бы все элементы %s.",
			acc, vars[d1], pyIntList([]int{vals[d1]}), vars[d2], pyIntList([]int{vals[d2]}), vars[d1], vars[d2], pyIntList(shared))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        "Что выведет код? Укажите вывод построчно — по одной строке на каждый `print`.",
		Code:        &itembank.CodeBlock{Lang: "python", Source: strings.TrimRight(code.String(), "\n")},
		Answer:      itembank.Answer{Text: strings.Join(outs, "\n")},
		Explanation: explain,
	}, nil
}
