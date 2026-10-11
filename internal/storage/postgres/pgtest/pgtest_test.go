package pgtest

import (
	"context"
	"testing"
)

func TestMigrationsApplied(t *testing.T) {
	tx := Tx(t)
	var version int64
	if err := tx.QueryRow(context.Background(), "SELECT max(version_id) FROM goose_db_version").Scan(&version); err != nil {
		t.Fatalf("goose_db_version: %v", err)
	}
	if version < 1 {
		t.Fatalf("expected schema version >= 1, got %d", version)
	}
	var ext int
	if err := tx.QueryRow(context.Background(),
		"SELECT count(*) FROM pg_extension WHERE extname IN ('citext','pgcrypto')").Scan(&ext); err != nil {
		t.Fatal(err)
	}
	if ext != 2 {
		t.Fatalf("expected citext and pgcrypto, got %d extensions", ext)
	}
}
