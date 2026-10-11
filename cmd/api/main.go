// Command api — HTTP API платформы «ФСП».
//
// Запуск локально: task dev (hot-reload) или go run ./cmd/api.
// Конфигурация — переменные окружения, см. .env.example.
// Флаг -openapi печатает спецификацию OpenAPI в YAML и завершает работу
// (база данных не нужна) — так экспортируется docs/api/openapi.yaml.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"fsp/internal/app/skills"
	apptesting "fsp/internal/app/testing"
	"fsp/internal/config"
	"fsp/internal/infrastructure/pdftext"
	"fsp/internal/server"
	"fsp/internal/storage/postgres"
	httptransport "fsp/internal/transport/http"
	_ "fsp/pkg/itembank/families/all" // регистрация банка заданий
)

// version подставляется при сборке: -ldflags "-X main.version=..."
var version = "dev"

func main() {
	// Дочерний процесс извлечения текста из PDF (см. pdftext.Isolated): только stdin → stdout,
	// без конфигурации, базы и сети.
	if len(os.Args) == 2 && os.Args[1] == pdftext.WorkerArg {
		os.Exit(pdftext.RunWorker(os.Stdin, os.Stdout, os.Stderr))
	}
	openapiOnly := flag.Bool("openapi", false, "вывести OpenAPI (YAML) и выйти")
	flag.Parse()

	if *openapiOnly {
		if err := printOpenAPI(); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func printOpenAPI() error {
	_, api := httptransport.NewRouter(httptransport.Deps{
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Version:   version,
		Auth:      &httptransport.AuthDeps{}, // операции регистрируются без сервиса: нужна только спецификация
		FSP:       &httptransport.FSPAuthDeps{},
		Candidate: &httptransport.CandidateDeps{},
		Testing:   &httptransport.TestingDeps{},
		Employer:  &httptransport.EmployerDeps{},
		Vacancies: &httptransport.VacancyDeps{},
		Admin:     &httptransport.AdminDeps{},
	})
	b, err := api.OpenAPI().YAML()
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}

func run() error {
	// .env — удобство локальной разработки; отсутствие файла не ошибка.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Миграции — до создания пула, чтобы соединения пула сразу знали enum-типы схемы.
	if cfg.MigrateOnStart {
		if err := postgres.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
	}
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	v, _ := postgres.Version(ctx, pool)
	log.Info("database ready", "schema_version", v)

	if cfg.MigrateOnStart {
		n, err := skills.Seed(ctx, pool)
		if err != nil {
			return err
		}
		log.Info("skills reference seeded", "count", n)
		nf, err := apptesting.SyncFamilies(ctx, pool)
		if err != nil {
			return err
		}
		log.Info("item families synced", "count", nf)
	}

	app, err := server.Build(ctx, cfg, pool, log, server.Options{Version: version})
	if err != nil {
		return err
	}
	handler, inviteSvc := app.Handler, app.Invitations

	// Калибровка сложности банка заданий по живым ответам: при старте и раз в сутки.
	go func() {
		recalibrate := func() {
			if _, err := app.Testing.Recalibrate(ctx, nil); err != nil {
				log.Error("recalibrate item bank", "err", err)
			}
		}
		recalibrate()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				recalibrate()
			}
		}
	}()

	// Фоновая задача: истечение приглашений по TTL (списки делают то же лениво при чтении).
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := inviteSvc.ExpireDue(ctx); err != nil {
					log.Error("expire invitations", "err", err)
				} else if n > 0 {
					log.Info("invitations expired", "count", n)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.AppEnv, "version", version,
			"docs", "http://"+strings.TrimPrefix(cfg.HTTPAddr, "http://")+httptransport.APIPrefix+"/docs")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down", "timeout", cfg.ShutdownTimeout)
	shCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func newLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if strings.EqualFold(format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
