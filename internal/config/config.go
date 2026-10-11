// Package config загружает конфигурацию приложения из переменных окружения.
//
// Источник истины — окружение процесса. Для локальной разработки значения
// подхватываются из файла .env (см. .env.example); в контейнерах и на стенде
// переменные задаются напрямую.
package config

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config — полная конфигурация процесса API.
type Config struct {
	// AppEnv — профиль окружения: dev | test | prod. По умолчанию prod: забытая переменная не должна
	// открывать /dev/outbox (коды подтверждения) и консольный мейлер на публичном стенде.
	// Локально dev включается через .env (см. .env.example).
	AppEnv string `env:"APP_ENV" envDefault:"prod"`

	// HTTPAddr — адрес прослушивания HTTP-сервера.
	HTTPAddr string `env:"HTTP_ADDR" envDefault:"127.0.0.1:8080"`

	// DatabaseURL — строка подключения к PostgreSQL.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// MigrateOnStart — применять миграции при старте процесса.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"true"`

	// LogLevel — debug | info | warn | error.
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// LogFormat — text | json.
	LogFormat string `env:"LOG_FORMAT" envDefault:"text"`

	// ShutdownTimeout — сколько ждать завершения активных запросов при остановке.
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`

	// CORSOrigins — разрешённые источники для CORS (dev-сервер Vite); пусто — выключено.
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

	// RateLimitPerMinute — общий лимит запросов на IP в минуту; 0 — без лимита.
	RateLimitPerMinute int `env:"RATE_LIMIT_PER_MINUTE" envDefault:"600"`

	// LLMProvider — провайдер LLM: mock (по умолчанию в MVP) | openai_compat | gigachat.
	LLMProvider string `env:"LLM_PROVIDER" envDefault:"mock"`
	// LLMBaseURL, LLMAPIKey, LLMModel — параметры реального провайдера (не используются при mock).
	LLMBaseURL string `env:"LLM_BASE_URL"`
	LLMAPIKey  string `env:"LLM_API_KEY"`
	LLMModel   string `env:"LLM_MODEL"`

	// UploadDir — каталог загрузок (PDF-резюме).
	UploadDir string `env:"UPLOAD_DIR" envDefault:"./uploads"`
	// WebDir — каталог собранного фронтенда (web/dist); пусто — автоопределение, "-" — не раздавать.
	WebDir string `env:"WEB_DIR" envDefault:""`
	// AdminEmails — e-mail пользователей, которым при старте выдаётся роль admin (через запятую).
	AdminEmails []string `env:"ADMIN_EMAILS" envSeparator:","`

	// GradeCooldownDays — минимальный интервал между попытками сменить грейд.
	GradeCooldownDays int `env:"GRADE_COOLDOWN_DAYS" envDefault:"90"`

	// JWTSecret — секрет подписи access-токенов (≥ 32 символов). В dev может быть пустым:
	// тогда генерируется случайный и сессии не переживают перезапуск.
	JWTSecret string `env:"JWT_SECRET"`
	// AccessTTL — срок access-токена.
	AccessTTL time.Duration `env:"ACCESS_TOKEN_TTL" envDefault:"15m"`
	// RefreshTTL — срок refresh-токена.
	RefreshTTL time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"336h"`
	// CookieSecure — Secure-флаг refresh-cookie (true за HTTPS). В prod обязателен; там же включается HSTS.
	CookieSecure bool `env:"COOKIE_SECURE" envDefault:"false"`
	// BaseURL — внешний адрес приложения для ссылок в письмах.
	BaseURL string `env:"BASE_URL" envDefault:"http://127.0.0.1:8080"`
	// ConsentVersion — версия текста согласия на обработку данных.
	ConsentVersion string `env:"CONSENT_VERSION" envDefault:"2026-10-01"`
	// MailProvider — console (dev, письма в /dev/outbox) | smtp.
	MailProvider string `env:"MAIL_PROVIDER" envDefault:"console"`
	// SMTP — параметры отправки при MAIL_PROVIDER=smtp (587 — STARTTLS, 465 — TLS).
	SMTPHost     string `env:"SMTP_HOST"`
	SMTPPort     int    `env:"SMTP_PORT" envDefault:"587"`
	SMTPUser     string `env:"SMTP_USER"`
	SMTPPassword string `env:"SMTP_PASSWORD"`
	MailFrom     string `env:"MAIL_FROM"`
	// TrustedProxies — адреса или подсети обратных прокси (Caddy), которым доверяется X-Forwarded-For.
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`
	// MetricsToken — bearer-токен для /metrics. В prod без него /metrics не публикуется.
	MetricsToken string `env:"METRICS_TOKEN"`
	// FrontendURL — адрес SPA для возврата после входа через ФСП ID (в dev — Vite, в prod = BaseURL).
	FrontendURL string `env:"FRONTEND_URL"`

	// ФСП ID (OpenID Connect, Keycloak-совместимый). Пустой issuer — вход через ФСП ID выключен.
	FSPOIDCIssuer       string `env:"FSP_OIDC_ISSUER"`
	FSPOIDCClientID     string `env:"FSP_OIDC_CLIENT_ID" envDefault:"fsp-platform"`
	FSPOIDCClientSecret string `env:"FSP_OIDC_CLIENT_SECRET"`
	FSPOIDCRedirectURL  string `env:"FSP_OIDC_REDIRECT_URL"`
	// FSPRegistryURL — базовый адрес реестра достижений ФСП (пусто — синхронизация выключена).
	FSPRegistryURL string `env:"FSP_REGISTRY_URL"`
}

// Load читает конфигурацию из окружения и проверяет обязательные поля.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	switch cfg.AppEnv {
	case "dev", "test", "prod":
	default:
		return Config{}, fmt.Errorf("config: APP_ENV must be dev, test or prod, got %q", cfg.AppEnv)
	}
	if cfg.FrontendURL == "" {
		cfg.FrontendURL = cfg.BaseURL
	}
	if cfg.FSPOIDCIssuer != "" && cfg.FSPOIDCRedirectURL == "" {
		cfg.FSPOIDCRedirectURL = cfg.BaseURL + "/api/v1/auth/fsp/callback"
	}
	switch cfg.MailProvider {
	case "console":
	case "smtp":
		if cfg.SMTPHost == "" || cfg.MailFrom == "" {
			return Config{}, fmt.Errorf("config: MAIL_PROVIDER=smtp requires SMTP_HOST and MAIL_FROM")
		}
	default:
		return Config{}, fmt.Errorf("config: MAIL_PROVIDER must be console or smtp, got %q", cfg.MailProvider)
	}
	if _, err := cfg.TrustedProxyPrefixes(); err != nil {
		return Config{}, err
	}
	if cfg.AppEnv == "prod" {
		if len(cfg.JWTSecret) < 32 {
			return Config{}, fmt.Errorf("config: JWT_SECRET must be set (>= 32 chars) in prod")
		}
		if cfg.MailProvider == "console" {
			return Config{}, fmt.Errorf("config: MAIL_PROVIDER=console is not allowed in prod")
		}
		if !cfg.CookieSecure {
			return Config{}, fmt.Errorf("config: COOKIE_SECURE=true is required in prod (HTTPS)")
		}
	}
	return cfg, nil
}

// TrustedProxyPrefixes разбирает TRUSTED_PROXIES: подсети в нотации CIDR или отдельные адреса.
func (c Config) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(c.TrustedProxies))
	for _, s := range c.TrustedProxies {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("config: TRUSTED_PROXIES: %q is neither an address nor a CIDR", s)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
}

// IsDev сообщает, запущен ли процесс в профиле разработки.
func (c Config) IsDev() bool { return c.AppEnv == "dev" }
