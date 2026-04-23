package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// GlipzProtocolOutboxDeliveryRow represents one delivery-queue row processed by a worker.
type GlipzProtocolOutboxDeliveryRow struct {
	ID            uuid.UUID
	AuthorUserID  uuid.UUID
	PostID        uuid.UUID
	Kind          string
	InboxURL      string
	Payload       json.RawMessage
	AttemptCount  int
	NextAttemptAt time.Time
	LockedUntil   *time.Time
	LastError     string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// GlipzProtocolOutboxDeliveryInsert is the input used to enqueue delivery work.
type GlipzProtocolOutboxDeliveryInsert struct {
	AuthorUserID uuid.UUID
	PostID       uuid.UUID
	Kind         string
	InboxURL     string
	Payload      json.RawMessage
}

// InsertGlipzProtocolOutboxDeliveries enqueues Create, Update, or Delete deliveries.
func (p *Pool) InsertGlipzProtocolOutboxDeliveries(ctx context.Context, items []GlipzProtocolOutboxDeliveryInsert) error {
	if len(items) == 0 {
		return nil
	}
	b := &strings.Builder{}
	args := make([]any, 0, len(items)*5)
	b.WriteString(`INSERT INTO glipz_protocol_outbox_deliveries (author_user_id, post_id, kind, inbox_url, payload) VALUES `)
	for i, it := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		n := len(args)
		b.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, it.AuthorUserID, it.PostID, it.Kind, it.InboxURL, it.Payload)
	}
	_, err := p.db.Exec(ctx, b.String(), args...)
	return err
}

// ClaimGlipzProtocolOutboxDeliveries locks and returns rows ready for delivery while reducing worker contention.
func (p *Pool) ClaimGlipzProtocolOutboxDeliveries(ctx context.Context, limit int) ([]GlipzProtocolOutboxDeliveryRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := p.db.Query(ctx, `
		UPDATE glipz_protocol_outbox_deliveries d
		SET locked_until = NOW() + INTERVAL '3 minutes',
		    updated_at = NOW()
		FROM (
			SELECT id FROM glipz_protocol_outbox_deliveries
			WHERE status = 'pending'
			  AND next_attempt_at <= NOW()
			  AND (locked_until IS NULL OR locked_until < NOW())
			ORDER BY next_attempt_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		) t
		WHERE d.id = t.id
		RETURNING d.id, d.author_user_id, d.post_id, d.kind, d.inbox_url, d.payload::text,
			d.attempt_count, d.next_attempt_at, d.locked_until, d.last_error, d.status, d.created_at, d.updated_at
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlipzProtocolOutboxDeliveryRow
	for rows.Next() {
		var r GlipzProtocolOutboxDeliveryRow
		var payloadStr string
		var locked pgtype.Timestamptz
		if err := rows.Scan(&r.ID, &r.AuthorUserID, &r.PostID, &r.Kind, &r.InboxURL, &payloadStr,
			&r.AttemptCount, &r.NextAttemptAt, &locked, &r.LastError, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Payload = json.RawMessage(payloadStr)
		if locked.Valid {
			t := locked.Time
			r.LockedUntil = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CompleteGlipzProtocolOutboxDelivery marks a delivery row as completed after success.
func (p *Pool) CompleteGlipzProtocolOutboxDelivery(ctx context.Context, id uuid.UUID) error {
	ct, err := p.db.Exec(ctx, `
		UPDATE glipz_protocol_outbox_deliveries
		SET status = 'completed', locked_until = NULL, last_error = '', updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("complete delivery: no row for id %s", id)
	}
	return nil
}

// FailGlipzProtocolOutboxDelivery records a failure and either schedules the next retry or marks the row dead.
func (p *Pool) FailGlipzProtocolOutboxDelivery(ctx context.Context, id uuid.UUID, attemptCount int, lastErr string, nextAttempt time.Time, dead bool) error {
	status := "pending"
	if dead {
		status = "dead"
	}
	if len(lastErr) > 2000 {
		lastErr = lastErr[:2000]
	}
	_, err := p.db.Exec(ctx, `
		UPDATE glipz_protocol_outbox_deliveries
		SET attempt_count = $2,
		    last_error = $3,
		    next_attempt_at = $4,
		    locked_until = NULL,
		    status = $5,
		    updated_at = NOW()
		WHERE id = $1
	`, id, attemptCount, lastErr, nextAttempt, status)
	return err
}

// CountGlipzProtocolOutboxDeliveriesByStatus returns queue counts for metrics and backlog monitoring.
func (p *Pool) CountGlipzProtocolOutboxDeliveriesByStatus(ctx context.Context, status string) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM glipz_protocol_outbox_deliveries WHERE status = $1
	`, status).Scan(&n)
	return n, err
}

// GlipzProtocolOutboxDeliveryAdminRow represents an admin-facing queue row without the payload body.
type GlipzProtocolOutboxDeliveryAdminRow struct {
	ID             uuid.UUID
	AuthorUserID   uuid.UUID
	PostID         uuid.UUID
	Kind           string
	InboxURL       string
	AttemptCount   int
	NextAttemptAt  time.Time
	Status         string
	LastErrorShort string
	CreatedAt      time.Time
}

// ListGlipzProtocolOutboxDeliveriesAdmin lists delivery-queue rows for administrative views.
func (p *Pool) ListGlipzProtocolOutboxDeliveriesAdmin(ctx context.Context, status string, limit int) ([]GlipzProtocolOutboxDeliveryAdminRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	status = strings.TrimSpace(strings.ToLower(status))
	var rows pgx.Rows
	var err error
	if status == "" || status == "all" {
		rows, err = p.db.Query(ctx, `
			SELECT id, author_user_id, post_id, kind, inbox_url, attempt_count, next_attempt_at, status,
				LEFT(last_error, 240), created_at
			FROM glipz_protocol_outbox_deliveries
			ORDER BY created_at DESC
			LIMIT $1
		`, limit)
	} else {
		rows, err = p.db.Query(ctx, `
			SELECT id, author_user_id, post_id, kind, inbox_url, attempt_count, next_attempt_at, status,
				LEFT(last_error, 240), created_at
			FROM glipz_protocol_outbox_deliveries
			WHERE status = $1
			ORDER BY created_at DESC
			LIMIT $2
		`, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlipzProtocolOutboxDeliveryAdminRow
	for rows.Next() {
		var r GlipzProtocolOutboxDeliveryAdminRow
		if err := rows.Scan(&r.ID, &r.AuthorUserID, &r.PostID, &r.Kind, &r.InboxURL, &r.AttemptCount, &r.NextAttemptAt, &r.Status, &r.LastErrorShort, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
