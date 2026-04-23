package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationDM creates idempotent tables for federated end-to-end direct messages.
func RunFederationDM(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation dm: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS federation_dm_threads (
			thread_id UUID PRIMARY KEY,
			local_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			remote_acct TEXT NOT NULL,
			state TEXT NOT NULL CHECK (state IN ('invited_inbound', 'invited_outbound', 'accepted', 'rejected')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (local_user_id, remote_acct)
		)`,
		`ALTER TABLE federation_dm_threads DROP CONSTRAINT IF EXISTS federation_dm_threads_local_user_id_remote_acct_key`,
		`ALTER TABLE federation_dm_threads DROP CONSTRAINT IF EXISTS federation_dm_threads_state_check`,
		`ALTER TABLE federation_dm_threads ADD CONSTRAINT federation_dm_threads_state_check CHECK (state IN ('invited_inbound', 'invited_outbound', 'accepted', 'rejected'))`,
		`CREATE INDEX IF NOT EXISTS idx_federation_dm_threads_local_remote ON federation_dm_threads (local_user_id, remote_acct)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_dm_threads_local_updated
			ON federation_dm_threads (local_user_id, updated_at DESC)`,

		`CREATE TABLE IF NOT EXISTS federation_dm_messages (
			message_id UUID PRIMARY KEY,
			thread_id UUID NOT NULL REFERENCES federation_dm_threads (thread_id) ON DELETE CASCADE,
			sender_acct TEXT NOT NULL,
			sender_payload JSONB,
			recipient_payload JSONB NOT NULL,
			attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
			sent_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE federation_dm_messages ADD COLUMN IF NOT EXISTS sender_payload JSONB`,
		`CREATE INDEX IF NOT EXISTS idx_federation_dm_messages_thread_created
			ON federation_dm_messages (thread_id, created_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation dm step %d: %w", i+1, err)
		}
	}
	return nil
}

