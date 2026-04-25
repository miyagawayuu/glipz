package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunSocial applies the idempotent migration for replies, likes, and reposts.
func RunSocial(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate social: check posts: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS reply_to_id UUID REFERENCES posts (id) ON DELETE CASCADE`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS reply_to_remote_object_iri TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_posts_reply_to ON posts (reply_to_id) WHERE reply_to_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_posts_reply_to_remote_object_iri
			ON posts (reply_to_remote_object_iri) WHERE COALESCE(btrim(reply_to_remote_object_iri), '') <> ''`,
		`CREATE TABLE IF NOT EXISTS post_likes (
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_likes_post_id ON post_likes (post_id)`,
		`CREATE INDEX IF NOT EXISTS idx_post_likes_user_created_at ON post_likes (user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS post_reactions (
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			emoji TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, post_id, emoji),
			CONSTRAINT post_reactions_emoji_non_empty CHECK (char_length(btrim(emoji)) > 0)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_reactions_post_id ON post_reactions (post_id)`,
		`CREATE INDEX IF NOT EXISTS idx_post_reactions_user_created_at ON post_reactions (user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS custom_emojis (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			shortcode_name TEXT NOT NULL,
			owner_user_id UUID REFERENCES users (id) ON DELETE CASCADE,
			domain TEXT NOT NULL DEFAULT '',
			object_key TEXT NOT NULL,
			is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT custom_emojis_shortcode_name_non_empty CHECK (char_length(btrim(shortcode_name)) > 0),
			CONSTRAINT custom_emojis_object_key_non_empty CHECK (char_length(btrim(object_key)) > 0)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_emojis_site_shortcode
			ON custom_emojis (lower(shortcode_name))
			WHERE owner_user_id IS NULL AND COALESCE(btrim(domain), '') = ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_emojis_user_shortcode
			ON custom_emojis (owner_user_id, lower(shortcode_name))
			WHERE owner_user_id IS NOT NULL AND COALESCE(btrim(domain), '') = ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_emojis_remote_shortcode
			ON custom_emojis (lower(shortcode_name), lower(domain))
			WHERE COALESCE(btrim(domain), '') <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_custom_emojis_enabled ON custom_emojis (is_enabled, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS post_reposts (
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, post_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_reposts_post_id ON post_reposts (post_id)`,
		`CREATE INDEX IF NOT EXISTS idx_post_reposts_created_at ON post_reposts (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_post_reposts_created_desc
			ON post_reposts (created_at DESC, user_id, post_id)`,
		`CREATE INDEX IF NOT EXISTS idx_post_reposts_user_created_at ON post_reposts (user_id, created_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate social step %d: %w", i+1, err)
		}
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE post_reposts ADD COLUMN IF NOT EXISTS comment_text TEXT`); err != nil {
		return fmt.Errorf("migrate social post_reposts.comment_text: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO post_reactions (user_id, post_id, emoji, created_at)
		SELECT pl.user_id, pl.post_id, '❤️', pl.created_at
		FROM post_likes pl
		ON CONFLICT (user_id, post_id, emoji) DO NOTHING
	`); err != nil {
		return fmt.Errorf("migrate social post_reactions migrate likes: %w", err)
	}
	return nil
}
