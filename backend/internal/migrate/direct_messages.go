package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunDirectMessages adds direct message tables.
func RunDirectMessages(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate direct messages: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`CREATE TABLE IF NOT EXISTS user_dm_identity_keys (
			user_id UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
			algorithm TEXT NOT NULL DEFAULT 'ECDH-P256',
			public_jwk JSONB NOT NULL,
			encrypted_private_jwk JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE user_dm_identity_keys ADD COLUMN IF NOT EXISTS encrypted_private_jwk JSONB`,
		`CREATE TABLE IF NOT EXISTS dm_threads (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_low_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			user_high_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			skyway_room_name TEXT NOT NULL UNIQUE,
			user_low_last_read_at TIMESTAMPTZ,
			user_high_last_read_at TIMESTAMPTZ,
			last_message_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT dm_threads_distinct_users CHECK (user_low_id <> user_high_id),
			CONSTRAINT dm_threads_pair_unique UNIQUE (user_low_id, user_high_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_threads_user_low ON dm_threads (user_low_id, COALESCE(last_message_at, updated_at) DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_threads_user_high ON dm_threads (user_high_id, COALESCE(last_message_at, updated_at) DESC)`,
		`CREATE TABLE IF NOT EXISTS dm_messages (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			thread_id UUID NOT NULL REFERENCES dm_threads (id) ON DELETE CASCADE,
			sender_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			sender_payload JSONB NOT NULL,
			recipient_payload JSONB NOT NULL,
			attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_messages_thread_created_at ON dm_messages (thread_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_messages_sender_created_at ON dm_messages (sender_id, created_at DESC)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS dm_invite_auto_accept BOOLEAN NOT NULL DEFAULT false`,
		`CREATE TABLE IF NOT EXISTS dm_auto_accept_pending_invites (
			inviter_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			invitee_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (inviter_id, invitee_id),
			CONSTRAINT dm_auto_accept_pending_distinct CHECK (inviter_id <> invitee_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_auto_accept_pending_invitee ON dm_auto_accept_pending_invites (invitee_id)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate direct messages step %d: %w", i+1, err)
		}
	}
	return nil
}
