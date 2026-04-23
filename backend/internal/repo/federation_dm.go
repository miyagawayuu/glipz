package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type FederationDMThreadRow struct {
	ThreadID   uuid.UUID
	RemoteAcct string
	State      string
	UpdatedAt  time.Time
	CreatedAt  time.Time
}

type FederationDMMessageRow struct {
	MessageID       uuid.UUID
	ThreadID        uuid.UUID
	SenderAcct      string
	SenderPayload   []byte
	RecipientPayload []byte
	Attachments     []byte
	SentAt          *time.Time
	CreatedAt       time.Time
}

func (p *Pool) UpsertFederationDMThread(ctx context.Context, threadID, localUserID uuid.UUID, remoteAcct, state string) error {
	remoteAcct = strings.TrimSpace(remoteAcct)
	state = strings.TrimSpace(state)
	if threadID == uuid.Nil || localUserID == uuid.Nil || remoteAcct == "" || state == "" {
		return errors.New("invalid_federation_dm_thread")
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_dm_threads (thread_id, local_user_id, remote_acct, state)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (thread_id) DO UPDATE SET
			local_user_id = EXCLUDED.local_user_id,
			remote_acct = EXCLUDED.remote_acct,
			state = EXCLUDED.state,
			updated_at = NOW()
	`, threadID, localUserID, remoteAcct, state)
	return err
}

func (p *Pool) InsertFederationDMMessage(ctx context.Context, messageID, threadID uuid.UUID, senderAcct string, senderPayloadJSON, recipientPayloadJSON, attachmentsJSON []byte, sentAt *time.Time) error {
	senderAcct = strings.TrimSpace(senderAcct)
	if messageID == uuid.Nil || threadID == uuid.Nil || senderAcct == "" || len(recipientPayloadJSON) == 0 {
		return errors.New("invalid_federation_dm_message")
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_dm_messages (message_id, thread_id, sender_acct, sender_payload, recipient_payload, attachments, sent_at)
		VALUES ($1, $2, $3, NULLIF($4::text,'')::jsonb, $5::jsonb, COALESCE($6::jsonb, '[]'::jsonb), $7)
		ON CONFLICT (message_id) DO NOTHING
	`, messageID, threadID, senderAcct, senderPayloadJSON, recipientPayloadJSON, attachmentsJSON, sentAt)
	return err
}

func (p *Pool) HasAcceptedRemoteFollowForUser(ctx context.Context, localUserID uuid.UUID, remoteAcct string) (bool, error) {
	remoteAcct = strings.TrimSpace(remoteAcct)
	var ok bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM federation_remote_follows
			WHERE local_user_id = $1
			AND lower(remote_actor_id) = lower($2)
			AND state = 'accepted'
		)
	`, localUserID, remoteAcct).Scan(&ok)
	return ok, err
}

func (p *Pool) HasGlipzProtocolRemoteFollower(ctx context.Context, localUserID uuid.UUID, remoteAcct string) (bool, error) {
	remoteAcct = strings.TrimSpace(remoteAcct)
	var ok bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM glipz_protocol_remote_followers
			WHERE local_user_id = $1
			AND lower(remote_actor_id) = lower($2)
		)
	`, localUserID, remoteAcct).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

func (p *Pool) ListFederationDMThreadsForUser(ctx context.Context, localUserID uuid.UUID, limit int) ([]FederationDMThreadRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT thread_id, remote_acct, state, updated_at, created_at
		FROM federation_dm_threads
		WHERE local_user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2
	`, localUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FederationDMThreadRow{}
	for rows.Next() {
		var r FederationDMThreadRow
		if err := rows.Scan(&r.ThreadID, &r.RemoteAcct, &r.State, &r.UpdatedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) ListFederationDMMessagesForUser(ctx context.Context, localUserID, threadID uuid.UUID, limit int) ([]FederationDMMessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	// Authorization: the thread must belong to the user.
	var n int
	if err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM federation_dm_threads WHERE thread_id = $1 AND local_user_id = $2
	`, threadID, localUserID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	rows, err := p.db.Query(ctx, `
		SELECT message_id, thread_id, sender_acct, COALESCE(sender_payload::text,''), recipient_payload::text, attachments::text, sent_at, created_at
		FROM federation_dm_messages
		WHERE thread_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FederationDMMessageRow{}
	for rows.Next() {
		var r FederationDMMessageRow
		var senderPayloadText, recipientPayloadText, attachmentsText string
		if err := rows.Scan(&r.MessageID, &r.ThreadID, &r.SenderAcct, &senderPayloadText, &recipientPayloadText, &attachmentsText, &r.SentAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.SenderPayload = []byte(senderPayloadText)
		r.RecipientPayload = []byte(recipientPayloadText)
		r.Attachments = []byte(attachmentsText)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) TouchFederationDMThread(ctx context.Context, threadID uuid.UUID) error {
	if threadID == uuid.Nil {
		return fmt.Errorf("invalid_thread_id")
	}
	_, err := p.db.Exec(ctx, `UPDATE federation_dm_threads SET updated_at = NOW() WHERE thread_id = $1`, threadID)
	return err
}

