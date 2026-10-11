package itembank

import (
	"math"
	"sort"
	"strings"
)

// Check сравнивает ответ кандидата с каноническим по формату семейства.
func Check(format Format, ans Answer, sub Submission) bool {
	switch format {
	case FormatSingleChoice:
		return len(sub.Keys) == 1 && len(ans.Keys) == 1 && sub.Keys[0] == ans.Keys[0]
	case FormatMultiChoice:
		return sameSet(ans.Keys, sub.Keys)
	case FormatNumeric:
		if sub.Number == nil {
			return false
		}
		tol := ans.Tolerance
		if tol < 1e-9 {
			tol = 1e-9
		}
		return math.Abs(*sub.Number-ans.Number) <= tol
	case FormatShortText, FormatCodeOutput:
		return NormalizeText(sub.Text) == NormalizeText(ans.Text)
	case FormatOrdering:
		if len(sub.Order) != len(ans.Order) {
			return false
		}
		for i := range ans.Order {
			if ans.Order[i] != sub.Order[i] {
				return false
			}
		}
		return true
	case FormatSQLResult:
		return sameRows(ans.Rows, sub.Rows, ans.OrderInsensitive)
	}
	return false
}

// NormalizeText приводит текстовый ответ к сравнимому виду: строки обрезаются,
// внутренние пробелы схлопываются, пустые строки в конце отбрасываются.
// Регистр сохраняется: для вывода программ он значим ("True" ≠ "true").
func NormalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.Join(strings.Fields(l), " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	return strings.Join(out, "\n")
}

func sameSet(a, b []string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		if m[x] == 0 {
			return false
		}
		m[x]--
	}
	return true
}

func normCell(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func rowKey(r []string) string {
	parts := make([]string, len(r))
	for i, c := range r {
		parts[i] = normCell(c)
	}
	return strings.Join(parts, "\x1f")
}

func sameRows(want, got [][]string, orderInsensitive bool) bool {
	if len(want) != len(got) {
		return false
	}
	wk := make([]string, len(want))
	gk := make([]string, len(got))
	for i := range want {
		wk[i] = rowKey(want[i])
		gk[i] = rowKey(got[i])
	}
	if orderInsensitive {
		sort.Strings(wk)
		sort.Strings(gk)
	}
	for i := range wk {
		if wk[i] != gk[i] {
			return false
		}
	}
	return true
}
