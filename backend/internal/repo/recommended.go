package repo

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// AuthorAffinityScores returns author affinity scores from the viewer's recent explicit reactions.
// It intentionally ignores view or dwell signals because those logs are not available.
func (p *Pool) AuthorAffinityScores(ctx context.Context, viewerID uuid.UUID, since time.Time) (map[uuid.UUID]float64, error) {
	if viewerID == uuid.Nil {
		return map[uuid.UUID]float64{}, nil
	}
	rows, err := p.db.Query(ctx, `
		WITH
		likes AS (
			SELECT p.user_id AS author_id, 1.0::float8 AS w
			FROM post_likes l
			JOIN posts p ON p.id = l.post_id
			WHERE l.user_id = $1 AND l.created_at >= $2
		),
		reposts AS (
			SELECT p.user_id AS author_id, 3.0::float8 AS w
			FROM post_reposts r
			JOIN posts p ON p.id = r.post_id
			WHERE r.user_id = $1 AND r.created_at >= $2
		),
		bookmarks AS (
			SELECT p.user_id AS author_id, 2.0::float8 AS w
			FROM post_bookmarks b
			JOIN posts p ON p.id = b.post_id
			WHERE b.user_id = $1 AND b.created_at >= $2
		),
		replies AS (
			SELECT parent.user_id AS author_id, 2.0::float8 AS w
			FROM posts child
			JOIN posts parent ON parent.id = child.reply_to_id
			WHERE child.user_id = $1
			  AND child.reply_to_id IS NOT NULL
			  AND child.created_at >= $2
		),
		all_events AS (
			SELECT * FROM likes
			UNION ALL SELECT * FROM reposts
			UNION ALL SELECT * FROM bookmarks
			UNION ALL SELECT * FROM replies
		)
		SELECT author_id, SUM(w)::float8 AS s
		FROM all_events
		WHERE author_id <> $1
		GROUP BY author_id
	`, viewerID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[uuid.UUID]float64)
	for rows.Next() {
		var authorID uuid.UUID
		var s float64
		if err := rows.Scan(&authorID, &s); err != nil {
			return nil, err
		}
		if authorID == uuid.Nil {
			continue
		}
		// Compress scores gently to avoid overfitting.
		if s < 0 {
			s = 0
		}
		out[authorID] = math.Log1p(s)
	}
	return out, rows.Err()
}

// RecommendedCandidatePostIDs returns candidate post IDs for the recommended feed.
// Sources include second-hop authors, high-affinity authors from viewer reactions,
// and recently popular posts as a fallback.
func (p *Pool) RecommendedCandidatePostIDs(ctx context.Context, viewerID uuid.UUID, limit int) ([]uuid.UUID, error) {
	if viewerID == uuid.Nil {
		return []uuid.UUID{}, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 400
	}

	// Keep time windows fixed for now; they can be made configurable later.
	seedSince := time.Now().Add(-90 * 24 * time.Hour).UTC()
	postSince := time.Now().Add(-14 * 24 * time.Hour).UTC()
	popularSince := time.Now().Add(-48 * time.Hour).UTC()

	rows, err := p.db.Query(ctx, `
		WITH
		followees AS (
			SELECT followee_id
			FROM user_follows
			WHERE follower_id = $1
		),
		fof AS (
			SELECT DISTINCT f2.followee_id AS author_id
			FROM followees f1
			JOIN user_follows f2 ON f2.follower_id = f1.followee_id
			WHERE f2.followee_id <> $1
		),
		aff_authors AS (
			SELECT DISTINCT p.user_id AS author_id
			FROM post_likes l
			JOIN posts p ON p.id = l.post_id
			WHERE l.user_id = $1 AND l.created_at >= $2
			UNION
			SELECT DISTINCT p.user_id AS author_id
			FROM post_reposts r
			JOIN posts p ON p.id = r.post_id
			WHERE r.user_id = $1 AND r.created_at >= $2
			UNION
			SELECT DISTINCT p.user_id AS author_id
			FROM post_bookmarks b
			JOIN posts p ON p.id = b.post_id
			WHERE b.user_id = $1 AND b.created_at >= $2
			UNION
			SELECT DISTINCT parent.user_id AS author_id
			FROM posts child
			JOIN posts parent ON parent.id = child.reply_to_id
			WHERE child.user_id = $1 AND child.reply_to_id IS NOT NULL AND child.created_at >= $2
		),
		seed_authors AS (
			SELECT author_id FROM fof
			UNION
			SELECT author_id FROM aff_authors
		),
		seed_posts AS (
			SELECT p.id
			FROM posts p
			WHERE p.user_id IN (SELECT author_id FROM seed_authors)
			  AND p.user_id <> $1
			  AND p.reply_to_id IS NULL
			  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
			  AND p.visible_at <= NOW()
			  AND p.visible_at >= $3
			  AND p.group_id IS NULL
			  AND ` + postReadableByViewerSQL("p", "$1") + `
		),
		popular_posts AS (
			SELECT p.id
			FROM posts p
			LEFT JOIN (
				SELECT post_id, COUNT(*)::bigint AS like_count
				FROM post_likes
				WHERE created_at >= $4
				GROUP BY post_id
			) lk ON lk.post_id = p.id
			LEFT JOIN (
				SELECT post_id, COUNT(*)::bigint AS repost_count
				FROM post_reposts
				WHERE created_at >= $4
				GROUP BY post_id
			) rp ON rp.post_id = p.id
			LEFT JOIN (
				SELECT reply_to_id AS post_id, COUNT(*)::bigint AS reply_count
				FROM posts
				WHERE reply_to_id IS NOT NULL AND created_at >= $4
				GROUP BY reply_to_id
			) rpl ON rpl.post_id = p.id
			WHERE p.user_id <> $1
			  AND p.reply_to_id IS NULL
			  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
			  AND p.visible_at <= NOW()
			  AND p.visible_at >= $4
			  AND p.group_id IS NULL
			  AND ` + postReadableByViewerSQL("p", "$1") + `
			ORDER BY (COALESCE(lk.like_count,0) + 2*COALESCE(rp.repost_count,0) + 2*COALESCE(rpl.reply_count,0)) DESC,
					 p.visible_at DESC,
					 p.id DESC
			LIMIT 220
		)
		SELECT id
		FROM (
			SELECT id FROM seed_posts
			UNION
			SELECT id FROM popular_posts
		) x
		LIMIT $5
	`, viewerID, seedSince, postSince, popularSince, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != uuid.Nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// PostRowsByIDsForViewer returns top-level visible posts using the same projection as the feed.
func (p *Pool) PostRowsByIDsForViewer(ctx context.Context, viewerID uuid.UUID, ids []uuid.UUID) ([]PostRow, error) {
	if len(ids) == 0 {
		return []PostRow{}, nil
	}
	if len(ids) > 1200 {
		return nil, fmt.Errorf("too many ids")
	}

	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			p.created_at, p.visible_at,
			(COALESCE(rpl.reply_count, 0) + COALESCE(frpl.reply_count, 0))::bigint,
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
			SELECT substring(reply_to_object_iri FROM '/posts/([0-9a-fA-F-]{36})$')::uuid AS post_id, COUNT(*)::bigint AS reply_count
			FROM federation_incoming_posts
			WHERE deleted_at IS NULL
			  AND COALESCE(btrim(reply_to_object_iri), '') ~ '/posts/[0-9a-fA-F-]{36}$'
			GROUP BY 1
		) frpl ON frpl.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_likes GROUP BY post_id
		) lk ON lk.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS like_count FROM post_remote_likes GROUP BY post_id
		) rlk ON rlk.post_id = p.id
		LEFT JOIN (
			SELECT post_id, COUNT(*)::bigint AS repost_count FROM post_reposts GROUP BY post_id
		) rp ON rp.post_id = p.id
		WHERE p.id = ANY($2::uuid[])
		  AND p.user_id <> $1
		  AND p.reply_to_id IS NULL
		  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
		  AND p.visible_at <= NOW()
		  AND p.group_id IS NULL
		  AND `+postReadableByViewerSQL("p", "$1")+`
	`, viewerID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

