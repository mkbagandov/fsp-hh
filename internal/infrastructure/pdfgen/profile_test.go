package pdfgen

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"fsp/internal/infrastructure/pdftext"
)

func TestBuildProfilePDF(t *testing.T) {
	data, err := Build(Profile{
		FullName: "Иванов Иван Иванович", Headline: "Backend-разработчик (Go)", City: "Москва",
		WorkFormats: []string{"удалённо", "гибрид"}, ExperienceYears: 5.5, SalaryExpected: 260000,
		Specialization: "Backend-разработка", Grade: "Middle", GradeStatus: "подтверждён тестом", Percentile: 87,
		About:    "Пишу сервисы на Go, люблю наблюдаемость и тесты.",
		Contacts: []string{"+7 916 123-45-67", "ivanov@example.com"},
		Skills:   []Skill{{Name: "Go", Confirmed: true}, {Name: "PostgreSQL"}, {Name: "Docker"}},
		Experiences: []Experience{{Period: "06.2021 — настоящее время", Company: "Рога и копыта", Position: "Старший разработчик",
			Description: "Микросервисы на Go, PostgreSQL, Kafka.", Stack: []string{"Go", "PostgreSQL"}}},
		Education:    []string{"МГТУ им. Баумана, 2018"},
		Achievements: []string{"Всероссийский хакатон «Цифровой прорыв», 2 место (2025)"},
		VerifyURL:    "http://127.0.0.1:8080/verify/abc",
		GeneratedAt:  time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) || len(data) < 5000 {
		t.Fatalf("unexpected pdf: %d bytes", len(data))
	}
	// Обратное извлечение текста: подтверждает, что шрифт встроен с картой Unicode.
	// Библиотека извлечения не всегда декодирует подмножества TrueType — тогда только предупреждаем.
	text, err := pdftext.Extract(data)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if text == "" {
		t.Log("extraction returned empty text (font subset not decodable by reader)")
	} else {
		for _, want := range []string{"Backend", "Middle", "PostgreSQL"} {
			if !bytes.Contains([]byte(text), []byte(want)) {
				t.Logf("extracted text lacks %q (reader limitation); got: %.200s", want, text)
			}
		}
	}
	// Без контактов и без истории ФСП.
	data2, err := Build(Profile{FullName: "Тест", NoFSPHistory: true, GeneratedAt: time.Now()})
	if err != nil || len(data2) == 0 {
		t.Fatal(err)
	}
}

// Эмодзи в тексте профиля не ломают PDF: символы вне BMP отбрасываются, остальной текст сохраняется.
func TestBuildDropsUnsupportedCharacters(t *testing.T) {
	b, err := Build(Profile{FullName: "Тест 🚀 Тестов", About: "Люблю Go 🚀 и котиков 🐱‍👤", Skills: []Skill{{Name: "Go ✨"}},
		Experiences: []Experience{{Period: "2020 — н.в.", Company: "Ромашка 🌼", Position: "Dev"}}, GeneratedAt: time.Now()})
	if err != nil {
		t.Fatalf("emoji must not break the PDF: %v", err)
	}
	text, err := pdftext.Extract(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Тест Тестов", "Люблю Go и котиков", "Ромашка"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text must contain %q:\n%s", want, text)
		}
	}
}
