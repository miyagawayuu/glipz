package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunSearchTags creates search and hashtag tables idempotently.
func RunSearchTags(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate search tags: check posts: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS hashtags (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tag TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT hashtags_tag_not_blank CHECK (btrim(tag) <> '')
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_hashtags_tag ON hashtags (tag)`,
		`CREATE TABLE IF NOT EXISTS post_hashtags (
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			hashtag_id UUID NOT NULL REFERENCES hashtags (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (post_id, hashtag_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_hashtags_hashtag
			ON post_hashtags (hashtag_id, post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_hashtags (
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			hashtag_id UUID NOT NULL REFERENCES hashtags (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (federation_incoming_post_id, hashtag_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_hashtags_hashtag
			ON federation_incoming_post_hashtags (hashtag_id, federation_incoming_post_id)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate search tags step %d: %w", i+1, err)
		}
	}
	return nil
}
