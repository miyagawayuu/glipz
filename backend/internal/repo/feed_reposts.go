package repo

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/google/uuid"
)

// RepostFeedEntry captures who reposted which top-level post and when, plus the embedded original post row.
type RepostFeedEntry struct {
	RepostedAt          time.Time
	RepostComment       *string
	ReposterID          uuid.UUID
	ReposterEmail       string
	ReposterHandle      string
	ReposterDisplayName string
	ReposterAvatarKey   *string
	Original            PostRow
}

// ListRecentRepostsAll returns recent reposts in reverse chronological order.
// Only visible top-level posts can appear as embedded originals.
func (p *Pool) ListRecentRepostsAll(ctx context.Context, viewerID uuid.UUID, limit int) ([]RepostFeedEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `
		SELECT rr.created_at,
			rr.comment_text,
			rr.user_id,
			ure.email, ure.handle, ure.display_name, ure.avatar_object_key,
			p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			` + postVisibilityExpr("p") + `,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			p.created_at, p.visible_at,
			COALESCE(rpl.reply_count, 0)::bigint,
			COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
			COALESCE(rp.repost_count, 0)::bigint,
			EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
			EXISTS (SELECT 1 FROM post_reposts r2 WHERE r2.post_id = p.id AND r2.user_id = $1),
			EXISTS (SELECT 1 FROM post_bookmarks b WHERE b.post_id = p.id AND b.user_id = $1)
		FROM post_reposts rr
		JOIN users ure ON ure.id = rr.user_id
		JOIN posts p ON p.id = rr.post_id
			AND p.reply_to_id IS NULL
			AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
			AND p.visible_at <= NOW()
			AND p.group_id IS NULL
			AND ` + postReadableByViewerSQL("p", "$1") + `
		JOIN users u ON u.id = p.user_id
		LEFT JOIN (
			SELECT reply_to_id AS post_id, COUNT(*)::bigint AS reply_count
			FROM posts
			WHERE reply_to_id IS NOT NULL
			GROUP BY reply_to_id
		) rpl ON rpl.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_likes GROUP BY post_id
		) lk ON lk.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_remote_likes GROUP BY post_id
		) rlk ON rlk.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS repost_count FROM post_reposts GROUP BY post_id
		) rp ON rp.post_id = p.id
		ORDER BY rr.created_at DESC, rr.user_id DESC, p.id DESC
		LIMIT $2
	`
	rows, err := p.db.Query(ctx, q, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRepostFeedRows(rows)
}

// ListRecentRepostsFollowing returns recent reposts from followed users in reverse chronological order.
func (p *Pool) ListRecentRepostsFollowing(ctx context.Context, viewerID uuid.UUID, limit int) ([]RepostFeedEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `
		SELECT rr.created_at,
			rr.comment_text,
			rr.user_id,
			ure.email, ure.handle, ure.display_name, ure.avatar_object_key,
			p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			` + postVisibilityExpr("p") + `,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			p.created_at, p.visible_at,
			COALESCE(rpl.reply_count, 0)::bigint,
			COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
			COALESCE(rp.repost_count, 0)::bigint,
			EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
			EXISTS (SELECT 1 FROM post_reposts r2 WHERE r2.post_id = p.id AND r2.user_id = $1),
			EXISTS (SELECT 1 FROM post_bookmarks b WHERE b.post_id = p.id AND b.user_id = $1)
		FROM post_reposts rr
		JOIN users ure ON ure.id = rr.user_id
		JOIN posts p ON p.id = rr.post_id
			AND p.reply_to_id IS NULL
			AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
			AND p.visible_at <= NOW()
			AND p.group_id IS NULL
			AND ` + postReadableByViewerSQL("p", "$1") + `
		JOIN users u ON u.id = p.user_id
		LEFT JOIN (
			SELECT reply_to_id AS post_id, COUNT(*)::bigint AS reply_count
			FROM posts
			WHERE reply_to_id IS NOT NULL
			GROUP BY reply_to_id
		) rpl ON rpl.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_likes GROUP BY post_id
		) lk ON lk.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_remote_likes GROUP BY post_id
		) rlk ON rlk.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS repost_count FROM post_reposts GROUP BY post_id
		) rp ON rp.post_id = p.id
		WHERE EXISTS (
			SELECT 1 FROM user_follows f
			WHERE f.follower_id = $1 AND f.followee_id = rr.user_id
		)
		ORDER BY rr.created_at DESC, rr.user_id DESC, p.id DESC
		LIMIT $2
	`
	rows, err := p.db.Query(ctx, q, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRepostFeedRows(rows)
}

func scanRepostFeedRows(rows pgx.Rows) ([]RepostFeedEntry, error) {
	var out []RepostFeedEntry
	for rows.Next() {
		var e RepostFeedEntry
		pr := &e.Original
		var rAv, pAv, repCom pgtype.Text
		var scope int
		var textRanges string
		if err := rows.Scan(
			&e.RepostedAt,
			&repCom,
			&e.ReposterID,
			&e.ReposterEmail, &e.ReposterHandle, &e.ReposterDisplayName, &rAv,
			&pr.ID, &pr.UserID, &pr.Email, &pr.UserHandle, &pr.DisplayName, &pAv,
			&pr.Caption, &pr.MediaType, &pr.ObjectKeys,
			&pr.IsNSFW, &pr.Visibility, &pr.HasViewPassword, &scope, &textRanges, &pr.CreatedAt, &pr.VisibleAt,
			&pr.ReplyCount, &pr.LikeCount, &pr.RepostCount, &pr.LikedByMe, &pr.RepostedByMe, &pr.BookmarkedByMe,
		); err != nil {
			return nil, err
		}
		if repCom.Valid {
			if s := strings.TrimSpace(repCom.String); s != "" {
				e.RepostComment = &s
			}
		}
		if rAv.Valid && strings.TrimSpace(rAv.String) != "" {
			s := strings.TrimSpace(rAv.String)
			e.ReposterAvatarKey = &s
		}
		if pAv.Valid && strings.TrimSpace(pAv.String) != "" {
			s := strings.TrimSpace(pAv.String)
			pr.AvatarObjectKey = &s
		} else {
			pr.AvatarObjectKey = nil
		}
		var err error
		pr.ViewPasswordScope, pr.ViewPasswordTextRanges, err = decodeStoredViewPasswordProtection(pr.HasViewPassword, pr.Caption, scope, textRanges)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
