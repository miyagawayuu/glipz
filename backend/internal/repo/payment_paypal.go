package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PayPalConnectionRow struct {
	UserID     uuid.UUID
	Env        string
	MerchantID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (p *Pool) UpsertPayPalConnection(ctx context.Context, userID uuid.UUID, env, merchantID string) error {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "sandbox"
	}
	merchantID = strings.TrimSpace(merchantID)
	_, err := p.db.Exec(ctx, `
		INSERT INTO user_paypal_connection (user_id, env, merchant_id, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id) DO UPDATE SET
			env = EXCLUDED.env,
			merchant_id = EXCLUDED.merchant_id,
			updated_at = now()
	`, userID, env, merchantID)
	return err
}

func (p *Pool) PayPalConnectionByUserID(ctx context.Context, userID uuid.UUID) (PayPalConnectionRow, error) {
	var row PayPalConnectionRow
	err := p.db.QueryRow(ctx, `
		SELECT user_id, env, merchant_id, created_at, updated_at
		FROM user_paypal_connection WHERE user_id = $1
	`, userID).Scan(&row.UserID, &row.Env, &row.MerchantID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PayPalConnectionRow{}, ErrNotFound
	}
	if err != nil {
		return PayPalConnectionRow{}, err
	}
	return row, nil
}

func (p *Pool) DeletePayPalConnection(ctx context.Context, userID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `DELETE FROM user_paypal_connection WHERE user_id = $1`, userID)
	return err
}

type CreatorPaymentPlanRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Provider  string
	PlanID    string
	Label     string
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (p *Pool) UpsertCreatorPaymentPlan(ctx context.Context, userID uuid.UUID, provider, planID, label string, active bool) (uuid.UUID, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "paypal"
	}
	planID = strings.TrimSpace(planID)
	label = strings.TrimSpace(label)
	var id uuid.UUID
	err := p.db.QueryRow(ctx, `
		INSERT INTO creator_payment_plans (user_id, provider, plan_id, label, active, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (provider, plan_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			label = EXCLUDED.label,
			active = EXCLUDED.active,
			updated_at = now()
		RETURNING id
	`, userID, provider, planID, label, active).Scan(&id)
	return id, err
}

func (p *Pool) ListCreatorPaymentPlans(ctx context.Context, userID uuid.UUID, provider string) ([]CreatorPaymentPlanRow, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "paypal"
	}
	rows, err := p.db.Query(ctx, `
		SELECT id, user_id, provider, plan_id, label, active, created_at, updated_at
		FROM creator_payment_plans
		WHERE user_id = $1 AND provider = $2
		ORDER BY created_at DESC
	`, userID, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CreatorPaymentPlanRow
	for rows.Next() {
		var r CreatorPaymentPlanRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Provider, &r.PlanID, &r.Label, &r.Active, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PaymentSubscriptionRow struct {
	ID              uuid.UUID
	Provider        string
	ViewerUserID    uuid.UUID
	CreatorUserID   uuid.UUID
	PlanID          string
	SubscriptionID  string
	Status          string
	StatusUpdatedAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (p *Pool) CreatePaymentSubscriptionLink(ctx context.Context, provider string, viewerUserID, creatorUserID uuid.UUID, planID, subscriptionID, status string, statusUpdatedAt time.Time) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "paypal"
	}
	planID = strings.TrimSpace(planID)
	subscriptionID = strings.TrimSpace(subscriptionID)
	status = normalizePaymentSubscriptionStatus(status)
	if statusUpdatedAt.IsZero() {
		statusUpdatedAt = time.Now().UTC()
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO payment_subscriptions (
			provider, viewer_user_id, creator_user_id, plan_id, subscription_id, status, status_updated_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7, now())
		ON CONFLICT (provider, subscription_id) DO UPDATE SET
			viewer_user_id = EXCLUDED.viewer_user_id,
			creator_user_id = EXCLUDED.creator_user_id,
			plan_id = EXCLUDED.plan_id,
			status = EXCLUDED.status,
			status_updated_at = EXCLUDED.status_updated_at,
			updated_at = now()
	`, provider, viewerUserID, creatorUserID, planID, subscriptionID, status, statusUpdatedAt.UTC())
	return err
}

// UpdatePaymentSubscriptionStatusFromWebhook updates a subscription status if the webhook event is newer than DB.
// Returns true when an update was applied.
func (p *Pool) UpdatePaymentSubscriptionStatusFromWebhook(ctx context.Context, provider, subscriptionID, status string, eventCreateTime time.Time) (bool, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "paypal"
	}
	subscriptionID = strings.TrimSpace(subscriptionID)
	status = normalizePaymentSubscriptionStatus(status)
	ct, err := p.db.Exec(ctx, `
		UPDATE payment_subscriptions
		SET status = $3,
			status_updated_at = $4,
			updated_at = now()
		WHERE provider = $1
		  AND subscription_id = $2
		  AND status_updated_at <= $4
	`, provider, subscriptionID, status, eventCreateTime.UTC())
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

func normalizePaymentSubscriptionStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func IsActivePaymentSubscriptionStatus(status string) bool {
	return normalizePaymentSubscriptionStatus(status) == "ACTIVE"
}

func (p *Pool) ViewerHasActivePaymentSubscription(ctx context.Context, provider string, viewerUserID, creatorUserID uuid.UUID, planID string) (bool, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "paypal"
	}
	planID = strings.TrimSpace(planID)
	var ok bool
	// Keep status semantics provider-specific in payment provider code; here we only check a canonical string.
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM payment_subscriptions
			WHERE provider = $1
			  AND viewer_user_id = $2
			  AND creator_user_id = $3
			  AND plan_id = $4
			  AND status = 'ACTIVE'
		)
	`, provider, viewerUserID, creatorUserID, planID).Scan(&ok)
	return ok, err
}
