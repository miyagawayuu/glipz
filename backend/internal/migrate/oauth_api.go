package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunOAuthAPI creates tables for bot and third-party OAuth clients, authorization codes, and personal access tokens.
func RunOAuthAPI(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("migrate oauth api: check users: %w", err)
	}
	if n == 0 {
		return nil
	}
	steps := []string{
		`CREATE TABLE IF NOT EXISTS oauth_clients (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			client_secret_hash TEXT NOT NULL,
			redirect_uris TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_clients_user ON oauth_clients(user_id)`,
		`CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			code_sha256 TEXT NOT NULL UNIQUE,
			client_id UUID NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			redirect_uri TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT 'posts',
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires ON oauth_authorization_codes(expires_at)`,
		`CREATE TABLE IF NOT EXISTS api_personal_access_tokens (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			label TEXT NOT NULL DEFAULT '',
			token_prefix TEXT NOT NULL,
			secret_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_used_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_pat_user ON api_personal_access_tokens(user_id)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate oauth api step %d: %w", i+1, err)
		}
	}
	return nil
}
