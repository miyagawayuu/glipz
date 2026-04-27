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
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_remote_account
			ON federation_incoming_posts (remote_account_id) WHERE remote_account_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_actor_portable
			ON federation_incoming_posts (actor_portable_id) WHERE COALESCE(btrim(actor_portable_id), '') <> ''`,

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
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate id portability step %d: %w", i+1, err)
		}
	}
	return nil
}
