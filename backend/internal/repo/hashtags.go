package repo

import (
	"context"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type hashtagDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// NormalizeHashtagQuery converts `#tag` or `tag` into a normalized searchable hashtag.
func NormalizeHashtagQuery(raw string) string {
	tag := strings.TrimSpace(raw)
	tag = strings.TrimPrefix(tag, "#")
	tag = strings.ToLower(tag)
	tag = normalizeHashtagToken(tag)
	return tag
}

// ExtractHashtags returns normalized hashtags from text without duplicates.
func ExtractHashtags(text string) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for i := 0; i < len(runes); i++ {
		if runes[i] != '#' {
			continue
		}
		if i > 0 {
			prev := runes[i-1]
			if isHashtagRune(prev) || prev == '/' || prev == '#' {
				continue
			}
		}
		if i+1 >= len(runes) || !isHashtagRune(runes[i+1]) {
			continue
		}
		j := i + 1
		for j < len(runes) && isHashtagRune(runes[j]) {
			j++
		}
		tag := normalizeHashtagToken(strings.ToLower(string(runes[i+1 : j])))
		if tag == "" {
			i = j - 1
			continue
		}
		if _, ok := seen[tag]; !ok {
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
		i = j - 1
	}
	return out
}

func isHashtagRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
}

func normalizeHashtagToken(tag string) string {
	tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
	if tag == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range tag {
		if isHashtagRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func upsertHashtagID(ctx context.Context, db hashtagDB, tag string) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		INSERT INTO hashtags (tag)
		VALUES ($1)
		ON CONFLICT (tag) DO UPDATE SET tag = EXCLUDED.tag
		RETURNING id
	`, tag).Scan(&id)
	return id, err
}

func syncPostHashtags(ctx context.Context, db hashtagDB, postID uuid.UUID, caption string) error {
	if _, err := db.Exec(ctx, `DELETE FROM post_hashtags WHERE post_id = $1`, postID); err != nil {
		return err
	}
	for _, tag := range ExtractHashtags(caption) {
		tagID, err := upsertHashtagID(ctx, db, tag)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO post_hashtags (post_id, hashtag_id)
			VALUES ($1, $2)
			ON CONFLICT (post_id, hashtag_id) DO NOTHING
		`, postID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func syncFederationIncomingPostHashtags(ctx context.Context, db hashtagDB, postID uuid.UUID, caption string) error {
	if _, err := db.Exec(ctx, `DELETE FROM federation_incoming_post_hashtags WHERE federation_incoming_post_id = $1`, postID); err != nil {
		return err
	}
	for _, tag := range ExtractHashtags(caption) {
		tagID, err := upsertHashtagID(ctx, db, tag)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO federation_incoming_post_hashtags (federation_incoming_post_id, hashtag_id)
			VALUES ($1, $2)
			ON CONFLICT (federation_incoming_post_id, hashtag_id) DO NOTHING
		`, postID, tagID); err != nil {
			return err
		}
	}
	return nil
}
