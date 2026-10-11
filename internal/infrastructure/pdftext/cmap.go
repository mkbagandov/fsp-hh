package pdftext

import (
	"strconv"
	"strings"
	"unicode/utf16"
)

// cmap — разобранная CMap: таблица ToUnicode или кодовое пространство кодировки составного шрифта.
//
// Своя реализация вместо библиотечной: библиотека в диапазоне bfrange прибавляет смещение только
// к последнему байту, поэтому широкий диапазон <0000> <FFFF> <0000> (его пишут fpdf/tFPDF —
// в том числе наш PDF-профиль) превращал кириллицу в латиницу и знаки препинания.
type cmap struct {
	space  [4][][2]string // кодовые пространства по длине кода (1–4 байта)
	chars  map[string][]rune
	ranges []cmapRange
}

type cmapRange struct {
	lo, hi string
	base   []rune   // назначение-строка: к последнему символу прибавляется смещение кода
	list   [][]rune // назначение-массив: по элементу на код
}

// parseCMap разбирает текст CMap. Нужны только операторы codespacerange, bfchar и bfrange,
// поэтому достаточно токенизатора — PostScript-словари не исполняются.
func parseCMap(data []byte) *cmap {
	toks := cmapTokens(data)
	m := &cmap{chars: map[string][]rune{}}
	isStr := func(i int) bool { return i < len(toks) && (toks[i].kind == tokHex || toks[i].kind == tokString) }
	for i := 0; i < len(toks); i++ {
		if toks[i].kind != tokKeyword {
			continue
		}
		switch toks[i].s {
		case "begincodespacerange":
			j := i + 1
			for ; isStr(j) && isStr(j+1); j += 2 {
				lo, hi := toks[j].s, toks[j+1].s
				if len(lo) == len(hi) && len(lo) >= 1 && len(lo) <= 4 {
					m.space[len(lo)-1] = append(m.space[len(lo)-1], [2]string{lo, hi})
				}
			}
			i = j - 1
		case "beginbfchar":
			j := i + 1
			for ; isStr(j) && j+1 < len(toks); j += 2 {
				if isStr(j + 1) {
					m.chars[toks[j].s] = utf16Runes(toks[j+1].s)
				}
			}
			i = j - 1
		case "beginbfrange":
			j := i + 1
			for isStr(j) && isStr(j+1) && j+2 < len(toks) {
				lo, hi := toks[j].s, toks[j+1].s
				valid := len(lo) == len(hi) && len(lo) >= 1 && len(lo) <= 4 && lo <= hi
				if isStr(j + 2) {
					if valid {
						m.ranges = append(m.ranges, cmapRange{lo: lo, hi: hi, base: utf16Runes(toks[j+2].s)})
					}
					j += 3
					continue
				}
				if toks[j+2].kind != tokArrayOpen {
					j = len(toks) // повреждённая запись — дальше не разбираем
					break
				}
				k := j + 3
				var list [][]rune
				for ; k < len(toks) && toks[k].kind != tokArrayClose; k++ {
					if isStr(k) {
						list = append(list, utf16Runes(toks[k].s))
					}
				}
				if valid {
					m.ranges = append(m.ranges, cmapRange{lo: lo, hi: hi, list: list})
				}
				j = k + 1
			}
			i = j - 1
		}
	}
	return m
}

func (m *cmap) hasSpace() bool {
	for _, s := range m.space {
		if len(s) > 0 {
			return true
		}
	}
	return false
}

// match сообщает, попадает ли код в одно из кодовых пространств (побайтно, PDF 32000-1, 9.7.6.2).
func (m *cmap) match(code string) bool {
	if len(code) < 1 || len(code) > 4 {
		return false
	}
	for _, r := range m.space[len(code)-1] {
		ok := true
		for k := 0; k < len(code); k++ {
			if code[k] < r[0][k] || code[k] > r[1][k] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// lookup возвращает текст для кода; ok=false — код в таблице не описан.
func (m *cmap) lookup(code string) (string, bool) {
	if r, ok := m.chars[code]; ok {
		return runesText(r), true
	}
	for _, rg := range m.ranges {
		if len(rg.lo) != len(code) || code < rg.lo || code > rg.hi {
			continue
		}
		off := beInt(code) - beInt(rg.lo)
		if rg.list != nil {
			if off < len(rg.list) {
				return runesText(rg.list[off]), true
			}
			return "", true
		}
		if len(rg.base) == 0 {
			return "", true
		}
		out := append([]rune(nil), rg.base...)
		out[len(out)-1] += rune(off) //nolint:gosec // код не длиннее 4 байт, смещение помещается в rune
		return runesText(out), true
	}
	return "", false
}

func beInt(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n<<8 | int(s[i])
	}
	return n
}

// utf16Runes декодирует назначение CMap (UTF-16BE, возможны суррогатные пары и несколько символов).
func utf16Runes(s string) []rune {
	if len(s)%2 == 1 {
		// Нестандартная однобайтовая запись — трактуем байты как коды символов.
		out := make([]rune, 0, len(s))
		for i := 0; i < len(s); i++ {
			out = append(out, rune(s[i]))
		}
		return out
	}
	u := make([]uint16, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		u = append(u, uint16(s[i])<<8|uint16(s[i+1]))
	}
	return utf16.Decode(u)
}

// runesText собирает результат, отбрасывая U+0000: так в CMap обозначают «пустые» глифы.
// Нулевую базу диапазона (<0000> <FFFF> <0000>) при этом не трогаем — к ней прибавляется смещение.
func runesText(r []rune) string {
	var b strings.Builder
	for _, c := range r {
		if c != 0 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

type tokKind byte

const (
	tokKeyword tokKind = iota
	tokNumber
	tokName
	tokHex
	tokString
	tokArrayOpen
	tokArrayClose
	tokOther
)

type token struct {
	kind tokKind
	s    string
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == 0
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// cmapTokens — минимальный токенизатор PostScript-подмножества CMap.
func cmapTokens(d []byte) []token {
	var out []token
	for i := 0; i < len(d); {
		c := d[i]
		switch {
		case isSpace(c):
			i++
		case c == '%':
			for i < len(d) && d[i] != '\n' && d[i] != '\r' {
				i++
			}
		case (c == '<' || c == '>') && i+1 < len(d) && d[i+1] == c:
			out = append(out, token{tokOther, string(d[i : i+2])})
			i += 2
		case c == '<':
			s, n := hexString(d[i:])
			out = append(out, token{tokHex, s})
			i += n
		case c == '(':
			s, n := literalString(d[i:])
			out = append(out, token{tokString, s})
			i += n
		case c == '[':
			out = append(out, token{tokArrayOpen, "["})
			i++
		case c == ']':
			out = append(out, token{tokArrayClose, "]"})
			i++
		case c == '/':
			j := i + 1
			for j < len(d) && !isSpace(d[j]) && !isDelim(d[j]) {
				j++
			}
			out = append(out, token{tokName, string(d[i+1 : j])})
			i = j
		case isDelim(c):
			out = append(out, token{tokOther, string(c)})
			i++
		default:
			j := i
			for j < len(d) && !isSpace(d[j]) && !isDelim(d[j]) {
				j++
			}
			w := string(d[i:j])
			if _, err := strconv.ParseFloat(w, 64); err == nil {
				out = append(out, token{tokNumber, w})
			} else {
				out = append(out, token{tokKeyword, w})
			}
			i = j
		}
	}
	return out
}

// hexString разбирает <…>: пробелы внутри пропускаются, нечётная последняя цифра дополняется нулём.
func hexString(d []byte) (string, int) {
	var b []byte
	var hi byte
	half := false
	i := 1
	for ; i < len(d) && d[i] != '>'; i++ {
		v, ok := hexVal(d[i])
		if !ok {
			continue
		}
		if half {
			b = append(b, hi<<4|v)
		} else {
			hi = v
		}
		half = !half
	}
	if half {
		b = append(b, hi<<4)
	}
	if i < len(d) {
		i++ // '>'
	}
	return string(b), i
}

// literalString разбирает строку в круглых скобках с экранированием и вложенными скобками.
func literalString(d []byte) (string, int) {
	var b []byte
	depth := 0
	i := 0
	for i < len(d) {
		c := d[i]
		switch {
		case c == '(':
			if depth > 0 {
				b = append(b, c)
			}
			depth++
			i++
		case c == ')':
			depth--
			i++
			if depth == 0 {
				return string(b), i
			}
			b = append(b, c)
		case c == '\\' && i+1 < len(d):
			e := d[i+1]
			i += 2
			switch e {
			case 'n':
				b = append(b, '\n')
			case 'r':
				b = append(b, '\r')
			case 't':
				b = append(b, '\t')
			case 'b':
				b = append(b, '\b')
			case 'f':
				b = append(b, '\f')
			case '\r', '\n':
				if e == '\r' && i < len(d) && d[i] == '\n' {
					i++
				}
			default:
				if e < '0' || e > '7' {
					b = append(b, e)
					continue
				}
				v := int(e - '0')
				for k := 0; k < 2 && i < len(d) && d[i] >= '0' && d[i] <= '7'; k++ {
					v = v*8 + int(d[i]-'0')
					i++
				}
				b = append(b, byte(v&0xFF))
			}
		default:
			b = append(b, c)
			i++
		}
	}
	return string(b), i
}
