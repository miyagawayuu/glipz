package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunPendingRegistrations creates the pending registration table for email verification.
func RunPendingRegistrations(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate pending registrations: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS pending_user_registrations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			handle TEXT NOT NULL DEFAULT '',
			birth_date DATE,
			token_sha256 TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			consumed_at TIMESTAMPTZ,
			verified_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pending_user_registrations_expires_at
			ON pending_user_registrations (expires_at)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS birth_date DATE`,
		`ALTER TABLE pending_user_registrations ADD COLUMN IF NOT EXISTS handle TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pending_user_registrations ADD COLUMN IF NOT EXISTS birth_date DATE`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate pending registrations step %d: %w", i+1, err)
		}
	}
	return nil
}
