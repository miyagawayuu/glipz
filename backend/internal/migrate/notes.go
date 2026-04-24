package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunNotes creates note tables idempotently.
func RunNotes(ctx context.Context, pool *pgxpool.Pool) error {
	steps := []string{
		`CREATE TABLE IF NOT EXISTS notes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			title TEXT NOT NULL DEFAULT '',
			body_md TEXT NOT NULL DEFAULT '',
			editor_mode TEXT NOT NULL DEFAULT 'markdown' CHECK (editor_mode IN ('markdown', 'richtext')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_user_updated ON notes (user_id, updated_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("notes migrate step %d: %w", i+1, err)
		}
	}
	return runNotesExtras(ctx, pool)
}

// runNotesExtras adds visibility, draft, paid-body, and optional view-password fields.
func runNotesExtras(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'notes'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("notes_extras: check notes: %w", err)
	}
	if n == 0 {
		return nil
	}

	noteSteps := []string{
		`ALTER TABLE notes ADD COLUMN IF NOT EXISTS body_premium_md TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notes ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'published'`,
		`ALTER TABLE notes ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'public'`,
		`ALTER TABLE notes ADD COLUMN IF NOT EXISTS view_password_hash TEXT`,
		`ALTER TABLE notes ADD COLUMN IF NOT EXISTS view_password_hint TEXT`,
		`ALTER TABLE notes ADD COLUMN IF NOT EXISTS patreon_required_reward_tier_id TEXT`,
		`ALTER TABLE notes ADD COLUMN IF NOT EXISTS patreon_campaign_id TEXT`,
		`DO $$ BEGIN
			ALTER TABLE notes ADD CONSTRAINT notes_status_check CHECK (status IN ('draft', 'published'));
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
		`DO $$ BEGIN
			ALTER TABLE notes ADD CONSTRAINT notes_visibility_check CHECK (visibility IN ('public', 'followers', 'private'));
		EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
	}
	for i, q := range noteSteps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("notes_extras note step %d: %w", i+1, err)
		}
	}

	var u int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&u); err != nil {
		return fmt.Errorf("notes_extras: check users: %w", err)
	}
	if u == 0 {
		return nil
	}
	userStep := `ALTER TABLE users
		ADD COLUMN IF NOT EXISTS patreon_creator_access_token TEXT,
		ADD COLUMN IF NOT EXISTS patreon_creator_refresh_token TEXT,
		ADD COLUMN IF NOT EXISTS patreon_creator_token_expires_at TIMESTAMPTZ,
		ADD COLUMN IF NOT EXISTS patreon_campaign_id TEXT,
		ADD COLUMN IF NOT EXISTS patreon_required_reward_tier_id TEXT,
		ADD COLUMN IF NOT EXISTS patreon_member_access_token TEXT,
		ADD COLUMN IF NOT EXISTS patreon_member_refresh_token TEXT,
		ADD COLUMN IF NOT EXISTS patreon_member_token_expires_at TIMESTAMPTZ,
		ADD COLUMN IF NOT EXISTS patreon_member_user_id TEXT`
	if _, err := pool.Exec(ctx, userStep); err != nil {
		return fmt.Errorf("notes_extras users columns: %w", err)
	}
	return nil
}
