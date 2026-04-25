package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunRemoveDMCallSchema drops the removed WebRTC DM call tables and user settings.
func RunRemoveDMCallSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate remove_dm_calls: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`DROP TABLE IF EXISTS dm_call_events`,
		`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_dm_call_scope_check`,
		`ALTER TABLE users DROP COLUMN IF EXISTS dm_call_timeout_seconds`,
		`ALTER TABLE users DROP COLUMN IF EXISTS dm_call_enabled`,
		`ALTER TABLE users DROP COLUMN IF EXISTS dm_call_scope`,
		`ALTER TABLE users DROP COLUMN IF EXISTS dm_call_allowed_user_ids`,
		`ALTER TABLE users DROP COLUMN IF EXISTS dm_call_allowed_group_ids`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate remove_dm_calls step %d: %w", i+1, err)
		}
	}
	return nil
}
