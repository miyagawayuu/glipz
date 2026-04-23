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

// PollCreateInput describes a poll attached to a newly created post.
type PollCreateInput struct {
	EndsAt time.Time
	Labels []string
}

// PostPollOption represents one poll option in feed responses.
type PostPollOption struct {
	ID    uuid.UUID
	Label string
	Votes int64
}

// PostPoll contains the poll summary exposed in feeds.
type PostPoll struct {
	EndsAt     time.Time
	Options    []PostPollOption
	MyOptionID *uuid.UUID
}

var (
	ErrPollClosed        = errors.New("poll closed")
	ErrPollNotFound      = errors.New("poll not found")
	ErrPollAlreadyVoted  = errors.New("poll already voted")
	ErrPollInvalidOption = errors.New("poll invalid option")
	ErrPollNotVisible    = errors.New("poll post not visible yet")
)

// AttachPollsToPosts attaches poll data to each post row, leaving Poll nil when none exists.
func (p *Pool) AttachPollsToPosts(ctx context.Context, viewerID uuid.UUID, rows []PostRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	idx := make(map[uuid.UUID]int, len(rows))
	for i := range rows {
		id := rows[i].ID
		if _, ok := idx[id]; ok {
			continue
		}
		idx[id] = i
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	rowsPoll, err := p.db.Query(ctx, `SELECT post_id, ends_at FROM post_polls WHERE post_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	type pollHead struct {
		endsAt time.Time
	}
	heads := make(map[uuid.UUID]pollHead)
	for rowsPoll.Next() {
		var pid uuid.UUID
		var ends time.Time
		if err := rowsPoll.Scan(&pid, &ends); err != nil {
			rowsPoll.Close()
			return err
		}
		heads[pid] = pollHead{endsAt: ends}
	}
	rowsPoll.Close()
	if err := rowsPoll.Err(); err != nil {
		return err
	}
	if len(heads) == 0 {
		return nil
	}

	optRows, err := p.db.Query(ctx, `
		SELECT o.post_id, o.id, o.position, o.label,
			COUNT(v.user_id)::bigint + COALESCE(rv.remote_votes, 0)::bigint AS votes
		FROM post_poll_options o
		LEFT JOIN post_poll_votes v ON v.option_id = o.id
		LEFT JOIN (
			SELECT ppo.post_id, prpv.option_position, COUNT(*)::bigint AS remote_votes
			FROM post_remote_poll_votes prpv
			JOIN post_poll_options ppo
				ON ppo.post_id = prpv.post_id
				AND ppo.position = prpv.option_position
			WHERE prpv.post_id = ANY($1::uuid[])
			GROUP BY ppo.post_id, prpv.option_position
		) rv ON rv.post_id = o.post_id AND rv.option_position = o.position
		WHERE o.post_id = ANY($1::uuid[])
		GROUP BY o.post_id, o.id, o.position, o.label, rv.remote_votes
		ORDER BY o.post_id, o.position ASC
	`, ids)
	if err != nil {
		return err
	}
	optsByPost := make(map[uuid.UUID][]PostPollOption)
	for optRows.Next() {
		var pid uuid.UUID
		var o PostPollOption
		var pos int16
		if err := optRows.Scan(&pid, &o.ID, &pos, &o.Label, &o.Votes); err != nil {
			optRows.Close()
			return err
		}
		optsByPost[pid] = append(optsByPost[pid], o)
	}
	optRows.Close()
	if err := optRows.Err(); err != nil {
		return err
	}

	myRows, err := p.db.Query(ctx, `
		SELECT post_id, option_id FROM post_poll_votes WHERE user_id = $1 AND post_id = ANY($2::uuid[])
	`, viewerID, ids)
	if err != nil {
		return err
	}
	myVote := make(map[uuid.UUID]uuid.UUID)
	for myRows.Next() {
		var pid, oid uuid.UUID
		if err := myRows.Scan(&pid, &oid); err != nil {
			myRows.Close()
			return err
		}
		myVote[pid] = oid
	}
	myRows.Close()
	if err := myRows.Err(); err != nil {
		return err
	}

	for pid, h := range heads {
		i, ok := idx[pid]
		if !ok {
			continue
		}
		pp := &PostPoll{
			EndsAt:  h.endsAt,
			Options: optsByPost[pid],
		}
		if oid, ok := myVote[pid]; ok {
			x := oid
			pp.MyOptionID = &x
		}
		rows[i].Poll = pp
	}
	return nil
}

// TryPublishRootPost marks a root post as broadcasted for the feed and returns the author ID for SSE.
func (p *Pool) TryPublishRootPost(ctx context.Context, postID uuid.UUID) (authorID uuid.UUID, ok bool, err error) {
	err = p.db.QueryRow(ctx, `
		UPDATE posts SET feed_broadcast_done = TRUE
		WHERE id = $1
		  AND reply_to_id IS NULL
		  AND COALESCE(btrim(reply_to_remote_object_iri), '') = ''
		  AND group_id IS NULL
		  AND NOT feed_broadcast_done
		  AND visible_at <= NOW()
		RETURNING user_id
	`, postID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return authorID, true, nil
}

// PendingFeedBroadcast represents a top-level post that still needs feed delivery, such as a scheduled post.
type PendingFeedBroadcast struct {
	PostID     uuid.UUID
	AuthorID   uuid.UUID
	Visibility string
}

// ClaimPendingFeedBroadcasts locks and marks due top-level posts as broadcasted, then returns them.
func (p *Pool) ClaimPendingFeedBroadcasts(ctx context.Context, limit int) ([]PendingFeedBroadcast, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := p.db.Query(ctx, `
		WITH c AS (
			SELECT id, user_id, `+postVisibilityExpr("posts")+` AS visibility
			FROM posts
			WHERE reply_to_id IS NULL
			  AND COALESCE(btrim(reply_to_remote_object_iri), '') = ''
			  AND group_id IS NULL
			  AND NOT feed_broadcast_done
			  AND visible_at <= NOW()
			ORDER BY visible_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE posts p
		SET feed_broadcast_done = TRUE
		FROM c
		WHERE p.id = c.id
		RETURNING p.id, p.user_id, c.visibility
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingFeedBroadcast
	for rows.Next() {
		var b PendingFeedBroadcast
		if err := rows.Scan(&b.PostID, &b.AuthorID, &b.Visibility); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CastPollVote records one vote and errors on closed, hidden, or duplicate votes.
func (p *Pool) CastPollVote(ctx context.Context, userID, postID, optionID uuid.UUID) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ends time.Time
	var vis time.Time
	var readable bool
	err = tx.QueryRow(ctx, `
		SELECT pp.ends_at, po.visible_at, `+postReadableByViewerSQL("po", "$2")+`
		FROM post_polls pp
		JOIN posts po ON po.id = pp.post_id
		WHERE pp.post_id = $1
	`, postID, userID).Scan(&ends, &vis, &readable)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPollNotFound
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if vis.After(now) {
		return ErrPollNotVisible
	}
	if !readable {
		return ErrPollNotVisible
	}
	if !ends.After(now) {
		return ErrPollClosed
	}

	var okOpt bool
	err = tx.QueryRow(ctx, `
		SELECT true FROM post_poll_options WHERE id = $1 AND post_id = $2
	`, optionID, postID).Scan(&okOpt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPollInvalidOption
	}
	if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO post_poll_votes (user_id, post_id, option_id) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, post_id) DO NOTHING
	`, userID, postID, optionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPollAlreadyVoted
	}

	return tx.Commit(ctx)
}

// ListScheduledRootPosts returns the caller's unpublished top-level posts scheduled for later visibility.
func (p *Pool) ListScheduledRootPosts(ctx context.Context, userID uuid.UUID, limit int) ([]PostRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			p.created_at,
			p.visible_at,
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
		WHERE p.user_id = $2
		  AND p.reply_to_id IS NULL
		  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
		  AND p.visible_at > NOW()
		ORDER BY p.visible_at ASC
		LIMIT $3
	`, userID, userID, limit)
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
	return out, rows.Err()
}
