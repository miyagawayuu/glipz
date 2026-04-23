package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunLiveSpaceCleanup drops legacy live/space tables after product removal.
func RunLiveSpaceCleanup(ctx context.Context, pool *pgxpool.Pool) error {
	steps := []string{
		`DROP TABLE IF EXISTS space_presence CASCADE`,
		`DROP TABLE IF EXISTS space_invites CASCADE`,
		`DROP TABLE IF EXISTS spaces CASCADE`,
		`DROP TABLE IF EXISTS live_streams CASCADE`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("live space cleanup step %d: %w", i+1, err)
		}
	}
	return nil
}
