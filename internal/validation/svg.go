package validation

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Палитра ФСП для графиков отчёта.
var palette = []string{"#402FFF", "#EC1D35", "#3D4E9C", "#624190", "#0F8A5F", "#B45309"}

const (
	svgFont = "JetBrains Mono, Consolas, monospace"
	// charW — ширина знака моноширинного шрифта 12 px (0,6 em у JetBrains Mono): по ней раскладываются подписи.
	charW = 7.2
)

// Series — серия для линейного графика.
type Series struct {
	Name   string
	Values []float64
}

// LineChart рисует линии по общей оси X. Деления осей — «круглые» числа, точки данных отмечены, если их немного.
func LineChart(title, xLabel, yLabel string, xs []float64, series []Series) string {
	w, h := 640.0, 330.0
	pl, pr, pt, pb := 60.0, 20.0, 60.0, 46.0
	ymin, ymax := 0.0, math.Inf(-1)
	for _, s := range series {
		for _, v := range s.Values {
			ymin, ymax = math.Min(ymin, v), math.Max(ymax, v)
		}
	}
	yt := niceTicks(ymin, ymax, 5)
	xt := niceTicks(xs[0], xs[len(xs)-1], 6)
	ylo, yhi := yt[0], yt[len(yt)-1]
	xlo, xhi := xt[0], xt[len(xt)-1]
	x := func(v float64) float64 { return pl + (v-xlo)/(xhi-xlo)*(w-pl-pr) }
	y := func(v float64) float64 { return pt + (yhi-v)/(yhi-ylo)*(h-pt-pb) }
	var b strings.Builder
	header(&b, w, h, title)
	legend(&b, 32, series)
	for _, v := range yt {
		fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" stroke="#E2E3E5"/><text x="%.1f" y="%.1f" text-anchor="end" fill="#6B6E78">%s</text>`, pl, w-pr, y(v), y(v), pl-8, y(v)+4, fmtTick(v))
	}
	for _, v := range xt {
		fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" stroke="#C8C9CA"/><text x="%.1f" y="%.1f" text-anchor="middle" fill="#6B6E78">%s</text>`, x(v), x(v), h-pb, h-pb+4, x(v), h-pb+18, fmtTick(v))
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#6B6E78">%s</text>`, (pl+w-pr)/2, h-8, esc(xLabel))
	yAxisLabel(&b, (pt+h-pb)/2, yLabel)
	marks := len(xs) <= 12
	for si, s := range series {
		color := palette[si%len(palette)]
		var d strings.Builder
		for i, v := range s.Values {
			cmd := "L"
			if i == 0 {
				cmd = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f ", cmd, x(xs[i]), y(v))
		}
		fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="2.2" stroke-linejoin="round"/>`, strings.TrimSpace(d.String()), color)
		if marks {
			for i, v := range s.Values {
				fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3.2" fill="#ffffff" stroke="%s" stroke-width="2"/>`, x(xs[i]), y(v), color)
			}
		}
	}
	b.WriteString("</svg>")
	return b.String()
}

// BarChart — вертикальные столбцы с подписями значений; несколько серий — группами.
// maxY > 0 задаёт верх оси (для долей — 1,05: запас под подписи над столбцами), деления ставятся не выше него.
// Подписи групп переносятся по словам, ширина группы подстраивается под длину подписей.
func BarChart(title, yLabel string, labels []string, series []Series, maxY float64) string {
	n, k := float64(len(labels)), float64(len(series))
	longest := 0
	for _, l := range labels {
		longest = max(longest, utf8.RuneCountInString(l))
	}
	pl, pr := 60.0, 16.0
	group := math.Max(k*48, float64(min(longest, 19))*charW+12)
	w := math.Max(480, math.Min(760, n*group+pl+pr))
	group = (w - pl - pr) / n
	perLine := int((group - 10) / charW)
	wrapped := make([][]string, len(labels))
	lines := 1
	for i, l := range labels {
		wrapped[i] = wrap(l, perLine, 3)
		lines = max(lines, len(wrapped[i]))
	}
	pt := 40.0
	if len(series) > 1 {
		pt = 60
	}
	pb := 14 + 15*float64(lines)
	h := pt + 220 + pb
	dataMax := 0.0
	for _, s := range series {
		for _, v := range s.Values {
			dataMax = math.Max(dataMax, v)
		}
	}
	top := maxY
	if top <= 0 {
		auto := niceTicks(0, dataMax, 5)
		top = auto[len(auto)-1]
		if top < dataMax*1.08 {
			top = dataMax * 1.12
		}
	}
	var ticks []float64
	for _, v := range niceTicks(0, top, 5) {
		if v <= top+1e-9 {
			ticks = append(ticks, v)
		}
	}
	y := func(v float64) float64 { return pt + (top-v)/top*(h-pt-pb) }
	bw := math.Min((group-16)/k, 56)
	var b strings.Builder
	header(&b, w, h, title)
	if len(series) > 1 {
		legend(&b, 32, series)
	}
	for _, v := range ticks {
		fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" stroke="#E2E3E5"/><text x="%.1f" y="%.1f" text-anchor="end" fill="#6B6E78">%s</text>`, pl, w-pr, y(v), y(v), pl-8, y(v)+4, fmtTick(v))
	}
	yAxisLabel(&b, (pt+h-pb)/2, yLabel)
	for li := range labels {
		left := pl + float64(li)*group + (group-bw*k)/2
		for si, s := range series {
			v := 0.0
			if li < len(s.Values) {
				v = s.Values[li]
			}
			bx := left + float64(si)*bw
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" rx="2"/>`, bx, y(v), bw-2, h-pb-y(v), palette[si%len(palette)])
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="10" fill="#1B1C21">%s</text>`, bx+(bw-2)/2, y(v)-4, fmtNum(v))
		}
		cx := pl + float64(li)*group + group/2
		for j, line := range wrapped[li] {
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#3B3D45">%s</text>`, cx, h-pb+18+15*float64(j), esc(line))
		}
	}
	b.WriteString("</svg>")
	return b.String()
}

// Heatmap — матрица ошибок 3×3 с подписями.
func Heatmap(title string, m Confusion) string {
	w, h := 360.0, 300.0
	cell := 70.0
	x0, y0 := 90.0, 60.0
	total := 0
	maxV := 1
	for i := range m {
		for j := range m[i] {
			total += m[i][j]
			if m[i][j] > maxV {
				maxV = m[i][j]
			}
		}
	}
	var b strings.Builder
	header(&b, w, h, title)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#6B6E78">оценка →</text>`, x0+cell*1.5, y0-24)
	for j, lbl := range Bands {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#3B3D45">%s</text>`, x0+cell*float64(j)+cell/2, y0-8, lbl)
	}
	for i := range m {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="end" fill="#3B3D45">%s</text>`, x0-8, y0+cell*float64(i)+cell/2+4, Bands[i])
		for j := range m[i] {
			alpha := 0.08 + 0.85*float64(m[i][j])/float64(maxV)
			fill := "#402FFF"
			if i != j {
				fill = "#EC1D35"
			}
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" fill-opacity="%.2f" stroke="#ffffff"/>`, x0+cell*float64(j), y0+cell*float64(i), cell, cell, fill, alpha)
			num, sub := "#1B1C21", "#3B3D45"
			if alpha > 0.55 { // на насыщенной заливке — светлый текст
				num, sub = "#FFFFFF", "#E6E4FF"
			}
			pct := 0.0
			if total > 0 {
				pct = 100 * float64(m[i][j]) / float64(total)
			}
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-weight="600" fill="%s">%d</text><text x="%.1f" y="%.1f" text-anchor="middle" font-size="10" fill="%s">%.0f %%</text>`, x0+cell*float64(j)+cell/2, y0+cell*float64(i)+cell/2, num, m[i][j], x0+cell*float64(j)+cell/2, y0+cell*float64(i)+cell/2+14, sub, pct)
		}
	}
	fmt.Fprintf(&b, `<text x="12" y="%.1f" font-size="11" fill="#6B6E78">строки — истинный грейд, столбцы — полоса оценки θ</text>`, h-10)
	b.WriteString("</svg>")
	return b.String()
}

// header открывает SVG: белый фон и заголовок.
func header(b *strings.Builder, w, h float64, title string) {
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" font-family="%s" font-size="12">`, w, h, w, h, svgFont)
	fmt.Fprintf(b, `<rect width="100%%" height="100%%" fill="#ffffff"/><text x="12" y="20" font-size="14" font-weight="600" fill="#1B1C21">%s</text>`, esc(title))
}

// legend — подписи серий отдельной строкой под заголовком; место под пункт — по длине имени.
func legend(b *strings.Builder, y float64, series []Series) {
	x := 12.0
	for si, s := range series {
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="10" height="10" rx="2" fill="%s"/><text x="%.1f" y="%.1f" fill="#1B1C21">%s</text>`, x, y, palette[si%len(palette)], x+15, y+9, esc(s.Name))
		x += 15 + float64(utf8.RuneCountInString(s.Name))*charW + 22
	}
}

func yAxisLabel(b *strings.Builder, mid float64, label string) {
	fmt.Fprintf(b, `<text x="16" y="%.1f" transform="rotate(-90 16 %.1f)" text-anchor="middle" fill="#6B6E78">%s</text>`, mid, mid, esc(label))
}

// niceTicks — «круглые» деления оси: шаг 1, 2, 2,5 или 5 × 10^k, число интервалов — ближайшее к target
// (при равенстве — крупнее шаг). Деления покрывают [lo, hi] с запасом до ближайшего кратного шага.
func niceTicks(lo, hi float64, target int) []float64 {
	if hi <= lo {
		hi = lo + 1
	}
	mag := math.Pow(10, math.Floor(math.Log10((hi-lo)/float64(target))))
	step, best := 0.0, math.Inf(1)
	for _, k := range []float64{0.1, 1, 10} {
		for _, m := range []float64{1, 2, 2.5, 5} {
			s := m * k * mag
			n := math.Ceil(hi/s-1e-9) - math.Floor(lo/s+1e-9)
			if d := math.Abs(n - float64(target)); d <= best {
				step, best = s, d
			}
		}
	}
	from := math.Floor(lo/step+1e-9) * step
	count := int(math.Round((math.Ceil(hi/step-1e-9)*step - from) / step))
	ticks := make([]float64, 0, count+1)
	for i := 0; i <= count; i++ {
		ticks = append(ticks, math.Round((from+float64(i)*step)*1e9)/1e9)
	}
	return ticks
}

// fmtTick — деление оси без лишних нулей, с десятичной запятой и типографским минусом.
func fmtTick(v float64) string {
	v = math.Round(v*1e6) / 1e6
	if v == 0 {
		v = 0 // без «−0»
	}
	return ruNum(strconv.FormatFloat(v, 'f', -1, 64))
}

// fmtNum — значение над столбцом: целое без дробной части, иначе два знака.
func fmtNum(v float64) string {
	if math.Abs(v) >= 100 || v == math.Trunc(v) {
		return fmtTick(math.Round(v))
	}
	return ruNum(fmt.Sprintf("%.2f", v))
}

func ruNum(s string) string {
	return strings.NewReplacer(".", ",", "-", "−").Replace(s)
}

// wrap разбивает подпись по словам на строки до perLine знаков; что не влезло в maxLines строк — многоточие.
func wrap(s string, perLine, maxLines int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		switch {
		case cur == "":
			cur = word
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(word) <= perLine:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] += "…"
	}
	for i, l := range lines {
		lines[i] = truncate(l, perLine)
	}
	return lines
}

func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
