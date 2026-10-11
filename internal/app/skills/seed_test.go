package skills

import (
	"context"
	"testing"

	"fsp/internal/storage/postgres/pgtest"
	"fsp/internal/storage/postgres/sqlcgen"
)

func TestSeedIsIdempotent(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := context.Background()
	n1, err := Seed(ctx, pool)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	n2, err := Seed(ctx, pool)
	if err != nil || n2 != n1 {
		t.Fatalf("second seed: n1=%d n2=%d err=%v", n1, n2, err)
	}
	total, err := sqlcgen.New(pool).CountSkills(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if int(total) != n1 || n1 < 250 {
		t.Fatalf("expected %d skills in db (>=250), got %d", n1, total)
	}
	s, err := sqlcgen.New(pool).GetSkillByCode(ctx, "postgresql")
	if err != nil || s.Name != "PostgreSQL" {
		t.Fatalf("postgresql skill: %+v %v", s, err)
	}
}
