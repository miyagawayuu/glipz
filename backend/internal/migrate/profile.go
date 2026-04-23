package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunProfile adds users.handle, bio, and image key columns idempotently.
func RunProfile(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate profile: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS handle TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS profile_external_urls jsonb NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_object_key TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS header_object_key TEXT`,
		`UPDATE users SET handle = COALESCE(NULLIF(trim(regexp_replace(lower(split_part(email, '@', 1)), '[^a-z0-9]+', '_', 'g')), ''), 'user') || '_' || substr(replace(cast(id as text), '-', ''), 1, 8)
			WHERE handle IS NULL OR trim(handle) = ''`,
		`ALTER TABLE users ALTER COLUMN handle SET NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_handle_lower ON users (lower(handle))`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate profile step %d: %w", i+1, err)
		}
	}
	return nil
}
