package repo

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PushSubscription struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Endpoint      string
	P256DH        string
	Auth          string
	UserAgent     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastSuccessAt *time.Time
	LastFailureAt *time.Time
	FailureReason *string
}

type UpsertPushSubscriptionInput struct {
	Endpoint  string
	P256DH    string
	Auth      string
	UserAgent string
}

func (p *Pool) UpsertPushSubscription(ctx context.Context, userID uuid.UUID, in UpsertPushSubscriptionInput) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO user_push_subscriptions (
			user_id, endpoint, p256dh, auth, user_agent, updated_at, failure_reason, last_failure_at
		)
		VALUES ($1, $2, $3, $4, NULLIF(trim($5), ''), NOW(), NULL, NULL)
		ON CONFLICT (endpoint) DO UPDATE
		SET user_id = EXCLUDED.user_id,
			p256dh = EXCLUDED.p256dh,
			auth = EXCLUDED.auth,
			user_agent = EXCLUDED.user_agent,
			updated_at = NOW(),
			failure_reason = NULL,
			last_failure_at = NULL
	`, userID, strings.TrimSpace(in.Endpoint), strings.TrimSpace(in.P256DH), strings.TrimSpace(in.Auth), strings.TrimSpace(in.UserAgent))
	return err
}

func (p *Pool) DeletePushSubscription(ctx context.Context, userID uuid.UUID, endpoint string) error {
	_, err := p.db.Exec(ctx, `
		DELETE FROM user_push_subscriptions
		WHERE user_id = $1 AND endpoint = $2
	`, userID, strings.TrimSpace(endpoint))
	return err
}

func (p *Pool) ListPushSubscriptionsByUser(ctx context.Context, userID uuid.UUID) ([]PushSubscription, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, COALESCE(user_agent, ''),
			created_at, updated_at, last_success_at, last_failure_at, failure_reason
		FROM user_push_subscriptions
		WHERE user_id = $1
		ORDER BY updated_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushSubscription
	for rows.Next() {
		var item PushSubscription
		var lastSuccessAt, lastFailureAt *time.Time
		var failureReason *string
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Endpoint, &item.P256DH, &item.Auth, &item.UserAgent,
			&item.CreatedAt, &item.UpdatedAt, &lastSuccessAt, &lastFailureAt, &failureReason,
		); err != nil {
			return nil, err
		}
		item.LastSuccessAt = lastSuccessAt
		item.LastFailureAt = lastFailureAt
		item.FailureReason = failureReason
		out = append(out, item)
	}
	return out, rows.Err()
}

func (p *Pool) CountPushSubscriptionsByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM user_push_subscriptions WHERE user_id = $1
	`, userID).Scan(&count)
	return count, err
}

func (p *Pool) MarkPushSubscriptionSuccess(ctx context.Context, endpoint string) error {
	_, err := p.db.Exec(ctx, `
		UPDATE user_push_subscriptions
		SET last_success_at = NOW(),
			failure_reason = NULL,
			last_failure_at = NULL,
			updated_at = NOW()
		WHERE endpoint = $1
	`, strings.TrimSpace(endpoint))
	return err
}

func (p *Pool) MarkPushSubscriptionFailure(ctx context.Context, endpoint, reason string) error {
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		reason = reason[:500]
	}
	_, err := p.db.Exec(ctx, `
		UPDATE user_push_subscriptions
		SET last_failure_at = NOW(),
			failure_reason = NULLIF($2, ''),
			updated_at = NOW()
		WHERE endpoint = $1
	`, strings.TrimSpace(endpoint), reason)
	return err
}

func (p *Pool) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := p.db.Exec(ctx, `
		DELETE FROM user_push_subscriptions WHERE endpoint = $1
	`, strings.TrimSpace(endpoint))
	return err
}
