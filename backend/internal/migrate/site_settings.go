package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunSiteSettings adds DB-backed instance settings edited from the admin panel.
func RunSiteSettings(ctx context.Context, pool *pgxpool.Pool) error {
	steps := []string{
		`CREATE TABLE IF NOT EXISTS site_settings (
			key TEXT PRIMARY KEY,
			value JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`INSERT INTO site_settings (key, value)
			VALUES ('registrations_enabled', 'true'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('server_name', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('server_description', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('admin_name', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('admin_email', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('terms_url', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('privacy_policy_url', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('nsfw_guidelines_url', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('federation_policy_summary', '""'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
		`INSERT INTO site_settings (key, value)
			VALUES ('operator_announcements', '[]'::jsonb)
			ON CONFLICT (key) DO NOTHING`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate site settings step %d: %w", i+1, err)
		}
	}
	return nil
}
