// Package common — общий блок заданий, доступный всем специализациям:
// алгоритмическая сложность, структуры данных, сети, git, HTTP.
package common

import (
	"fmt"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(loopComplexity{})
	itembank.Register(growthOrder{})
}

// loopComplexity — «какова временная сложность функции»: структура циклов выбирается
// случайно, ответ следует из структуры, код генерируется на одном из трёх языков.
type loopComplexity struct{}

func (loopComplexity) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.complexity.loop-complexity.v1", Spec: itembank.SpecCommon, Topic: "complexity",
		Grade: itembank.GradeJunior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: -0.7, C: 0.25}, TimeLimit: 90 * time.Second, Tags: []string{"big-o"},
	}
}

type loopShape struct {
	label   string
	explain string
	goCode  func(fn, v, acc string) string
	pyCode  func(fn, v, acc string) string
	jsCode  func(fn, v, acc string) string
}

var loopShapes = []loopShape{
	{
		label: "O(1)", explain: "Тело функции не содержит циклов, зависящих от n: число операций постоянно.",
		goCode: func(fn, v, acc string) string {
			return fmt.Sprintf("func %s(%s int) int {\n\t%s := %s * (%s + 1) / 2\n\treturn %s\n}", fn, v, acc, v, v, acc)
		},
		pyCode: func(fn, v, acc string) string {
			return fmt.Sprintf("def %s(%s):\n    %s = %s * (%s + 1) // 2\n    return %s", fn, v, acc, v, v, acc)
		},
		jsCode: func(fn, v, acc string) string {
			return fmt.Sprintf("function %s(%s) {\n  const %s = %s * (%s + 1) / 2;\n  return %s;\n}", fn, v, acc, v, v, acc)
		},
	},
	{
		label: "O(log n)", explain: "Переменная цикла каждый раз делится пополам, поэтому итераций примерно log₂ n.",
		goCode: func(fn, v, acc string) string {
			return fmt.Sprintf("func %s(%s int) int {\n\t%s := 0\n\tfor k := %s; k > 0; k /= 2 {\n\t\t%s += k\n\t}\n\treturn %s\n}", fn, v, acc, v, acc, acc)
		},
		pyCode: func(fn, v, acc string) string {
			return fmt.Sprintf("def %s(%s):\n    %s = 0\n    k = %s\n    while k > 0:\n        %s += k\n        k //= 2\n    return %s", fn, v, acc, v, acc, acc)
		},
		jsCode: func(fn, v, acc string) string {
			return fmt.Sprintf("function %s(%s) {\n  let %s = 0;\n  for (let k = %s; k > 0; k = Math.floor(k / 2)) {\n    %s += k;\n  }\n  return %s;\n}", fn, v, acc, v, acc, acc)
		},
	},
	{
		label: "O(n)", explain: "Один цикл с n итерациями и постоянной работой в теле даёт линейную сложность.",
		goCode: func(fn, v, acc string) string {
			return fmt.Sprintf("func %s(%s int) int {\n\t%s := 0\n\tfor i := 0; i < %s; i++ {\n\t\t%s += i\n\t}\n\treturn %s\n}", fn, v, acc, v, acc, acc)
		},
		pyCode: func(fn, v, acc string) string {
			return fmt.Sprintf("def %s(%s):\n    %s = 0\n    for i in range(%s):\n        %s += i\n    return %s", fn, v, acc, v, acc, acc)
		},
		jsCode: func(fn, v, acc string) string {
			return fmt.Sprintf("function %s(%s) {\n  let %s = 0;\n  for (let i = 0; i < %s; i++) {\n    %s += i;\n  }\n  return %s;\n}", fn, v, acc, v, acc, acc)
		},
	},
	{
		label: "O(n log n)", explain: "Внешний цикл выполняется n раз, внутренний — log₂ n раз (деление пополам): n·log n.",
		goCode: func(fn, v, acc string) string {
			return fmt.Sprintf("func %s(%s int) int {\n\t%s := 0\n\tfor i := 0; i < %s; i++ {\n\t\tfor k := %s; k > 0; k /= 2 {\n\t\t\t%s += k\n\t\t}\n\t}\n\treturn %s\n}", fn, v, acc, v, v, acc, acc)
		},
		pyCode: func(fn, v, acc string) string {
			return fmt.Sprintf("def %s(%s):\n    %s = 0\n    for i in range(%s):\n        k = %s\n        while k > 0:\n            %s += k\n            k //= 2\n    return %s", fn, v, acc, v, v, acc, acc)
		},
		jsCode: func(fn, v, acc string) string {
			return fmt.Sprintf("function %s(%s) {\n  let %s = 0;\n  for (let i = 0; i < %s; i++) {\n    for (let k = %s; k > 0; k = Math.floor(k / 2)) {\n      %s += k;\n    }\n  }\n  return %s;\n}", fn, v, acc, v, v, acc, acc)
		},
	},
	{
		label: "O(n²)", explain: "Два вложенных цикла по n итераций каждый: n·n операций.",
		goCode: func(fn, v, acc string) string {
			return fmt.Sprintf("func %s(%s int) int {\n\t%s := 0\n\tfor i := 0; i < %s; i++ {\n\t\tfor j := 0; j < %s; j++ {\n\t\t\t%s += i * j\n\t\t}\n\t}\n\treturn %s\n}", fn, v, acc, v, v, acc, acc)
		},
		pyCode: func(fn, v, acc string) string {
			return fmt.Sprintf("def %s(%s):\n    %s = 0\n    for i in range(%s):\n        for j in range(%s):\n            %s += i * j\n    return %s", fn, v, acc, v, v, acc, acc)
		},
		jsCode: func(fn, v, acc string) string {
			return fmt.Sprintf("function %s(%s) {\n  let %s = 0;\n  for (let i = 0; i < %s; i++) {\n    for (let j = 0; j < %s; j++) {\n      %s += i * j;\n    }\n  }\n  return %s;\n}", fn, v, acc, v, v, acc, acc)
		},
	},
	{
		label: "O(n³)", explain: "Три вложенных цикла по n итераций: n³ операций.",
		goCode: func(fn, v, acc string) string {
			return fmt.Sprintf("func %s(%s int) int {\n\t%s := 0\n\tfor i := 0; i < %s; i++ {\n\t\tfor j := 0; j < %s; j++ {\n\t\t\tfor k := 0; k < %s; k++ {\n\t\t\t\t%s += i + j + k\n\t\t\t}\n\t\t}\n\t}\n\treturn %s\n}", fn, v, acc, v, v, v, acc, acc)
		},
		pyCode: func(fn, v, acc string) string {
			return fmt.Sprintf("def %s(%s):\n    %s = 0\n    for i in range(%s):\n        for j in range(%s):\n            for k in range(%s):\n                %s += i + j + k\n    return %s", fn, v, acc, v, v, v, acc, acc)
		},
		jsCode: func(fn, v, acc string) string {
			return fmt.Sprintf("function %s(%s) {\n  let %s = 0;\n  for (let i = 0; i < %s; i++) {\n    for (let j = 0; j < %s; j++) {\n      for (let k = 0; k < %s; k++) {\n        %s += i + j + k;\n      }\n    }\n  }\n  return %s;\n}", fn, v, acc, v, v, v, acc, acc)
		},
	},
}

func (f loopComplexity) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE01)
	shape := itembank.Pick(r, loopShapes)
	fn := itembank.Ident(r, "func")
	v := itembank.Pick(r, []string{"n", "size", "count", "limit"})
	acc := itembank.Pick(r, []string{"total", "acc", "sum", "result"})
	lang := itembank.Ident(r, "lang")
	var code string
	switch lang {
	case "python":
		code = shape.pyCode(fn, v, acc)
	case "javascript":
		code = shape.jsCode(fn, v, acc)
	default:
		code = shape.goCode(fn, v, acc)
	}
	var distractors []string
	for _, s := range loopShapes {
		if s.label != shape.label {
			distractors = append(distractors, s.label)
		}
	}
	options, key := itembank.ShuffledOptions(r, shape.label, itembank.PickN(r, distractors, 3)...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    fmt.Sprintf("Какова временная сложность функции `%s` относительно `%s`?", fn, v),
		Code:    &itembank.CodeBlock{Lang: lang, Source: code},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: shape.explain + " Типичная ошибка — считать вложенность по отступам, а не по зависимости числа итераций от n: " +
			"цикл с делением пополам даёт логарифм, а не линейный множитель.",
	}, nil
}

// growthOrder — упорядочить функции по скорости роста.
type growthOrder struct{}

func (growthOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.complexity.growth-order.v1", Spec: itembank.SpecCommon, Topic: "complexity",
		Grade: itembank.GradeMiddle, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.1, B: 0.0, C: 0.0}, TimeLimit: 90 * time.Second, Tags: []string{"big-o"},
	}
}

type growth struct {
	rank  int
	exprs []string
}

var growths = []growth{
	{1, []string{"log₂ n", "100·log n", "ln n + 5"}},
	{2, []string{"√n", "3·√n", "n^0.5"}},
	{3, []string{"n", "3n + 10", "5n"}},
	{4, []string{"n·log n", "n·log n + n", "2n·log₂ n"}},
	{5, []string{"n²", "n²/2", "n² + n"}},
	{6, []string{"n³", "n³ + n²", "0.1·n³"}},
	{7, []string{"2ⁿ", "2ⁿ + n²", "3·2ⁿ"}},
	{8, []string{"n!", "n! + 2ⁿ", "(n+1)!"}},
}

func (f growthOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE02)
	chosen := itembank.PickN(r, growths, 4)
	// Сортируем по рангу: это и есть верный порядок.
	for i := 0; i < len(chosen); i++ {
		for j := i + 1; j < len(chosen); j++ {
			if chosen[j].rank < chosen[i].rank {
				chosen[i], chosen[j] = chosen[j], chosen[i]
			}
		}
	}
	inOrder := make([]string, 0, 4)
	for _, g := range chosen {
		inOrder = append(inOrder, itembank.Pick(r, g.exprs))
	}
	options, order := itembank.OrderingOptions(r, inOrder)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    "Расположите функции по возрастанию скорости роста при n → ∞ (от самой медленной к самой быстрой): " + strings.Join(inOrder, ", ") + ".",
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: "Порядок классов роста: log n < √n < n < n·log n < n² < n³ < 2ⁿ < n!. Константы и младшие слагаемые " +
			"не влияют на асимптотику, поэтому 3n + 10 растёт так же, как n, а n² + n — как n².",
	}, nil
}
