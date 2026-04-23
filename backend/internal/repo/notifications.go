package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// PostAuthorID returns the user_id for a post.
func (p *Pool) PostAuthorID(ctx context.Context, postID uuid.UUID) (uuid.UUID, error) {
	var uid uuid.UUID
	err := p.db.QueryRow(ctx, `SELECT user_id FROM posts WHERE id = $1`, postID).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return uid, err
}

// InsertNotification inserts one notification.
func (p *Pool) InsertNotification(ctx context.Context, recipientID, actorID uuid.UUID, kind string, subjectPostID, actorPostID *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := p.db.QueryRow(ctx, `
		INSERT INTO notifications (recipient_id, actor_id, kind, subject_post_id, actor_post_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, recipientID, actorID, kind, subjectPostID, actorPostID).Scan(&id)
	return id, err
}

// NotificationJSONForRecipient returns the notification payload for the recipient, suitable for Redis or SSE.
func (p *Pool) NotificationJSONForRecipient(ctx context.Context, notifID, recipientID uuid.UUID) (map[string]any, error) {
	var kind, handle, display string
	var actorID uuid.UUID
	var actorBadges []string
	var createdAt time.Time
	var readAt pgtype.Timestamptz
	var subjectPostID, actorPostID *uuid.UUID
	var subjectAuthorHandle string
	err := p.db.QueryRow(ctx, `
		SELECT n.kind, n.actor_id, u.handle,
			COALESCE(NULLIF(trim(u.display_name), ''), trim(split_part(u.email, '@', 1))),
			COALESCE(u.badges, '{}'::text[]),
			n.subject_post_id, n.actor_post_id, n.created_at, n.read_at,
			COALESCE(su.handle, '')
		FROM notifications n
		JOIN users u ON u.id = n.actor_id
		LEFT JOIN posts sp ON sp.id = n.subject_post_id
		LEFT JOIN users su ON su.id = sp.user_id
		WHERE n.id = $1 AND n.recipient_id = $2
	`, notifID, recipientID).Scan(&kind, &actorID, &handle, &display, &actorBadges, &subjectPostID, &actorPostID, &createdAt, &readAt, &subjectAuthorHandle)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"v":                  1,
		"id":                 notifID.String(),
		"kind":               kind,
		"actor_user_id":      actorID.String(),
		"actor_handle":       handle,
		"actor_display_name": display,
		"actor_badges":       NormalizeUserBadges(actorBadges),
		"created_at":         createdAt.UTC().Format(time.RFC3339),
	}
	if subjectPostID != nil {
		m["subject_post_id"] = subjectPostID.String()
	} else {
		m["subject_post_id"] = nil
	}
	if actorPostID != nil {
		m["actor_post_id"] = actorPostID.String()
	} else {
		m["actor_post_id"] = nil
	}
	if readAt.Valid {
		m["read_at"] = readAt.Time.UTC().Format(time.RFC3339)
	} else {
		m["read_at"] = nil
	}
	if subjectAuthorHandle != "" {
		m["subject_author_handle"] = subjectAuthorHandle
	} else {
		m["subject_author_handle"] = nil
	}
	return m, nil
}

// ListNotifications returns notifications with unread items slightly prioritized ahead of older read items.
// When kindFilter is "reply", only reply notifications are included.
func (p *Pool) ListNotifications(ctx context.Context, recipientID uuid.UUID, limit int, kindFilter string) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	kindFilter = strings.TrimSpace(strings.ToLower(kindFilter))
	kindClause := ""
	if kindFilter == "reply" {
		kindClause = `AND n.kind = 'reply'`
	}
	rows, err := p.db.Query(ctx, `
		SELECT n.id, n.kind, n.actor_id, u.handle,
			COALESCE(NULLIF(trim(u.display_name), ''), trim(split_part(u.email, '@', 1))),
			COALESCE(u.badges, '{}'::text[]),
			n.subject_post_id, n.actor_post_id, n.created_at, n.read_at,
			COALESCE(su.id, '00000000-0000-0000-0000-000000000000'::uuid),
			COALESCE(su.handle, ''),
			COALESCE(NULLIF(trim(su.display_name), ''), NULLIF(trim(split_part(su.email, '@', 1)), ''), ''),
			COALESCE(su.badges, '{}'::text[]),
			COALESCE(NULLIF(trim(su.avatar_object_key), ''), ''),
			COALESCE(NULLIF(trim(u.avatar_object_key), ''), ''),
			COALESCE(NULLIF(trim(sp.caption), ''), ''),
			COALESCE(NULLIF(trim(sp.media_type), ''), ''),
			CASE
				WHEN sp.object_keys IS NOT NULL AND cardinality(sp.object_keys) > 0 THEN sp.object_keys[1]
				ELSE ''
			END,
			COALESCE(NULLIF(trim(ap.caption), ''), ''),
			COALESCE(NULLIF(trim(ap.media_type), ''), ''),
			CASE
				WHEN ap.object_keys IS NOT NULL AND cardinality(ap.object_keys) > 0 THEN ap.object_keys[1]
				ELSE ''
			END
		FROM notifications n
		JOIN users u ON u.id = n.actor_id
		LEFT JOIN posts sp ON sp.id = n.subject_post_id
		LEFT JOIN users su ON su.id = sp.user_id
		LEFT JOIN posts ap ON ap.id = n.actor_post_id
		WHERE n.recipient_id = $1
		`+kindClause+`
		ORDER BY (n.read_at IS NULL) DESC, n.created_at DESC
		LIMIT $2
	`, recipientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, actorID, subjectAuthorID uuid.UUID
		var kind, handle, display string
		var actorBadges, subjectAuthorBadges []string
		var createdAt time.Time
		var readAt pgtype.Timestamptz
		var subjectPostID, actorPostID *uuid.UUID
		var subjectAuthorHandle, subjectAuthorDisplay, subjectAuthorAvatarKey string
		var actorAvatarKey, subjectCaption, subjectMediaType, subjectObjKey string
		var actorPostCaption, actorPostMediaType, actorPostObjKey string
		if err := rows.Scan(&id, &kind, &actorID, &handle, &display, &actorBadges, &subjectPostID, &actorPostID, &createdAt, &readAt, &subjectAuthorID, &subjectAuthorHandle,
			&subjectAuthorDisplay, &subjectAuthorBadges, &subjectAuthorAvatarKey, &actorAvatarKey, &subjectCaption, &subjectMediaType, &subjectObjKey,
			&actorPostCaption, &actorPostMediaType, &actorPostObjKey); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id":                 id.String(),
			"kind":               kind,
			"actor_user_id":      actorID.String(),
			"actor_handle":       handle,
			"actor_display_name": display,
			"actor_badges":       NormalizeUserBadges(actorBadges),
			"created_at":         createdAt.UTC().Format(time.RFC3339),
		}
		if actorAvatarKey != "" {
			m["actor_avatar_key"] = actorAvatarKey
		}
		if subjectCaption != "" {
			m["subject_caption"] = subjectCaption
		}
		if subjectMediaType != "" {
			m["subject_media_type"] = subjectMediaType
		}
		if subjectObjKey != "" {
			m["subject_object_key"] = subjectObjKey
		}
		if actorPostCaption != "" {
			m["actor_post_caption"] = actorPostCaption
		}
		if actorPostMediaType != "" {
			m["actor_post_media_type"] = actorPostMediaType
		}
		if actorPostObjKey != "" {
			m["actor_post_object_key"] = actorPostObjKey
		}
		if subjectAuthorHandle != "" {
			m["subject_author_user_id"] = subjectAuthorID.String()
			m["subject_author_handle"] = subjectAuthorHandle
		} else {
			m["subject_author_user_id"] = nil
			m["subject_author_handle"] = nil
		}
		if strings.TrimSpace(subjectAuthorDisplay) != "" {
			m["subject_author_display_name"] = strings.TrimSpace(subjectAuthorDisplay)
		}
		if len(subjectAuthorBadges) > 0 {
			m["subject_author_badges"] = NormalizeUserBadges(subjectAuthorBadges)
		}
		if subjectAuthorAvatarKey != "" {
			m["subject_author_avatar_key"] = subjectAuthorAvatarKey
		}
		if subjectPostID != nil {
			m["subject_post_id"] = subjectPostID.String()
		} else {
			m["subject_post_id"] = nil
		}
		if actorPostID != nil {
			m["actor_post_id"] = actorPostID.String()
		} else {
			m["actor_post_id"] = nil
		}
		if readAt.Valid {
			m["read_at"] = readAt.Time.UTC().Format(time.RFC3339)
		} else {
			m["read_at"] = nil
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// NotificationUnreadCount returns the unread notification count.
func (p *Pool) NotificationUnreadCount(ctx context.Context, recipientID uuid.UUID) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM notifications WHERE recipient_id = $1 AND read_at IS NULL
	`, recipientID).Scan(&n)
	return n, err
}

// MarkAllNotificationsRead marks all unread notifications for the recipient as read.
func (p *Pool) MarkAllNotificationsRead(ctx context.Context, recipientID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `
		UPDATE notifications SET read_at = now() WHERE recipient_id = $1 AND read_at IS NULL
	`, recipientID)
	return err
}
