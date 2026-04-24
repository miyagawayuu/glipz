package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationIncomingNoteUnlocks creates per-user unlock storage for federated incoming notes.
func RunFederationIncomingNoteUnlocks(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation incoming note unlocks: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS federation_incoming_note_unlocks (
			federation_incoming_note_id UUID NOT NULL REFERENCES federation_incoming_notes (id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			body_premium_md TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ,
			PRIMARY KEY (federation_incoming_note_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_incoming_note_unlocks_user
			ON federation_incoming_note_unlocks (user_id, expires_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation incoming note unlocks step %d: %w", i+1, err)
		}
	}
	return nil
}

