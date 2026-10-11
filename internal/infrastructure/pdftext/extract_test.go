package pdftext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"

	"fsp/internal/infrastructure/llm"
)

// Дочерний процесс тестов: Isolated запускает этот же бинарник с аргументом WorkerArg.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == WorkerArg {
		os.Exit(RunWorker(os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// buildPDF собирает PDF из тел объектов (номера 1..n) с корректной таблицей xref.
// selfPrev добавляет в трейлер /Prev на саму таблицу — цикл, на котором библиотека зависает.
func buildPDF(objs []string, selfPrev bool) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	prev := ""
	if selfPrev {
		prev = fmt.Sprintf(" /Prev %d", xref)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R%s >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, prev, xref)
	return b.Bytes()
}

func stream(dict, data string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

const helvetica = `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>`

// samplePDF проверяет ключевые случаи обхода: кернинг и пробелы в TJ, составной шрифт с ToUnicode
// (bfchar, bfrange с массивом и строкой), форму XObject, «жирный» наложением, два фрагмента на одной
// строке с большим зазором, операторы TL / T* / '.
func samplePDF() []byte {
	widths := strings.TrimSpace(strings.Repeat("500 ", 95))
	content := strings.Join([]string{
		`BT /F1 12 Tf 72 760 Td [(Hel) -20 (lo) -300 (World)] TJ ET`,
		`BT /F2 12 Tf 72 740 Td <0001000200030004> Tj ET`,
		`q 1 0 0 1 72 700 cm /X1 Do Q`,
		`BT /F1 12 Tf 72 680 Td (Bold) Tj ET`,
		`BT /F1 12 Tf 72.3 680 Td (Bold) Tj ET`,
		`BT /F1 10 Tf 72 660 Td (Left) Tj 200 0 Td (Right) Tj ET`,
		`BT /F1 10 Tf 72 640 Td 12 TL (Line1) Tj T* (Line2) Tj (Line3) ' ET`,
	}, "\n")
	cmapText := strings.Join([]string{
		`/CIDInit /ProcSet findresource begin 12 dict begin begincmap`,
		`/CMapName /Test-UCS def % комментарий`,
		`1 begincodespacerange <0000> <FFFF> endcodespacerange`,
		`1 beginbfchar <0001> <0418> endbfchar`,
		`2 beginbfrange <0002> <0003> [<0432> <0430>] <0004> <0004> <043D> endbfrange`,
		`endcmap CMapName currentdict /CMap defineresource pop end end`,
	}, "\n")
	return buildPDF([]string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 5 0 R /F2 6 0 R >> /XObject << /X1 9 0 R >> >> /Contents 4 0 R >>`,
		stream("", content),
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 32 /LastChar 126 /Widths [` + widths + `] >>`,
		`<< /Type /Font /Subtype /Type0 /BaseFont /TestCID /Encoding /Identity-H /DescendantFonts [7 0 R] /ToUnicode 8 0 R >>`,
		`<< /Type /Font /Subtype /CIDFontType2 /BaseFont /TestCID /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 600 /W [1 [500 500] 3 4 550] >>`,
		stream("", cmapText),
		stream(`/Type /XObject /Subtype /Form /BBox [0 0 200 50] /Resources << /Font << /F1 5 0 R >> >>`, `BT /F1 12 Tf 0 0 Td (Form text) Tj ET`),
	}, false)
}

func TestExtractLayout(t *testing.T) {
	text, err := Extract(samplePDF())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Hello World", "Иван", "Form text", "Bold", "Left Right", "Line1", "Line2", "Line3"}
	if got := strings.Split(text, "\n"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines:\n got %q\nwant %q", got, want)
	}
}

func TestExtractFPDF(t *testing.T) {
	font, err := os.ReadFile("../pdfgen/fonts/JetBrainsMono-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddUTF8FontFromBytes("JB", "", font)
	doc.AddPage()
	doc.SetFont("JB", "", 14)
	doc.Text(20, 20, "Иванов Иван Иванович")
	doc.SetFont("JB", "", 10)
	doc.Text(20, 30, "Январь 2020 — настоящее время")
	doc.Text(110, 30, "ООО «Ромашка»")
	doc.Text(20, 40, "Go, PostgreSQL, Kafka; ёжик")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	doc.Text(20, 20, "Second page in Helvetica")
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		t.Fatal(err)
	}
	text, err := Extract(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Иванов Иван Иванович\nЯнварь 2020 — настоящее время ООО «Ромашка»\nGo, PostgreSQL, Kafka; ёжик",
		"\n\nSecond page in Helvetica",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text must contain %q, got:\n%s", want, text)
		}
	}
}

// Резюме, сгенерированное fpdf (так же, как PDF-профиль платформы), проходит весь путь до черновика.
func TestResumePipeline(t *testing.T) {
	font, err := os.ReadFile("../pdfgen/fonts/JetBrainsMono-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddUTF8FontFromBytes("JB", "", font)
	doc.AddPage()
	y := 20.0
	line := func(size float64, s string) {
		doc.SetFont("JB", "", size)
		doc.Text(20, y, s)
		y += size * 0.6
	}
	line(16, "Петрова Мария Сергеевна")
	line(10, "Backend-разработчик")
	line(10, "Казань · +7 (912) 345-67-89 · maria.petrova@example.com · @mpetrova")
	line(12, "Опыт работы")
	line(10, "Март 2021 — настоящее время  Backend-разработчик — ООО «Волга Тех»")
	line(10, "Сервисы на Go, PostgreSQL, Kafka, Docker")
	line(12, "Навыки")
	line(10, "Go, PostgreSQL, Kafka, Docker, Kubernetes")
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		t.Fatal(err)
	}
	text, err := Extract(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := llm.NewMock().ExtractResume(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if d.FirstName != "Мария" || d.LastName != "Петрова" || d.Email != "maria.petrova@example.com" || d.Phone != "+79123456789" || d.City != "Казань" {
		t.Fatalf("draft identity: %+v\ntext:\n%s", d, text)
	}
	if len(d.Experiences) != 1 || len(d.Skills) < 4 {
		t.Fatalf("draft experience/skills: %+v / %+v\ntext:\n%s", d.Experiences, d.Skills, text)
	}
}

// Циклы в дереве страниц и в цепочке /Parent не зацикливают разбор внутри процесса.
func TestExtractCyclicTreeTerminates(t *testing.T) {
	doc := buildPDF([]string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [2 0 R 2 0 R 3 0 R] /Count 2147483647 >>`,
		`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 ` + helvetica + ` >> >> /Contents 4 0 R >>`,
		stream("", `BT /F1 12 Tf 72 700 Td (Cycle) Tj ET`),
		`<< /Type /Page /Parent 5 0 R /Contents 4 0 R >>`,
	}, false)
	start := time.Now()
	text, err := Extract(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Cycle") || time.Since(start) > 5*time.Second {
		t.Fatalf("cyclic tree: %q in %v", text, time.Since(start))
	}
	// Страница, у которой /Parent указывает сама на себя и нет ресурсов: пустой текст, без зависания.
	if lines := pageList(buildPDFValue(t, []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 3 0 R >>`,
		stream("", `BT /F1 12 Tf 72 700 Td (Orphan) Tj ET`),
	}), 5); len(lines) != 1 || parentChainBounded(lines[0]) {
		t.Fatalf("self-parent page must be listed once and flagged as unbounded")
	}
}

func TestBudgetStopsHugeContent(t *testing.T) {
	saved := limits
	t.Cleanup(func() { limits = saved })
	limits.ops = 500
	content := strings.Repeat("BT /F1 12 Tf 72 700 Td (x) Tj ET\n", 400)
	doc := buildPDF([]string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 ` + helvetica + ` >> >> /Contents 4 0 R >>`,
		stream("", content),
	}, false)
	text, err := Extract(doc)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(text, "x"); n == 0 || n >= 400 {
		t.Fatalf("budget must cut the page short but keep collected text, got %d glyphs", n)
	}
}

func TestExtractRejectsGarbage(t *testing.T) {
	if _, err := Extract([]byte("hello")); !errors.Is(err, ErrNotPDF) {
		t.Fatalf("not a pdf: %v", err)
	}
	for _, doc := range [][]byte{
		[]byte("%PDF-1.4\ngarbage without xref"),
		samplePDF()[:200],
	} {
		if _, err := Extract(doc); err == nil {
			t.Fatalf("broken document must fail cleanly: %q", doc[:20])
		}
	}
}

// Изоляция: обычный документ разбирается в дочернем процессе, а цикл /Prev, на котором библиотека
// зависает навсегда, обрывается тайм-аутом — процесс API не страдает.
func TestIsolatedWorker(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	text, err := NewIsolated(exe, 20*time.Second, 1).Extract(ctx, samplePDF())
	if err != nil || !strings.HasPrefix(text, "Hello World\nИван") {
		t.Fatalf("isolated extract: %q %v", text, err)
	}
	if _, err := NewIsolated(exe, time.Second, 1).Extract(ctx, []byte("nope")); !errors.Is(err, ErrNotPDF) {
		t.Fatalf("non-pdf must be rejected before spawning: %v", err)
	}
	hang := buildPDF([]string{`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [] /Count 0 >>`}, true)
	start := time.Now()
	_, err = NewIsolated(exe, 2*time.Second, 1).Extract(ctx, hang)
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 15*time.Second {
		t.Fatalf("hanging document must time out, got %v after %v", err, time.Since(start))
	}
}
