// Package fsp — синхронизация достижений участника из реестра ФСП в кэш платформы.
//
// Токен ФСП ID не хранится: синхронизация выполняется в момент входа или привязки,
// пока access-токен ещё в руках; повторное обновление — новый проход через ФСП ID.
package fsp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/infrastructure/fspregistry"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Syncer загружает достижения из реестра и обновляет fsp_achievements.
type Syncer struct {
	pool     *pgxpool.Pool
	q        *sqlcgen.Queries
	registry fspregistry.Registry
	log      *slog.Logger
}

// New создаёт синхронизатор. registry может быть nil — тогда синхронизация пропускается.
func New(pool *pgxpool.Pool, registry fspregistry.Registry, log *slog.Logger) *Syncer {
	return &Syncer{pool: pool, q: sqlcgen.New(pool), registry: registry, log: log}
}

// Sync получает достижения и приводит кэш к состоянию реестра. Возвращает число записей.
// Ошибка реестра не фатальна для входа: она записывается в fsp_links.sync_error.
func (s *Syncer) Sync(ctx context.Context, userID uuid.UUID, fspID, accessToken string) (int, error) {
	if s == nil || s.registry == nil {
		return 0, nil
	}
	items, err := s.registry.Achievements(ctx, fspID, accessToken)
	if err != nil {
		msg := err.Error()
		_ = s.q.SetFSPLinkSynced(ctx, sqlcgen.SetFSPLinkSyncedParams{UserID: userID, SyncError: &msg})
		return 0, fmt.Errorf("fsp sync: %w", err)
	}
	kept := make([]string, 0, len(items))
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, a := range items {
			p, ok := toParams(userID, a)
			if !ok {
				if s.log != nil {
					s.log.Warn("fsp sync: skip invalid achievement", "external_id", a.ExternalID, "discipline", a.Discipline, "level", a.Level)
				}
				continue
			}
			if err := q.UpsertFSPAchievement(ctx, p); err != nil {
				return err
			}
			kept = append(kept, a.ExternalID)
		}
		if err := q.DeleteFSPAchievementsNotIn(ctx, sqlcgen.DeleteFSPAchievementsNotInParams{UserID: userID, Keep: kept}); err != nil {
			return err
		}
		return q.SetFSPLinkSynced(ctx, sqlcgen.SetFSPLinkSyncedParams{UserID: userID, SyncError: nil})
	})
	if err != nil {
		return 0, err
	}
	return len(kept), nil
}

func toParams(userID uuid.UUID, a fspregistry.Achievement) (sqlcgen.UpsertFSPAchievementParams, bool) {
	d, l, k := sqlcgen.FspDiscipline(a.Discipline), sqlcgen.FspLevel(a.Level), sqlcgen.FspResultKind(a.ResultKind)
	if a.ExternalID == "" || !d.Valid() || !l.Valid() || !k.Valid() || a.EventName == "" {
		return sqlcgen.UpsertFSPAchievementParams{}, false
	}
	date, err := time.Parse("2006-01-02", a.EventDate)
	if err != nil {
		return sqlcgen.UpsertFSPAchievementParams{}, false
	}
	p := sqlcgen.UpsertFSPAchievementParams{
		UserID: userID, ExternalID: a.ExternalID, Discipline: d, Level: l, EventName: a.EventName,
		EventDate: date, ResultKind: k,
	}
	if a.Place != nil {
		v := int32(*a.Place) //nolint:gosec // место в протоколе — небольшое число
		p.Place = &v
	}
	if a.Rating != nil {
		v := int32(*a.Rating) //nolint:gosec // рейтинг — небольшое число
		p.Rating = &v
	}
	if a.RankTitle != "" {
		v := a.RankTitle
		p.RankTitle = &v
	}
	if a.Role != "" {
		v := a.Role
		p.Role = &v
	}
	if a.TeamName != "" {
		v := a.TeamName
		p.TeamName = &v
	}
	return p, true
}

// Achievement — достижение в ответе API.
type Achievement struct {
	ExternalID string    `json:"external_id"`
	Discipline string    `json:"discipline" enum:"algorithmic,product,security,robotics,uav"`
	Level      string    `json:"level" enum:"regional,national,international"`
	EventName  string    `json:"event_name"`
	EventDate  time.Time `json:"event_date"`
	ResultKind string    `json:"result_kind" enum:"place,rank,rating,participation"`
	Place      *int32    `json:"place,omitempty"`
	RankTitle  *string   `json:"rank_title,omitempty"`
	Rating     *int32    `json:"rating,omitempty"`
	Role       *string   `json:"role,omitempty"`
	TeamName   *string   `json:"team_name,omitempty"`
	Verified   bool      `json:"verified"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// List возвращает достижения пользователя из кэша.
func (s *Syncer) List(ctx context.Context, userID uuid.UUID) ([]Achievement, error) {
	rows, err := s.q.ListFSPAchievements(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Achievement, 0, len(rows))
	for _, r := range rows {
		out = append(out, Achievement{
			ExternalID: r.ExternalID, Discipline: string(r.Discipline), Level: string(r.Level),
			EventName: r.EventName, EventDate: r.EventDate, ResultKind: string(r.ResultKind),
			Place: r.Place, RankTitle: r.RankTitle, Rating: r.Rating, Role: r.Role, TeamName: r.TeamName,
			Verified: r.Verified, FetchedAt: r.FetchedAt,
		})
	}
	return out, nil
}
