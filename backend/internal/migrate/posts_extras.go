package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunPostsExtras adds schema support for text-only posts, scheduled publishing, and polls.
func RunPostsExtras(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("posts_extras: check posts: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS visible_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`CREATE INDEX IF NOT EXISTS idx_posts_visible_at ON posts (visible_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_posts_user_visible_at ON posts (user_id, visible_at DESC)`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'public'`,
		`UPDATE posts SET visibility = 'public' WHERE COALESCE(btrim(visibility), '') = ''`,

		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS feed_broadcast_done BOOLEAN NOT NULL DEFAULT TRUE`,

		// Membership locks (fanclub-style entitlements).
		// When membership_provider is non-empty, the post is considered membership-locked.
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS membership_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS membership_creator_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS membership_tier_id TEXT NOT NULL DEFAULT ''`,

		`ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_object_keys_len`,
		`ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_video_single`,
		`ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_media_type_check`,
		`ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_visibility_check`,

		`ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_media_object_keys`,

		`ALTER TABLE posts ADD CONSTRAINT posts_media_type_check CHECK (media_type IN ('image', 'video', 'none'))`,
		`ALTER TABLE posts ADD CONSTRAINT posts_visibility_check CHECK (visibility IN ('public', 'logged_in', 'followers', 'private'))`,
		`ALTER TABLE posts ADD CONSTRAINT posts_media_object_keys CHECK (
			(media_type = 'none' AND cardinality(object_keys) = 0)
			OR (media_type = 'image' AND cardinality(object_keys) BETWEEN 1 AND 4)
			OR (media_type = 'video' AND cardinality(object_keys) = 1)
		)`,

		`CREATE TABLE IF NOT EXISTS post_polls (
			post_id UUID PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
			ends_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS post_poll_options (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			position SMALLINT NOT NULL,
			label TEXT NOT NULL,
			UNIQUE (post_id, position)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_poll_options_post ON post_poll_options(post_id)`,
		`CREATE TABLE IF NOT EXISTS post_poll_votes (
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			option_id UUID NOT NULL REFERENCES post_poll_options(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_poll_votes_post ON post_poll_votes(post_id)`,
		`CREATE INDEX IF NOT EXISTS idx_post_poll_votes_option ON post_poll_votes(option_id)`,

		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS group_id UUID`,
	}

	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("posts_extras step %d: %w", i+1, err)
		}
	}
	return nil
}
