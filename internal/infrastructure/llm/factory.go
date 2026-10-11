package llm

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Config — настройки провайдера из окружения.
type Config struct {
	Provider string // mock | openai_compat | gigachat
	BaseURL  string // для openai_compat: https://openrouter.ai/api/v1, http://127.0.0.1:11434/v1 (Ollama) и т. п.
	APIKey   string
	Model    string
	Timeout  time.Duration
}

// New создаёт клиент по конфигурации. Неизвестный провайдер — ошибка, а не тихий mock:
// так переключение на реальную модель нельзя сломать опечаткой.
func New(cfg Config, log *slog.Logger) (Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "mock", "off":
		return NewMock(), nil
	case "openai_compat", "openai", "openrouter", "ollama", "groq":
		if cfg.BaseURL == "" || cfg.Model == "" {
			return nil, fmt.Errorf("llm: provider %q requires LLM_BASE_URL and LLM_MODEL", cfg.Provider)
		}
		return NewOpenAICompat(cfg, log), nil
	case "gigachat":
		return nil, fmt.Errorf("llm: provider gigachat is prepared as a contract but not wired yet; use openai_compat or mock")
	default:
		return nil, fmt.Errorf("llm: unknown provider %q", cfg.Provider)
	}
}
