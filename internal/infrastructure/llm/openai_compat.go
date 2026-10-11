package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// OpenAICompat — адаптер к любому OpenAI-совместимому Chat Completions API
// (OpenRouter, Groq, Ollama, vLLM). Готов к подключению: ключ и модель — из окружения.
// Используется только по явному решению (LLM_PROVIDER=openai_compat).
type OpenAICompat struct {
	cfg  Config
	http *http.Client
	log  *slog.Logger
	mock *Mock // запасной путь: при ошибке сети возвращаем результат эмулятора с предупреждением
}

// NewOpenAICompat создаёт адаптер.
func NewOpenAICompat(cfg Config, log *slog.Logger) *OpenAICompat {
	return &OpenAICompat{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}, log: log, mock: NewMock()}
}

// Name — имя провайдера.
func (c *OpenAICompat) Name() string { return "openai_compat" }

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompat) chat(ctx context.Context, system, user string, jsonMode bool) (string, Usage, error) {
	start := time.Now()
	req := chatRequest{
		Model:       c.cfg.Model,
		Temperature: 0,
		Messages:    []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
	}
	if jsonMode {
		req.ResponseFormat = &respFormat{Type: "json_object"}
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", Usage{}, fmt.Errorf("llm: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", Usage{}, fmt.Errorf("llm: decode (%d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || out.Error != nil || len(out.Choices) == 0 {
		msg := ""
		if out.Error != nil {
			msg = out.Error.Message
		}
		return "", Usage{}, fmt.Errorf("llm: status %d: %s", resp.StatusCode, msg)
	}
	usage := Usage{Provider: c.Name(), Model: c.cfg.Model, InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens, Duration: time.Since(start)}
	return out.Choices[0].Message.Content, usage, nil
}

const resumeSystemPrompt = `Ты извлекаешь структурированные данные из резюме ИТ-специалиста. Отвечай только JSON по схеме:
{"first_name":"","last_name":"","middle_name":"","email":"","phone":"","telegram":"","city":"","headline":"","about":"",
"experience_years":0,"primary_specialization":"backend|frontend|data_ml|devops|qa|security",
"skills":[{"code":"","name":"","confidence":0.0}],
"experiences":[{"company":"","position":"","started_on":"YYYY-MM-DD","ended_on":"YYYY-MM-DD|","description":"","stack":[]}],
"education":[{"institution":"","degree":"","field":"","graduated_year":0}],
"links":{"github":""},"warnings":[]}
Коды навыков бери из переданного списка; если навык отсутствует в списке — пропусти. Не выдумывай данные: пустое поле лучше догадки.`

const needSystemPrompt = `Ты извлекаешь структурированные данные из описания потребности работодателя в ИТ-специалисте. Отвечай только JSON по схеме:
{"specialization":"backend|frontend|data_ml|devops|qa|security","grade_min":"junior|middle|senior","grade_max":"junior|middle|senior",
"must_skills":[],"nice_skills":[],"salary_from":0,"salary_to":0,"work_formats":["office","hybrid","remote"],"city":"","remote_ok":true,"title":"","warnings":[]}
Коды навыков бери из переданного списка. Зарплата — в рублях в месяц. Не выдумывай данные.`

// ExtractResume — извлечение через модель; при недоступности — эмулятор с предупреждением.
func (c *OpenAICompat) ExtractResume(ctx context.Context, text string) (ResumeDraft, Usage, error) {
	content, usage, err := c.chat(ctx, resumeSystemPrompt+"\n\nСписок кодов навыков: "+c.mock.skillCodes(), text, true)
	if err != nil {
		c.log.Warn("llm unavailable, falling back to mock", "err", err)
		d, u, mErr := c.mock.ExtractResume(ctx, text)
		d.Warnings = append(d.Warnings, "Модель недоступна, использован разбор по правилам")
		return d, u, mErr
	}
	var d ResumeDraft
	if err := json.Unmarshal([]byte(extractJSON(content)), &d); err != nil {
		return ResumeDraft{}, usage, fmt.Errorf("llm: resume json: %w", err)
	}
	return c.mock.sanitizeResume(d), usage, nil
}

// ExtractNeed — извлечение потребности через модель; при недоступности — эмулятор.
func (c *OpenAICompat) ExtractNeed(ctx context.Context, text string) (NeedDraft, Usage, error) {
	content, usage, err := c.chat(ctx, needSystemPrompt+"\n\nСписок кодов навыков: "+c.mock.skillCodes(), text, true)
	if err != nil {
		c.log.Warn("llm unavailable, falling back to mock", "err", err)
		d, u, mErr := c.mock.ExtractNeed(ctx, text)
		d.Warnings = append(d.Warnings, "Модель недоступна, использован разбор по правилам")
		return d, u, mErr
	}
	var d NeedDraft
	if err := json.Unmarshal([]byte(extractJSON(content)), &d); err != nil {
		return NeedDraft{}, usage, fmt.Errorf("llm: need json: %w", err)
	}
	return c.mock.sanitizeNeed(d), usage, nil
}

// Complete — короткий текст по инструкции.
func (c *OpenAICompat) Complete(ctx context.Context, instruction, input string) (string, Usage, error) {
	content, usage, err := c.chat(ctx, instruction, input, false)
	if err != nil {
		return c.mock.Complete(ctx, instruction, input)
	}
	return strings.TrimSpace(content), usage, nil
}

// extractJSON вырезает JSON-объект из ответа модели (модели любят оборачивать его в ```).
func extractJSON(s string) string {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
