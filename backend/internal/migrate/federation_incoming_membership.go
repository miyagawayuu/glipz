package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationIncomingMembership adds Patreon (and future) membership lock metadata to inbound federation posts.
func RunFederationIncomingMembership(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'federation_incoming_posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("federation_incoming_membership: check table: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS membership_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS membership_creator_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE federation_incoming_posts ADD COLUMN IF NOT EXISTS membership_tier_id TEXT NOT NULL DEFAULT ''`,
	}
	for _, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("federation_incoming_membership: %s: %w", q, err)
		}
	}
	return nil
}
