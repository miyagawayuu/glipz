package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunFederationRemoteCustomEmojis creates a cache table for resolving remote custom emoji shortcodes to image URLs.
func RunFederationRemoteCustomEmojis(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate federation remote custom emojis: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS federation_remote_custom_emojis (
			domain TEXT NOT NULL,
			shortcode_name TEXT NOT NULL,
			image_url TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (domain, shortcode_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_remote_custom_emojis_expires
			ON federation_remote_custom_emojis (expires_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate federation remote custom emojis step %d: %w", i+1, err)
		}
	}
	return nil
}

