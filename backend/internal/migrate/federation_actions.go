package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationActions creates idempotent tables for federated likes, reactions, and poll synchronization.
func RunFederationActions(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation actions: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS post_remote_likes (
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			remote_actor_id TEXT NOT NULL,
			remote_actor_acct TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (post_id, remote_actor_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_remote_likes_post ON post_remote_likes (post_id)`,
		`CREATE TABLE IF NOT EXISTS post_remote_reactions (
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			remote_actor_id TEXT NOT NULL,
			remote_actor_acct TEXT NOT NULL DEFAULT '',
			emoji TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (post_id, remote_actor_id, emoji)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_remote_reactions_post ON post_remote_reactions (post_id)`,
		`CREATE TABLE IF NOT EXISTS post_remote_poll_votes (
			post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
			remote_actor_id TEXT NOT NULL,
			remote_actor_acct TEXT NOT NULL DEFAULT '',
			option_position SMALLINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (post_id, remote_actor_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_remote_poll_votes_post ON post_remote_poll_votes (post_id)`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS like_count BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS reply_to_object_iri TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS repost_of_object_iri TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS repost_comment TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS has_view_password BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS view_password_scope INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS view_password_text_ranges JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS unlock_url TEXT`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_likes (
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (federation_incoming_post_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_likes_post
			ON federation_incoming_post_likes (federation_incoming_post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_reposts (
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			comment_text TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (federation_incoming_post_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_reposts_post
			ON federation_incoming_post_reposts (federation_incoming_post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_reactions (
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			emoji TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (federation_incoming_post_id, user_id, emoji)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_reactions_post
			ON federation_incoming_post_reactions (federation_incoming_post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_remote_reactions (
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			remote_actor_id TEXT NOT NULL,
			remote_actor_acct TEXT NOT NULL DEFAULT '',
			emoji TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (federation_incoming_post_id, remote_actor_id, emoji)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_remote_reactions_post
			ON federation_incoming_post_remote_reactions (federation_incoming_post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_polls (
			federation_incoming_post_id UUID PRIMARY KEY REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			ends_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_poll_options (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			position SMALLINT NOT NULL,
			label TEXT NOT NULL,
			votes BIGINT NOT NULL DEFAULT 0,
			UNIQUE (federation_incoming_post_id, position)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_poll_options_post
			ON federation_incoming_post_poll_options (federation_incoming_post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_poll_votes (
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			option_id UUID NOT NULL REFERENCES federation_incoming_post_poll_options (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (federation_incoming_post_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_poll_votes_post
			ON federation_incoming_post_poll_votes (federation_incoming_post_id)`,
		`CREATE TABLE IF NOT EXISTS federation_incoming_post_unlocks (
			federation_incoming_post_id UUID NOT NULL REFERENCES federation_incoming_posts (id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			caption_text TEXT NOT NULL DEFAULT '',
			media_type TEXT NOT NULL CHECK (media_type IN ('image', 'video', 'none')),
			media_urls TEXT[] NOT NULL DEFAULT '{}',
			is_nsfw BOOLEAN NOT NULL DEFAULT FALSE,
			expires_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (federation_incoming_post_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_post_unlocks_user
			ON federation_incoming_post_unlocks (user_id, expires_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation actions step %d: %w", i+1, err)
		}
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE glipz_protocol_outbox_deliveries DROP CONSTRAINT IF EXISTS activitypub_outbox_deliveries_kind_check`); err != nil {
		return fmt.Errorf("migrate federation actions: drop old kind check: %w", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE glipz_protocol_outbox_deliveries DROP CONSTRAINT IF EXISTS glipz_protocol_outbox_deliveries_kind_check`); err != nil {
		return fmt.Errorf("migrate federation actions: drop kind check: %w", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE glipz_protocol_outbox_deliveries ADD CONSTRAINT glipz_protocol_outbox_deliveries_kind_check CHECK (`+federationDeliveryKindsCheck+`)`); err != nil {
		return fmt.Errorf("migrate federation actions: add kind check: %w", err)
	}
	for i, q := range []string{
		`ALTER TABLE federation_incoming_post_unlocks DROP CONSTRAINT IF EXISTS federation_incoming_post_unlocks_media_type_check`,
		`ALTER TABLE federation_incoming_post_unlocks ADD CONSTRAINT federation_incoming_post_unlocks_media_type_check CHECK (media_type IN ('image', 'video', 'audio', 'none'))`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation actions unlocks media_type step %d: %w", i+1, err)
		}
	}
	return nil
}
