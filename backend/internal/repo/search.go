package repo

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SearchAccountRow struct {
	ID              uuid.UUID
	Email           string
	Handle          string
	DisplayName     string
	Bio             string
	AvatarObjectKey *string
}

func (p *Pool) SearchPostsForViewer(ctx context.Context, viewerID uuid.UUID, query string, limit int) ([]PostRow, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []PostRow{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	isTagSearch := strings.HasPrefix(query, "#")
	var rows pgx.Rows
	var err error
	if isTagSearch {
		tag := NormalizeHashtagQuery(query)
		if tag == "" {
			return []PostRow{}, nil
		}
		pattern := "%#" + tag + "%"
		rows, err = p.db.Query(ctx, `
			SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
				p.is_nsfw,
				`+postVisibilityExpr("p")+`,
				(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
				COALESCE(p.view_password_scope, 0),
				COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
				p.created_at, p.visible_at,
				COALESCE(rpl.reply_count, 0)::bigint,
				COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
				COALESCE(rp.repost_count, 0)::bigint,
				EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
				EXISTS (SELECT 1 FROM post_reposts r WHERE r.post_id = p.id AND r.user_id = $1),
				EXISTS (SELECT 1 FROM post_bookmarks b WHERE b.post_id = p.id AND b.user_id = $1)
			FROM posts p
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
			WHERE p.visible_at <= NOW()
				AND p.group_id IS NULL
				AND `+postReadableByViewerSQL("p", "$1")+`
				AND (
					EXISTS (
						SELECT 1
						FROM post_hashtags ph
						JOIN hashtags h ON h.id = ph.hashtag_id
						WHERE ph.post_id = p.id AND h.tag = $2
					)
					OR LOWER(COALESCE(p.caption, '')) LIKE $3
				)
			ORDER BY p.visible_at DESC, p.id DESC
			LIMIT $4
		`, viewerID, tag, pattern, limit)
	} else {
		pattern := "%" + query + "%"
		rows, err = p.db.Query(ctx, `
			SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
				p.is_nsfw,
				`+postVisibilityExpr("p")+`,
				(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
				COALESCE(p.view_password_scope, 0),
				COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
				p.created_at, p.visible_at,
				COALESCE(rpl.reply_count, 0)::bigint,
				COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
				COALESCE(rp.repost_count, 0)::bigint,
				EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
				EXISTS (SELECT 1 FROM post_reposts r WHERE r.post_id = p.id AND r.user_id = $1),
				EXISTS (SELECT 1 FROM post_bookmarks b WHERE b.post_id = p.id AND b.user_id = $1)
			FROM posts p
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
			WHERE p.visible_at <= NOW()
				AND p.group_id IS NULL
				AND `+postReadableByViewerSQL("p", "$1")+`
				AND COALESCE(p.caption, '') ILIKE $2
			ORDER BY p.visible_at DESC, p.id DESC
			LIMIT $3
		`, viewerID, pattern, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostRows(rows)
}

func (p *Pool) SearchAccounts(ctx context.Context, query string, limit int) ([]SearchAccountRow, error) {
	query = strings.TrimSpace(query)
	if query == "" || strings.HasPrefix(query, "#") {
		return []SearchAccountRow{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	pattern := "%" + query + "%"
	prefix := query + "%"
	rows, err := p.db.Query(ctx, `
		SELECT id, email, handle, COALESCE(display_name, ''), COALESCE(bio, ''), avatar_object_key
		FROM users
		WHERE handle ILIKE $1
			OR COALESCE(display_name, '') ILIKE $1
			OR COALESCE(bio, '') ILIKE $1
		ORDER BY
			CASE
				WHEN lower(handle) = lower($2) THEN 0
				WHEN handle ILIKE $3 THEN 1
				WHEN COALESCE(display_name, '') ILIKE $3 THEN 2
				ELSE 3
			END,
			handle ASC
		LIMIT $4
	`, pattern, query, prefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SearchAccountRow, 0, limit)
	for rows.Next() {
		var row SearchAccountRow
		var av pgtype.Text
		if err := rows.Scan(&row.ID, &row.Email, &row.Handle, &row.DisplayName, &row.Bio, &av); err != nil {
			return nil, err
		}
		if av.Valid && strings.TrimSpace(av.String) != "" {
			s := strings.TrimSpace(av.String)
			row.AvatarObjectKey = &s
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) SearchFederatedIncomingForViewer(ctx context.Context, viewerID uuid.UUID, query string, limit int) ([]FederatedIncomingPost, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []FederatedIncomingPost{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	isTagSearch := strings.HasPrefix(query, "#")
	var rows pgx.Rows
	var err error
	if isTagSearch {
		tag := NormalizeHashtagQuery(query)
		if tag == "" {
			return []FederatedIncomingPost{}, nil
		}
		pattern := "%#" + tag + "%"
		rows, err = p.db.Query(ctx, `
			SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
				COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
				f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count
			FROM federation_incoming_posts f
			WHERE f.deleted_at IS NULL
				AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)
				AND (
					EXISTS (
						SELECT 1
						FROM federation_incoming_post_hashtags fih
						JOIN hashtags h ON h.id = fih.hashtag_id
						WHERE fih.federation_incoming_post_id = f.id AND h.tag = $2
					)
					OR LOWER(COALESCE(f.caption_text, '')) LIKE $3
				)
			ORDER BY f.published_at DESC, f.id DESC
			LIMIT $4
		`, viewerID, tag, pattern, limit)
	} else {
		pattern := "%" + query + "%"
		rows, err = p.db.Query(ctx, `
			SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
				COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
				f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count
			FROM federation_incoming_posts f
			WHERE f.deleted_at IS NULL
				AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)
				AND COALESCE(f.caption_text, '') ILIKE $2
			ORDER BY f.published_at DESC, f.id DESC
			LIMIT $3
		`, viewerID, pattern, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := p.AttachPollsToFederatedIncoming(ctx, viewerID, out); err != nil {
		return nil, err
	}
	return out, nil
}

func scanPostRows(rows pgx.Rows) ([]PostRow, error) {
	var out []PostRow
	for rows.Next() {
		var r PostRow
		var av pgtype.Text
		var scope int
		var textRanges string
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Email, &r.UserHandle, &r.DisplayName, &av, &r.Caption, &r.MediaType, &r.ObjectKeys,
			&r.IsNSFW, &r.Visibility, &r.HasViewPassword, &scope, &textRanges, &r.CreatedAt, &r.VisibleAt,
			&r.ReplyCount, &r.LikeCount, &r.RepostCount, &r.LikedByMe, &r.RepostedByMe, &r.BookmarkedByMe,
		); err != nil {
			return nil, err
		}
		if av.Valid && strings.TrimSpace(av.String) != "" {
			s := strings.TrimSpace(av.String)
			r.AvatarObjectKey = &s
		}
		var err error
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeStoredViewPasswordProtection(r.HasViewPassword, r.Caption, scope, textRanges)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
