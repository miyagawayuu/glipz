package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationUserPrivacy creates per-user federation block/mute tables (local-only; not federated).
func RunFederationUserPrivacy(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation user privacy: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS federation_user_blocks (
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			target_acct TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, target_acct),
			CONSTRAINT federation_user_blocks_target_nonempty CHECK (char_length(btrim(target_acct)) > 0)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_user_blocks_user_created
			ON federation_user_blocks (user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS federation_user_mutes (
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			target_acct TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, target_acct),
			CONSTRAINT federation_user_mutes_target_nonempty CHECK (char_length(btrim(target_acct)) > 0)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_user_mutes_user_created
			ON federation_user_mutes (user_id, created_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation user privacy step %d: %w", i+1, err)
		}
	}
	return nil
}
