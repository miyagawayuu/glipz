package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const federationDeliveryKindsCheck = "kind IN ('create', 'update', 'delete', 'announce', 'post_created', 'post_updated', 'post_deleted', 'repost_created', 'post_liked', 'post_unliked', 'post_reaction_added', 'post_reaction_removed', 'poll_voted', 'poll_tally_updated', 'dm_invite', 'dm_accept', 'dm_reject', 'dm_message', 'account_moved')"

// RunGlipzProtocolOutboxDelivery creates the durable outbound Glipz Protocol delivery queue with retries.
func RunGlipzProtocolOutboxDelivery(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate glipz protocol outbox delivery: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	renameSteps := []string{
		`ALTER TABLE IF EXISTS activitypub_outbox_deliveries RENAME TO glipz_protocol_outbox_deliveries`,
		`ALTER INDEX IF EXISTS idx_ap_outbox_del_pending RENAME TO idx_glipz_protocol_outbox_pending`,
		`ALTER INDEX IF EXISTS idx_ap_outbox_del_author RENAME TO idx_glipz_protocol_outbox_author`,
		`ALTER INDEX IF EXISTS idx_ap_outbox_del_status_created RENAME TO idx_glipz_protocol_outbox_status_created`,
	}
	for i, q := range renameSteps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate glipz protocol outbox rename step %d: %w", i+1, err)
		}
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS glipz_protocol_outbox_deliveries (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			author_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			post_id UUID NOT NULL,
			kind TEXT NOT NULL CHECK (` + federationDeliveryKindsCheck + `),
			inbox_url TEXT NOT NULL,
			payload JSONB NOT NULL,
			attempt_count INT NOT NULL DEFAULT 0,
			next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			locked_until TIMESTAMPTZ,
			last_error TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'dead')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_glipz_protocol_outbox_pending
			ON glipz_protocol_outbox_deliveries (next_attempt_at)
			WHERE status = 'pending'`,
		`CREATE INDEX IF NOT EXISTS idx_glipz_protocol_outbox_author
			ON glipz_protocol_outbox_deliveries (author_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_glipz_protocol_outbox_status_created
			ON glipz_protocol_outbox_deliveries (status, created_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate glipz protocol outbox delivery step %d: %w", i+1, err)
		}
	}
	// If an existing database still has the old kind CHECK, replace it with one that allows the current federation event set.
	if _, err := pool.Exec(ctx, `ALTER TABLE glipz_protocol_outbox_deliveries DROP CONSTRAINT IF EXISTS activitypub_outbox_deliveries_kind_check`); err != nil {
		return fmt.Errorf("migrate glipz protocol outbox delivery: drop old kind check: %w", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE glipz_protocol_outbox_deliveries DROP CONSTRAINT IF EXISTS glipz_protocol_outbox_deliveries_kind_check`); err != nil {
		return fmt.Errorf("migrate glipz protocol outbox delivery: drop kind check: %w", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE glipz_protocol_outbox_deliveries ADD CONSTRAINT glipz_protocol_outbox_deliveries_kind_check CHECK (`+federationDeliveryKindsCheck+`)`); err != nil {
		return fmt.Errorf("migrate glipz protocol outbox delivery: add kind check: %w", err)
	}
	return nil
}
