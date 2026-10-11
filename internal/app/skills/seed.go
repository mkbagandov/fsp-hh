// Package skills — справочник навыков в базе: сидирование из refdata и доступ для API.
package skills

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/domain/refdata"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Seed приводит таблицу skills к содержимому skills.yaml (идемпотентно, по коду навыка).
// Вызывается при старте API после миграций: справочник живёт в коде, база — его проекция
// для внешних ключей и фильтров.
func Seed(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	idx, err := refdata.Skills()
	if err != nil {
		return 0, err
	}
	n := 0
	err = postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		for _, s := range idx.Skills {
			specs := make([]sqlcgen.SpecializationCode, 0, len(s.Specs))
			for _, sp := range s.Specs {
				specs = append(specs, sqlcgen.SpecializationCode(sp))
			}
			var topic *string
			if s.Topic != "" {
				t := s.Topic
				topic = &t
			}
			synonyms := s.Synonyms
			if synonyms == nil {
				synonyms = []string{}
			}
			if _, err := q.UpsertSkill(ctx, sqlcgen.UpsertSkillParams{
				Code: s.Code, Name: s.Name, GroupCode: s.Group, Specs: specs, Topic: topic, Synonyms: synonyms,
			}); err != nil {
				return fmt.Errorf("seed skill %q: %w", s.Code, err)
			}
			n++
		}
		return nil
	})
	return n, err
}
