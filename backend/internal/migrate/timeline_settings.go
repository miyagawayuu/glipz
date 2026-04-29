package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunTimelineSettings adds user-scoped timeline customization settings.
func RunTimelineSettings(ctx context.Context, pool *pgxpool.Pool) error {
	steps := []string{
		`CREATE TABLE IF NOT EXISTS user_timeline_settings (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			settings JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate timeline settings step %d: %w", i+1, err)
		}
	}
	return nil
}
