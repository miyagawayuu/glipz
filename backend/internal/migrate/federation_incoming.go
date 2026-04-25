package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationIncoming creates the idempotent table for posts received through federation.
func RunFederationIncoming(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation incoming: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS federation_incoming_posts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			object_iri TEXT NOT NULL,
			create_activity_iri TEXT,
			actor_iri TEXT NOT NULL,
			actor_acct TEXT NOT NULL DEFAULT '',
			actor_name TEXT NOT NULL DEFAULT '',
			actor_icon_url TEXT,
			actor_profile_url TEXT,
			caption_text TEXT NOT NULL DEFAULT '',
			media_type TEXT NOT NULL CHECK (media_type IN ('image', 'video', 'none')),
			media_urls TEXT[] NOT NULL DEFAULT '{}',
			is_nsfw BOOLEAN NOT NULL DEFAULT FALSE,
			published_at TIMESTAMPTZ NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			recipient_user_id UUID REFERENCES users (id) ON DELETE CASCADE,
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_incoming_object_iri
			ON federation_incoming_posts (object_iri)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_list
			ON federation_incoming_posts (published_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_recipient
			ON federation_incoming_posts (recipient_user_id, published_at DESC) WHERE deleted_at IS NULL`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation incoming step %d: %w", i+1, err)
		}
	}
	// Relax media_type for inbound posts (older DBs created with image/video/none only).
	for i, q := range []string{
		`ALTER TABLE federation_incoming_posts DROP CONSTRAINT IF EXISTS federation_incoming_posts_media_type_check`,
		`ALTER TABLE federation_incoming_posts ADD CONSTRAINT federation_incoming_posts_media_type_check CHECK (media_type IN ('image', 'video', 'audio', 'none'))`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation incoming media_type relax step %d: %w", i+1, err)
		}
	}
	return nil
}
