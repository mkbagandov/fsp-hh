package pdftext

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
)

func TestCMapDecoding(t *testing.T) {
	m := parseCMap([]byte(`
% fpdf/tFPDF пишут тождественный диапазон на всё пространство — библиотека декодировала его неверно
1 begincodespacerange <0000> <FFFF> endcodespacerange
3 beginbfchar
<0005> <D83DDE00>
<0006> <00660069>
<00 07> (\000A)
endbfchar
1 beginbfrange <0000> <FFFF> <0000> endbfrange
`))
	cases := map[string]string{
		"\x04\x1f": "П",
		"\x04\x51": "ё",
		"\x00\x41": "A",
		"\x00\x05": "😀",
		"\x00\x06": "fi",
		"\x00\x07": "A",
		"\x00\x00": "",
	}
	for code, want := range cases {
		if got, ok := m.lookup(code); !ok || got != want {
			t.Errorf("lookup(% x) = %q, %v; want %q", code, got, ok, want)
		}
	}

	r := parseCMap([]byte(`2 begincodespacerange <00> <7F> <8000> <FFFF> endcodespacerange
2 beginbfrange <10> <12> <0041> <8000> <8002> [<0410> <0411> <D83DDE00>] endbfrange`))
	if got, _ := r.lookup("\x11"); got != "B" {
		t.Errorf("1-byte range offset: %q", got)
	}
	if got, _ := r.lookup("\x80\x02"); got != "😀" {
		t.Errorf("array destination: %q", got)
	}
	if _, ok := r.lookup("\x13"); ok {
		t.Error("code outside ranges must be unknown")
	}
	if !r.match("\x7f") || r.match("\x80") || !r.match("\x80\x00") || r.match("\x7f\x00\x00\x00\x00") {
		t.Error("codespace matching is byte-wise and length-aware")
	}

	// Повреждённые CMap не роняют разбор.
	for _, bad := range []string{"", "beginbfrange <00", "1 beginbfrange <0000> <0001> /Name endbfrange", "<<>> [ ( unterminated", "beginbfchar <01>"} {
		_ = parseCMap([]byte(bad))
	}
}

func TestLayoutJoinsAndDeduplicates(t *testing.T) {
	runs := []run{
		{x0: 100, x1: 130, y: 700, size: 10, text: "World"},
		{x0: 60, x1: 95, y: 700.5, size: 10, text: "Hello"},
		{x0: 60.2, x1: 95.2, y: 700, size: 10, text: "Hello"}, // наложение — дубль
		{x0: 60, x1: 66, y: 688, size: 10, text: "Ни"},
		{x0: 66.5, x1: 80, y: 688, size: 10, text: "жний"}, // зазор меньше пробела — одно слово
		{x0: 95.3, x1: 97, y: 703, size: 4, text: "²"},     // надстрочный знак — та же строка
	}
	got := layout(runs)
	if strings.Join(got, "|") != "Hello² World|Нижний" {
		t.Fatalf("layout: %q", got)
	}
	if layout(nil) != nil {
		t.Fatal("empty layout")
	}
}

func TestMatrix(t *testing.T) {
	m := translate(10, 20).mul(matrix{2, 0, 0, 2, 5, 5})
	if m != (matrix{2, 0, 0, 2, 25, 45}) {
		t.Fatalf("mul: %v", m)
	}
	if identity.mul(m) != m || m.mul(identity) != m {
		t.Fatal("identity")
	}
}

// buildPDFValue открывает собранный документ и возвращает корень дерева страниц.
func buildPDFValue(t *testing.T, objs []string) pdf.Value {
	t.Helper()
	data := buildPDF(objs, false)
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return r.Trailer().Key("Root").Key("Pages")
}
