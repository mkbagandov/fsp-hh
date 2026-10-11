// Package pdfgen строит стандартизированный PDF-профиль кандидата в фирменном стиле ФСП
// (JetBrains Mono, красный #EC1D35, синий #402FFF, тёмный #1B1C21).
package pdfgen

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed fonts/JetBrainsMono-Regular.ttf
var fontRegular []byte

//go:embed fonts/JetBrainsMono-Bold.ttf
var fontBold []byte

// Profile — данные для PDF (уже отфильтрованные по приватности и гранту).
type Profile struct {
	FullName        string
	Headline        string
	City            string
	WorkFormats     []string
	Relocation      bool
	ExperienceYears float64
	SalaryExpected  int64 // 0 — скрыто или не указано
	Specialization  string
	Grade           string // Junior/Middle/Senior или пусто
	GradeStatus     string // «подтверждён тестом», «заявлен, не подтверждён», ...
	Percentile      float64
	About           string
	Contacts        []string // пусто, если контакты закрыты
	Skills          []Skill
	Experiences     []Experience
	Education       []string
	Achievements    []string
	VerifyURL       string
	GeneratedAt     time.Time
	NoFSPHistory    bool
}

// Skill — навык с отметкой подтверждения.
type Skill struct {
	Name      string
	Confirmed bool
}

// Experience — место работы.
type Experience struct {
	Period      string
	Company     string
	Position    string
	Description string
	Stack       []string
}

var (
	colorInk   = [3]int{0x1B, 0x1C, 0x21}
	colorRed   = [3]int{0xEC, 0x1D, 0x35}
	colorBlue  = [3]int{0x40, 0x2F, 0xFF}
	colorMuted = [3]int{0x6B, 0x6E, 0x78}
	colorLine  = [3]int{0xC8, 0xC9, 0xCA}
	colorPaper = [3]int{0xED, 0xED, 0xED}
)

// Build возвращает PDF-документ.
func Build(p Profile) ([]byte, error) {
	p = p.pdfSafe()
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddUTF8FontFromBytes("JB", "", fontRegular)
	pdf.AddUTF8FontFromBytes("JB", "B", fontBold)
	pdf.SetTitle("Профиль кандидата — ФСП", true)
	pdf.SetAuthor("Платформа «ФСП»", true)
	pdf.AddPage()
	w, _ := pdf.GetPageSize()
	content := w - 32

	// Шапка: тёмная полоса, знак ФСП из цветных блоков, название платформы.
	pdf.SetFillColor(colorInk[0], colorInk[1], colorInk[2])
	pdf.Rect(0, 0, w, 26, "F")
	pdf.SetFillColor(colorRed[0], colorRed[1], colorRed[2])
	pdf.Rect(16, 9, 4, 8, "F")
	pdf.SetFillColor(colorBlue[0], colorBlue[1], colorBlue[2])
	pdf.Rect(21, 9, 4, 8, "F")
	pdf.SetFillColor(0xFF, 0xFF, 0xFF)
	pdf.Rect(26, 9, 4, 8, "F")
	pdf.SetTextColor(0xFF, 0xFF, 0xFF)
	pdf.SetFont("JB", "B", 13)
	pdf.SetXY(33, 9)
	pdf.CellFormat(80, 8, "ФСП · профиль кандидата", "", 0, "L", false, 0, "")
	pdf.SetFont("JB", "", 8)
	pdf.SetXY(w-16-70, 10)
	pdf.CellFormat(70, 6, "Сформировано "+p.GeneratedAt.Format("02.01.2006 15:04"), "", 0, "R", false, 0, "")

	// Имя и заголовок.
	pdf.SetY(34)
	pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
	pdf.SetFont("JB", "B", 18)
	pdf.MultiCell(content, 9, nz(p.FullName, "Кандидат"), "", "L", false)
	if p.Headline != "" {
		pdf.SetFont("JB", "", 11)
		pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
		pdf.MultiCell(content, 6, p.Headline, "", "L", false)
	}
	meta := []string{}
	if p.City != "" {
		meta = append(meta, p.City)
	}
	if len(p.WorkFormats) > 0 {
		meta = append(meta, strings.Join(p.WorkFormats, " / "))
	}
	if p.Relocation {
		meta = append(meta, "готов к переезду")
	}
	if p.ExperienceYears > 0 {
		meta = append(meta, fmt.Sprintf("стаж %.1f г.", p.ExperienceYears))
	}
	if p.SalaryExpected > 0 {
		meta = append(meta, fmt.Sprintf("ожидание %s ₽", formatMoney(p.SalaryExpected)))
	}
	if len(meta) > 0 {
		pdf.SetFont("JB", "", 9)
		pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
		pdf.MultiCell(content, 5, strings.Join(meta, "  ·  "), "", "L", false)
	}
	pdf.Ln(2)

	// Категория — главный блок.
	pdf.SetFillColor(colorPaper[0], colorPaper[1], colorPaper[2])
	y := pdf.GetY()
	pdf.Rect(16, y, content, 20, "F")
	pdf.SetFillColor(colorBlue[0], colorBlue[1], colorBlue[2])
	pdf.Rect(16, y, 2, 20, "F")
	pdf.SetXY(21, y+3)
	pdf.SetFont("JB", "", 8)
	pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
	pdf.CellFormat(content-10, 4, "КАТЕГОРИЯ · СПЕЦИАЛИЗАЦИЯ × ГРЕЙД", "", 1, "L", false, 0, "")
	pdf.SetX(21)
	pdf.SetFont("JB", "B", 12)
	pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
	cat := nz(p.Specialization, "Специализация не выбрана")
	if p.Grade != "" {
		cat += "  ·  " + p.Grade
	}
	pdf.CellFormat(content-10, 6, cat, "", 1, "L", false, 0, "")
	pdf.SetX(21)
	pdf.SetFont("JB", "", 8)
	status := nz(p.GradeStatus, "тест ещё не пройден")
	if p.Percentile > 0 {
		status += fmt.Sprintf("  ·  %d-й процентиль в категории", int(p.Percentile))
	}
	pdf.SetTextColor(colorBlue[0], colorBlue[1], colorBlue[2])
	pdf.CellFormat(content-10, 4, status, "", 1, "L", false, 0, "")
	pdf.SetY(y + 24)

	section(pdf, "Контакты")
	pdf.SetFont("JB", "", 9)
	pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
	if len(p.Contacts) == 0 {
		pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
		pdf.MultiCell(content, 5, "Контакты раскрываются после принятия приглашения или отклика кандидата", "", "L", false)
	} else {
		pdf.MultiCell(content, 5, strings.Join(p.Contacts, "  ·  "), "", "L", false)
	}

	if p.About != "" {
		section(pdf, "О себе")
		pdf.SetFont("JB", "", 9)
		pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
		pdf.MultiCell(content, 5, p.About, "", "L", false)
	}

	section(pdf, "Навыки")
	if len(p.Skills) == 0 {
		muted(pdf, content, "Навыки не указаны")
	} else {
		parts := make([]string, 0, len(p.Skills))
		confirmed := 0
		for _, s := range p.Skills {
			if s.Confirmed {
				parts = append(parts, s.Name+"*")
				confirmed++
			} else {
				parts = append(parts, s.Name)
			}
		}
		pdf.SetFont("JB", "", 9)
		pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
		pdf.MultiCell(content, 5, strings.Join(parts, ", "), "", "L", false)
		if confirmed > 0 {
			pdf.SetFont("JB", "", 7)
			pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
			pdf.MultiCell(content, 4, "* — тема подтверждена тестированием на платформе", "", "L", false)
		}
	}

	section(pdf, "Опыт работы")
	if len(p.Experiences) == 0 {
		muted(pdf, content, "Опыт не указан")
	}
	for _, e := range p.Experiences {
		pdf.SetFont("JB", "B", 9.5)
		pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
		pdf.MultiCell(content, 5, nz(e.Position, "Должность не указана"), "", "L", false)
		pdf.SetFont("JB", "", 8.5)
		pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
		pdf.MultiCell(content, 4.5, strings.TrimSpace(e.Company+"  ·  "+e.Period), "", "L", false)
		if e.Description != "" {
			pdf.SetFont("JB", "", 8.5)
			pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
			pdf.MultiCell(content, 4.5, e.Description, "", "L", false)
		}
		if len(e.Stack) > 0 {
			pdf.SetFont("JB", "", 8)
			pdf.SetTextColor(colorBlue[0], colorBlue[1], colorBlue[2])
			pdf.MultiCell(content, 4.5, strings.Join(e.Stack, " · "), "", "L", false)
		}
		pdf.Ln(1.5)
	}

	section(pdf, "Образование")
	if len(p.Education) == 0 {
		muted(pdf, content, "Не указано")
	}
	for _, e := range p.Education {
		pdf.SetFont("JB", "", 9)
		pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
		pdf.MultiCell(content, 5, e, "", "L", false)
	}

	section(pdf, "Достижения ФСП")
	switch {
	case p.NoFSPHistory:
		muted(pdf, content, "Нет данных реестра ФСП — ранжирование выполняется по результатам тестирования")
	case len(p.Achievements) == 0:
		muted(pdf, content, "ФСП ID привязан, достижений в реестре пока нет")
	default:
		for _, a := range p.Achievements {
			pdf.SetFont("JB", "", 9)
			pdf.SetTextColor(colorInk[0], colorInk[1], colorInk[2])
			pdf.MultiCell(content, 5, "• "+a, "", "L", false)
		}
		pdf.SetFont("JB", "", 7)
		pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
		pdf.MultiCell(content, 4, "Записи подтверждены реестром Федерации спортивного программирования", "", "L", false)
	}

	// Подвал: QR на страницу проверки.
	if p.VerifyURL != "" {
		png, err := qrcode.Encode(p.VerifyURL, qrcode.Medium, 256)
		if err == nil {
			_, h := pdf.GetPageSize()
			if pdf.GetY() > h-50 {
				pdf.AddPage()
			}
			y := pdf.GetY() + 4
			pdf.RegisterImageOptionsReader("verify-qr", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(png))
			pdf.ImageOptions("verify-qr", 16, y, 22, 22, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
			pdf.SetXY(42, y+2)
			pdf.SetFont("JB", "", 8)
			pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
			pdf.MultiCell(content-26, 4.5, "Проверить актуальность категории и подтверждённого грейда:\n"+p.VerifyURL, "", "L", false)
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdfgen: %w", err)
	}
	return buf.Bytes(), nil
}

func section(pdf *fpdf.Fpdf, title string) {
	pdf.Ln(3)
	pdf.SetFont("JB", "B", 8)
	pdf.SetTextColor(colorRed[0], colorRed[1], colorRed[2])
	pdf.CellFormat(0, 5, strings.ToUpper(title), "", 1, "L", false, 0, "")
	pdf.SetDrawColor(colorLine[0], colorLine[1], colorLine[2])
	x, y := pdf.GetX(), pdf.GetY()
	w, _ := pdf.GetPageSize()
	pdf.Line(x, y, w-16, y)
	pdf.Ln(2)
}

func muted(pdf *fpdf.Fpdf, width float64, text string) {
	pdf.SetFont("JB", "", 9)
	pdf.SetTextColor(colorMuted[0], colorMuted[1], colorMuted[2])
	pdf.MultiCell(width, 5, text, "", "L", false)
}

func nz(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func formatMoney(v int64) string {
	s := fmt.Sprintf("%d", v)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, c)
	}
	return string(out)
}

// safeText убирает символы, которые fpdf не умеет кодировать (вне BMP: эмодзи и т. п., а также
// невидимые модификаторы эмодзи). Без этого один смайлик в «о себе» ломает весь PDF-профиль.
func safeText(s string) string {
	drop := func(r rune) bool { return r > 0xFFFF || r == 0xFE0F || r == 0x200D }
	if !strings.ContainsFunc(s, drop) {
		return s
	}
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if drop(r) {
			return -1
		}
		return r
	}, s)), " ")
}

func safeAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = safeText(s)
	}
	return out
}

// pdfSafe возвращает копию профиля с безопасным для PDF текстом во всех полях.
func (p Profile) pdfSafe() Profile {
	p.FullName, p.Headline, p.City, p.About = safeText(p.FullName), safeText(p.Headline), safeText(p.City), safeText(p.About)
	p.Specialization, p.Grade, p.GradeStatus, p.VerifyURL = safeText(p.Specialization), safeText(p.Grade), safeText(p.GradeStatus), safeText(p.VerifyURL)
	p.WorkFormats, p.Contacts, p.Education, p.Achievements = safeAll(p.WorkFormats), safeAll(p.Contacts), safeAll(p.Education), safeAll(p.Achievements)
	skills := make([]Skill, len(p.Skills))
	for i, s := range p.Skills {
		skills[i] = Skill{Name: safeText(s.Name), Confirmed: s.Confirmed}
	}
	p.Skills = skills
	exps := make([]Experience, len(p.Experiences))
	for i, e := range p.Experiences {
		exps[i] = Experience{Period: safeText(e.Period), Company: safeText(e.Company), Position: safeText(e.Position),
			Description: safeText(e.Description), Stack: safeAll(e.Stack)}
	}
	p.Experiences = exps
	return p
}
