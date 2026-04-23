package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFollow creates the user_follows table idempotently.
func RunFollow(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate follow: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS user_follows (
			follower_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			followee_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (follower_id, followee_id),
			CONSTRAINT user_follows_no_self CHECK (follower_id <> followee_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_follows_followee ON user_follows (followee_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_follows_follower ON user_follows (follower_id)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate follow step %d: %w", i+1, err)
		}
	}
	return nil
}
