package pdftext

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// Ограничения обхода: защищают обработчик загрузки от «бомб» в сжатых потоках и зацикленных форм.
var limits = struct {
	ops, runs, pages, depth int
	budget                  time.Duration
}{ops: 2_000_000, runs: 200_000, pages: 50, depth: 4, budget: 4 * time.Second}

var errBudget = errors.New("pdftext: content budget exceeded")

type abort struct{ err error }

// matrix — аффинная матрица PDF [a b c d e f]; точка — вектор-строка: [x y 1] × M.
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func translate(tx, ty float64) matrix { return matrix{1, 0, 0, 1, tx, ty} }

func toMatrix(v []pdf.Value) matrix {
	var m matrix
	for i := range m {
		m[i] = v[i].Float64()
	}
	return m
}

// run — фрагмент текста, нарисованный одним оператором (или элементом массива TJ).
type run struct {
	x0, x1, y, size float64
	text            string
}

type gstate struct {
	ctm                        matrix
	tc, tw, th, tl, tfs, trise float64
	font                       *font
}

type walker struct {
	runs     []run
	ops      int
	deadline time.Time
}

func (w *walker) tick() {
	w.ops++
	if w.ops > limits.ops || (w.ops%4096 == 0 && time.Now().After(w.deadline)) {
		panic(abort{errBudget})
	}
}

// page обходит страницу и раскладывает текст по строкам. При исчерпании бюджета возвращает
// уже собранное вместе с errBudget; при прочих сбоях — ошибку (вызывающий переходит на запасной путь).
func (w *walker) page(page pdf.Value) (lines []string, err error) {
	w.runs = w.runs[:0]
	defer func() {
		if r := recover(); r != nil {
			if a, ok := r.(abort); ok {
				err = a.err
			} else {
				err = fmt.Errorf("pdftext: page: %v", r)
			}
		}
		if err == nil || errors.Is(err, errBudget) {
			lines = layout(w.runs)
		}
	}()
	contents := page.Key("Contents")
	if contents.IsNull() {
		return nil, nil
	}
	w.content(contents, inherited(page, "Resources"), identity, 0)
	return nil, nil
}

func (w *walker) content(strm, res pdf.Value, base matrix, depth int) {
	fonts := map[string]*font{}
	fontFor := func(name string) *font {
		if f, ok := fonts[name]; ok {
			return f
		}
		var f *font
		if fv := res.Key("Font").Key(name); fv.Kind() == pdf.Dict {
			f = newFont(fv)
		}
		fonts[name] = f
		return f
	}
	g := gstate{ctm: base, th: 1}
	var stack []gstate
	tm, tlm := identity, identity
	nextLine := func() {
		tlm = translate(0, -g.tl).mul(tlm)
		tm = tlm
	}
	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		w.tick()
		args := make([]pdf.Value, stk.Len())
		for i := len(args) - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}
		switch op {
		case "q":
			stack = append(stack, g)
		case "Q":
			if n := len(stack); n > 0 {
				g, stack = stack[n-1], stack[:n-1]
			}
		case "cm":
			if len(args) == 6 {
				g.ctm = toMatrix(args).mul(g.ctm)
			}
		case "BT":
			tm, tlm = identity, identity
		case "Tc", "Tw", "Tz", "TL", "Ts":
			if len(args) == 1 {
				setParam(&g, op, args[0].Float64())
			}
		case "Tf":
			if len(args) == 2 {
				g.font = fontFor(args[0].Name())
				g.tfs = args[1].Float64()
			}
		case "Td", "TD":
			if len(args) == 2 {
				if op == "TD" {
					g.tl = -args[1].Float64()
				}
				tlm = translate(args[0].Float64(), args[1].Float64()).mul(tlm)
				tm = tlm
			}
		case "Tm":
			if len(args) == 6 {
				tlm = toMatrix(args)
				tm = tlm
			}
		case "T*":
			nextLine()
		case "Tj":
			if len(args) == 1 {
				w.show(&g, &tm, args[0].RawString())
			}
		case "'":
			if len(args) == 1 {
				nextLine()
				w.show(&g, &tm, args[0].RawString())
			}
		case "\"":
			if len(args) == 3 {
				g.tw, g.tc = args[0].Float64(), args[1].Float64()
				nextLine()
				w.show(&g, &tm, args[2].RawString())
			}
		case "TJ":
			if len(args) == 1 {
				w.showArray(&g, &tm, args[0])
			}
		case "Do":
			if len(args) == 1 && depth < limits.depth {
				w.form(res, args[0].Name(), g.ctm, depth)
			}
		}
	})
}

func setParam(g *gstate, op string, v float64) {
	switch op {
	case "Tc":
		g.tc = v
	case "Tw":
		g.tw = v
	case "Tz":
		g.th = v / 100
	case "TL":
		g.tl = v
	case "Ts":
		g.trise = v
	}
}

// form обходит XObject-форму (в ней бывает весь текст страницы — так печатают некоторые драйверы).
func (w *walker) form(res pdf.Value, name string, ctm matrix, depth int) {
	x := res.Key("XObject").Key(name)
	if x.Kind() != pdf.Stream || x.Key("Subtype").Name() != "Form" {
		return
	}
	m := identity
	if mv := x.Key("Matrix"); mv.Len() == 6 {
		for i := range m {
			m[i] = mv.Index(i).Float64()
		}
	}
	xres := x.Key("Resources")
	if xres.Kind() != pdf.Dict {
		xres = res
	}
	w.content(x, xres, m.mul(ctm), depth+1)
}

// origin — точка начала текста в координатах страницы и кегль с учётом всех преобразований.
func origin(g *gstate, tm matrix) (x, y, size float64) {
	trm := matrix{g.tfs * g.th, 0, 0, g.tfs, 0, g.trise}.mul(tm).mul(g.ctm)
	return trm[4], trm[5], math.Hypot(trm[2], trm[3])
}

func (w *walker) showArray(g *gstate, tm *matrix, arr pdf.Value) {
	for i := 0; i < arr.Len(); i++ {
		el := arr.Index(i)
		if el.Kind() == pdf.String {
			w.show(g, tm, el.RawString())
			continue
		}
		*tm = translate(-el.Float64()/1000*g.tfs*g.th, 0).mul(*tm)
	}
}

func (w *walker) show(g *gstate, tm *matrix, raw string) {
	f := g.font
	if f == nil || raw == "" {
		return
	}
	x0, y, size := origin(g, *tm)
	var b strings.Builder
	for _, code := range f.codes(raw) {
		b.WriteString(f.text(code))
		tx := f.width(code)*g.tfs + g.tc
		if len(code) == 1 && code[0] == ' ' {
			tx += g.tw
		}
		*tm = translate(tx*g.th, 0).mul(*tm)
	}
	x1, _, _ := origin(g, *tm)
	if b.Len() == 0 {
		return
	}
	if len(w.runs) >= limits.runs {
		panic(abort{errBudget})
	}
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	w.runs = append(w.runs, run{x0: x0, x1: x1, y: y, size: size, text: b.String()})
}

// layout собирает фрагменты в строки: сверху вниз по базовой линии, внутри строки — слева направо.
// Пробел вставляется, если между фрагментами есть заметный зазор (так восстанавливаются слова,
// выставленные позиционированием без символа пробела). Повторная печать того же текста со сдвигом
// («жирный» наложением) отбрасывается.
func layout(runs []run) []string {
	if len(runs) == 0 {
		return nil
	}
	sorted := append([]run(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].y > sorted[j].y })
	var lines [][]run
	for _, r := range sorted {
		if n := len(lines); n > 0 {
			ref := lines[n-1][0]
			if ref.y-r.y <= 0.45*math.Max(math.Max(ref.size, r.size), 1) {
				lines[n-1] = append(lines[n-1], r)
				continue
			}
		}
		lines = append(lines, []run{r})
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		sort.SliceStable(l, func(i, j int) bool { return l[i].x0 < l[j].x0 })
		var b strings.Builder
		var prev *run
		for i := range l {
			r := &l[i]
			if prev != nil {
				sz := math.Max(math.Max(prev.size, r.size), 1)
				if r.text == prev.text && math.Abs(r.x0-prev.x0) < 0.3*sz {
					continue
				}
				if r.x0-prev.x1 > 0.15*sz {
					b.WriteByte(' ')
				}
			}
			b.WriteString(r.text)
			prev = r
		}
		out = append(out, b.String())
	}
	return out
}
