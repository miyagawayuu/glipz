package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrDMIdentityUnavailable = errors.New("dm identity unavailable")

type DMContact struct {
	UserID          uuid.UUID
	Handle          string
	DisplayName     string
	AvatarObjectKey *string
	Algorithm       string
	PublicJWK       []byte
}

type DMThreadSummary struct {
	ID              uuid.UUID
	PeerID          uuid.UUID
	PeerHandle      string
	PeerDisplayName string
	PeerAvatarKey   *string
	PeerAlgorithm   string
	PeerPublicJWK   []byte
	SkyWayRoomName  string
	LastMessageAt   *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	UnreadCount     int64
}

type DMMessageViewer struct {
	ID                uuid.UUID
	SentByMe          bool
	SenderID          uuid.UUID
	SenderHandle      string
	SenderDisplayName string
	SenderAvatarKey   *string
	Ciphertext        []byte
	Attachments       []byte
	CreatedAt         time.Time
}

type CreateDMMessageInput struct {
	ThreadID         uuid.UUID
	SenderID         uuid.UUID
	SenderPayload    json.RawMessage
	RecipientPayload json.RawMessage
	Attachments      json.RawMessage
}

type CreatedDMMessage struct {
	ID          uuid.UUID
	RecipientID uuid.UUID
	CreatedAt   time.Time
}

func canonicalDMUsers(a, b uuid.UUID) (uuid.UUID, uuid.UUID) {
	if strings.Compare(a.String(), b.String()) <= 0 {
		return a, b
	}
	return b, a
}

func dmSkyWayRoomName(a, b uuid.UUID) string {
	low, high := canonicalDMUsers(a, b)
	return "dm-" + strings.ReplaceAll(low.String(), "-", "")[:12] + "-" + strings.ReplaceAll(high.String(), "-", "")[:12]
}

func (p *Pool) UpsertDMIdentityKey(ctx context.Context, userID uuid.UUID, algorithm string, publicJWK, encryptedPrivateJWK json.RawMessage) error {
	if len(publicJWK) == 0 || len(encryptedPrivateJWK) == 0 {
		return ErrDMIdentityUnavailable
	}
	if strings.TrimSpace(algorithm) == "" {
		algorithm = "ECDH-P256"
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO user_dm_identity_keys (user_id, algorithm, public_jwk, encrypted_private_jwk, updated_at)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET algorithm = EXCLUDED.algorithm,
			public_jwk = EXCLUDED.public_jwk,
			encrypted_private_jwk = EXCLUDED.encrypted_private_jwk,
			updated_at = NOW()
	`, userID, algorithm, []byte(publicJWK), []byte(encryptedPrivateJWK))
	return err
}

func (p *Pool) DMIdentityKeyForUser(ctx context.Context, userID uuid.UUID) (string, []byte, []byte, error) {
	var algorithm string
	var publicJWK []byte
	var encryptedPrivateJWK []byte
	err := p.db.QueryRow(ctx, `
		SELECT algorithm, public_jwk, COALESCE(encrypted_private_jwk, '{}'::jsonb)
		FROM user_dm_identity_keys
		WHERE user_id = $1
	`, userID).Scan(&algorithm, &publicJWK, &encryptedPrivateJWK)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil, ErrDMIdentityUnavailable
	}
	if err != nil {
		return "", nil, nil, err
	}
	if strings.TrimSpace(algorithm) == "" || len(publicJWK) == 0 || len(encryptedPrivateJWK) == 0 || string(encryptedPrivateJWK) == "{}" {
		return "", nil, nil, ErrDMIdentityUnavailable
	}
	return algorithm, publicJWK, encryptedPrivateJWK, nil
}

func (p *Pool) DMContactByHandle(ctx context.Context, handle string) (DMContact, error) {
	var row DMContact
	err := p.db.QueryRow(ctx, `
		SELECT u.id,
			u.handle,
			COALESCE(NULLIF(trim(u.display_name), ''), trim(split_part(u.email, '@', 1))),
			u.avatar_object_key,
			COALESCE(k.algorithm, ''),
			COALESCE(k.public_jwk, '{}'::jsonb)
		FROM users u
		LEFT JOIN user_dm_identity_keys k ON k.user_id = u.id
		WHERE lower(u.handle) = lower($1)
	`, handle).Scan(&row.UserID, &row.Handle, &row.DisplayName, &row.AvatarObjectKey, &row.Algorithm, &row.PublicJWK)
	if errors.Is(err, pgx.ErrNoRows) {
		return DMContact{}, ErrNotFound
	}
	if err != nil {
		return DMContact{}, err
	}
	if strings.TrimSpace(row.Algorithm) == "" || len(row.PublicJWK) == 0 || string(row.PublicJWK) == "{}" {
		return DMContact{}, ErrDMIdentityUnavailable
	}
	return row, nil
}

// UserDMInviteAutoAccept returns users.dm_invite_auto_accept, or ErrNotFound when the user does not exist.
func (p *Pool) UserDMInviteAutoAccept(ctx context.Context, userID uuid.UUID) (bool, error) {
	var v bool
	err := p.db.QueryRow(ctx, `
		SELECT COALESCE(dm_invite_auto_accept, false) FROM users WHERE id = $1
	`, userID).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return v, err
}

// UpsertDMAutoAcceptPendingInvite records a pending invite for a peer who has auto-accept enabled but no DM keys yet.
func (p *Pool) UpsertDMAutoAcceptPendingInvite(ctx context.Context, inviterID, inviteeID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO dm_auto_accept_pending_invites (inviter_id, invitee_id)
		VALUES ($1, $2)
		ON CONFLICT (inviter_id, invitee_id) DO NOTHING
	`, inviterID, inviteeID)
	return err
}

// ProcessDMAutoAcceptPendingInvitesForInvitee opens threads with pending inviters after the invitee sets DM keys.
func (p *Pool) ProcessDMAutoAcceptPendingInvitesForInvitee(ctx context.Context, inviteeID uuid.UUID) error {
	rows, err := p.db.Query(ctx, `
		SELECT inviter_id FROM dm_auto_accept_pending_invites WHERE invitee_id = $1
	`, inviteeID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var inviters []uuid.UUID
	for rows.Next() {
		var inv uuid.UUID
		if err := rows.Scan(&inv); err != nil {
			return err
		}
		inviters = append(inviters, inv)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, inv := range inviters {
		if _, errE := p.EnsureDMThread(ctx, inv, inviteeID); errE != nil {
			continue
		}
		_, _ = p.db.Exec(ctx, `
			DELETE FROM dm_auto_accept_pending_invites
			WHERE invitee_id = $1 AND inviter_id = $2
		`, inviteeID, inv)
	}
	return nil
}

func (p *Pool) EnsureDMThread(ctx context.Context, userID, peerID uuid.UUID) (DMThreadSummary, error) {
	if userID == peerID {
		return DMThreadSummary{}, ErrForbidden
	}
	low, high := canonicalDMUsers(userID, peerID)
	var threadID uuid.UUID
	err := p.db.QueryRow(ctx, `
		INSERT INTO dm_threads (user_low_id, user_high_id, skyway_room_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_low_id, user_high_id) DO UPDATE
		SET updated_at = dm_threads.updated_at
		RETURNING id
	`, low, high, dmSkyWayRoomName(low, high)).Scan(&threadID)
	if err != nil {
		return DMThreadSummary{}, err
	}
	return p.DMThreadByIDForUser(ctx, userID, threadID)
}

func (p *Pool) DMThreadByIDForUser(ctx context.Context, userID, threadID uuid.UUID) (DMThreadSummary, error) {
	var row DMThreadSummary
	err := p.db.QueryRow(ctx, `
		SELECT t.id,
			CASE WHEN $1 = t.user_low_id THEN t.user_high_id ELSE t.user_low_id END AS peer_id,
			CASE WHEN $1 = t.user_low_id THEN u2.handle ELSE u1.handle END AS peer_handle,
			CASE WHEN $1 = t.user_low_id
				THEN COALESCE(NULLIF(trim(u2.display_name), ''), trim(split_part(u2.email, '@', 1)))
				ELSE COALESCE(NULLIF(trim(u1.display_name), ''), trim(split_part(u1.email, '@', 1)))
			END AS peer_display_name,
			CASE WHEN $1 = t.user_low_id THEN u2.avatar_object_key ELSE u1.avatar_object_key END AS peer_avatar_key,
			CASE WHEN $1 = t.user_low_id THEN COALESCE(k2.algorithm, '') ELSE COALESCE(k1.algorithm, '') END AS peer_algorithm,
			CASE WHEN $1 = t.user_low_id THEN COALESCE(k2.public_jwk, '{}'::jsonb) ELSE COALESCE(k1.public_jwk, '{}'::jsonb) END AS peer_public_jwk,
			t.skyway_room_name,
			t.last_message_at,
			t.created_at,
			t.updated_at,
			(
				SELECT COUNT(*)::bigint
				FROM dm_messages m
				WHERE m.thread_id = t.id
					AND m.sender_id <> $1
					AND m.created_at > COALESCE(
						CASE WHEN $1 = t.user_low_id THEN t.user_low_last_read_at ELSE t.user_high_last_read_at END,
						to_timestamp(0)
					)
			) AS unread_count
		FROM dm_threads t
		JOIN users u1 ON u1.id = t.user_low_id
		JOIN users u2 ON u2.id = t.user_high_id
		LEFT JOIN user_dm_identity_keys k1 ON k1.user_id = u1.id
		LEFT JOIN user_dm_identity_keys k2 ON k2.user_id = u2.id
		WHERE t.id = $2 AND ($1 = t.user_low_id OR $1 = t.user_high_id)
	`, userID, threadID).Scan(
		&row.ID,
		&row.PeerID,
		&row.PeerHandle,
		&row.PeerDisplayName,
		&row.PeerAvatarKey,
		&row.PeerAlgorithm,
		&row.PeerPublicJWK,
		&row.SkyWayRoomName,
		&row.LastMessageAt,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.UnreadCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DMThreadSummary{}, ErrNotFound
	}
	return row, err
}

func (p *Pool) ListDMThreads(ctx context.Context, userID uuid.UUID, limit int) ([]DMThreadSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT t.id,
			CASE WHEN $1 = t.user_low_id THEN t.user_high_id ELSE t.user_low_id END AS peer_id,
			CASE WHEN $1 = t.user_low_id THEN u2.handle ELSE u1.handle END AS peer_handle,
			CASE WHEN $1 = t.user_low_id
				THEN COALESCE(NULLIF(trim(u2.display_name), ''), trim(split_part(u2.email, '@', 1)))
				ELSE COALESCE(NULLIF(trim(u1.display_name), ''), trim(split_part(u1.email, '@', 1)))
			END AS peer_display_name,
			CASE WHEN $1 = t.user_low_id THEN u2.avatar_object_key ELSE u1.avatar_object_key END AS peer_avatar_key,
			CASE WHEN $1 = t.user_low_id THEN COALESCE(k2.algorithm, '') ELSE COALESCE(k1.algorithm, '') END AS peer_algorithm,
			CASE WHEN $1 = t.user_low_id THEN COALESCE(k2.public_jwk, '{}'::jsonb) ELSE COALESCE(k1.public_jwk, '{}'::jsonb) END AS peer_public_jwk,
			t.skyway_room_name,
			t.last_message_at,
			t.created_at,
			t.updated_at,
			(
				SELECT COUNT(*)::bigint
				FROM dm_messages m
				WHERE m.thread_id = t.id
					AND m.sender_id <> $1
					AND m.created_at > COALESCE(
						CASE WHEN $1 = t.user_low_id THEN t.user_low_last_read_at ELSE t.user_high_last_read_at END,
						to_timestamp(0)
					)
			) AS unread_count
		FROM dm_threads t
		JOIN users u1 ON u1.id = t.user_low_id
		JOIN users u2 ON u2.id = t.user_high_id
		LEFT JOIN user_dm_identity_keys k1 ON k1.user_id = u1.id
		LEFT JOIN user_dm_identity_keys k2 ON k2.user_id = u2.id
		WHERE $1 = t.user_low_id OR $1 = t.user_high_id
		ORDER BY COALESCE(t.last_message_at, t.updated_at) DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DMThreadSummary, 0, limit)
	for rows.Next() {
		var row DMThreadSummary
		if err := rows.Scan(
			&row.ID,
			&row.PeerID,
			&row.PeerHandle,
			&row.PeerDisplayName,
			&row.PeerAvatarKey,
			&row.PeerAlgorithm,
			&row.PeerPublicJWK,
			&row.SkyWayRoomName,
			&row.LastMessageAt,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.UnreadCount,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) ListDMMessages(ctx context.Context, userID, threadID uuid.UUID, before *time.Time, limit int) ([]DMMessageViewer, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT m.id,
			(m.sender_id = $2) AS sent_by_me,
			m.sender_id,
			su.handle,
			COALESCE(NULLIF(trim(su.display_name), ''), trim(split_part(su.email, '@', 1))),
			su.avatar_object_key,
			CASE WHEN m.sender_id = $2 THEN m.sender_payload ELSE m.recipient_payload END AS ciphertext,
			m.attachments,
			m.created_at
		FROM dm_messages m
		JOIN dm_threads t ON t.id = m.thread_id
		JOIN users su ON su.id = m.sender_id
		WHERE m.thread_id = $1
			AND ($2 = t.user_low_id OR $2 = t.user_high_id)
			AND ($3::timestamptz IS NULL OR m.created_at < $3)
		ORDER BY m.created_at DESC
		LIMIT $4
	`, threadID, userID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DMMessageViewer, 0, limit)
	for rows.Next() {
		var row DMMessageViewer
		if err := rows.Scan(
			&row.ID,
			&row.SentByMe,
			&row.SenderID,
			&row.SenderHandle,
			&row.SenderDisplayName,
			&row.SenderAvatarKey,
			&row.Ciphertext,
			&row.Attachments,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) DMMessageByIDForViewer(ctx context.Context, userID, messageID uuid.UUID) (DMMessageViewer, error) {
	var row DMMessageViewer
	err := p.db.QueryRow(ctx, `
		SELECT m.id,
			(m.sender_id = $2) AS sent_by_me,
			m.sender_id,
			su.handle,
			COALESCE(NULLIF(trim(su.display_name), ''), trim(split_part(su.email, '@', 1))),
			su.avatar_object_key,
			CASE WHEN m.sender_id = $2 THEN m.sender_payload ELSE m.recipient_payload END AS ciphertext,
			m.attachments,
			m.created_at
		FROM dm_messages m
		JOIN dm_threads t ON t.id = m.thread_id
		JOIN users su ON su.id = m.sender_id
		WHERE m.id = $1 AND ($2 = t.user_low_id OR $2 = t.user_high_id)
	`, messageID, userID).Scan(
		&row.ID,
		&row.SentByMe,
		&row.SenderID,
		&row.SenderHandle,
		&row.SenderDisplayName,
		&row.SenderAvatarKey,
		&row.Ciphertext,
		&row.Attachments,
		&row.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DMMessageViewer{}, ErrNotFound
	}
	return row, err
}

func (p *Pool) CreateDMMessage(ctx context.Context, in CreateDMMessageInput) (CreatedDMMessage, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return CreatedDMMessage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var low, high uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT user_low_id, user_high_id
		FROM dm_threads
		WHERE id = $1
	`, in.ThreadID).Scan(&low, &high)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreatedDMMessage{}, ErrNotFound
	}
	if err != nil {
		return CreatedDMMessage{}, err
	}
	if in.SenderID != low && in.SenderID != high {
		return CreatedDMMessage{}, ErrForbidden
	}
	recipientID := high
	if in.SenderID == high {
		recipientID = low
	}

	var out CreatedDMMessage
	err = tx.QueryRow(ctx, `
		INSERT INTO dm_messages (thread_id, sender_id, sender_payload, recipient_payload, attachments)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb)
		RETURNING id, created_at
	`, in.ThreadID, in.SenderID, []byte(in.SenderPayload), []byte(in.RecipientPayload), []byte(in.Attachments)).Scan(&out.ID, &out.CreatedAt)
	if err != nil {
		return CreatedDMMessage{}, err
	}
	if in.SenderID == low {
		_, err = tx.Exec(ctx, `
			UPDATE dm_threads
			SET last_message_at = $2,
				updated_at = NOW(),
				user_low_last_read_at = $2
			WHERE id = $1
		`, in.ThreadID, out.CreatedAt)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE dm_threads
			SET last_message_at = $2,
				updated_at = NOW(),
				user_high_last_read_at = $2
			WHERE id = $1
		`, in.ThreadID, out.CreatedAt)
	}
	if err != nil {
		return CreatedDMMessage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreatedDMMessage{}, err
	}
	out.RecipientID = recipientID
	return out, nil
}

func (p *Pool) MarkDMThreadRead(ctx context.Context, userID, threadID uuid.UUID, at time.Time) error {
	tag, err := p.db.Exec(ctx, `
		UPDATE dm_threads
		SET
			user_low_last_read_at = CASE WHEN user_low_id = $1 THEN GREATEST(COALESCE(user_low_last_read_at, to_timestamp(0)), $3) ELSE user_low_last_read_at END,
			user_high_last_read_at = CASE WHEN user_high_id = $1 THEN GREATEST(COALESCE(user_high_last_read_at, to_timestamp(0)), $3) ELSE user_high_last_read_at END,
			updated_at = CASE WHEN user_low_id = $1 OR user_high_id = $1 THEN NOW() ELSE updated_at END
		WHERE id = $2 AND ($1 = user_low_id OR $1 = user_high_id)
	`, userID, threadID, at.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) DMUnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(unread_count), 0)::bigint
		FROM (
			SELECT (
				SELECT COUNT(*)::bigint
				FROM dm_messages m
				WHERE m.thread_id = t.id
					AND m.sender_id <> $1
					AND m.created_at > COALESCE(
						CASE WHEN $1 = t.user_low_id THEN t.user_low_last_read_at ELSE t.user_high_last_read_at END,
						to_timestamp(0)
					)
			) AS unread_count
			FROM dm_threads t
			WHERE $1 = t.user_low_id OR $1 = t.user_high_id
		) q
	`, userID).Scan(&n)
	return n, err
}

func (p *Pool) DMStreamEventForRecipient(ctx context.Context, recipientID, messageID uuid.UUID) (map[string]any, error) {
	var threadID uuid.UUID
	var senderID uuid.UUID
	var senderHandle, senderDisplay string
	var senderBadges []string
	var createdAt time.Time
	err := p.db.QueryRow(ctx, `
		SELECT m.thread_id,
			m.sender_id,
			u.handle,
			COALESCE(NULLIF(trim(u.display_name), ''), trim(split_part(u.email, '@', 1))),
			COALESCE(u.badges, '{}'::text[]),
			m.created_at
		FROM dm_messages m
		JOIN users u ON u.id = m.sender_id
		JOIN dm_threads t ON t.id = m.thread_id
		WHERE m.id = $1
			AND (
				(t.user_low_id = $2 AND m.sender_id <> t.user_low_id) OR
				(t.user_high_id = $2 AND m.sender_id <> t.user_high_id)
			)
	`, messageID, recipientID).Scan(&threadID, &senderID, &senderHandle, &senderDisplay, &senderBadges, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"v":                   1,
		"kind":                "message",
		"thread_id":           threadID.String(),
		"message_id":          messageID.String(),
		"sender_user_id":      senderID.String(),
		"sender_handle":       senderHandle,
		"sender_display_name": senderDisplay,
		"sender_badges":       NormalizeUserBadges(senderBadges),
		"created_at":          createdAt.UTC().Format(time.RFC3339),
	}, nil
}
