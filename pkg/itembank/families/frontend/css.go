package frontend

import (
	"fmt"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(flexItemWidth{})
	itembank.Register(boxModelWidth{})
	itembank.Register(gridAutoPlacement{})
	itembank.Register(marginCollapse{})
}

// flexItemWidth — ширина flex-элемента после распределения свободного места по flex-grow.
type flexItemWidth struct{}

func (flexItemWidth) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.css_layout.flex-item-width.v1", Spec: itembank.SpecFrontend, Topic: "css_layout",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.0, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"css", "flexbox"},
	}
}

func (f flexItemWidth) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF201)
	n := itembank.IntBetween(r, 3, 4)
	names := itembank.Pick(r, [][]string{
		{"a", "b", "c", "d"}, {"left", "center", "right", "extra"}, {"nav", "main", "aside", "tools"}, {"col-1", "col-2", "col-3", "col-4"},
	})[:n]
	container := itembank.Pick(r, []string{"row", "toolbar", "layout", "panel"})
	basis := make([]int, n)
	grow := make([]int, n)
	sumBasis, sumGrow := 0, 0
	for sumGrow == 0 {
		sumBasis, sumGrow = 0, 0
		for i := range basis {
			basis[i] = itembank.Pick(r, []int{100, 120, 150, 200, 250})
			grow[i] = itembank.Pick(r, []int{0, 1, 1, 2, 3})
			sumBasis += basis[i]
			sumGrow += grow[i]
		}
	}
	gap := itembank.Pick(r, []int{0, 0, 10, 20, 30})
	// Подбираем ширину контейнера так, чтобы свободное место делилось нацело.
	free := itembank.IntBetween(r, 10, 30) * 10
	free -= free % sumGrow
	if free == 0 {
		free = sumGrow * 10
	}
	width := sumBasis + gap*(n-1) + free
	asked := r.IntN(n)
	answer := basis[asked] + free*grow[asked]/sumGrow

	rules := []string{fmt.Sprintf(".%s { display: flex; width: %dpx; }", container, width)}
	if gap > 0 {
		rules[0] = fmt.Sprintf(".%s { display: flex; width: %dpx; gap: %dpx; }", container, width, gap)
	}
	shorthand := r.IntN(2) == 0
	for i := range names {
		if shorthand {
			rules = append(rules, fmt.Sprintf(".%s { flex: %d 1 %dpx; }", names[i], grow[i], basis[i]))
		} else {
			rules = append(rules, fmt.Sprintf(".%s { flex-grow: %d; flex-basis: %dpx; }", names[i], grow[i], basis[i]))
		}
	}
	gapNote := ""
	if gap > 0 {
		gapNote = fmt.Sprintf(" и %d промежутков по %dpx", n-1, gap)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Элементы `.%s` лежат в контейнере `.%s` в указанном порядке, другие стили не заданы, "+
			"содержимое помещается в базовую ширину. Какой будет ширина элемента `.%s` в пикселях?",
			strings.Join(names, "`, `."), container, names[asked]),
		Code:   &itembank.CodeBlock{Lang: "css", Source: strings.Join(rules, "\n")},
		Answer: itembank.Answer{Number: float64(answer), Tolerance: 0.5},
		Explanation: fmt.Sprintf("Свободное место = %d − сумма базисов %d%s = %d px. Оно делится пропорционально flex-grow "+
			"(сумма %d): `.%s` получает %d + %d·%d/%d = %d px. Типичная ошибка — делить всю ширину контейнера "+
			"пропорционально grow, забывая про базис и gap.", width, sumBasis, gapNote, free, sumGrow, names[asked],
			basis[asked], free, grow[asked], sumGrow, answer),
	}, nil
}

// boxModelWidth — занимаемая ширина элемента при content-box и border-box.
type boxModelWidth struct{}

func (boxModelWidth) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.css_layout.box-model-width.v1", Spec: itembank.SpecFrontend, Topic: "css_layout",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: -0.7, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"css", "box-model"},
	}
}

func (f boxModelWidth) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF202)
	class := itembank.Pick(r, []string{"card", "panel", "box", "banner", "tile"})
	width := itembank.IntBetween(r, 20, 60) * 10
	border := itembank.IntBetween(r, 1, 4)
	borderBox := r.IntN(2) == 0
	// Разные формы записи padding: горизонтальный отступ приходится извлекать из шорткода.
	var padding string
	var padH int
	switch r.IntN(4) {
	case 0:
		p := itembank.IntBetween(r, 1, 4) * 8
		padding, padH = fmt.Sprintf("%dpx", p), 2*p
	case 1:
		v, h := itembank.IntBetween(r, 1, 3)*4, itembank.IntBetween(r, 2, 6)*4
		padding, padH = fmt.Sprintf("%dpx %dpx", v, h), 2*h
	case 2:
		t, h, b := itembank.IntBetween(r, 1, 3)*4, itembank.IntBetween(r, 2, 6)*4, itembank.IntBetween(r, 1, 3)*4
		padding, padH = fmt.Sprintf("%dpx %dpx %dpx", t, h, b), 2*h
	default:
		t, rt, b, l := itembank.IntBetween(r, 1, 3)*4, itembank.IntBetween(r, 2, 6)*4, itembank.IntBetween(r, 1, 3)*4, itembank.IntBetween(r, 2, 6)*4
		padding, padH = fmt.Sprintf("%dpx %dpx %dpx %dpx", t, rt, b, l), rt+l
	}
	margin := itembank.IntBetween(r, 1, 5) * 4
	sizing := "content-box"
	if borderBox {
		sizing = "border-box"
	}
	code := fmt.Sprintf(".%s {\n  box-sizing: %s;\n  width: %dpx;\n  padding: %s;\n  border: %dpx solid #333;\n  margin: 0 %dpx;\n}",
		class, sizing, width, padding, border, margin)
	askOuter := r.IntN(2) == 0
	var answer int
	var question, explain string
	outer, content := width+padH+2*border, width
	if borderBox {
		outer, content = width, width-padH-2*border
	}
	if askOuter {
		answer = outer
		question = fmt.Sprintf("Сколько пикселей по горизонтали займёт рамка элемента `.%s` (значение `offsetWidth`)?", class)
	} else {
		answer = content
		question = fmt.Sprintf("Какой будет ширина области содержимого элемента `.%s` в пикселях?", class)
	}
	if borderBox {
		explain = fmt.Sprintf("При `border-box` свойство width задаёт ширину вместе с padding и border: offsetWidth = %d, "+
			"а содержимому остаётся %d − %d (padding) − %d (border) = %d px.", width, width, padH, 2*border, content)
	} else {
		explain = fmt.Sprintf("При `content-box` width — это только содержимое (%d px), а padding (%d) и border (%d) добавляются снаружи: "+
			"offsetWidth = %d px.", width, padH, 2*border, outer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        question,
		Code:        &itembank.CodeBlock{Lang: "css", Source: code},
		Answer:      itembank.Answer{Number: float64(answer), Tolerance: 0},
		Explanation: explain + " Margin в размер элемента не входит ни в одной модели — он влияет только на расстояние до соседей.",
	}, nil
}

// gridAutoPlacement — где окажется элемент при автоматическом размещении в grid с span.
type gridAutoPlacement struct{}

func (gridAutoPlacement) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.css_layout.grid-auto-placement.v1", Spec: itembank.SpecFrontend, Topic: "css_layout",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.7, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"css", "grid"},
	}
}

type gridItem struct {
	span     int
	fullRow  bool
	row, col int
}

// placeSparse реализует алгоритм авторазмещения CSS Grid (grid-auto-flow: row, без dense).
func placeSparse(items []gridItem, cols int) int {
	occupied := map[[2]int]bool{}
	fits := func(row, col, span int) bool {
		for c := col; c < col+span; c++ {
			if occupied[[2]int{row, c}] {
				return false
			}
		}
		return true
	}
	mark := func(row, col, span int) {
		for c := col; c < col+span; c++ {
			occupied[[2]int{row, c}] = true
		}
	}
	curRow, curCol, rows := 1, 1, 1
	for i := range items {
		it := &items[i]
		span := it.span
		if it.fullRow {
			span = cols
			if curCol > 1 {
				curRow++
			}
			curCol = 1
			for !fits(curRow, 1, span) {
				curRow++
			}
		} else {
			for curCol+span-1 > cols || !fits(curRow, curCol, span) {
				curCol++
				if curCol+span-1 > cols {
					curRow++
					curCol = 1
				}
			}
		}
		it.row, it.col = curRow, curCol
		mark(curRow, curCol, span)
		if curRow > rows {
			rows = curRow
		}
	}
	return rows
}

func (f gridAutoPlacement) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF203)
	cols := itembank.IntBetween(r, 3, 4)
	n := itembank.IntBetween(r, 5, 7)
	items := make([]gridItem, n)
	for i := range items {
		items[i].span = 1
	}
	// 2–3 элемента со span, возможно один на всю строку.
	for _, i := range itembank.PickN(r, []int{0, 1, 2, 3, 4, 5, 6}[:n], itembank.IntBetween(r, 2, 3)) {
		items[i].span = itembank.IntBetween(r, 2, cols-1)
	}
	if r.IntN(2) == 0 {
		items[itembank.IntBetween(r, 1, n-2)].fullRow = true
	}
	rows := placeSparse(items, cols)
	rules := []string{fmt.Sprintf(".grid { display: grid; grid-template-columns: repeat(%d, 1fr); }", cols)}
	for i, it := range items {
		switch {
		case it.fullRow:
			rules = append(rules, fmt.Sprintf(".item-%d { grid-column: 1 / -1; }", i+1))
		case it.span > 1:
			rules = append(rules, fmt.Sprintf(".item-%d { grid-column: span %d; }", i+1, it.span))
		}
	}
	var layout []string
	for row := 1; row <= rows; row++ {
		var inRow []string
		for i, it := range items {
			if it.row == row {
				inRow = append(inRow, fmt.Sprintf("item-%d", i+1))
			}
		}
		layout = append(layout, fmt.Sprintf("строка %d: %s", row, strings.Join(inRow, ", ")))
	}
	askRows := r.IntN(3) == 0
	var stem string
	var answer int
	if askRows {
		answer = rows
		stem = fmt.Sprintf("В контейнере `.grid` лежат %d элементов `.item-1` … `.item-%d` в порядке номеров; остальные стили не заданы. "+
			"Сколько строк займёт сетка?", n, n)
	} else {
		asked := itembank.IntBetween(r, 2, n-1)
		answer = items[asked].row
		stem = fmt.Sprintf("В контейнере `.grid` лежат %d элементов `.item-1` … `.item-%d` в порядке номеров; остальные стили не заданы. "+
			"В какой строке сетки окажется `.item-%d`?", n, n, asked+1)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "css", Source: strings.Join(rules, "\n")},
		Answer: itembank.Answer{Number: float64(answer), Tolerance: 0},
		Explanation: "Без `grid-auto-flow: dense` курсор авторазмещения движется только вперёд: элемент со span, не помещающийся в оставшиеся " +
			"колонки, переносится на новую строку, а пропущенные ячейки остаются пустыми; `grid-column: 1 / -1` начинает новую строку, " +
			"если текущая уже занята. Размещение: " + strings.Join(layout, "; ") + ".",
	}, nil
}

// marginCollapse — расстояние между соседями: схлопывание вертикальных margin в блочном потоке.
type marginCollapse struct{}

func (marginCollapse) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.css_layout.margin-collapse.v1", Spec: itembank.SpecFrontend, Topic: "css_layout",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.4, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"css", "box-model"},
	}
}

func (f marginCollapse) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF204)
	pair := itembank.Pick(r, [][]string{{"title", "text"}, {"header", "content"}, {"intro", "body"}, {"first", "second"}, {"alert", "form"}})
	parent := itembank.Pick(r, []string{"section", "page", "wrapper", "article"})
	x := itembank.IntBetween(r, 2, 12) * 4
	y := x
	for y == x {
		y = itembank.IntBetween(r, 2, 12) * 4
	}
	parentDisplay := itembank.Pick(r, []string{"block", "block", "flex", "grid"})
	var parentRule string
	switch parentDisplay {
	case "flex":
		parentRule = fmt.Sprintf(".%s { display: flex; flex-direction: column; }", parent)
	case "grid":
		parentRule = fmt.Sprintf(".%s { display: grid; grid-template-columns: 1fr; }", parent)
	default:
		parentRule = fmt.Sprintf(".%s { display: block; }", parent)
	}
	aRule := fmt.Sprintf(".%s { margin-bottom: %dpx; }", pair[0], x)
	bRule := fmt.Sprintf(".%s { margin-top: %dpx; }", pair[1], y)
	if r.IntN(2) == 0 {
		aRule = fmt.Sprintf(".%s { margin: %dpx 0; }", pair[0], x)
		bRule = fmt.Sprintf(".%s { margin: %dpx 0; }", pair[1], y)
	}
	answer := x + y
	if parentDisplay == "block" {
		answer = max(x, y)
	}
	explain := fmt.Sprintf("В блочном потоке соседние вертикальные margin схлопываются: расстояние равно большему из них, max(%d, %d) = %d px, "+
		"а не сумме.", x, y, answer)
	if parentDisplay != "block" {
		explain = fmt.Sprintf("Внутри %s-контейнера элементы — не блоки в обычном потоке, margin не схлопываются и складываются: "+
			"%d + %d = %d px. Схлопывание работает только в блочном форматирующем контексте.", parentDisplay, x, y, answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Блоки `<div class=\"%s\">` и `<div class=\"%s\">` — соседи внутри `<div class=\"%s\">`, других стилей и "+
			"содержимого между ними нет. Каким будет расстояние в пикселях между нижним краем `.%s` и верхним краем `.%s`?",
			pair[0], pair[1], parent, pair[0], pair[1]),
		Code:        &itembank.CodeBlock{Lang: "css", Source: strings.Join([]string{parentRule, aRule, bRule}, "\n")},
		Answer:      itembank.Answer{Number: float64(answer), Tolerance: 0},
		Explanation: explain,
	}, nil
}
