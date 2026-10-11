// Package postgres содержит подключение к PostgreSQL, миграции и
// сгенерированный sqlc-слой запросов.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// enumTypeNames — enum-типы, используемые в массивах; регистрируются в pgx при подключении.
var enumTypeNames = []string{
	"specialization_code", "_specialization_code",
	"work_format", "_work_format",
	"grade_code", "_grade_code",
}

// Connect создаёт пул соединений и проверяет доступность базы.
// При недоступности повторяет попытки, чтобы переживать старт раньше БД
// (типичная ситуация в docker-compose).
func Connect(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	cfg.MaxConns = 16
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	// Массивы наших enum-типов (specialization_code[], work_format[]) pgx не знает без регистрации.
	// Миграции выполняются до создания пула (см. Migrate), поэтому типы уже существуют;
	// если их нет (пул поднят с MIGRATE_ON_START=false на пустой базе) — предупреждаем, но не падаем.
	// Контекст конструктора пула может быть уже отменён к моменту ленивого открытия соединения
	// (pgxpool использует его для MinConns), поэтому регистрация идёт в независимом контексте.
	// Ошибка возвращается наружу: соединение без типов непригодно и должно быть отброшено пулом.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		types, err := conn.LoadTypes(loadCtx, enumTypeNames)
		if err != nil {
			log.Warn("postgres: enum types not registered (run migrations first)", "err", err)
			return fmt.Errorf("postgres: load enum types: %w", err)
		}
		conn.TypeMap().RegisterTypes(types)
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: new pool: %w", err)
	}

	const attempts = 10
	for i := 1; i <= attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		log.Warn("postgres not ready", "attempt", i, "of", attempts, "err", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(time.Duration(i) * 500 * time.Millisecond):
		}
	}
	pool.Close()
	return nil, fmt.Errorf("postgres: not reachable after %d attempts: %w", attempts, err)
}
