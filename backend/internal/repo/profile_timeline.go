package repo

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProfileNoteListItem represents one note row shown on a profile page.
type ProfileNoteListItem struct {
	ID        uuid.UUID
	Title     string
	Status    string
	UpdatedAt time.Time
}

// ListUserNotesForProfile returns a user's notes ordered by most recent update.
// Drafts are included only when viewerID matches the author; otherwise results must be published and visible to the viewer.
func (p *Pool) ListUserNotesForProfile(ctx context.Context, authorID, viewerID uuid.UUID, limit int) ([]ProfileNoteListItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT n.id, n.title, n.status, n.updated_at
		FROM notes n
		WHERE n.user_id = $1
		AND (
			$2 = $1
			OR (
				n.status = 'published'
				AND (
					n.visibility = 'public'
					OR (
						n.visibility = 'followers'
						AND EXISTS (
							SELECT 1 FROM user_follows f
							WHERE f.follower_id = $2 AND f.followee_id = $1
						)
					)
				)
			)
		)
		ORDER BY n.updated_at DESC, n.id DESC
		LIMIT $3
	`, authorID, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProfileNoteListItem
	for rows.Next() {
		var it ProfileNoteListItem
		if err := rows.Scan(&it.ID, &it.Title, &it.Status, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// PostMediaTileRow represents one profile media-grid slot using the first media item from a post.
type PostMediaTileRow struct {
	PostID            uuid.UUID
	AuthorUserID      uuid.UUID
	MediaType         string
	FirstObjectKey    string
	HasViewPassword   bool
	ViewPasswordScope int
}

// ListUserPostMediaTiles returns recent top-level posts with images or videos for a user's profile grid.
// Each post contributes only object_keys[1], so later media items from the same post are excluded.
func (p *Pool) ListUserPostMediaTiles(ctx context.Context, authorID, viewerID uuid.UUID, limit int) ([]PostMediaTileRow, error) {
	if limit <= 0 || limit > 120 {
		limit = 90
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, p.media_type, p.object_keys[1],
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_pw,
			COALESCE(p.view_password_scope, 0)
		FROM posts p
		WHERE p.user_id = $1
			AND p.reply_to_id IS NULL
			AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
			AND p.visible_at <= NOW()
			AND p.group_id IS NULL
			AND `+postReadableByViewerSQL("p", "$2")+`
			AND p.media_type IN ('image', 'video')
			AND cardinality(p.object_keys) > 0
		ORDER BY p.visible_at DESC, p.id DESC
		LIMIT $3
	`, authorID, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostMediaTileRow
	for rows.Next() {
		var r PostMediaTileRow
		var key string
		if err := rows.Scan(&r.PostID, &r.AuthorUserID, &r.MediaType, &key, &r.HasViewPassword, &r.ViewPasswordScope); err != nil {
			return nil, err
		}
		r.FirstObjectKey = strings.TrimSpace(key)
		out = append(out, r)
	}
	return out, rows.Err()
}
