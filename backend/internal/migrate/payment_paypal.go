package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunPaymentPayPal adds schema support for user-to-user PayPal subscription paywalls.
func RunPaymentPayPal(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'posts'
	`).Scan(&n)
	if err != nil {
		return fmt.Errorf("payment_paypal: check posts: %w", err)
	}
	if n == 0 {
		return nil
	}

	steps := []string{
		// Payment locks (payment-style entitlements; distinct from fanclub membership locks).
		// When payment_provider is non-empty, the post is considered payment-locked.
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS payment_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS payment_creator_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE posts ADD COLUMN IF NOT EXISTS payment_plan_id TEXT NOT NULL DEFAULT ''`,

		// Creator PayPal connection (minimal: merchant id + environment).
		`CREATE TABLE IF NOT EXISTS user_paypal_connection (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			env TEXT NOT NULL DEFAULT 'sandbox',
			merchant_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CHECK (env IN ('sandbox', 'live'))
		)`,

		// Plans registered by creators (PayPal-side plan_id; creator makes plans in PayPal dashboard).
		`CREATE TABLE IF NOT EXISTS creator_payment_plans (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL DEFAULT 'paypal',
			plan_id TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (provider, plan_id),
			CHECK (provider IN ('paypal'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_creator_payment_plans_user ON creator_payment_plans (user_id, created_at DESC)`,

		// Viewer subscriptions for entitlement checks. Status is derived from webhooks.
		`CREATE TABLE IF NOT EXISTS payment_subscriptions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			provider TEXT NOT NULL DEFAULT 'paypal',
			viewer_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			creator_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			plan_id TEXT NOT NULL,
			subscription_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT '',
			status_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (provider, subscription_id),
			CHECK (provider IN ('paypal'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_payment_subscriptions_viewer ON payment_subscriptions (viewer_user_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_payment_subscriptions_creator_plan ON payment_subscriptions (creator_user_id, plan_id, updated_at DESC)`,
	}

	for i, q := range steps {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("payment_paypal step %d: %w", i+1, err)
		}
	}
	return nil
}

