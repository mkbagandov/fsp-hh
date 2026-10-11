// Package server собирает приложение из конфигурации: сервисы, мейлер, извлечение PDF и HTTP-обработчик.
// Используется точкой входа API и интеграционными тестами HTTP-уровня (одна и та же сборка).
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/app/admin"
	"fsp/internal/app/applications"
	"fsp/internal/app/auth"
	"fsp/internal/app/candidates"
	"fsp/internal/app/employers"
	"fsp/internal/app/fsp"
	"fsp/internal/app/invitations"
	"fsp/internal/app/matching"
	"fsp/internal/app/notifications"
	apptesting "fsp/internal/app/testing"
	"fsp/internal/app/vacancies"
	"fsp/internal/config"
	"fsp/internal/infrastructure/authtoken"
	"fsp/internal/infrastructure/fspregistry"
	"fsp/internal/infrastructure/llm"
	"fsp/internal/infrastructure/mail"
	"fsp/internal/infrastructure/oidc"
	"fsp/internal/infrastructure/pdftext"
	"fsp/internal/infrastructure/secrets"
	httptransport "fsp/internal/transport/http"
)

// Options — то, что не задаётся конфигурацией: версия сборки и подменяемые в тестах зависимости.
type Options struct {
	Version string
	// Extract — извлечение текста из PDF; nil — изолированный дочерний процесс текущего бинарника.
	Extract func(ctx context.Context, data []byte) (string, error)
}

// App — собранное приложение: HTTP-обработчик и сервисы для фоновых задач.
type App struct {
	Handler     http.Handler
	Invitations *invitations.Service
	// Testing — для ежесуточной калибровки банка заданий.
	Testing *apptesting.Service
	// Outbox — письма консольного мейлера (в dev доступны на /api/v1/dev/outbox).
	Outbox *mail.Console
}

// Build собирает сервисы и HTTP-обработчик. Миграции, справочники и банк заданий готовит вызывающий.
func Build(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, log *slog.Logger, opt Options) (*App, error) {
	var err error
	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		jwtSecret, err = secrets.RandomToken(48)
		if err != nil {
			return nil, err
		}
		log.Warn("JWT_SECRET не задан: сгенерирован случайный, сессии не переживут перезапуск")
	}
	tokens, err := authtoken.New(jwtSecret, cfg.AccessTTL, "fsp")
	if err != nil {
		return nil, err
	}
	console := mail.NewConsole(log, 200)
	var mailer mail.Mailer = console
	if cfg.MailProvider == "smtp" {
		smtpMailer, err := mail.NewSMTP(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.MailFrom)
		if err != nil {
			return nil, err
		}
		mailer = smtpMailer
		log.Info("mail provider", "name", "smtp", "host", cfg.SMTPHost, "port", cfg.SMTPPort)
	}
	if cfg.IsDev() {
		log.Warn("APP_ENV=dev: письма с кодами доступны всем на /api/v1/dev/outbox — только для локальной разработки и демо-стенда")
	}
	trusted, err := cfg.TrustedProxyPrefixes()
	if err != nil {
		return nil, err
	}
	authSvc := auth.New(auth.Config{
		RefreshTTL: cfg.RefreshTTL, ConsentVersion: cfg.ConsentVersion, BaseURL: cfg.BaseURL,
	}, pool, tokens, mailer, log)

	var fspDeps *httptransport.FSPAuthDeps
	registry := fspregistry.New(cfg.FSPRegistryURL)
	achievements := fsp.New(pool, nil, log)
	if registry.Enabled() {
		achievements = fsp.New(pool, registry, log)
		authSvc.SetFSPSyncer(achievements)
	}
	if cfg.FSPOIDCIssuer != "" {
		oidcClient := oidc.New(oidc.Config{
			Issuer: cfg.FSPOIDCIssuer, ClientID: cfg.FSPOIDCClientID,
			ClientSecret: cfg.FSPOIDCClientSecret, RedirectURL: cfg.FSPOIDCRedirectURL,
		})
		fspDeps = &httptransport.FSPAuthDeps{
			Client: oidcClient, Tokens: tokens, Service: authSvc, Achievements: achievements,
			FrontendURL: strings.TrimRight(cfg.FrontendURL, "/"), CookieSecure: cfg.CookieSecure,
		}
		log.Info("fsp id enabled", "issuer", cfg.FSPOIDCIssuer, "registry", cfg.FSPRegistryURL)
	} else {
		fspDeps = &httptransport.FSPAuthDeps{Service: authSvc, Tokens: tokens, Achievements: achievements, FrontendURL: strings.TrimRight(cfg.FrontendURL, "/")}
	}

	llmClient, err := llm.New(llm.Config{Provider: cfg.LLMProvider, BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Model: cfg.LLMModel}, log)
	if err != nil {
		return nil, err
	}
	log.Info("llm provider", "name", llmClient.Name())
	candSvc := candidates.New(pool, log)
	extract := opt.Extract
	if extract == nil {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("executable path: %w", err)
		}
		extract = pdftext.NewIsolated(exe, 20*time.Second, 2).Extract
	}
	candSvc.SetResumeDeps(candidates.ResumeDeps{LLM: llmClient, UploadDir: cfg.UploadDir, Extract: extract})
	candSvc.SetPDFDeps(candidates.PDFDeps{
		Achievements: achievements,
		VerifyBase:   strings.TrimRight(cfg.FrontendURL, "/") + "/verify",
		FSPLinked: func(ctx context.Context, userID uuid.UUID) (bool, error) {
			info, err := authSvc.FSPLink(ctx, userID)
			return info.Linked, err
		},
	})

	employerSvc := employers.New(pool, llmClient, log)
	matchSvc := matching.New(pool, log)
	inviteSvc := invitations.New(invitations.Config{}, pool, log)
	notifySvc := notifications.New(pool, mailer, cfg.FrontendURL, log)
	adminSvc := admin.New(pool, log)
	if n, err := adminSvc.PromoteAdmins(ctx, cfg.AdminEmails); err != nil {
		return nil, fmt.Errorf("promote admins: %w", err)
	} else if n > 0 {
		log.Info("admins promoted", "count", n)
	}
	inviteSvc.SetNotifier(notifySvc)
	vacancySvc := vacancies.New(pool, log)
	applicationSvc := applications.New(pool, inviteSvc, log)
	applicationSvc.SetNotifier(notifySvc)

	webDir := cfg.WebDir
	switch webDir {
	case "":
		if _, err := os.Stat(filepath.Join("web", "dist", "index.html")); err == nil {
			webDir = filepath.Join("web", "dist")
		}
	case "-":
		webDir = ""
	}
	if webDir != "" {
		log.Info("serving web ui", "dir", webDir)
	}

	testingSvc := apptesting.New(apptesting.Config{GradeCooldown: time.Duration(cfg.GradeCooldownDays) * 24 * time.Hour}, pool, log)
	handler, _ := httptransport.NewRouter(httptransport.Deps{
		Logger:             log,
		Pool:               pool,
		Version:            opt.Version,
		IsDev:              cfg.IsDev(),
		TrustedProxies:     trusted,
		HSTS:               cfg.CookieSecure,
		MetricsToken:       cfg.MetricsToken,
		UserActive:         authSvc.IsActive,
		CORSOrigins:        cfg.CORSOrigins,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		Tokens:             tokens,
		Auth: &httptransport.AuthDeps{
			Service: authSvc, CookieSecure: cfg.CookieSecure, RefreshTTL: cfg.RefreshTTL,
		},
		Outbox:    console,
		FSP:       fspDeps,
		Candidate: &httptransport.CandidateDeps{Service: candSvc},
		Testing:   &httptransport.TestingDeps{Service: testingSvc},
		Employer:  &httptransport.EmployerDeps{Employers: employerSvc, Matching: matchSvc, Invitations: inviteSvc},
		Vacancies: &httptransport.VacancyDeps{Employers: employerSvc, Vacancies: vacancySvc, Applications: applicationSvc, Matching: matchSvc, Notifications: notifySvc},
		Admin:     &httptransport.AdminDeps{Service: adminSvc, Testing: testingSvc},
		WebDir:    webDir,
	})
	return &App{Handler: handler, Invitations: inviteSvc, Testing: testingSvc, Outbox: console}, nil
}
