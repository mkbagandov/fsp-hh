package pgtest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestBeginTxAfterConnect воспроизводит ошибку «failed to deallocate cached statement(s): conn closed»:
// несколько транзакций подряд на пуле с AfterConnect (регистрация enum-типов).
func TestBeginTxAfterConnect(t *testing.T) {
	pool := Pool(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatalf("iteration %d: begin: %v", i, err)
		}
		var one int
		if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			t.Fatalf("iteration %d: query: %v", i, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("iteration %d: commit: %v", i, err)
		}
	}
	// Параллельные транзакции: заставляем пул открыть новые соединения с AfterConnect.
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				errCh <- err
				return
			}
			var one int
			if err := tx.QueryRow(ctx, "SELECT pg_sleep(0.05), 1").Scan(nil, &one); err != nil {
				_ = tx.Rollback(ctx)
				errCh <- err
				return
			}
			errCh <- tx.Commit(ctx)
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("parallel tx: %v", err)
		}
	}
}
