package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunUserBadges adds the users.badges column idempotently.
func RunUserBadges(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate user badges: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS badges TEXT[] NOT NULL DEFAULT '{}'::text[]`,
		`UPDATE users SET badges = '{}'::text[] WHERE badges IS NULL`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate user badges step %d: %w", i+1, err)
		}
	}
	return nil
}
