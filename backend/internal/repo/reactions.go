package repo

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

type PostReaction struct {
	Emoji       string
	Count       int64
	ReactedByMe bool
}

var ErrInvalidReactionEmoji = errors.New("invalid reaction emoji")

func (p *Pool) AttachReactionsToPosts(ctx context.Context, viewerID uuid.UUID, rows []PostRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	seen := make(map[uuid.UUID]struct{}, len(rows))
	for i := range rows {
		if _, ok := seen[rows[i].ID]; ok {
			continue
		}
		seen[rows[i].ID] = struct{}{}
		ids = append(ids, rows[i].ID)
	}
	if len(ids) == 0 {
		return nil
	}
	reactionsByPost := make(map[uuid.UUID][]PostReaction, len(ids))
	// Keep legacy likes visible as hearts until post_likes is fully retired.
	// UNION avoids double-counting rows that were already migrated into post_reactions.
	sql := `
		WITH merged_reactions AS (
			SELECT user_id, post_id, emoji
			FROM post_reactions
			WHERE post_id = ANY($1::uuid[])
			UNION
			SELECT user_id, post_id, '❤️' AS emoji
			FROM post_likes
			WHERE post_id = ANY($1::uuid[])
			UNION
			SELECT NULL::uuid AS user_id, post_id, emoji
			FROM post_remote_reactions
			WHERE post_id = ANY($1::uuid[])
			UNION
			SELECT NULL::uuid AS user_id, post_id, '❤️' AS emoji
			FROM post_remote_likes
			WHERE post_id = ANY($1::uuid[])
		)
		SELECT
			post_id,
			emoji,
			COUNT(*)::bigint AS reaction_count,
			BOOL_OR(CASE WHEN $2::uuid = '00000000-0000-0000-0000-000000000000'::uuid THEN false ELSE user_id = $2 END) AS reacted_by_me
		FROM merged_reactions
		GROUP BY post_id, emoji
		ORDER BY post_id, reaction_count DESC, emoji ASC
	`
	rowsDB, err := p.db.Query(ctx, sql, ids, viewerID)
	if err != nil {
		return err
	}
	defer rowsDB.Close()
	for rowsDB.Next() {
		var postID uuid.UUID
		var reaction PostReaction
		if err := rowsDB.Scan(&postID, &reaction.Emoji, &reaction.Count, &reaction.ReactedByMe); err != nil {
			return err
		}
		reactionsByPost[postID] = append(reactionsByPost[postID], reaction)
	}
	if err := rowsDB.Err(); err != nil {
		return err
	}
	for i := range rows {
		rows[i].Reactions = reactionsByPost[rows[i].ID]
		if rows[i].Reactions == nil {
			rows[i].Reactions = []PostReaction{}
		}
	}
	return nil
}

func NormalizePostReactionEmoji(raw string) (string, bool) {
	emoji := strings.TrimSpace(raw)
	if emoji == "" {
		return "", false
	}
	if ref, ok := ParseEmojiReference(emoji); ok && ref.IsShortcode {
		return MakeCustomEmojiShortcode(ref.Name, ref.OwnerHandle, ref.Domain), true
	}
	if utf8.RuneCountInString(emoji) > 16 {
		return "", false
	}
	if strings.IndexFunc(emoji, unicode.IsSpace) >= 0 {
		return "", false
	}
	return emoji, true
}

func (p *Pool) AddPostReaction(ctx context.Context, userID, postID uuid.UUID, emoji string) (bool, error) {
	ok, err := p.CanViewerReadPost(ctx, userID, postID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	normalized, valid := NormalizePostReactionEmoji(emoji)
	if !valid {
		return false, ErrInvalidReactionEmoji
	}
	if ref, ok := ParseEmojiReference(normalized); ok && ref.IsShortcode && (ref.OwnerHandle != "" || ref.Domain != "") {
		if _, err := p.FindEnabledCustomEmojiByReference(ctx, ref); err != nil && !errors.Is(err, ErrNotFound) {
			return false, err
		} else if errors.Is(err, ErrNotFound) {
			return false, ErrInvalidReactionEmoji
		}
	}
	tag, err := p.db.Exec(ctx, `
		INSERT INTO post_reactions (user_id, post_id, emoji)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, post_id, emoji) DO NOTHING
	`, userID, postID, normalized)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (p *Pool) RemovePostReaction(ctx context.Context, userID, postID uuid.UUID, emoji string) (bool, error) {
	ok, err := p.CanViewerReadPost(ctx, userID, postID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	normalized, valid := NormalizePostReactionEmoji(emoji)
	if !valid {
		return false, ErrInvalidReactionEmoji
	}
	if ref, ok := ParseEmojiReference(normalized); ok && ref.IsShortcode && (ref.OwnerHandle != "" || ref.Domain != "") {
		if _, err := p.FindEnabledCustomEmojiByReference(ctx, ref); err != nil && !errors.Is(err, ErrNotFound) {
			return false, err
		} else if errors.Is(err, ErrNotFound) {
			return false, ErrInvalidReactionEmoji
		}
	}
	tag, err := p.db.Exec(ctx, `DELETE FROM post_reactions WHERE user_id = $1 AND post_id = $2 AND emoji = $3`, userID, postID, normalized)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
