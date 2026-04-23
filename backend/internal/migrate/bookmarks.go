package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunBookmarks applies the idempotent migration for post bookmarks.
// Must run after RunFederationIncoming: federation_incoming_post_bookmarks references federation_incoming_posts.
func RunBookmarks(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate bookmarks: check posts: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`CREATE TABLE IF NOT EXISTS post_bookmarks (
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_bookmarks_created_at ON post_bookmarks (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_post_bookmarks_user_created_at ON post_bookmarks (user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_post_bookmarks_post_id ON post_bookmarks (post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_bookmarks (
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, federation_incoming_post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_bookmarks_created_at
			ON federation_incoming_post_bookmarks (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_bookmarks_post_id
			ON federation_incoming_post_bookmarks (federation_incoming_post_id)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate bookmarks step %d: %w", i+1, err)
		}
	}
	return nil
}
