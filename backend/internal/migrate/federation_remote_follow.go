package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationRemoteFollow creates the idempotent Glipz-to-remote follow state table.
func RunFederationRemoteFollow(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation remote follow: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS federation_remote_follows (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			local_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			remote_actor_id TEXT NOT NULL,
			remote_inbox TEXT NOT NULL,
			state TEXT NOT NULL CHECK (state IN ('pending', 'accepted')),
			follow_activity_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (local_user_id, remote_actor_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_remote_follows_local ON federation_remote_follows (local_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_remote_follows_actor_accepted
			ON federation_remote_follows (remote_actor_id, local_user_id)
			WHERE state = 'accepted'`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation remote follow step %d: %w", i+1, err)
		}
	}
	return nil
}
