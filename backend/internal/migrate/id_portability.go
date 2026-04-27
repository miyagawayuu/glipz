package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunIDPortability adds portable account identity columns and remote account registry tables.
func RunIDPortability(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate id portability: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS portable_id TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS account_public_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS account_private_key_encrypted TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS moved_to_acct TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS moved_at TIMESTAMPTZ`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS also_known_as TEXT[] NOT NULL DEFAULT '{}'`,
		`UPDATE users
			SET portable_id = 'glipz:id:local-' || replace(id::text, '-', '')
			WHERE portable_id IS NULL OR btrim(portable_id) = ''`,
		`ALTER TABLE users ALTER COLUMN portable_id SET NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_portable_id_unique ON users (portable_id)`,

		`CREATE TABLE IF NOT EXISTS federation_remote_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			portable_id TEXT NOT NULL,
			current_acct TEXT NOT NULL DEFAULT '',
			profile_url TEXT NOT NULL DEFAULT '',
			posts_url TEXT NOT NULL DEFAULT '',
			inbox_url TEXT NOT NULL DEFAULT '',
			public_key TEXT NOT NULL DEFAULT '',
			moved_to TEXT NOT NULL DEFAULT '',
			moved_from TEXT NOT NULL DEFAULT '',
			also_known_as TEXT[] NOT NULL DEFAULT '{}',
			last_verified_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT federation_remote_accounts_portable_nonempty CHECK (char_length(btrim(portable_id)) > 0)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_remote_accounts_portable
			ON federation_remote_accounts (portable_id)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_remote_accounts_current_acct
			ON federation_remote_accounts (lower(current_acct)) WHERE COALESCE(btrim(current_acct), '') <> ''`,

		`ALTER TABLE IF EXISTS federation_incoming_posts ADD COLUMN IF NOT EXISTS actor_portable_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE IF EXISTS federation_incoming_posts ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS federation_incoming_posts ADD COLUMN IF NOT EXISTS object_id TEXT NOT NULL DEFAULT ''`,
		`DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'federation_incoming_posts'
  ) THEN
    UPDATE federation_incoming_posts SET object_id = object_iri WHERE COALESCE(btrim(object_id), '') = '';
  END IF;
END $$`,
		`DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'federation_incoming_posts'
  ) THEN
    CREATE INDEX IF NOT EXISTS idx_federation_incoming_remote_account
      ON federation_incoming_posts (remote_account_id) WHERE remote_account_id IS NOT NULL;
    CREATE INDEX IF NOT EXISTS idx_federation_incoming_actor_portable
      ON federation_incoming_posts (actor_portable_id) WHERE COALESCE(btrim(actor_portable_id), '') <> '';
  END IF;
END $$`,

		`ALTER TABLE IF EXISTS federation_remote_follows ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS federation_remote_follows ADD COLUMN IF NOT EXISTS remote_current_acct TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE IF EXISTS glipz_protocol_remote_followers ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS glipz_protocol_remote_followers ADD COLUMN IF NOT EXISTS remote_current_acct TEXT NOT NULL DEFAULT ''`,

		`ALTER TABLE IF EXISTS post_remote_likes ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS post_remote_reactions ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS post_remote_poll_votes ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS federation_incoming_post_remote_reactions ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS federation_dm_threads ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS federation_user_blocks ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,
		`ALTER TABLE IF EXISTS federation_user_mutes ADD COLUMN IF NOT EXISTS remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL`,

		`CREATE TABLE IF NOT EXISTS identity_transfer_sessions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			portable_id TEXT NOT NULL DEFAULT '',
			token_hash TEXT NOT NULL,
			token_nonce TEXT NOT NULL DEFAULT '',
			allowed_target_origin TEXT NOT NULL DEFAULT '',
			include_private BOOLEAN NOT NULL DEFAULT FALSE,
			include_gated BOOLEAN NOT NULL DEFAULT FALSE,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ,
			revoked_at TIMESTAMPTZ,
			created_ip_hash TEXT NOT NULL DEFAULT '',
			last_used_ip_hash TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT identity_transfer_sessions_token_hash_nonempty CHECK (char_length(btrim(token_hash)) > 0)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_identity_transfer_sessions_user
			ON identity_transfer_sessions (user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_identity_transfer_sessions_active
			ON identity_transfer_sessions (id, expires_at)
			WHERE revoked_at IS NULL AND used_at IS NULL`,

		`CREATE TABLE IF NOT EXISTS identity_transfer_import_jobs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			source_origin TEXT NOT NULL,
			target_origin TEXT NOT NULL DEFAULT '',
			source_session_id UUID NOT NULL,
			source_token_encrypted TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
			total_posts INTEGER NOT NULL DEFAULT 0,
			imported_posts INTEGER NOT NULL DEFAULT 0,
			failed_posts INTEGER NOT NULL DEFAULT 0,
			total_items INTEGER NOT NULL DEFAULT 0,
			imported_items INTEGER NOT NULL DEFAULT 0,
			stats JSONB NOT NULL DEFAULT '{}'::jsonb,
			next_cursor TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			locked_until TIMESTAMPTZ,
			last_error TEXT NOT NULL DEFAULT '',
			include_private BOOLEAN NOT NULL DEFAULT FALSE,
			include_gated BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE IF EXISTS identity_transfer_import_jobs ADD COLUMN IF NOT EXISTS target_origin TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE IF EXISTS identity_transfer_import_jobs ADD COLUMN IF NOT EXISTS total_items INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE IF EXISTS identity_transfer_import_jobs ADD COLUMN IF NOT EXISTS imported_items INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE IF EXISTS identity_transfer_import_jobs ADD COLUMN IF NOT EXISTS stats JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`CREATE INDEX IF NOT EXISTS idx_identity_transfer_import_jobs_claim
			ON identity_transfer_import_jobs (next_attempt_at, created_at)
			WHERE status IN ('pending', 'running')`,
		`CREATE INDEX IF NOT EXISTS idx_identity_transfer_import_jobs_user
			ON identity_transfer_import_jobs (user_id, created_at DESC)`,

		`CREATE TABLE IF NOT EXISTS identity_transfer_post_mappings (
			job_id UUID NOT NULL REFERENCES identity_transfer_import_jobs (id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			source_post_id TEXT NOT NULL,
			original_object_id TEXT NOT NULL,
			new_post_id UUID REFERENCES posts (id) ON DELETE SET NULL,
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'imported', 'failed', 'skipped')),
			last_error TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (job_id, original_object_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_identity_transfer_post_mappings_user
			ON identity_transfer_post_mappings (user_id, created_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate id portability step %d: %w", i+1, err)
		}
	}
	return nil
}
