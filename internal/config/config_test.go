package config

import (
	"strings"
	"testing"
)

// clearEnv обнуляет переменные, которые могли прийти из окружения разработчика.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"APP_ENV", "JWT_SECRET", "MAIL_PROVIDER", "COOKIE_SECURE", "SMTP_HOST", "MAIL_FROM", "TRUSTED_PROXIES", "METRICS_TOKEN", "FRONTEND_URL", "BASE_URL"} {
		t.Setenv(k, "")
	}
	t.Setenv("DATABASE_URL", "postgres://u:p@127.0.0.1:5432/db")
}

func TestDefaultsFailClosed(t *testing.T) {
	clearEnv(t)
	// Без APP_ENV процесс считается боевым и не стартует с dev-настройками.
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("unset APP_ENV must mean prod and demand a JWT secret, got %v", err)
	}
	t.Setenv("JWT_SECRET", strings.Repeat("x", 40))
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "console") {
		t.Fatalf("prod must refuse the console mailer, got %v", err)
	}
	t.Setenv("MAIL_PROVIDER", "smtp")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP_HOST") {
		t.Fatalf("smtp needs a host, got %v", err)
	}
	t.Setenv("SMTP_HOST", "smtp.example.ru")
	t.Setenv("MAIL_FROM", "ФСП <noreply@example.ru>")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("prod must require secure cookies, got %v", err)
	}
	t.Setenv("COOKIE_SECURE", "true")
	cfg, err := Load()
	if err != nil || cfg.IsDev() || cfg.AppEnv != "prod" {
		t.Fatalf("complete prod config: %+v %v", cfg.AppEnv, err)
	}
}

func TestDevAndValidation(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "dev")
	cfg, err := Load()
	if err != nil || !cfg.IsDev() || cfg.FrontendURL != cfg.BaseURL {
		t.Fatalf("dev: %+v %v", cfg, err)
	}
	t.Setenv("APP_ENV", "staging")
	if _, err := Load(); err == nil {
		t.Fatal("unknown APP_ENV must fail")
	}
	t.Setenv("APP_ENV", "dev")
	t.Setenv("MAIL_PROVIDER", "pigeon")
	if _, err := Load(); err == nil {
		t.Fatal("unknown mail provider must fail")
	}
	t.Setenv("MAIL_PROVIDER", "console")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 127.0.0.1,::1")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	prefixes, err := cfg.TrustedProxyPrefixes()
	if err != nil || len(prefixes) != 3 || prefixes[1].String() != "127.0.0.1/32" || prefixes[2].String() != "::1/128" {
		t.Fatalf("trusted proxies: %v %v", prefixes, err)
	}
	t.Setenv("TRUSTED_PROXIES", "caddy")
	if _, err := Load(); err == nil {
		t.Fatal("a host name is not a proxy address")
	}
}
