package postgres

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// migrationsFS — SQL-миграции, встроенные в бинарник: сервис сам приводит
// схему к нужной версии при старте, без внешних инструментов.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrateLockKey — ключ advisory-блокировки миграций; один на все процессы, работающие с базой.
const migrateLockKey = 0x46535031 // "FSP1"

// Migrate применяет все неприменённые миграции (goose, формат SQL) через отдельное
// короткоживущее соединение. Вызывается ДО создания пула: тогда каждое соединение пула
// при открытии уже видит enum-типы и регистрирует их (см. Connect/AfterConnect),
// и сбрасывать пул после миграций не нужно.
func Migrate(ctx context.Context, url string) error {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return fmt.Errorf("migrate: parse config: %w", err)
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("migrate: ping: %w", err)
	}
	// Миграции сериализуются между процессами advisory-блокировкой: `go test ./...` запускает пакеты
	// параллельно, и на пустой базе два goose одновременно создавали одни и те же типы (23505).
	// Блокировка сессионная — держится на отдельном соединении до конца миграций.
	lock, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: lock connection: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if _, err := lock.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrateLockKey); err != nil {
		return fmt.Errorf("migrate: lock: %w", err)
	}
	defer func() {
		_, _ = lock.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrateLockKey)
	}()
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}

// Version возвращает текущую версию схемы по данным goose.
func Version(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var v int64
	err := pool.QueryRow(ctx, "SELECT COALESCE(max(version_id), 0) FROM goose_db_version").Scan(&v)
	return v, err
}
