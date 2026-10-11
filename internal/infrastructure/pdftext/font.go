package pdftext

import (
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// font — всё, что нужно для извлечения текста: разбиение строки на коды, текст кода и ширина глифа.
type font struct {
	composite bool             // Type0: многобайтовые коды
	identity  bool             // Identity-H/V: 2 байта, CID = код
	ucs2      bool             // предопределённые UCS2-кодировки: код — это UTF-16
	space     *cmap            // кодовое пространство прочих составных кодировок
	uni       *cmap            // ToUnicode
	simple    pdf.TextEncoding // кодировка простого шрифта (WinAnsi, Differences, ...)
	first     int
	widths    []float64
	missing   float64
	cidW      map[int]float64
	cidRanges []cidRange
	dw        float64
	scale     float64 // единицы ширины → доли кегля: 1/1000, у Type3 — из FontMatrix
}

type cidRange struct {
	lo, hi int
	w      float64
}

// maxCMapBytes ограничивает размер разбираемой CMap (защита от «бомб» в сжатых потоках).
const maxCMapBytes = 4 << 20

func readStream(v pdf.Value) []byte {
	if v.Kind() != pdf.Stream {
		return nil
	}
	rc := v.Reader()
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(io.LimitReader(rc, maxCMapBytes)) // частично прочитанная CMap лучше, чем никакой
	return b
}

func newFont(fv pdf.Value) *font {
	f := &font{scale: 0.001, dw: 1000}
	if tu := fv.Key("ToUnicode"); tu.Kind() == pdf.Stream {
		f.uni = parseCMap(readStream(tu))
	}
	enc := fv.Key("Encoding")
	if fv.Key("Subtype").Name() == "Type0" {
		f.composite = true
		switch enc.Kind() {
		case pdf.Name:
			n := enc.Name()
			f.identity = n == "Identity-H" || n == "Identity-V"
			f.ucs2 = strings.Contains(n, "UCS2")
		case pdf.Stream:
			if m := parseCMap(readStream(enc)); m.hasSpace() {
				f.space = m
			}
		}
		if !f.identity && !f.ucs2 && f.space == nil && f.uni != nil && f.uni.hasSpace() {
			f.space = f.uni
		}
		d := fv.Key("DescendantFonts").Index(0)
		if dw := d.Key("DW"); dw.Kind() == pdf.Integer || dw.Kind() == pdf.Real {
			f.dw = dw.Float64()
		}
		f.cidW, f.cidRanges = parseW(d.Key("W"))
		return f
	}
	pf := pdf.Font{V: fv}
	f.simple = pf.Encoder()
	f.first = pf.FirstChar()
	f.widths = pf.Widths()
	f.missing = fv.Key("FontDescriptor").Key("MissingWidth").Float64()
	if f.missing == 0 {
		// Стандартные шрифты без таблицы ширин: средняя ширина, у Courier — моноширинная.
		f.missing = 500
		if strings.Contains(pf.BaseFont(), "Courier") {
			f.missing = 600
		}
	}
	if fv.Key("Subtype").Name() == "Type3" {
		if m := fv.Key("FontMatrix"); m.Len() >= 1 && m.Index(0).Float64() != 0 {
			f.scale = m.Index(0).Float64()
		}
	}
	return f
}

func parseW(w pdf.Value) (map[int]float64, []cidRange) {
	m := map[int]float64{}
	var rs []cidRange
	for i := 0; i+1 < w.Len(); {
		first := int(w.Index(i).Int64())
		next := w.Index(i + 1)
		if next.Kind() == pdf.Array {
			for k := 0; k < next.Len(); k++ {
				m[first+k] = next.Index(k).Float64()
			}
			i += 2
			continue
		}
		if i+2 >= w.Len() {
			break
		}
		rs = append(rs, cidRange{first, int(next.Int64()), w.Index(i + 2).Float64()})
		i += 3
	}
	return m, rs
}

// codes разбивает строку оператора показа текста на коды символов.
func (f *font) codes(raw string) []string {
	out := make([]string, 0, len(raw))
	if !f.composite {
		for i := 0; i < len(raw); i++ {
			out = append(out, raw[i:i+1])
		}
		return out
	}
	for i := 0; i < len(raw); {
		n := 0
		if f.space != nil {
			for k := 1; k <= 4 && i+k <= len(raw); k++ {
				if f.space.match(raw[i : i+k]) {
					n = k
					break
				}
			}
		}
		if n == 0 {
			n = min(2, len(raw)-i)
		}
		out = append(out, raw[i:i+n])
		i += n
	}
	return out
}

// text возвращает Unicode-текст кода: сначала ToUnicode (приоритет по PDF 32000-1, 9.10.2), затем кодировка.
func (f *font) text(code string) string {
	if f.uni != nil {
		if s, ok := f.uni.lookup(code); ok {
			return s
		}
	}
	if f.composite {
		if f.ucs2 && len(code) == 2 {
			return string(utf16Runes(code))
		}
		return "" // идентификаторы глифов без ToUnicode не декодируются
	}
	if f.simple != nil {
		return f.simple.Decode(code)
	}
	return code
}

// width — ширина глифа в долях кегля.
func (f *font) width(code string) float64 {
	if f.composite {
		if f.identity && len(code) == 2 {
			cid := beInt(code)
			if w, ok := f.cidW[cid]; ok {
				return w * f.scale
			}
			for _, r := range f.cidRanges {
				if cid >= r.lo && cid <= r.hi {
					return r.w * f.scale
				}
			}
		}
		return f.dw * f.scale
	}
	if i := int(code[0]) - f.first; i >= 0 && i < len(f.widths) && f.widths[i] > 0 {
		return f.widths[i] * f.scale
	}
	return f.missing * f.scale
}
