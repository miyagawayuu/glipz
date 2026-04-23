package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunModeration adds idempotent columns and tables for user suspensions and post reports.
func RunModeration(ctx context.Context, pool *pgxpool.Pool) error {
	var usersN int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&usersN); err != nil {
		return fmt.Errorf("migrate moderation: check users: %w", err)
	}
	if usersN == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ`,
		`CREATE TABLE IF NOT EXISTS post_reports (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			reporter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (reporter_user_id, post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_reports_post_created_at ON post_reports (post_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_reports (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			reporter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (reporter_user_id, federation_incoming_post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_reports_target_created_at
			ON federation_incoming_post_reports (federation_incoming_post_id, created_at DESC)`,
		`ALTER TABLE post_reports ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE post_reports ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'open'`,
		`ALTER TABLE post_reports ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ`,
		`ALTER TABLE federation_incoming_post_reports ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE federation_incoming_post_reports ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'open'`,
		`ALTER TABLE federation_incoming_post_reports ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ`,
		`ALTER TABLE post_reports DROP CONSTRAINT IF EXISTS post_reports_status_check`,
		`ALTER TABLE post_reports
			ADD CONSTRAINT post_reports_status_check
			CHECK (status IN ('open', 'resolved', 'dismissed', 'spam'))`,
		`ALTER TABLE federation_incoming_post_reports DROP CONSTRAINT IF EXISTS federation_incoming_post_reports_status_check`,
		`ALTER TABLE federation_incoming_post_reports
			ADD CONSTRAINT federation_incoming_post_reports_status_check
			CHECK (status IN ('open', 'resolved', 'dismissed', 'spam'))`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate moderation step %d: %w", i+1, err)
		}
	}
	return nil
}
