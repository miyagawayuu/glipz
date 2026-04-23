package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunWebPush adds the Web Push subscription table.
func RunWebPush(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate web push: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`CREATE TABLE IF NOT EXISTS user_push_subscriptions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			endpoint TEXT NOT NULL UNIQUE,
			p256dh TEXT NOT NULL,
			auth TEXT NOT NULL,
			user_agent TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_success_at TIMESTAMPTZ,
			last_failure_at TIMESTAMPTZ,
			failure_reason TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_push_subscriptions_user_id ON user_push_subscriptions (user_id, updated_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate web push step %d: %w", i+1, err)
		}
	}
	return nil
}
