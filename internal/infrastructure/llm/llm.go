// Package llm — единый интерфейс к языковым моделям для вспомогательных задач:
// разбор резюме и описаний потребности, краткие тексты объяснений.
//
// Решение проекта (docs/adr/0005): в MVP используется детерминированный эмулятор
// (провайдер mock) — правила и справочник синонимов; реальные провайдеры подключаются
// переменной LLM_PROVIDER без изменения вызывающего кода. Ответ любой модели — черновик,
// который подтверждает пользователь; категоризация и ранжирование LLM не используют.
package llm

import (
	"context"
	"time"
)

// ResumeDraft — структурированный черновик профиля из текста резюме.
type ResumeDraft struct {
	FirstName             string            `json:"first_name"`
	LastName              string            `json:"last_name"`
	MiddleName            string            `json:"middle_name,omitempty"`
	Email                 string            `json:"email,omitempty"`
	Phone                 string            `json:"phone,omitempty"`
	Telegram              string            `json:"telegram,omitempty"`
	City                  string            `json:"city,omitempty"`
	Headline              string            `json:"headline,omitempty"`
	About                 string            `json:"about,omitempty"`
	ExperienceYears       float64           `json:"experience_years"`
	PrimarySpecialization string            `json:"primary_specialization,omitempty"`
	Skills                []DraftSkill      `json:"skills"`
	Experiences           []DraftExperience `json:"experiences"`
	Education             []DraftEducation  `json:"education"`
	Links                 map[string]string `json:"links,omitempty"`
	Warnings              []string          `json:"warnings,omitempty"`
}

// DraftSkill — найденный навык (код из справочника).
type DraftSkill struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

// DraftExperience — место работы.
type DraftExperience struct {
	Company     string   `json:"company"`
	Position    string   `json:"position"`
	StartedOn   string   `json:"started_on"`         // YYYY-MM-DD
	EndedOn     string   `json:"ended_on,omitempty"` // пусто — по настоящее время
	Description string   `json:"description,omitempty"`
	Stack       []string `json:"stack,omitempty"`
}

// DraftEducation — образование.
type DraftEducation struct {
	Institution   string `json:"institution"`
	Degree        string `json:"degree,omitempty"`
	Field         string `json:"field,omitempty"`
	GraduatedYear int    `json:"graduated_year,omitempty"`
}

// NeedDraft — структурированный черновик потребности работодателя из свободного текста.
type NeedDraft struct {
	Specialization string   `json:"specialization,omitempty"`
	GradeMin       string   `json:"grade_min,omitempty"`
	GradeMax       string   `json:"grade_max,omitempty"`
	MustSkills     []string `json:"must_skills"` // коды навыков
	NiceSkills     []string `json:"nice_skills"`
	SalaryFrom     int64    `json:"salary_from,omitempty"`
	SalaryTo       int64    `json:"salary_to,omitempty"`
	WorkFormats    []string `json:"work_formats,omitempty"`
	City           string   `json:"city,omitempty"`
	RemoteOK       bool     `json:"remote_ok"`
	Title          string   `json:"title,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

// Usage — расход токенов и время (для журнала и бюджета).
type Usage struct {
	Provider     string        `json:"provider"`
	Model        string        `json:"model,omitempty"`
	InputTokens  int           `json:"input_tokens,omitempty"`
	OutputTokens int           `json:"output_tokens,omitempty"`
	Duration     time.Duration `json:"duration"`
}

// Client — операции, которые нужны платформе. Любая реализация обязана быть
// детерминированной по входу настолько, насколько возможно, и никогда не получать
// контакты и ФИО сверх необходимого (обезличивание — на стороне вызывающего кода).
type Client interface {
	// Name — имя провайдера (mock, openai_compat, gigachat).
	Name() string
	// ExtractResume разбирает текст резюме.
	ExtractResume(ctx context.Context, text string) (ResumeDraft, Usage, error)
	// ExtractNeed разбирает описание потребности работодателя.
	ExtractNeed(ctx context.Context, text string) (NeedDraft, Usage, error)
	// Complete — свободная генерация короткого текста (объяснения, подсказки).
	Complete(ctx context.Context, instruction, input string) (string, Usage, error)
}
