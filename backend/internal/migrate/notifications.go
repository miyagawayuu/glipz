package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunNotifications adds the notifications table idempotently.
func RunNotifications(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate notifications: check users: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`CREATE TABLE IF NOT EXISTS notifications (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			recipient_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			actor_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			kind TEXT NOT NULL CHECK (kind IN ('reply', 'like', 'repost', 'follow')),
			subject_post_id UUID REFERENCES posts (id) ON DELETE SET NULL,
			actor_post_id UUID REFERENCES posts (id) ON DELETE SET NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			read_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_recipient_created ON notifications (recipient_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_recipient_unread ON notifications (recipient_id) WHERE read_at IS NULL`,
		`ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check`,
		`ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('reply', 'like', 'repost', 'follow', 'dm_invite'))`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate notifications step %d: %w", i+1, err)
		}
	}
	return nil
}
