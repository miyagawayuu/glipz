package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunDMCallPolicyColumns adds DM call policy columns and updates the dm_call_scope constraint after group removal.
func RunDMCallPolicyColumns(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate dm_call_policy: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS dm_call_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS dm_call_scope TEXT NOT NULL DEFAULT 'none'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS dm_call_allowed_user_ids UUID[] NOT NULL DEFAULT '{}'::uuid[]`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS dm_call_allowed_group_ids UUID[] NOT NULL DEFAULT '{}'::uuid[]`,
		`UPDATE users SET dm_call_scope = 'none', dm_call_allowed_group_ids = '{}'::uuid[] WHERE dm_call_scope = 'specific_groups'`,
		`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_dm_call_scope_check`,
		`ALTER TABLE users ADD CONSTRAINT users_dm_call_scope_check CHECK (dm_call_scope IN ('none', 'all', 'followers', 'specific_users'))`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate dm_call_policy step %d: %w", i+1, err)
		}
	}
	return nil
}
