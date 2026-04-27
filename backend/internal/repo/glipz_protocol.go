package repo

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// GlipzProtocolPostRow represents a single post exposed through federation.
type GlipzProtocolPostRow struct {
	ID                     uuid.UUID
	Caption                string
	MediaType              string
	ObjectKeys             []string
	CreatedAt              time.Time
	VisibleAt              time.Time
	IsNSFW                 bool
	LikeCount              int64
	Poll                   *PostPoll
	ReplyToID              *uuid.UUID
	HasViewPassword        bool
	ViewPasswordScope      int
	ViewPasswordTextRanges []ViewPasswordTextRange
	HasMembershipLock      bool
	MembershipProvider     string
	MembershipCreatorID    string
	MembershipTierID       string
}

// GlipzProtocolUserKeys stores the PEM key pair used for HTTP signatures.
type GlipzProtocolUserKeys struct {
	PrivateKeyPEM string
	PublicKeyPEM  string
}

// EnsureGlipzProtocolKeys returns the user's Glipz Protocol RSA keys, creating and storing them if needed.
func (p *Pool) EnsureGlipzProtocolKeys(ctx context.Context, userID uuid.UUID) (GlipzProtocolUserKeys, error) {
	var priv, pub string
	err := p.db.QueryRow(ctx, `
		SELECT private_key_pem, public_key_pem FROM user_glipz_protocol_keys WHERE user_id = $1
	`, userID).Scan(&priv, &pub)
	if err == nil {
		return GlipzProtocolUserKeys{PrivateKeyPEM: priv, PublicKeyPEM: pub}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return GlipzProtocolUserKeys{}, err
	}

	bit := 2048
	key, err := rsa.GenerateKey(rand.Reader, bit)
	if err != nil {
		return GlipzProtocolUserKeys{}, err
	}
	privBytes := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return GlipzProtocolUserKeys{}, err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	_, err = p.db.Exec(ctx, `
		INSERT INTO user_glipz_protocol_keys (user_id, private_key_pem, public_key_pem)
		VALUES ($1, $2, $3)
	`, userID, string(privPEM), string(pubPEM))
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			// Another request created the row concurrently. Read it back.
		} else {
			return GlipzProtocolUserKeys{}, err
		}
	}

	err = p.db.QueryRow(ctx, `
		SELECT private_key_pem, public_key_pem FROM user_glipz_protocol_keys WHERE user_id = $1
	`, userID).Scan(&priv, &pub)
	if err != nil {
		return GlipzProtocolUserKeys{}, err
	}
	return GlipzProtocolUserKeys{PrivateKeyPEM: priv, PublicKeyPEM: pub}, nil
}

// ListGlipzProtocolOutboxPosts returns public posts for the federation outbox in reverse chronological order.
func (p *Pool) ListGlipzProtocolOutboxPosts(ctx context.Context, authorID uuid.UUID, limit int, beforeVisibleAt *time.Time, beforeID *uuid.UUID) ([]GlipzProtocolPostRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var rows pgx.Rows
	var err error
	if beforeVisibleAt != nil && beforeID != nil {
		rows, err = p.db.Query(ctx, `
			SELECT p.id, COALESCE(p.caption, ''), p.media_type, p.object_keys, p.created_at, p.visible_at, p.is_nsfw,
				COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
				p.reply_to_id, (COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
				COALESCE(p.view_password_scope, 0), COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
				(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
				COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, '')
			FROM posts p
			LEFT JOIN (
				SELECT post_id, COUNT(*)::bigint AS like_count FROM post_likes GROUP BY post_id
			) lk ON lk.post_id = p.id
			LEFT JOIN (
				SELECT post_id, COUNT(*)::bigint AS like_count FROM post_remote_likes GROUP BY post_id
			) rlk ON rlk.post_id = p.id
			WHERE p.user_id = $1
				AND p.visible_at <= NOW()
				AND p.group_id IS NULL
				AND `+postVisibilityExpr("p")+` = 'public'
				AND (
					p.visible_at < $2::timestamptz
					OR (p.visible_at = $2::timestamptz AND p.id < $3::uuid)
				)
			ORDER BY p.visible_at DESC, p.id DESC
			LIMIT $4
		`, authorID, *beforeVisibleAt, *beforeID, limit)
	} else {
		rows, err = p.db.Query(ctx, `
			SELECT p.id, COALESCE(p.caption, ''), p.media_type, p.object_keys, p.created_at, p.visible_at, p.is_nsfw,
				COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
				p.reply_to_id, (COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
				COALESCE(p.view_password_scope, 0), COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
				(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
				COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, '')
			FROM posts p
			LEFT JOIN (
				SELECT post_id, COUNT(*)::bigint AS like_count FROM post_likes GROUP BY post_id
			) lk ON lk.post_id = p.id
			LEFT JOIN (
				SELECT post_id, COUNT(*)::bigint AS like_count FROM post_remote_likes GROUP BY post_id
			) rlk ON rlk.post_id = p.id
			WHERE p.user_id = $1
				AND p.visible_at <= NOW()
				AND p.group_id IS NULL
				AND `+postVisibilityExpr("p")+` = 'public'
			ORDER BY p.visible_at DESC, p.id DESC
			LIMIT $2
		`, authorID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlipzProtocolPostRow
	for rows.Next() {
		var r GlipzProtocolPostRow
		var reply pgtype.UUID
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.Caption, &r.MediaType, &r.ObjectKeys, &r.CreatedAt, &r.VisibleAt, &r.IsNSFW, &r.LikeCount, &reply, &r.HasViewPassword, &scope, &textRanges, &r.HasMembershipLock, &r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		if reply.Valid {
			x := uuid.UUID(reply.Bytes)
			r.ReplyToID = &x
		}
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeStoredViewPasswordProtection(r.HasViewPassword, r.Caption, scope, textRanges)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := p.AttachPollsToGlipzProtocolPosts(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// CountGlipzProtocolOutboxPosts returns the number of posts eligible for federation exposure.
func (p *Pool) CountGlipzProtocolOutboxPosts(ctx context.Context, authorID uuid.UUID) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM posts p
		WHERE p.user_id = $1
			AND p.visible_at <= NOW()
			AND p.group_id IS NULL
			AND `+postVisibilityExpr("p")+` = 'public'
	`, authorID).Scan(&n)
	return n, err
}

// UpsertGlipzProtocolRemoteFollower records a remote follower idempotently.
func (p *Pool) UpsertGlipzProtocolRemoteFollower(ctx context.Context, localUserID uuid.UUID, remoteActorID, remoteInbox string) error {
	if strings.TrimSpace(remoteActorID) == "" || strings.TrimSpace(remoteInbox) == "" {
		return fmt.Errorf("empty remote actor or inbox")
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO glipz_protocol_remote_followers (local_user_id, remote_actor_id, remote_inbox)
		VALUES ($1, $2, $3)
		ON CONFLICT (local_user_id, remote_actor_id) DO UPDATE SET remote_inbox = EXCLUDED.remote_inbox
	`, localUserID, remoteActorID, remoteInbox)
	return err
}

func (p *Pool) AttachRemoteAccountToSubscriber(ctx context.Context, localUserID uuid.UUID, remoteActorID string, remoteAccountID uuid.UUID, currentAcct string) error {
	remoteActorID = strings.TrimSpace(remoteActorID)
	currentAcct = NormalizeFederationTargetAcct(currentAcct)
	if localUserID == uuid.Nil || remoteActorID == "" || remoteAccountID == uuid.Nil {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		UPDATE glipz_protocol_remote_followers
		SET remote_account_id = $3,
			remote_current_acct = $4
		WHERE local_user_id = $1 AND remote_actor_id = $2
	`, localUserID, remoteActorID, remoteAccountID, currentAcct)
	return err
}

// DeleteGlipzProtocolRemoteFollower removes a remote follower, for example after Undo Follow.
func (p *Pool) DeleteGlipzProtocolRemoteFollower(ctx context.Context, localUserID uuid.UUID, remoteActorID string) error {
	_, err := p.db.Exec(ctx, `
		DELETE FROM glipz_protocol_remote_followers WHERE local_user_id = $1 AND remote_actor_id = $2
	`, localUserID, remoteActorID)
	return err
}

// RepointGlipzProtocolRemoteFollowerActor rewrites remote_actor_id from old to new during Move handling.
// Conflicting duplicate rows are removed first.
func (p *Pool) RepointGlipzProtocolRemoteFollowerActor(ctx context.Context, oldActorID, newActorID string) error {
	oldActorID = strings.TrimSpace(oldActorID)
	newActorID = strings.TrimSpace(newActorID)
	if oldActorID == "" || newActorID == "" || strings.EqualFold(oldActorID, newActorID) {
		return nil
	}
	_, _ = p.db.Exec(ctx, `
		DELETE FROM glipz_protocol_remote_followers a
		USING glipz_protocol_remote_followers b
		WHERE a.remote_actor_id = $1 AND b.local_user_id = a.local_user_id AND b.remote_actor_id = $2
	`, oldActorID, newActorID)
	_, err := p.db.Exec(ctx, `
		UPDATE glipz_protocol_remote_followers SET remote_actor_id = $2 WHERE remote_actor_id = $1
	`, oldActorID, newActorID)
	return err
}

// CountGlipzProtocolRemoteFollowers returns the number of remote accounts following through Glipz Protocol.
func (p *Pool) CountGlipzProtocolRemoteFollowers(ctx context.Context, localUserID uuid.UUID) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM glipz_protocol_remote_followers WHERE local_user_id = $1
	`, localUserID).Scan(&n)
	return n, err
}

// UserHandleByID returns a user's handle.
func (p *Pool) UserHandleByID(ctx context.Context, userID uuid.UUID) (string, error) {
	var h string
	err := p.db.QueryRow(ctx, `SELECT handle FROM users WHERE id = $1`, userID).Scan(&h)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(h), nil
}

// ListGlipzProtocolRemoteFollowerInboxes returns unique destination inbox URLs for delivery.
func (p *Pool) ListGlipzProtocolRemoteFollowerInboxes(ctx context.Context, localUserID uuid.UUID) ([]string, error) {
	rows, err := p.db.Query(ctx, `
		SELECT DISTINCT r.remote_inbox FROM glipz_protocol_remote_followers r
		WHERE r.local_user_id = $1 AND COALESCE(btrim(r.remote_inbox), '') <> ''
			AND NOT EXISTS (
				SELECT 1 FROM federation_user_blocks b
				WHERE b.user_id = r.local_user_id
					AND b.target_acct = lower(trim(both from r.remote_actor_id))
			)
		ORDER BY 1
	`, localUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, strings.TrimSpace(u))
	}
	return out, rows.Err()
}

type GlipzProtocolRemoteFollowerRow struct {
	RemoteActorID string
	RemoteInbox   string
	CreatedAt     time.Time
}

func (p *Pool) ListGlipzProtocolRemoteFollowers(ctx context.Context, localUserID uuid.UUID, limit, offset int) ([]GlipzProtocolRemoteFollowerRow, error) {
	limit = clampListLimit(limit)
	offset = clampListOffset(offset)
	rows, err := p.db.Query(ctx, `
		SELECT remote_actor_id, remote_inbox, created_at
		FROM glipz_protocol_remote_followers
		WHERE local_user_id = $1
		ORDER BY created_at DESC, remote_actor_id
		LIMIT $2 OFFSET $3
	`, localUserID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlipzProtocolRemoteFollowerRow
	for rows.Next() {
		var r GlipzProtocolRemoteFollowerRow
		if err := rows.Scan(&r.RemoteActorID, &r.RemoteInbox, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetGlipzProtocolPostForDelivery returns one post eligible for federation delivery from the given author.
func (p *Pool) GetGlipzProtocolPostForDelivery(ctx context.Context, authorID, postID uuid.UUID) (GlipzProtocolPostRow, error) {
	var r GlipzProtocolPostRow
	var reply pgtype.UUID
	var scope int
	var textRanges string
	err := p.db.QueryRow(ctx, `
		SELECT p.id, COALESCE(p.caption, ''), p.media_type, p.object_keys, p.created_at, p.visible_at, p.is_nsfw,
			COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
			p.reply_to_id, (COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0), COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, '')
		FROM posts p
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_likes GROUP BY post_id
		) lk ON lk.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_remote_likes GROUP BY post_id
		) rlk ON rlk.post_id = p.id
		WHERE p.id = $1
			AND p.user_id = $2
			AND p.visible_at <= NOW()
			AND p.group_id IS NULL
			AND `+postVisibilityExpr("p")+` = 'public'
	`, postID, authorID).Scan(&r.ID, &r.Caption, &r.MediaType, &r.ObjectKeys, &r.CreatedAt, &r.VisibleAt, &r.IsNSFW, &r.LikeCount, &reply, &r.HasViewPassword, &scope, &textRanges, &r.HasMembershipLock, &r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID)
	if errors.Is(err, pgx.ErrNoRows) {
		return GlipzProtocolPostRow{}, ErrNotFound
	}
	if err != nil {
		return GlipzProtocolPostRow{}, err
	}
	if reply.Valid {
		x := uuid.UUID(reply.Bytes)
		r.ReplyToID = &x
	}
	r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeStoredViewPasswordProtection(r.HasViewPassword, r.Caption, scope, textRanges)
	if err != nil {
		return GlipzProtocolPostRow{}, err
	}
	rows := []GlipzProtocolPostRow{r}
	if err := p.AttachPollsToGlipzProtocolPosts(ctx, rows); err != nil {
		return GlipzProtocolPostRow{}, err
	}
	return rows[0], nil
}

// PostIsRootAndVisibleInPast reports whether a post exists, is top-level, and has already become visible.
func (p *Pool) PostIsRootAndVisibleInPast(ctx context.Context, postID uuid.UUID) (authorID uuid.UUID, ok bool, err error) {
	var uid uuid.UUID
	var replyTo pgtype.UUID
	var remoteReply string
	var vis time.Time
	err = p.db.QueryRow(ctx, `
		SELECT user_id, reply_to_id, COALESCE(reply_to_remote_object_iri, ''), visible_at
		FROM posts
		WHERE id = $1 AND group_id IS NULL
	`, postID).Scan(&uid, &replyTo, &remoteReply, &vis)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	if replyTo.Valid || strings.TrimSpace(remoteReply) != "" {
		return uuid.Nil, false, nil
	}
	if vis.After(time.Now().UTC()) {
		return uuid.Nil, false, nil
	}
	return uid, true, nil
}
