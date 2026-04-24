package repo

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// FederatedIncomingPost represents one inbound federated post row.
type FederatedIncomingPost struct {
	ID                     uuid.UUID
	ObjectIRI              string
	CreateActivityIRI      string
	ActorIRI               string
	ActorAcct              string
	ActorName              string
	ActorIconURL           string
	ActorProfileURL        string
	CaptionText            string
	MediaType              string
	MediaURLs              []string
	IsNSFW                 bool
	PublishedAt            time.Time
	ReceivedAt             time.Time
	RecipientUserID        *uuid.UUID
	LikeCount              int64
	LikedByMe              bool
	Reactions              []PostReaction
	RepostCount            int64
	RepostedByMe           bool
	BookmarkedByMe         bool
	Poll                   *PostPoll
	ReplyToObjectIRI       string
	RepostOfObjectIRI      string
	RepostComment          string
	HasViewPassword        bool
	ViewPasswordScope      int
	ViewPasswordTextRanges []ViewPasswordTextRange
	UnlockURL              string
	MembershipProvider     string
	MembershipCreatorID    string
	MembershipTierID       string
	UnlockedCaptionText    string
	UnlockedMediaType      string
	UnlockedMediaURLs      []string
	UnlockedIsNSFW         bool
}

type InsertFederatedIncomingInput struct {
	ObjectIRI              string
	CreateActivityIRI      string
	ActorIRI               string
	ActorAcct              string
	ActorName              string
	ActorIconURL           string
	ActorProfileURL        string
	CaptionText            string
	MediaType              string
	MediaURLs              []string
	IsNSFW                 bool
	PublishedAt            time.Time
	RecipientUserID        *uuid.UUID
	LikeCount              int64
	ReplyToObjectIRI       string
	RepostOfObjectIRI      string
	RepostComment          string
	HasViewPassword        bool
	ViewPasswordScope      int
	ViewPasswordTextRanges []ViewPasswordTextRange
	UnlockURL              string
	MembershipProvider     string
	MembershipCreatorID    string
	MembershipTierID       string
}

// InsertFederatedIncomingPost stores an inbound Create payload.
// If object_iri already exists, it returns false with no error.
func (p *Pool) InsertFederatedIncomingPost(ctx context.Context, in InsertFederatedIncomingInput) (inserted bool, err error) {
	if strings.TrimSpace(in.ObjectIRI) == "" || strings.TrimSpace(in.ActorIRI) == "" {
		return false, fmt.Errorf("missing object or actor iri")
	}
	if in.MediaURLs == nil {
		in.MediaURLs = []string{}
	}
	mt := strings.TrimSpace(strings.ToLower(in.MediaType))
	switch mt {
	case "image", "video", "none":
	default:
		return false, fmt.Errorf("invalid media_type")
	}
	in.ViewPasswordScope = EffectiveViewPasswordScope(in.HasViewPassword, in.ViewPasswordScope)
	if in.ViewPasswordScope == ViewPasswordScopeAll || in.ViewPasswordScope&ViewPasswordScopeText == 0 {
		in.ViewPasswordTextRanges = nil
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var postID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO federation_incoming_posts (
			object_iri, create_activity_iri, actor_iri, actor_acct, actor_name, actor_icon_url, actor_profile_url,
			caption_text, media_type, media_urls, is_nsfw, published_at, recipient_user_id, like_count,
			reply_to_object_iri, repost_of_object_iri, repost_comment,
			has_view_password, view_password_scope, view_password_text_ranges, unlock_url,
			membership_provider, membership_creator_id, membership_tier_id
		) VALUES ($1, NULLIF(trim($2), ''), $3, $4, $5, NULLIF(trim($6), ''), NULLIF(trim($7), ''),
			$8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20::jsonb, NULLIF(trim($21), ''),
			NULLIF(trim($22), ''), NULLIF(trim($23), ''), NULLIF(trim($24), ''))
		ON CONFLICT (object_iri) DO NOTHING
		RETURNING id
	`, strings.TrimSpace(in.ObjectIRI), strings.TrimSpace(in.CreateActivityIRI),
		strings.TrimSpace(in.ActorIRI), truncateRunes(strings.TrimSpace(in.ActorAcct), 200),
		truncateRunes(strings.TrimSpace(in.ActorName), 200),
		strings.TrimSpace(in.ActorIconURL), strings.TrimSpace(in.ActorProfileURL),
		truncateRunes(in.CaptionText, 10000), mt, in.MediaURLs, in.IsNSFW, in.PublishedAt.UTC(), in.RecipientUserID, maxInt64(in.LikeCount, 0),
		strings.TrimSpace(in.ReplyToObjectIRI), strings.TrimSpace(in.RepostOfObjectIRI), truncateRunes(strings.TrimSpace(in.RepostComment), 2000),
		in.HasViewPassword, in.ViewPasswordScope, MarshalViewPasswordTextRanges(in.ViewPasswordTextRanges), strings.TrimSpace(in.UnlockURL),
		strings.TrimSpace(in.MembershipProvider), strings.TrimSpace(in.MembershipCreatorID), strings.TrimSpace(in.MembershipTierID),
	).Scan(&postID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if err := syncFederationIncomingPostHashtags(ctx, tx, postID, in.CaptionText); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= max {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

func decodeFederatedIncomingViewPasswordProtection(hasPassword bool, storedScope int, rawRanges string) (int, []ViewPasswordTextRange, error) {
	scope := EffectiveViewPasswordScope(hasPassword, storedScope)
	if scope == ViewPasswordScopeNone {
		return ViewPasswordScopeNone, nil, nil
	}
	ranges, err := ParseViewPasswordTextRanges(rawRanges)
	if err != nil {
		return ViewPasswordScopeNone, nil, err
	}
	if scope == ViewPasswordScopeAll || scope&ViewPasswordScopeText == 0 {
		return scope, nil, nil
	}
	return scope, ranges, nil
}

// SoftDeleteFederatedIncomingByObjectIRI soft-deletes rows matching object_iri after a Delete activity.
func (p *Pool) SoftDeleteFederatedIncomingByObjectIRI(ctx context.Context, objectIRI string) error {
	oi := strings.TrimSpace(objectIRI)
	if oi == "" {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		UPDATE federation_incoming_posts SET deleted_at = NOW()
		WHERE deleted_at IS NULL AND object_iri = $1
	`, oi)
	return err
}

// ListFederatedIncomingForViewer returns inbound posts for the federated timeline in reverse chronological order.
// Rows with recipient_user_id NULL are visible to everyone; non-NULL rows are visible only to that user.
func (p *Pool) ListFederatedIncomingForViewer(ctx context.Context, viewerID uuid.UUID, limit int, beforePublished *time.Time, beforeID *uuid.UUID) ([]FederatedIncomingPost, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows pgx.Rows
	var err error
	if beforePublished != nil && beforeID != nil {
		rows, err = p.db.Query(ctx, `
			SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
				COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
				f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count,
				COALESCE(f.reply_to_object_iri, ''), COALESCE(f.repost_of_object_iri, ''), COALESCE(f.repost_comment, ''),
				f.has_view_password, COALESCE(f.view_password_scope, 0), COALESCE(f.view_password_text_ranges, '[]'::jsonb)::text, COALESCE(f.unlock_url, ''),
			COALESCE(f.membership_provider, ''), COALESCE(f.membership_creator_id, ''), COALESCE(f.membership_tier_id, '')
			FROM federation_incoming_posts f
			WHERE f.deleted_at IS NULL
				AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)
				AND COALESCE(btrim(f.reply_to_object_iri), '') = ''
				AND (
					f.published_at < $2::timestamptz
					OR (f.published_at = $2::timestamptz AND f.id < $3::uuid)
				)`+FedIncomingActorVisibleSQL("f", "$1")+`
			ORDER BY f.published_at DESC, f.id DESC
			LIMIT $4
		`, viewerID, *beforePublished, *beforeID, limit)
	} else {
		rows, err = p.db.Query(ctx, `
			SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
				COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
				f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count,
				COALESCE(f.reply_to_object_iri, ''), COALESCE(f.repost_of_object_iri, ''), COALESCE(f.repost_comment, ''),
				f.has_view_password, COALESCE(f.view_password_scope, 0), COALESCE(f.view_password_text_ranges, '[]'::jsonb)::text, COALESCE(f.unlock_url, ''),
			COALESCE(f.membership_provider, ''), COALESCE(f.membership_creator_id, ''), COALESCE(f.membership_tier_id, '')
			FROM federation_incoming_posts f
			WHERE f.deleted_at IS NULL
				AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)
				AND COALESCE(btrim(f.reply_to_object_iri), '') = ''
			`+FedIncomingActorVisibleSQL("f", "$1")+`
			ORDER BY f.published_at DESC, f.id DESC
			LIMIT $2
		`, viewerID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
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
	if err := p.AttachReactionsToFederatedIncoming(ctx, viewerID, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFederatedIncomingByID returns one non-deleted inbound federated post.
func (p *Pool) GetFederatedIncomingByID(ctx context.Context, id uuid.UUID) (FederatedIncomingPost, error) {
	var r FederatedIncomingPost
	var recipient pgtype.UUID
	var textRanges string
	var scope int
	err := p.db.QueryRow(ctx, `
		SELECT id, object_iri, COALESCE(create_activity_iri, ''), actor_iri, actor_acct, actor_name,
			COALESCE(actor_icon_url, ''), COALESCE(actor_profile_url, ''),
			caption_text, media_type, media_urls, is_nsfw, published_at, received_at, recipient_user_id, like_count,
			COALESCE(reply_to_object_iri, ''), COALESCE(repost_of_object_iri, ''), COALESCE(repost_comment, ''),
			has_view_password, COALESCE(view_password_scope, 0), COALESCE(view_password_text_ranges, '[]'::jsonb)::text, COALESCE(unlock_url, ''),
			COALESCE(membership_provider, ''), COALESCE(membership_creator_id, ''), COALESCE(membership_tier_id, '')
		FROM federation_incoming_posts
		WHERE deleted_at IS NULL AND id = $1
	`, id).Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
		&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
		&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &recipient, &r.LikeCount,
		&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
		&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
		&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID)
	if recipient.Valid {
		id := uuid.UUID(recipient.Bytes)
		r.RecipientUserID = &id
	}
	if err == nil {
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
	}
	return r, err
}

func (p *Pool) GetFederatedIncomingByObjectIRI(ctx context.Context, objectIRI string) (FederatedIncomingPost, error) {
	var r FederatedIncomingPost
	var recipient pgtype.UUID
	var textRanges string
	var scope int
	err := p.db.QueryRow(ctx, `
		SELECT id, object_iri, COALESCE(create_activity_iri, ''), actor_iri, actor_acct, actor_name,
			COALESCE(actor_icon_url, ''), COALESCE(actor_profile_url, ''),
			caption_text, media_type, media_urls, is_nsfw, published_at, received_at, recipient_user_id, like_count,
			COALESCE(reply_to_object_iri, ''), COALESCE(repost_of_object_iri, ''), COALESCE(repost_comment, ''),
			has_view_password, COALESCE(view_password_scope, 0), COALESCE(view_password_text_ranges, '[]'::jsonb)::text, COALESCE(unlock_url, ''),
			COALESCE(membership_provider, ''), COALESCE(membership_creator_id, ''), COALESCE(membership_tier_id, '')
		FROM federation_incoming_posts
		WHERE deleted_at IS NULL AND object_iri = $1
	`, strings.TrimSpace(objectIRI)).Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
		&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
		&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &recipient, &r.LikeCount,
		&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
		&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
		&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID)
	if recipient.Valid {
		id := uuid.UUID(recipient.Bytes)
		r.RecipientUserID = &id
	}
	if err == nil {
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
	}
	return r, err
}

type BookmarkedFederatedIncomingPost struct {
	FederatedIncomingPost
	BookmarkedAt time.Time
}

func (p *Pool) ListBookmarkedFederatedIncoming(ctx context.Context, viewerID uuid.UUID, limit int) ([]BookmarkedFederatedIncomingPost, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
			COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
			f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count,
			COALESCE(f.reply_to_object_iri, ''), COALESCE(f.repost_of_object_iri, ''), COALESCE(f.repost_comment, ''),
			f.has_view_password, COALESCE(f.view_password_scope, 0), COALESCE(f.view_password_text_ranges, '[]'::jsonb)::text, COALESCE(f.unlock_url, ''),
			COALESCE(f.membership_provider, ''), COALESCE(f.membership_creator_id, ''), COALESCE(f.membership_tier_id, ''),
			fb.created_at
		FROM federation_incoming_post_bookmarks fb
		JOIN federation_incoming_posts f ON f.id = fb.federation_incoming_post_id
		WHERE fb.user_id = $1
			AND f.deleted_at IS NULL
			AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)`+FedIncomingActorVisibleSQL("f", "$1")+`
		ORDER BY fb.created_at DESC, f.id DESC
		LIMIT $2
	`, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookmarkedFederatedIncomingPost
	for rows.Next() {
		var r BookmarkedFederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID, &r.BookmarkedAt); err != nil {
			return nil, err
		}
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
			return nil, err
		}
		r.BookmarkedByMe = true
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	base := make([]FederatedIncomingPost, len(out))
	for i := range out {
		base[i] = out[i].FederatedIncomingPost
	}
	if err := p.AttachPollsToFederatedIncoming(ctx, viewerID, base); err != nil {
		return nil, err
	}
	if err := p.AttachReactionsToFederatedIncoming(ctx, viewerID, base); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].FederatedIncomingPost = base[i]
		out[i].BookmarkedByMe = true
	}
	return out, nil
}

func (p *Pool) ListFederatedIncomingRepliesByObjectIRI(ctx context.Context, viewerID uuid.UUID, objectIRI string, limit int) ([]FederatedIncomingPost, error) {
	objectIRI = strings.TrimSpace(objectIRI)
	if objectIRI == "" {
		return []FederatedIncomingPost{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
			COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
			f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count,
			COALESCE(f.reply_to_object_iri, ''), COALESCE(f.repost_of_object_iri, ''), COALESCE(f.repost_comment, ''),
			f.has_view_password, COALESCE(f.view_password_scope, 0), COALESCE(f.view_password_text_ranges, '[]'::jsonb)::text, COALESCE(f.unlock_url, ''),
			COALESCE(f.membership_provider, ''), COALESCE(f.membership_creator_id, ''), COALESCE(f.membership_tier_id, '')
		FROM federation_incoming_posts f
		WHERE f.deleted_at IS NULL
			AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)
			AND COALESCE(btrim(f.reply_to_object_iri), '') = $2`+FedIncomingActorVisibleSQL("f", "$1")+`
		ORDER BY f.published_at ASC, f.id ASC
		LIMIT $3
	`, viewerID, objectIRI, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
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
	if err := p.AttachReactionsToFederatedIncoming(ctx, viewerID, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Pool) ListFederatedIncomingRepliesByLocalPostIDSuffix(ctx context.Context, viewerID uuid.UUID, postID uuid.UUID, limit int) ([]FederatedIncomingPost, error) {
	if postID == uuid.Nil {
		return []FederatedIncomingPost{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	suffix := "/posts/" + postID.String()
	rows, err := p.db.Query(ctx, `
		SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
			COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
			f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count,
			COALESCE(f.reply_to_object_iri, ''), COALESCE(f.repost_of_object_iri, ''), COALESCE(f.repost_comment, ''),
			f.has_view_password, COALESCE(f.view_password_scope, 0), COALESCE(f.view_password_text_ranges, '[]'::jsonb)::text, COALESCE(f.unlock_url, ''),
			COALESCE(f.membership_provider, ''), COALESCE(f.membership_creator_id, ''), COALESCE(f.membership_tier_id, '')
		FROM federation_incoming_posts f
		WHERE f.deleted_at IS NULL
			AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)
			AND COALESCE(btrim(f.reply_to_object_iri), '') LIKE '%' || $2`+FedIncomingActorVisibleSQL("f", "$1")+`
		ORDER BY f.published_at ASC, f.id ASC
		LIMIT $3
	`, viewerID, suffix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
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
	if err := p.AttachReactionsToFederatedIncoming(ctx, viewerID, out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateFederatedIncomingFromNote updates caption, media, and related fields for the row matching an inbound Note object ID.
func (p *Pool) UpdateFederatedIncomingFromNote(ctx context.Context, objectIRI, caption string, mediaType string, mediaURLs []string, isNSFW bool, publishedAt time.Time, likeCount int64, replyToObjectIRI, repostOfObjectIRI, repostComment string, hasViewPassword bool, viewPasswordScope int, viewPasswordTextRanges []ViewPasswordTextRange, unlockURL, membershipProvider, membershipCreatorID, membershipTierID string) error {
	oi := strings.TrimSpace(objectIRI)
	if oi == "" {
		return fmt.Errorf("empty object iri")
	}
	mt := strings.TrimSpace(strings.ToLower(mediaType))
	switch mt {
	case "image", "video", "none":
	default:
		return fmt.Errorf("invalid media_type")
	}
	if mediaURLs == nil {
		mediaURLs = []string{}
	}
	scope := EffectiveViewPasswordScope(hasViewPassword, viewPasswordScope)
	ranges := viewPasswordTextRanges
	if scope == ViewPasswordScopeAll || scope&ViewPasswordScopeText == 0 {
		ranges = nil
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var postID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE federation_incoming_posts SET
			caption_text = $2,
			media_type = $3,
			media_urls = $4,
			is_nsfw = $5,
			published_at = $6,
			like_count = $7,
			reply_to_object_iri = $8,
			repost_of_object_iri = $9,
			repost_comment = $10,
			has_view_password = $11,
			view_password_scope = $12,
			view_password_text_ranges = $13::jsonb,
			unlock_url = NULLIF(trim($14), ''),
			membership_provider = NULLIF(trim($15), ''),
			membership_creator_id = NULLIF(trim($16), ''),
			membership_tier_id = NULLIF(trim($17), '')
		WHERE deleted_at IS NULL AND object_iri = $1
		RETURNING id
	`, oi, truncateRunes(caption, 10000), mt, mediaURLs, isNSFW, publishedAt.UTC(), maxInt64(likeCount, 0),
		strings.TrimSpace(replyToObjectIRI), strings.TrimSpace(repostOfObjectIRI), truncateRunes(strings.TrimSpace(repostComment), 2000),
		hasViewPassword, scope, MarshalViewPasswordTextRanges(ranges), strings.TrimSpace(unlockURL),
		strings.TrimSpace(membershipProvider), strings.TrimSpace(membershipCreatorID), strings.TrimSpace(membershipTierID)).Scan(&postID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	if err := syncFederationIncomingPostHashtags(ctx, tx, postID, caption); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateFederatedIncomingActorDisplay refreshes display metadata for all inbound posts whose actor_iri matches.
func (p *Pool) UpdateFederatedIncomingActorDisplay(ctx context.Context, actorIRI, acct, name, iconURL, profileURL string) error {
	ai := strings.TrimSpace(actorIRI)
	if ai == "" {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		UPDATE federation_incoming_posts SET
			actor_acct = $2,
			actor_name = $3,
			actor_icon_url = NULLIF(trim($4), ''),
			actor_profile_url = NULLIF(trim($5), '')
		WHERE deleted_at IS NULL AND actor_iri = $1
	`, ai, truncateRunes(strings.TrimSpace(acct), 200), truncateRunes(strings.TrimSpace(name), 200),
		strings.TrimSpace(iconURL), strings.TrimSpace(profileURL))
	return err
}

// RepointFederatedIncomingActor rewrites actor_iri from old to new during Move handling.
func (p *Pool) RepointFederatedIncomingActor(ctx context.Context, oldActorIRI, newActorIRI string) error {
	oldActorIRI = strings.TrimSpace(oldActorIRI)
	newActorIRI = strings.TrimSpace(newActorIRI)
	if oldActorIRI == "" || newActorIRI == "" || strings.EqualFold(oldActorIRI, newActorIRI) {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		UPDATE federation_incoming_posts SET actor_iri = $2
		WHERE deleted_at IS NULL AND actor_iri = $1
	`, oldActorIRI, newActorIRI)
	return err
}

// ListFederatedIncomingPublicByActorIRI lists public inbound posts for a single remote actor.
func (p *Pool) ListFederatedIncomingPublicByActorIRI(ctx context.Context, actorIRI string, limit int) ([]FederatedIncomingPost, error) {
	actorIRI = strings.TrimSpace(actorIRI)
	if actorIRI == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT id, object_iri, COALESCE(create_activity_iri, ''), actor_iri, actor_acct, actor_name,
			COALESCE(actor_icon_url, ''), COALESCE(actor_profile_url, ''),
			caption_text, media_type, media_urls, is_nsfw, published_at, received_at, like_count,
			COALESCE(reply_to_object_iri, ''), COALESCE(repost_of_object_iri, ''), COALESCE(repost_comment, ''),
			has_view_password, COALESCE(view_password_scope, 0), COALESCE(view_password_text_ranges, '[]'::jsonb)::text, COALESCE(unlock_url, ''),
			COALESCE(membership_provider, ''), COALESCE(membership_creator_id, ''), COALESCE(membership_tier_id, '')
		FROM federation_incoming_posts
		WHERE deleted_at IS NULL
			AND recipient_user_id IS NULL
			AND COALESCE(btrim(reply_to_object_iri), '') = ''
			AND actor_iri = $1
		ORDER BY published_at DESC, id DESC
		LIMIT $2
	`, actorIRI, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		var err error
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// viewer is anonymous for public remote profiles.
	if err := p.AttachPollsToFederatedIncoming(ctx, uuid.Nil, out); err != nil {
		return nil, err
	}
	if err := p.AttachReactionsToFederatedIncoming(ctx, uuid.Nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFederatedIncomingPublicByActorIRIForViewer is like ListFederatedIncomingPublicByActorIRI but hides actors blocked/muted by viewerID.
func (p *Pool) ListFederatedIncomingPublicByActorIRIForViewer(ctx context.Context, viewerID uuid.UUID, actorIRI string, limit int) ([]FederatedIncomingPost, error) {
	if viewerID == uuid.Nil {
		return p.ListFederatedIncomingPublicByActorIRI(ctx, actorIRI, limit)
	}
	actorIRI = strings.TrimSpace(actorIRI)
	if actorIRI == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
			COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
			f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count,
			COALESCE(f.reply_to_object_iri, ''), COALESCE(f.repost_of_object_iri, ''), COALESCE(f.repost_comment, ''),
			f.has_view_password, COALESCE(f.view_password_scope, 0), COALESCE(f.view_password_text_ranges, '[]'::jsonb)::text, COALESCE(f.unlock_url, ''),
			COALESCE(f.membership_provider, ''), COALESCE(f.membership_creator_id, ''), COALESCE(f.membership_tier_id, '')
		FROM federation_incoming_posts f
		WHERE f.deleted_at IS NULL
			AND f.recipient_user_id IS NULL
			AND COALESCE(btrim(f.reply_to_object_iri), '') = ''
			AND f.actor_iri = $1`+FedIncomingActorVisibleSQL("f", "$2")+`
		ORDER BY f.published_at DESC, f.id DESC
		LIMIT $3
	`, actorIRI, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		var err error
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
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
	if err := p.AttachReactionsToFederatedIncoming(ctx, viewerID, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFederatedIncomingForRemoteFollows returns posts only from actors the viewer follows through accepted remote follows.
func (p *Pool) ListFederatedIncomingForRemoteFollows(ctx context.Context, viewerID uuid.UUID, limit int) ([]FederatedIncomingPost, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT f.id, f.object_iri, COALESCE(f.create_activity_iri, ''), f.actor_iri, f.actor_acct, f.actor_name,
			COALESCE(f.actor_icon_url, ''), COALESCE(f.actor_profile_url, ''),
			f.caption_text, f.media_type, f.media_urls, f.is_nsfw, f.published_at, f.received_at, f.like_count,
			COALESCE(f.reply_to_object_iri, ''), COALESCE(f.repost_of_object_iri, ''), COALESCE(f.repost_comment, ''),
			f.has_view_password, COALESCE(f.view_password_scope, 0), COALESCE(f.view_password_text_ranges, '[]'::jsonb)::text, COALESCE(f.unlock_url, ''),
			COALESCE(f.membership_provider, ''), COALESCE(f.membership_creator_id, ''), COALESCE(f.membership_tier_id, '')
		FROM federation_incoming_posts f
		WHERE f.deleted_at IS NULL
			AND (f.recipient_user_id IS NULL OR f.recipient_user_id = $1)
			AND COALESCE(btrim(f.reply_to_object_iri), '') = ''
			AND EXISTS (
				SELECT 1 FROM federation_remote_follows r
				WHERE r.local_user_id = $1
					AND r.state = 'accepted'
					AND r.remote_actor_id = f.actor_iri
			)`+FedIncomingActorVisibleSQL("f", "$1")+`
		ORDER BY f.published_at DESC, f.id DESC
		LIMIT $2
	`, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
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
	if err := p.AttachReactionsToFederatedIncoming(ctx, viewerID, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFederatedIncomingByActorIRI returns inbound posts for one actor in reverse chronological order.
func (p *Pool) ListFederatedIncomingByActorIRI(ctx context.Context, actorIRI string, limit int) ([]FederatedIncomingPost, error) {
	actorIRI = strings.TrimSpace(actorIRI)
	if actorIRI == "" {
		return nil, fmt.Errorf("empty actor iri")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := p.db.Query(ctx, `
		SELECT id, object_iri, COALESCE(create_activity_iri, ''), actor_iri, actor_acct, actor_name,
			COALESCE(actor_icon_url, ''), COALESCE(actor_profile_url, ''),
			caption_text, media_type, media_urls, is_nsfw, published_at, received_at, like_count,
			COALESCE(reply_to_object_iri, ''), COALESCE(repost_of_object_iri, ''), COALESCE(repost_comment, ''),
			has_view_password, COALESCE(view_password_scope, 0), COALESCE(view_password_text_ranges, '[]'::jsonb)::text, COALESCE(unlock_url, ''),
			COALESCE(membership_provider, ''), COALESCE(membership_creator_id, ''), COALESCE(membership_tier_id, '')
		FROM federation_incoming_posts
		WHERE deleted_at IS NULL
			AND actor_iri = $1
			AND COALESCE(btrim(reply_to_object_iri), '') = ''
		ORDER BY published_at DESC, id DESC
		LIMIT $2
	`, actorIRI, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingPost
	for rows.Next() {
		var r FederatedIncomingPost
		var scope int
		var textRanges string
		if err := rows.Scan(&r.ID, &r.ObjectIRI, &r.CreateActivityIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName,
			&r.ActorIconURL, &r.ActorProfileURL, &r.CaptionText, &r.MediaType, &r.MediaURLs,
			&r.IsNSFW, &r.PublishedAt, &r.ReceivedAt, &r.LikeCount,
			&r.ReplyToObjectIRI, &r.RepostOfObjectIRI, &r.RepostComment,
			&r.HasViewPassword, &scope, &textRanges, &r.UnlockURL,
			&r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID); err != nil {
			return nil, err
		}
		r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeFederatedIncomingViewPasswordProtection(r.HasViewPassword, scope, textRanges)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := p.AttachPollsToFederatedIncoming(ctx, uuid.Nil, out); err != nil {
		return nil, err
	}
	if err := p.AttachReactionsToFederatedIncoming(ctx, uuid.Nil, out); err != nil {
		return nil, err
	}
	return out, nil
}
