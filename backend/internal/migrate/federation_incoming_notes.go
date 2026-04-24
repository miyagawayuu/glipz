package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationIncomingNotes creates the idempotent table for notes received through federation.
func RunFederationIncomingNotes(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation incoming notes: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS federation_incoming_notes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			object_iri TEXT NOT NULL,
			actor_iri TEXT NOT NULL,
			actor_acct TEXT NOT NULL DEFAULT '',
			actor_name TEXT NOT NULL DEFAULT '',
			actor_icon_url TEXT,
			actor_profile_url TEXT,
			title TEXT NOT NULL DEFAULT '',
			body_md TEXT NOT NULL DEFAULT '',
			visibility TEXT NOT NULL DEFAULT 'public',
			published_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			has_premium BOOLEAN NOT NULL DEFAULT FALSE,
			paywall_provider TEXT NOT NULL DEFAULT '',
			patreon_campaign_id TEXT NOT NULL DEFAULT '',
			patreon_required_reward_tier_id TEXT NOT NULL DEFAULT '',
			unlock_url TEXT NOT NULL DEFAULT '',
			unlocked_body_premium_md TEXT NOT NULL DEFAULT '',
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_incoming_notes_object_iri
			ON federation_incoming_notes (object_iri)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_notes_actor
			ON federation_incoming_notes (actor_iri, published_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_notes_published
			ON federation_incoming_notes (published_at DESC, id DESC) WHERE deleted_at IS NULL`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation incoming notes step %d: %w", i+1, err)
		}
	}
	return nil
}

