package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run applies idempotent startup-time database adjustments such as posts.object_keys migration.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate: check posts table: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS object_keys TEXT[]`,
		`DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'posts' AND column_name = 'object_key'
  ) THEN
    UPDATE posts SET object_keys = ARRAY[object_key]::text[]
      WHERE object_keys IS NULL AND object_key IS NOT NULL;
  END IF;
  UPDATE posts SET object_keys = ARRAY['legacy']::text[]
    WHERE object_keys IS NULL;
END $$`,
		`ALTER TABLE posts ALTER COLUMN object_keys SET NOT NULL`,
		`ALTER TABLE posts DROP COLUMN IF EXISTS object_key`,
	}

	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate step %d: %w", i+1, err)
		}
	}
	if err := RunIDPortability(ctx, pool); err != nil {
		return err
	}
	if err := RunBookmarks(ctx, pool); err != nil {
		return err
	}
	if err := RunCommunities(ctx, pool); err != nil {
		return err
	}
	if err := RunPinnedPosts(ctx, pool); err != nil {
		return err
	}
	if err := RunTimelineSettings(ctx, pool); err != nil {
		return err
	}
	if err := RunModeration(ctx, pool); err != nil {
		return err
	}
	if err := RunLegalCompliance(ctx, pool); err != nil {
		return err
	}
	return nil
}
