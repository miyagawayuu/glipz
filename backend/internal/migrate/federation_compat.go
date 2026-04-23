package migrate

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationSubscribers creates the remote subscriber table.
// Existing databases are migrated from activitypub_remote_followers to glipz_protocol_remote_followers.
func RunFederationSubscribers(ctx context.Context, pool *pgxpool.Pool) error {
	return RunGlipzProtocolTables(ctx, pool)
}

// RunFederationDeliveries creates the outbound federation delivery queue.
// Existing databases are migrated from activitypub_outbox_deliveries to glipz_protocol_outbox_deliveries.
func RunFederationDeliveries(ctx context.Context, pool *pgxpool.Pool) error {
	return RunGlipzProtocolOutboxDelivery(ctx, pool)
}
