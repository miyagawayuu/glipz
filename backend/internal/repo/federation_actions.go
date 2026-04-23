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

type FederatedIncomingPollOptionSnapshot struct {
	Position int
	Label    string
	Votes    int64
}

type FederatedIncomingPollSnapshot struct {
	EndsAt  time.Time
	Options []FederatedIncomingPollOptionSnapshot
}

func (p *Pool) TotalLikesForPost(ctx context.Context, postID uuid.UUID) (int64, error) {
	var count int64
	err := p.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM post_likes WHERE post_id = $1) +
			(SELECT COUNT(*)::bigint FROM post_remote_likes WHERE post_id = $1)
	`, postID).Scan(&count)
	return count, err
}

func (p *Pool) ApplyRemoteLikeToLocalPost(ctx context.Context, postID uuid.UUID, remoteActorID, remoteActorAcct string, liked bool) (bool, int64, error) {
	remoteActorID = strings.TrimSpace(remoteActorID)
	remoteActorAcct = strings.TrimSpace(remoteActorAcct)
	if remoteActorID == "" {
		return false, 0, fmt.Errorf("empty remote actor id")
	}
	ok, err := p.PostExists(ctx, postID)
	if err != nil {
		return false, 0, err
	}
	if !ok {
		return false, 0, ErrNotFound
	}
	var changed bool
	if liked {
		tag, err := p.db.Exec(ctx, `
			INSERT INTO post_remote_likes (post_id, remote_actor_id, remote_actor_acct)
			VALUES ($1, $2, $3)
			ON CONFLICT (post_id, remote_actor_id) DO NOTHING
		`, postID, remoteActorID, remoteActorAcct)
		if err != nil {
			return false, 0, err
		}
		changed = tag.RowsAffected() > 0
	} else {
		tag, err := p.db.Exec(ctx, `DELETE FROM post_remote_likes WHERE post_id = $1 AND remote_actor_id = $2`, postID, remoteActorID)
		if err != nil {
			return false, 0, err
		}
		changed = tag.RowsAffected() > 0
	}
	count, err := p.TotalLikesForPost(ctx, postID)
	if err != nil {
		return false, 0, err
	}
	return changed, count, nil
}

func (p *Pool) ApplyRemoteReactionToLocalPost(ctx context.Context, postID uuid.UUID, remoteActorID, remoteActorAcct, emoji string, added bool) (bool, error) {
	remoteActorID = strings.TrimSpace(remoteActorID)
	remoteActorAcct = strings.TrimSpace(remoteActorAcct)
	if remoteActorID == "" {
		return false, fmt.Errorf("empty remote actor id")
	}
	normalized, valid := NormalizePostReactionEmoji(emoji)
	if !valid {
		return false, ErrInvalidReactionEmoji
	}
	ok, err := p.PostExists(ctx, postID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	if added {
		tag, err := p.db.Exec(ctx, `
			INSERT INTO post_remote_reactions (post_id, remote_actor_id, remote_actor_acct, emoji)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (post_id, remote_actor_id, emoji) DO NOTHING
		`, postID, remoteActorID, remoteActorAcct, normalized)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil
	}
	tag, err := p.db.Exec(ctx, `
		DELETE FROM post_remote_reactions
		WHERE post_id = $1 AND remote_actor_id = $2 AND emoji = $3
	`, postID, remoteActorID, normalized)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (p *Pool) ApplyRemotePollVoteToLocalPost(ctx context.Context, postID uuid.UUID, remoteActorID, remoteActorAcct string, optionPosition int) (bool, error) {
	remoteActorID = strings.TrimSpace(remoteActorID)
	remoteActorAcct = strings.TrimSpace(remoteActorAcct)
	if remoteActorID == "" {
		return false, fmt.Errorf("empty remote actor id")
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var endsAt time.Time
	var visibleAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT pp.ends_at, p.visible_at
		FROM post_polls pp
		JOIN posts p ON p.id = pp.post_id
		WHERE pp.post_id = $1
	`, postID).Scan(&endsAt, &visibleAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrPollNotFound
	}
	if err != nil {
		return false, err
	}
	if visibleAt.After(time.Now().UTC()) {
		return false, ErrPollNotVisible
	}
	if !endsAt.After(time.Now().UTC()) {
		return false, ErrPollClosed
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM post_poll_options WHERE post_id = $1 AND position = $2
		)
	`, postID, optionPosition).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, ErrPollInvalidOption
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO post_remote_poll_votes (post_id, remote_actor_id, remote_actor_acct, option_position)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (post_id, remote_actor_id) DO NOTHING
	`, postID, remoteActorID, remoteActorAcct, optionPosition)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (p *Pool) ToggleFederatedIncomingLike(ctx context.Context, userID, incomingID uuid.UUID) (bool, int64, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM federation_incoming_posts
			WHERE id = $1 AND deleted_at IS NULL
		)
	`, incomingID).Scan(&exists); err != nil {
		return false, 0, err
	}
	if !exists {
		return false, 0, ErrNotFound
	}
	liked := false
	tag, err := tx.Exec(ctx, `
		DELETE FROM federation_incoming_post_likes
		WHERE federation_incoming_post_id = $1 AND user_id = $2
	`, incomingID, userID)
	if err != nil {
		return false, 0, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO federation_incoming_post_likes (federation_incoming_post_id, user_id)
			VALUES ($1, $2)
		`, incomingID, userID); err != nil {
			return false, 0, err
		}
		liked = true
	}
	var count int64
	delta := int64(-1)
	if liked {
		delta = 1
	}
	if err := tx.QueryRow(ctx, `
		UPDATE federation_incoming_posts
		SET like_count = GREATEST(like_count + $2, 0)
		WHERE id = $1
		RETURNING like_count
	`, incomingID, delta).Scan(&count); err != nil {
		return false, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return liked, count, nil
}

func (p *Pool) ToggleFederatedIncomingRepost(ctx context.Context, userID, incomingID uuid.UUID, comment *string) (bool, int64, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM federation_incoming_posts
			WHERE id = $1 AND deleted_at IS NULL
		)
	`, incomingID).Scan(&exists); err != nil {
		return false, 0, err
	}
	if !exists {
		return false, 0, ErrNotFound
	}
	reposted := false
	tag, err := tx.Exec(ctx, `
		DELETE FROM federation_incoming_post_reposts
		WHERE federation_incoming_post_id = $1 AND user_id = $2
	`, incomingID, userID)
	if err != nil {
		return false, 0, err
	}
	if tag.RowsAffected() == 0 {
		commentText := ""
		if comment != nil {
			commentText = truncateRunes(strings.TrimSpace(*comment), 2000)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO federation_incoming_post_reposts (federation_incoming_post_id, user_id, comment_text)
			VALUES ($1, $2, $3)
		`, incomingID, userID, commentText); err != nil {
			return false, 0, err
		}
		reposted = true
	}
	var count int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM federation_incoming_post_reposts
		WHERE federation_incoming_post_id = $1
	`, incomingID).Scan(&count); err != nil {
		return false, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return reposted, count, nil
}

func (p *Pool) ToggleFederatedIncomingBookmark(ctx context.Context, userID, incomingID uuid.UUID) (bool, error) {
	var exists bool
	if err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM federation_incoming_posts
			WHERE id = $1 AND deleted_at IS NULL
		)
	`, incomingID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, ErrNotFound
	}
	tag, err := p.db.Exec(ctx, `
		DELETE FROM federation_incoming_post_bookmarks
		WHERE federation_incoming_post_id = $1 AND user_id = $2
	`, incomingID, userID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := p.db.Exec(ctx, `
			INSERT INTO federation_incoming_post_bookmarks (federation_incoming_post_id, user_id)
			VALUES ($1, $2)
		`, incomingID, userID); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (p *Pool) SetFederatedIncomingLikeCountByObjectIRI(ctx context.Context, objectIRI string, likeCount int64) error {
	objectIRI = strings.TrimSpace(objectIRI)
	if objectIRI == "" {
		return nil
	}
	if likeCount < 0 {
		likeCount = 0
	}
	_, err := p.db.Exec(ctx, `
		UPDATE federation_incoming_posts
		SET like_count = $2
		WHERE deleted_at IS NULL AND object_iri = $1
	`, objectIRI, likeCount)
	return err
}

func (p *Pool) SyncFederatedIncomingPollByObjectIRI(ctx context.Context, objectIRI string, poll *FederatedIncomingPollSnapshot) error {
	objectIRI = strings.TrimSpace(objectIRI)
	if objectIRI == "" {
		return nil
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var postID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM federation_incoming_posts
		WHERE deleted_at IS NULL AND object_iri = $1
	`, objectIRI).Scan(&postID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if poll == nil {
		if _, err := tx.Exec(ctx, `DELETE FROM federation_incoming_post_polls WHERE federation_incoming_post_id = $1`, postID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM federation_incoming_post_poll_options WHERE federation_incoming_post_id = $1`, postID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO federation_incoming_post_polls (federation_incoming_post_id, ends_at)
		VALUES ($1, $2)
		ON CONFLICT (federation_incoming_post_id) DO UPDATE SET ends_at = EXCLUDED.ends_at
	`, postID, poll.EndsAt.UTC()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM federation_incoming_post_poll_options WHERE federation_incoming_post_id = $1`, postID); err != nil {
		return err
	}
	for _, opt := range poll.Options {
		if _, err := tx.Exec(ctx, `
			INSERT INTO federation_incoming_post_poll_options (federation_incoming_post_id, position, label, votes)
			VALUES ($1, $2, $3, $4)
		`, postID, opt.Position, truncateRunes(strings.TrimSpace(opt.Label), 200), maxInt64(opt.Votes, 0)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Pool) CastFederatedIncomingPollVote(ctx context.Context, userID, incomingID, optionID uuid.UUID) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var endsAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT p.ends_at
		FROM federation_incoming_post_polls p
		JOIN federation_incoming_posts fp ON fp.id = p.federation_incoming_post_id
		WHERE p.federation_incoming_post_id = $1 AND fp.deleted_at IS NULL
	`, incomingID).Scan(&endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPollNotFound
	}
	if err != nil {
		return err
	}
	if !endsAt.After(time.Now().UTC()) {
		return ErrPollClosed
	}
	var optPostID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT federation_incoming_post_id
		FROM federation_incoming_post_poll_options
		WHERE id = $1
	`, optionID).Scan(&optPostID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPollInvalidOption
		}
		return err
	}
	if optPostID != incomingID {
		return ErrPollInvalidOption
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO federation_incoming_post_poll_votes (federation_incoming_post_id, user_id, option_id)
		VALUES ($1, $2, $3)
	`, incomingID, userID, optionID); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrPollAlreadyVoted
		}
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE federation_incoming_post_poll_options
		SET votes = votes + 1
		WHERE id = $1
	`, optionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Pool) FederatedIncomingPollOptionPosition(ctx context.Context, incomingID, optionID uuid.UUID) (int, error) {
	var pos int
	err := p.db.QueryRow(ctx, `
		SELECT position
		FROM federation_incoming_post_poll_options
		WHERE federation_incoming_post_id = $1 AND id = $2
	`, incomingID, optionID).Scan(&pos)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrPollInvalidOption
	}
	return pos, err
}

func (p *Pool) FederatedIncomingPollOptionIDByPosition(ctx context.Context, incomingID uuid.UUID, position int) (uuid.UUID, error) {
	var optionID uuid.UUID
	err := p.db.QueryRow(ctx, `
		SELECT id
		FROM federation_incoming_post_poll_options
		WHERE federation_incoming_post_id = $1 AND position = $2
	`, incomingID, position).Scan(&optionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrPollInvalidOption
	}
	return optionID, err
}

func maxInt64(v, floor int64) int64 {
	if v < floor {
		return floor
	}
	return v
}

func (p *Pool) UpsertFederatedIncomingUnlock(ctx context.Context, incomingID, userID uuid.UUID, caption, mediaType string, mediaURLs []string, isNSFW bool, expiresAt time.Time) error {
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))
	switch mediaType {
	case "image", "video", "none":
	default:
		return fmt.Errorf("invalid media_type")
	}
	if mediaURLs == nil {
		mediaURLs = []string{}
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_incoming_post_unlocks (
			federation_incoming_post_id, user_id, caption_text, media_type, media_urls, is_nsfw, expires_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (federation_incoming_post_id, user_id) DO UPDATE SET
			caption_text = EXCLUDED.caption_text,
			media_type = EXCLUDED.media_type,
			media_urls = EXCLUDED.media_urls,
			is_nsfw = EXCLUDED.is_nsfw,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`, incomingID, userID, truncateRunes(caption, 10000), mediaType, mediaURLs, isNSFW, expiresAt.UTC())
	return err
}

func (p *Pool) AttachPollsToFederatedIncoming(ctx context.Context, viewerID uuid.UUID, rows []FederatedIncomingPost) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	idx := make(map[uuid.UUID]int, len(rows))
	for i := range rows {
		if _, ok := idx[rows[i].ID]; ok {
			continue
		}
		idx[rows[i].ID] = i
		ids = append(ids, rows[i].ID)
	}
	if len(ids) == 0 {
		return nil
	}
	pollRows, err := p.db.Query(ctx, `
		SELECT federation_incoming_post_id, ends_at
		FROM federation_incoming_post_polls
		WHERE federation_incoming_post_id = ANY($1::uuid[])
	`, ids)
	if err != nil {
		return err
	}
	heads := map[uuid.UUID]time.Time{}
	for pollRows.Next() {
		var pid uuid.UUID
		var endsAt time.Time
		if err := pollRows.Scan(&pid, &endsAt); err != nil {
			pollRows.Close()
			return err
		}
		heads[pid] = endsAt
	}
	pollRows.Close()
	if err := pollRows.Err(); err != nil {
		return err
	}
	likeRows, err := p.db.Query(ctx, `
		SELECT federation_incoming_post_id
		FROM federation_incoming_post_likes
		WHERE user_id = $1 AND federation_incoming_post_id = ANY($2::uuid[])
	`, viewerID, ids)
	if err != nil {
		return err
	}
	likedByMe := make(map[uuid.UUID]bool)
	for likeRows.Next() {
		var pid uuid.UUID
		if err := likeRows.Scan(&pid); err != nil {
			likeRows.Close()
			return err
		}
		likedByMe[pid] = true
	}
	likeRows.Close()
	if err := likeRows.Err(); err != nil {
		return err
	}
	for pid := range likedByMe {
		if i, ok := idx[pid]; ok {
			rows[i].LikedByMe = true
		}
	}
	repostCountRows, err := p.db.Query(ctx, `
		SELECT federation_incoming_post_id, COUNT(*)::bigint
		FROM federation_incoming_post_reposts
		WHERE federation_incoming_post_id = ANY($1::uuid[])
		GROUP BY federation_incoming_post_id
	`, ids)
	if err != nil {
		return err
	}
	for repostCountRows.Next() {
		var pid uuid.UUID
		var count int64
		if err := repostCountRows.Scan(&pid, &count); err != nil {
			repostCountRows.Close()
			return err
		}
		if i, ok := idx[pid]; ok {
			rows[i].RepostCount = count
		}
	}
	repostCountRows.Close()
	if err := repostCountRows.Err(); err != nil {
		return err
	}
	if viewerID != uuid.Nil {
		repostedRows, err := p.db.Query(ctx, `
			SELECT federation_incoming_post_id
			FROM federation_incoming_post_reposts
			WHERE user_id = $1 AND federation_incoming_post_id = ANY($2::uuid[])
		`, viewerID, ids)
		if err != nil {
			return err
		}
		for repostedRows.Next() {
			var pid uuid.UUID
			if err := repostedRows.Scan(&pid); err != nil {
				repostedRows.Close()
				return err
			}
			if i, ok := idx[pid]; ok {
				rows[i].RepostedByMe = true
			}
		}
		repostedRows.Close()
		if err := repostedRows.Err(); err != nil {
			return err
		}
	}
	if viewerID != uuid.Nil {
		bookmarkedRows, err := p.db.Query(ctx, `
			SELECT federation_incoming_post_id
			FROM federation_incoming_post_bookmarks
			WHERE user_id = $1 AND federation_incoming_post_id = ANY($2::uuid[])
		`, viewerID, ids)
		if err != nil {
			return err
		}
		for bookmarkedRows.Next() {
			var pid uuid.UUID
			if err := bookmarkedRows.Scan(&pid); err != nil {
				bookmarkedRows.Close()
				return err
			}
			if i, ok := idx[pid]; ok {
				rows[i].BookmarkedByMe = true
			}
		}
		bookmarkedRows.Close()
		if err := bookmarkedRows.Err(); err != nil {
			return err
		}
	}
	if viewerID != uuid.Nil {
		unlockRows, err := p.db.Query(ctx, `
			SELECT federation_incoming_post_id, caption_text, media_type, media_urls, is_nsfw
			FROM federation_incoming_post_unlocks
			WHERE user_id = $1
				AND expires_at > NOW()
				AND federation_incoming_post_id = ANY($2::uuid[])
		`, viewerID, ids)
		if err != nil {
			return err
		}
		for unlockRows.Next() {
			var pid uuid.UUID
			var caption string
			var mediaType string
			var mediaURLs []string
			var isNSFW bool
			if err := unlockRows.Scan(&pid, &caption, &mediaType, &mediaURLs, &isNSFW); err != nil {
				unlockRows.Close()
				return err
			}
			if i, ok := idx[pid]; ok {
				rows[i].UnlockedCaptionText = caption
				rows[i].UnlockedMediaType = mediaType
				rows[i].UnlockedMediaURLs = append([]string(nil), mediaURLs...)
				rows[i].UnlockedIsNSFW = isNSFW
			}
		}
		unlockRows.Close()
		if err := unlockRows.Err(); err != nil {
			return err
		}
	}
	if len(heads) == 0 {
		return nil
	}
	optRows, err := p.db.Query(ctx, `
		SELECT id, federation_incoming_post_id, position, label, votes
		FROM federation_incoming_post_poll_options
		WHERE federation_incoming_post_id = ANY($1::uuid[])
		ORDER BY federation_incoming_post_id, position ASC
	`, ids)
	if err != nil {
		return err
	}
	optsByPost := make(map[uuid.UUID][]PostPollOption)
	for optRows.Next() {
		var pid uuid.UUID
		var opt PostPollOption
		var pos int16
		if err := optRows.Scan(&opt.ID, &pid, &pos, &opt.Label, &opt.Votes); err != nil {
			optRows.Close()
			return err
		}
		optsByPost[pid] = append(optsByPost[pid], opt)
	}
	optRows.Close()
	if err := optRows.Err(); err != nil {
		return err
	}
	myRows, err := p.db.Query(ctx, `
		SELECT federation_incoming_post_id, option_id
		FROM federation_incoming_post_poll_votes
		WHERE user_id = $1 AND federation_incoming_post_id = ANY($2::uuid[])
	`, viewerID, ids)
	if err != nil {
		return err
	}
	myVotes := make(map[uuid.UUID]uuid.UUID)
	for myRows.Next() {
		var pid, oid uuid.UUID
		if err := myRows.Scan(&pid, &oid); err != nil {
			myRows.Close()
			return err
		}
		myVotes[pid] = oid
	}
	myRows.Close()
	if err := myRows.Err(); err != nil {
		return err
	}
	for pid, endsAt := range heads {
		i, ok := idx[pid]
		if !ok {
			continue
		}
		pp := &PostPoll{
			EndsAt:  endsAt,
			Options: optsByPost[pid],
		}
		if oid, ok := myVotes[pid]; ok {
			x := oid
			pp.MyOptionID = &x
		}
		rows[i].Poll = pp
	}
	return nil
}

func (p *Pool) AttachPollsToGlipzProtocolPosts(ctx context.Context, rows []GlipzProtocolPostRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	idx := make(map[uuid.UUID]int, len(rows))
	for i := range rows {
		if _, ok := idx[rows[i].ID]; ok {
			continue
		}
		idx[rows[i].ID] = i
		ids = append(ids, rows[i].ID)
	}
	if len(ids) == 0 {
		return nil
	}
	pollRows, err := p.db.Query(ctx, `SELECT post_id, ends_at FROM post_polls WHERE post_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	heads := map[uuid.UUID]time.Time{}
	for pollRows.Next() {
		var pid uuid.UUID
		var endsAt time.Time
		if err := pollRows.Scan(&pid, &endsAt); err != nil {
			pollRows.Close()
			return err
		}
		heads[pid] = endsAt
	}
	pollRows.Close()
	if err := pollRows.Err(); err != nil {
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
	optsByPost := map[uuid.UUID][]PostPollOption{}
	for optRows.Next() {
		var pid uuid.UUID
		var opt PostPollOption
		var pos int16
		if err := optRows.Scan(&pid, &opt.ID, &pos, &opt.Label, &opt.Votes); err != nil {
			optRows.Close()
			return err
		}
		optsByPost[pid] = append(optsByPost[pid], opt)
	}
	optRows.Close()
	if err := optRows.Err(); err != nil {
		return err
	}
	for pid, endsAt := range heads {
		i, ok := idx[pid]
		if !ok {
			continue
		}
		rows[i].Poll = &PostPoll{
			EndsAt:  endsAt,
			Options: optsByPost[pid],
		}
	}
	return nil
}
