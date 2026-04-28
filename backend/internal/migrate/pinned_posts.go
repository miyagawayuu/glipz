package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunPinnedPosts adds per-user profile pinned posts.
func RunPinnedPosts(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("pinned_posts: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS pinned_post_id UUID`,
		`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_pinned_post_id_fkey`,
		`ALTER TABLE users ADD CONSTRAINT users_pinned_post_id_fkey
			FOREIGN KEY (pinned_post_id) REFERENCES posts(id) ON DELETE SET NULL`,
		`CREATE INDEX IF NOT EXISTS idx_users_pinned_post_id
			ON users (pinned_post_id)
			WHERE pinned_post_id IS NOT NULL`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("pinned_posts step %d: %w", i+1, err)
		}
	}
	return nil
}
