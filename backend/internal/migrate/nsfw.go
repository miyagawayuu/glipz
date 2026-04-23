package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunNSFW adds NSFW and view-password columns to posts idempotently.
func RunNSFW(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate nsfw: check posts: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS is_nsfw BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS view_password_hash TEXT`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS view_password_scope INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS view_password_text_ranges JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`UPDATE posts SET view_password_scope = 4 WHERE COALESCE(btrim(view_password_hash), '') <> '' AND COALESCE(view_password_scope, 0) = 0`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate nsfw step %d: %w", i+1, err)
		}
	}
	return nil
}
