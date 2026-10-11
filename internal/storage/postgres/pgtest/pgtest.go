// Package pgtest — обвязка интеграционных тестов против реального PostgreSQL.
//
// Тесты используют TEST_DATABASE_URL (см. .env.example). Если переменная не задана,
// тест пропускается — юнит-тесты остаются быстрыми, а CI с сервисом postgres гоняет всё.
// Миграции применяются один раз на процесс; каждый тест получает транзакцию,
// которая откатывается в cleanup, поэтому тесты не видят данных друг друга.
package pgtest

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"fsp/internal/storage/postgres"
)

var (
	once sync.Once
	pool *pgxpool.Pool
	perr error
)

// Pool возвращает общий пул к тестовой базе с применёнными миграциями.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	once.Do(func() {
		// .env лежит в корне модуля; тесты запускаются из каталога пакета — ищем вверх.
		for _, p := range []string{".env", "../.env", "../../.env", "../../../.env", "../../../../.env"} {
			if err := godotenv.Load(p); err == nil {
				break
			}
		}
		url := os.Getenv("TEST_DATABASE_URL")
		if url == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Сначала миграции отдельным соединением, затем пул: соединения пула регистрируют enum-типы.
		if perr = postgres.Migrate(ctx, url); perr != nil {
			return
		}
		// Пул живёт дольше этой функции — ему нужен неотменяемый контекст.
		pool, perr = postgres.Connect(context.Background(), url, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	})
	if pool == nil && perr == nil {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	if perr != nil {
		t.Fatalf("pgtest: %v", perr)
	}
	return pool
}

// Tx открывает транзакцию, которая откатывается по завершении теста.
func Tx(t *testing.T) pgx.Tx {
	t.Helper()
	p := Pool(t)
	tx, err := p.Begin(context.Background())
	if err != nil {
		t.Fatalf("pgtest: begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
