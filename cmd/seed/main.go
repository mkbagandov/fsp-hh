// Command seed заполняет базу демо-аккаунтами и синтетическими данными (этап 14).
//
//	go run ./cmd/seed                 — 300 кандидатов, 10 компаний, потребности, вакансии, приглашения, отклики
//	go run ./cmd/seed -candidates 1000
//	go run ./cmd/seed -reset          — удалить всех сид-пользователей и их данные
//	go run ./cmd/seed -reset -run     — сбросить и сгенерировать заново
//	go run ./cmd/seed -recreate-db    — пересоздать базу из DATABASE_URL (только *_e2e) и заполнить её
//
// Демо-аккаунты (пароль Demo1234!): demo-candidate@seed.fsp.local, demo-candidate-new@seed.fsp.local,
// demo-employer@seed.fsp.local, demo-admin@seed.fsp.local.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"fsp/internal/app/skills"
	"fsp/internal/config"
	"fsp/internal/seed"
	"fsp/internal/storage/postgres"
	_ "fsp/pkg/itembank/families/all"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	candidates := flag.Int("candidates", 300, "сколько синтетических кандидатов создать")
	seedVal := flag.Uint64("seed", 42, "зерно генератора")
	reset := flag.Bool("reset", false, "удалить сид-данные")
	runAfter := flag.Bool("run", false, "после -reset сгенерировать заново")
	skipTests := flag.Bool("skip-tests", false, "не проходить тест-сессии (быстрее, но без категорий)")
	recreate := flag.Bool("recreate-db", false, "удалить и создать заново базу из DATABASE_URL (разрешено только для баз *_e2e), затем заполнить")
	flag.Parse()

	_ = godotenv.Load()
	cfg, err := config.Load()
	if err == nil && cfg.AppEnv == "prod" {
		err = fmt.Errorf("сид создаёт демо-аккаунты с известным паролем и в prod не запускается (APP_ENV=prod)")
	}
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	if *recreate {
		if err := recreateDatabase(ctx, cfg.DatabaseURL, log); err != nil {
			return fmt.Errorf("recreate db: %w", err)
		}
	}
	if err := postgres.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	// Справочник навыков нужен до генерации профилей (в свежей базе его ещё нет).
	if _, err := skills.Seed(ctx, pool); err != nil {
		return fmt.Errorf("skills: %w", err)
	}

	started := time.Now()
	if *reset {
		n, err := seed.Reset(ctx, pool)
		if err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		log.Info("seed: reset", "users_deleted", n)
		if !*runAfter {
			return nil
		}
	}
	if err := seed.Run(ctx, pool, log, seed.Options{Candidates: *candidates, Seed: *seedVal, SkipTests: *skipTests}); err != nil {
		return err
	}
	log.Info("seed: finished", "elapsed", time.Since(started).Round(time.Second).String())
	return nil
}

// recreateDatabase удаляет и создаёт заново базу из DSN. Защита: имя базы обязано оканчиваться на _e2e,
// чтобы флаг нельзя было случайно применить к рабочей или чужой базе.
func recreateDatabase(ctx context.Context, dsn string, log *slog.Logger) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return err
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !strings.HasSuffix(name, "_e2e") {
		return fmt.Errorf("пересоздавать можно только базу *_e2e, а не %q", name)
	}
	admin := *u
	admin.Path = "/postgres"
	conn, err := pgx.Connect(ctx, admin.String())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		return err
	}
	log.Info("seed: database recreated", "db", name)
	return nil
}
