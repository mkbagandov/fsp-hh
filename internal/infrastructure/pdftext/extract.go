// Package pdftext извлекает текст из PDF-резюме. Чистый Go, без внешних бинарников.
// Сканы без текстового слоя дают пустой результат — тогда профиль заполняется вручную.
//
// PDF приходит от пользователя, поэтому разбор защищён в два слоя: в процессе — ограничения обхода
// (страницы, глубина форм, число операторов, время; см. walk.go), а в API извлечение выполняется
// в дочернем процессе с тайм-аутом и лимитами памяти и стека (worker.go): у библиотеки разбора есть
// циклы без защиты (цепочки /Prev, /Extends), которые в процессе API не остановить.
package pdftext

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// MaxBytes — ограничение размера входного файла.
const MaxBytes = 10 << 20

// ErrNotPDF — файл не похож на PDF.
var ErrNotPDF = errors.New("pdftext: not a PDF file")

// IsPDF проверяет сигнатуру файла.
func IsPDF(data []byte) bool {
	return len(data) > 4 && bytes.HasPrefix(data, []byte("%PDF"))
}

// Extract возвращает текст документа с нормализованными переводами строк.
//
// Основной путь — собственный обход содержимого страниц (walk.go): корректная ToUnicode, позиционирование
// Td/TD/T*/Tm/cm, формы XObject и раскладка по строкам. Если страница не разбирается, используется
// построчное извлечение библиотеки — только для страниц с конечной цепочкой /Parent.
func Extract(data []byte) (text string, err error) {
	if !IsPDF(data) {
		return "", ErrNotPDF
	}
	defer func() {
		// Библиотека паникует на некоторых повреждённых файлах; превращаем панику в ошибку.
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("pdftext: malformed document: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdftext: open: %w", err)
	}
	w := &walker{deadline: time.Now().Add(limits.budget)}
	var b strings.Builder
	for _, page := range pageList(r.Trailer().Key("Root").Key("Pages"), limits.pages) {
		lines, perr := w.page(page)
		budget := errors.Is(perr, errBudget)
		if !budget && (perr != nil || len(lines) == 0) && parentChainBounded(page) {
			lines = libraryRows(pdf.Page{V: page})
		}
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if budget {
			break
		}
	}
	return Normalize(b.String()), nil
}

// Ограничения дерева страниц: /Count и /Kids из файла не заслуживают доверия.
const (
	maxTreeDepth = 32
	maxTreeNodes = 5000
)

// pageList перечисляет листья дерева страниц в порядке документа. Глубина и число посещённых узлов
// ограничены, поэтому цикл в /Kids (в том числе «широкий», с несколькими ссылками на себя) конечен.
func pageList(root pdf.Value, maxPages int) []pdf.Value {
	var out []pdf.Value
	visited := 0
	var walk func(v pdf.Value, depth int)
	walk = func(v pdf.Value, depth int) {
		if len(out) >= maxPages || depth > maxTreeDepth || visited >= maxTreeNodes || v.Kind() != pdf.Dict {
			return
		}
		visited++
		kids := v.Key("Kids")
		switch {
		case v.Key("Type").Name() == "Page", kids.Kind() != pdf.Array && !v.Key("Contents").IsNull():
			out = append(out, v)
		case kids.Kind() == pdf.Array:
			for i := 0; i < kids.Len() && len(out) < maxPages; i++ {
				walk(kids.Index(i), depth+1)
			}
		}
	}
	walk(root, 0)
	return out
}

// inherited ищет наследуемый атрибут страницы (Resources, MediaBox, ...) вверх по /Parent с ограничением глубины.
func inherited(v pdf.Value, key string) pdf.Value {
	for i := 0; i <= maxTreeDepth && v.Kind() == pdf.Dict; i++ {
		if r := v.Key(key); !r.IsNull() {
			return r
		}
		v = v.Key("Parent")
	}
	return pdf.Value{}
}

// parentChainBounded сообщает, что цепочка /Parent конечна: библиотечный поиск ресурсов её не ограничивает.
func parentChainBounded(v pdf.Value) bool {
	for i := 0; i <= maxTreeDepth; i++ {
		if v.Kind() != pdf.Dict {
			return true
		}
		v = v.Key("Parent")
	}
	return false
}

// libraryRows — запасной путь: построчный текст средствами библиотеки.
func libraryRows(p pdf.Page) (lines []string) {
	defer func() {
		if recover() != nil {
			lines = nil
		}
	}()
	rows, err := p.GetTextByRow()
	if err != nil {
		return nil
	}
	for _, row := range rows {
		var line strings.Builder
		for _, w := range row.Content {
			line.WriteString(w.S)
		}
		lines = append(lines, strings.TrimSpace(line.String()))
	}
	return lines
}

// Normalize схлопывает пробелы, убирает управляющие символы и пустые строки подряд.
func Normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out strings.Builder
	emptyRun := 0
	for _, line := range strings.Split(s, "\n") {
		cleaned := strings.Map(func(r rune) rune {
			if r == '\t' {
				return ' '
			}
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, line)
		cleaned = strings.Join(strings.Fields(cleaned), " ")
		if cleaned == "" {
			emptyRun++
			if emptyRun > 1 {
				continue
			}
		} else {
			emptyRun = 0
		}
		out.WriteString(cleaned)
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String())
}
