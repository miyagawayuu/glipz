package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunCommunities adds community timelines and membership tables.
func RunCommunities(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("communities: check posts: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		`CREATE TABLE IF NOT EXISTS communities (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			icon_object_key TEXT,
			header_object_key TEXT,
			creator_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT communities_name_non_empty CHECK (btrim(name) <> '')
		)`,
		`ALTER TABLE communities ADD COLUMN IF NOT EXISTS details TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE communities ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}'::text[]`,
		`ALTER TABLE communities ADD COLUMN IF NOT EXISTS icon_object_key TEXT`,
		`ALTER TABLE communities ADD COLUMN IF NOT EXISTS header_object_key TEXT`,
		`ALTER TABLE communities DROP CONSTRAINT IF EXISTS communities_slug_non_empty`,
		`DROP INDEX IF EXISTS idx_communities_slug_lower`,
		`ALTER TABLE communities DROP COLUMN IF EXISTS slug`,
		`CREATE INDEX IF NOT EXISTS idx_communities_created_at ON communities (created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS community_members (
			community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (community_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_community_members_user ON community_members (user_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_community_members_pending ON community_members (community_id, created_at DESC) WHERE status = 'pending'`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS group_id UUID`,
		`ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_group_id_fkey`,
		`ALTER TABLE posts ADD CONSTRAINT posts_group_id_fkey
			FOREIGN KEY (group_id) REFERENCES communities(id) ON DELETE SET NULL`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("communities step %d: %w", i+1, err)
		}
	}
	return nil
}
