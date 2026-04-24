package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFanclubPatreon stores per-user Patreon OAuth tokens (never exposed in public JSON).
func RunFanclubPatreon(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("fanclub_patreon: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS user_patreon_oauth (
			user_id uuid NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			patreon_user_id text NOT NULL,
			access_token text NOT NULL,
			refresh_token text NOT NULL,
			token_expires_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_user_patreon_oauth_patreon_user
			ON user_patreon_oauth (patreon_user_id);
	`)
	if err != nil {
		return fmt.Errorf("fanclub_patreon: create user_patreon_oauth: %w", err)
	}
	return nil
}
