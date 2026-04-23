package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunGlipzProtocolTables creates Glipz Protocol tables idempotently.
func RunGlipzProtocolTables(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate glipz protocol: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	renameSteps := []string{
		`ALTER TABLE IF EXISTS user_activitypub_keys RENAME TO user_glipz_protocol_keys`,
		`ALTER TABLE IF EXISTS activitypub_remote_followers RENAME TO glipz_protocol_remote_followers`,
		`ALTER INDEX IF EXISTS idx_ap_remote_followers_local RENAME TO idx_glipz_protocol_remote_followers_local`,
	}
	for i, q := range renameSteps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate glipz protocol rename step %d: %w", i+1, err)
		}
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS user_glipz_protocol_keys (
			user_id UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
			private_key_pem TEXT NOT NULL,
			public_key_pem TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS glipz_protocol_remote_followers (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			local_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			remote_actor_id TEXT NOT NULL,
			remote_inbox TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (local_user_id, remote_actor_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_glipz_protocol_remote_followers_local ON glipz_protocol_remote_followers (local_user_id)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate glipz protocol step %d: %w", i+1, err)
		}
	}
	return nil
}
