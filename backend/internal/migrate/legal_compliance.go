package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunLegalCompliance adds legal request, preservation, audit, and DM report tables.
func RunLegalCompliance(ctx context.Context, pool *pgxpool.Pool) error {
	var usersN int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&usersN); err != nil {
		return fmt.Errorf("migrate legal compliance: check users: %w", err)
	}
	if usersN == 0 {
		return nil
	}

	steps := []string{
		`CREATE TABLE IF NOT EXISTS law_enforcement_requests (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			request_type TEXT NOT NULL DEFAULT 'legal_process',
			agency_name TEXT NOT NULL,
			jurisdiction TEXT NOT NULL DEFAULT '',
			legal_basis TEXT NOT NULL DEFAULT '',
			external_reference TEXT NOT NULL DEFAULT '',
			target_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
			target_handle TEXT NOT NULL DEFAULT '',
			target_from_at TIMESTAMPTZ,
			target_until_at TIMESTAMPTZ,
			data_types TEXT[] NOT NULL DEFAULT '{}'::text[],
			emergency BOOLEAN NOT NULL DEFAULT false,
			status TEXT NOT NULL DEFAULT 'incoming',
			due_at TIMESTAMPTZ,
			assigned_admin_id UUID REFERENCES users(id) ON DELETE SET NULL,
			response_summary TEXT NOT NULL DEFAULT '',
			user_notice_status TEXT NOT NULL DEFAULT 'not_applicable',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE law_enforcement_requests DROP CONSTRAINT IF EXISTS law_enforcement_requests_type_check`,
		`ALTER TABLE law_enforcement_requests
			ADD CONSTRAINT law_enforcement_requests_type_check
			CHECK (request_type IN ('legal_process', 'preservation', 'emergency', 'user_notice', 'other'))`,
		`ALTER TABLE law_enforcement_requests DROP CONSTRAINT IF EXISTS law_enforcement_requests_status_check`,
		`ALTER TABLE law_enforcement_requests
			ADD CONSTRAINT law_enforcement_requests_status_check
			CHECK (status IN ('incoming', 'reviewing', 'preserved', 'responded', 'rejected', 'closed'))`,
		`ALTER TABLE law_enforcement_requests DROP CONSTRAINT IF EXISTS law_enforcement_requests_notice_check`,
		`ALTER TABLE law_enforcement_requests
			ADD CONSTRAINT law_enforcement_requests_notice_check
			CHECK (user_notice_status IN ('not_applicable', 'pending', 'sent', 'delayed', 'prohibited'))`,
		`CREATE INDEX IF NOT EXISTS idx_law_enforcement_requests_created_at ON law_enforcement_requests (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_law_enforcement_requests_target_user ON law_enforcement_requests (target_user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS legal_preservation_holds (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			request_id UUID NOT NULL REFERENCES law_enforcement_requests(id) ON DELETE CASCADE,
			target_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
			resource_type TEXT NOT NULL,
			resource_id UUID,
			reason TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ NOT NULL,
			released_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_legal_preservation_holds_target ON legal_preservation_holds (target_user_id, released_at, expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_legal_preservation_holds_request ON legal_preservation_holds (request_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS admin_audit_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			admin_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
			action TEXT NOT NULL,
			target_type TEXT NOT NULL DEFAULT '',
			target_id UUID,
			request_id UUID REFERENCES law_enforcement_requests(id) ON DELETE SET NULL,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			prev_hash TEXT NOT NULL DEFAULT '',
			event_hash TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_admin_audit_events_created_at ON admin_audit_events (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_admin_audit_events_request ON admin_audit_events (request_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS dm_reports (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			reporter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			thread_id UUID NOT NULL REFERENCES dm_threads(id) ON DELETE CASCADE,
			message_id UUID NOT NULL REFERENCES dm_messages(id) ON DELETE CASCADE,
			category TEXT NOT NULL DEFAULT 'other',
			reason TEXT NOT NULL,
			include_plaintext BOOLEAN NOT NULL DEFAULT false,
			reporter_submitted_plaintext TEXT NOT NULL DEFAULT '',
			attachments_note TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			resolved_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (reporter_user_id, message_id)
		)`,
		`ALTER TABLE dm_reports ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other'`,
		`ALTER TABLE dm_reports DROP CONSTRAINT IF EXISTS dm_reports_category_check`,
		`ALTER TABLE dm_reports
			ADD CONSTRAINT dm_reports_category_check
			CHECK (category IN ('other', 'spam', 'abuse', 'legal', 'safety'))`,
		`ALTER TABLE dm_reports DROP CONSTRAINT IF EXISTS dm_reports_status_check`,
		`ALTER TABLE dm_reports
			ADD CONSTRAINT dm_reports_status_check
			CHECK (status IN ('open', 'resolved', 'dismissed', 'spam'))`,
		`CREATE INDEX IF NOT EXISTS idx_dm_reports_status_created_at ON dm_reports (status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_reports_thread_created_at ON dm_reports (thread_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS user_access_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_access_events_user_created_at ON user_access_events (user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_user_access_events_created_at ON user_access_events (created_at DESC)`,
	}
	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate legal compliance step %d: %w", i+1, err)
		}
	}
	return nil
}
