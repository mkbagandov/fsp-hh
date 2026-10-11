// Package itembank — банк параметрических семейств заданий.
//
// Единица банка — семейство (Family): генератор, который по seed строит уникальный
// экземпляр задания и сам вычисляет правильный ответ. Утечка банка даёт только шаблоны:
// чтобы ответить на свой вариант, нужно решить задачу, то есть обладать навыком
// (docs/04-testing.md, раздел 8). Параметры IRT семейства задаёт автор, калибровка уточняет.
//
// Пакет не зависит от приложения и БД.
package itembank

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Spec — специализация, к которой относится семейство (common — общий блок для всех).
type Spec string

// Специализации.
const (
	SpecCommon   Spec = "common"
	SpecBackend  Spec = "backend"
	SpecFrontend Spec = "frontend"
	SpecDataML   Spec = "data_ml"
	SpecDevOps   Spec = "devops"
	SpecQA       Spec = "qa"
	SpecSecurity Spec = "security"
)

// Grade — целевая полоса грейда семейства.
type Grade string

// Грейды.
const (
	GradeJunior Grade = "junior"
	GradeMiddle Grade = "middle"
	GradeSenior Grade = "senior"
)

// Format — формат ответа.
type Format string

// Форматы.
const (
	FormatSingleChoice Format = "single_choice"
	FormatMultiChoice  Format = "multi_choice"
	FormatNumeric      Format = "numeric"
	FormatShortText    Format = "short_text"
	FormatCodeOutput   Format = "code_output"
	FormatOrdering     Format = "ordering"
	FormatSQLResult    Format = "sql_result"
)

// IRTParams — параметры трёхпараметрической логистической модели.
type IRTParams struct {
	A float64 `json:"a"` // дискриминативность, 0.5–2.5
	B float64 `json:"b"` // сложность на шкале θ, −3…3
	C float64 `json:"c"` // угадывание, 0–0.5
}

// Meta — паспорт семейства.
type Meta struct {
	ID        string        `json:"id"`     // "<spec>.<topic>.<slug>.v<n>"
	Spec      Spec          `json:"spec"`   // специализация
	Topic     string        `json:"topic"`  // тема из справочника
	Grade     Grade         `json:"grade"`  // целевая полоса
	Format    Format        `json:"format"` // формат ответа
	IRT       IRTParams     `json:"irt"`    // априорные параметры
	TimeLimit time.Duration `json:"time_limit"`
	Tags      []string      `json:"tags,omitempty"`
}

// CodeBlock — фрагмент кода в задании.
type CodeBlock struct {
	Lang   string `json:"lang"`
	Source string `json:"source"`
}

// Option — вариант ответа.
type Option struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// Answer — канонический ответ; заполняются поля, соответствующие формату.
type Answer struct {
	Keys             []string   `json:"keys,omitempty"`              // single_choice (один ключ), multi_choice (все верные)
	Number           float64    `json:"number,omitempty"`            // numeric
	Tolerance        float64    `json:"tolerance,omitempty"`         // numeric: абсолютный допуск
	Text             string     `json:"text,omitempty"`              // short_text, code_output
	Order            []string   `json:"order,omitempty"`             // ordering: ключи в верном порядке
	Rows             [][]string `json:"rows,omitempty"`              // sql_result
	OrderInsensitive bool       `json:"order_insensitive,omitempty"` // sql_result: порядок строк не важен
}

// Instance — конкретный вариант задания.
type Instance struct {
	FamilyID    string     `json:"family_id"`
	Seed        uint64     `json:"seed"`
	Stem        string     `json:"stem"` // Markdown, русский
	Code        *CodeBlock `json:"code,omitempty"`
	Options     []Option   `json:"options,omitempty"`
	Answer      Answer     `json:"answer"`
	Explanation string     `json:"explanation"`
}

// Submission — ответ кандидата.
type Submission struct {
	Keys   []string   `json:"keys,omitempty"`
	Number *float64   `json:"number,omitempty"`
	Text   string     `json:"text,omitempty"`
	Order  []string   `json:"order,omitempty"`
	Rows   [][]string `json:"rows,omitempty"`
}

// Family — генератор заданий.
type Family interface {
	Meta() Meta
	Generate(seed uint64) (Instance, error)
}

var idPattern = regexp.MustCompile(`^(common|backend|frontend|data_ml|devops|qa|security)\.[a-z_]+\.[a-z0-9-]+\.v\d+$`)

// ValidSpec, ValidGrade, ValidFormat — проверки перечислений.
func ValidSpec(s Spec) bool {
	switch s {
	case SpecCommon, SpecBackend, SpecFrontend, SpecDataML, SpecDevOps, SpecQA, SpecSecurity:
		return true
	}
	return false
}

func ValidGrade(g Grade) bool { return g == GradeJunior || g == GradeMiddle || g == GradeSenior }

func ValidFormat(f Format) bool {
	switch f {
	case FormatSingleChoice, FormatMultiChoice, FormatNumeric, FormatShortText, FormatCodeOutput, FormatOrdering, FormatSQLResult:
		return true
	}
	return false
}

// Validate проверяет паспорт семейства по правилам скилла item-family-authoring.
func (m Meta) Validate() error {
	var errs []error
	if !idPattern.MatchString(m.ID) {
		errs = append(errs, fmt.Errorf("id %q must match <spec>.<topic>.<slug>.v<n>", m.ID))
	}
	if !ValidSpec(m.Spec) {
		errs = append(errs, fmt.Errorf("unknown spec %q", m.Spec))
	}
	if m.Topic == "" {
		errs = append(errs, errors.New("topic is empty"))
	}
	if !ValidGrade(m.Grade) {
		errs = append(errs, fmt.Errorf("unknown grade %q", m.Grade))
	}
	if !ValidFormat(m.Format) {
		errs = append(errs, fmt.Errorf("unknown format %q", m.Format))
	}
	if m.IRT.A < 0.5 || m.IRT.A > 2.5 {
		errs = append(errs, fmt.Errorf("irt.a=%.2f out of [0.5, 2.5]", m.IRT.A))
	}
	if m.IRT.B < -3 || m.IRT.B > 3 {
		errs = append(errs, fmt.Errorf("irt.b=%.2f out of [-3, 3]", m.IRT.B))
	}
	if m.IRT.C < 0 || m.IRT.C > 0.5 {
		errs = append(errs, fmt.Errorf("irt.c=%.2f out of [0, 0.5]", m.IRT.C))
	}
	if m.TimeLimit < 30*time.Second || m.TimeLimit > 5*time.Minute {
		errs = append(errs, fmt.Errorf("time limit %s out of [30s, 5m]", m.TimeLimit))
	}
	return errors.Join(errs...)
}

// Validate проверяет согласованность экземпляра с форматом.
func (in Instance) Validate(format Format) error {
	var errs []error
	if in.Stem == "" {
		errs = append(errs, errors.New("stem is empty"))
	}
	if len(in.Explanation) < 40 {
		errs = append(errs, errors.New("explanation shorter than 40 characters"))
	}
	switch format {
	case FormatSingleChoice, FormatMultiChoice:
		if len(in.Options) < 3 || len(in.Options) > 6 {
			errs = append(errs, fmt.Errorf("%d options, want 3..6", len(in.Options)))
		}
		seen := map[string]bool{}
		texts := map[string]bool{}
		for _, o := range in.Options {
			if seen[o.Key] {
				errs = append(errs, fmt.Errorf("duplicate option key %q", o.Key))
			}
			seen[o.Key] = true
			if texts[o.Text] {
				errs = append(errs, fmt.Errorf("duplicate option text %q", o.Text))
			}
			texts[o.Text] = true
		}
		if len(in.Answer.Keys) == 0 {
			errs = append(errs, errors.New("answer keys are empty"))
		}
		if format == FormatSingleChoice && len(in.Answer.Keys) != 1 {
			errs = append(errs, errors.New("single_choice must have exactly one answer key"))
		}
		if format == FormatMultiChoice && len(in.Answer.Keys) >= len(in.Options) {
			errs = append(errs, errors.New("multi_choice must leave at least one wrong option"))
		}
		for _, k := range in.Answer.Keys {
			if !seen[k] {
				errs = append(errs, fmt.Errorf("answer key %q not among options", k))
			}
		}
	case FormatNumeric:
		if in.Answer.Tolerance < 0 {
			errs = append(errs, errors.New("negative tolerance"))
		}
	case FormatShortText, FormatCodeOutput:
		if in.Answer.Text == "" {
			errs = append(errs, errors.New("answer text is empty"))
		}
		if format == FormatCodeOutput && in.Code == nil {
			errs = append(errs, errors.New("code_output requires a code block"))
		}
	case FormatOrdering:
		if len(in.Options) < 3 || len(in.Answer.Order) != len(in.Options) {
			errs = append(errs, errors.New("ordering requires >=3 options and a full order"))
		}
		seen := map[string]bool{}
		for _, o := range in.Options {
			seen[o.Key] = true
		}
		for _, k := range in.Answer.Order {
			if !seen[k] {
				errs = append(errs, fmt.Errorf("order key %q not among options", k))
			}
		}
	case FormatSQLResult:
		if in.Code == nil {
			errs = append(errs, errors.New("sql_result requires the query in a code block"))
		}
		if in.Answer.Rows == nil {
			errs = append(errs, errors.New("sql_result requires rows (possibly empty slice)"))
		}
	}
	return errors.Join(errs...)
}
