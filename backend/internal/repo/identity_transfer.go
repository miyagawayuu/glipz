package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const IdentityTransferJobMaxAttempts = 10

type IdentityTransferSessionInsert struct {
	UserID              uuid.UUID
	PortableID          string
	TokenHash           string
	TokenNonce          string
	AllowedTargetOrigin string
	IncludePrivate      bool
	IncludeGated        bool
	ExpiresAt           time.Time
	CreatedIPHash       string
}

type IdentityTransferSession struct {
	ID                  uuid.UUID  `json:"id"`
	UserID              uuid.UUID  `json:"-"`
	PortableID          string     `json:"portable_id"`
	TokenHash           string     `json:"-"`
	TokenNonce          string     `json:"-"`
	AllowedTargetOrigin string     `json:"allowed_target_origin"`
	IncludePrivate      bool       `json:"include_private"`
	IncludeGated        bool       `json:"include_gated"`
	ExpiresAt           time.Time  `json:"expires_at"`
	UsedAt              *time.Time `json:"used_at,omitempty"`
	RevokedAt           *time.Time `json:"revoked_at,omitempty"`
	AttemptCount        int        `json:"attempt_count"`
	CreatedAt           time.Time  `json:"created_at"`
}

type IdentityTransferManifest struct {
	TotalPosts     int64 `json:"total_posts"`
	MediaItems     int64 `json:"media_items"`
	MediaBytes     int64 `json:"media_bytes"`
	ProfileItems   int64 `json:"profile_items"`
	FollowingItems int64 `json:"following_items"`
	FollowerItems  int64 `json:"follower_items"`
	BookmarkItems  int64 `json:"bookmark_items"`
	TotalItems     int64 `json:"total_items"`
}

type IdentityTransferCategoryStats struct {
	Total    int `json:"total"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

type IdentityTransferStats struct {
	Profile   IdentityTransferCategoryStats `json:"profile"`
	Posts     IdentityTransferCategoryStats `json:"posts"`
	Following IdentityTransferCategoryStats `json:"following"`
	Followers IdentityTransferCategoryStats `json:"followers"`
	Bookmarks IdentityTransferCategoryStats `json:"bookmarks"`
}

type TransferPollPayload struct {
	EndsAt  time.Time `json:"ends_at"`
	Options []string  `json:"options"`
}

type TransferPostPayload struct {
	ID                     string                  `json:"id"`
	ObjectID               string                  `json:"object_id"`
	Caption                string                  `json:"caption"`
	MediaType              string                  `json:"media_type"`
	ObjectKeys             []string                `json:"object_keys"`
	IsNSFW                 bool                    `json:"is_nsfw"`
	Visibility             string                  `json:"visibility"`
	ViewPasswordHash       string                  `json:"view_password_hash,omitempty"`
	ViewPasswordScope      int                     `json:"view_password_scope,omitempty"`
	ViewPasswordTextRanges []ViewPasswordTextRange `json:"view_password_text_ranges,omitempty"`
	VisibleAt              time.Time               `json:"visible_at"`
	CreatedAt              time.Time               `json:"created_at"`
	ReplyToObjectID        string                  `json:"reply_to_object_id,omitempty"`
	MembershipProvider     string                  `json:"membership_provider,omitempty"`
	MembershipCreatorID    string                  `json:"membership_creator_id,omitempty"`
	MembershipTierID       string                  `json:"membership_tier_id,omitempty"`
	Poll                   *TransferPollPayload    `json:"poll,omitempty"`
}

type TransferProfilePayload struct {
	Handle          string   `json:"handle,omitempty"`
	DisplayName     string   `json:"display_name"`
	Bio             string   `json:"bio"`
	AlsoKnownAs     []string `json:"also_known_as,omitempty"`
	AvatarObjectKey string   `json:"avatar_object_key,omitempty"`
	HeaderObjectKey string   `json:"header_object_key,omitempty"`
}

type TransferFollowingPayload struct {
	Kind               string    `json:"kind"`
	FolloweePortableID string    `json:"followee_portable_id,omitempty"`
	FolloweeHandle     string    `json:"followee_handle,omitempty"`
	RemoteActorID      string    `json:"remote_actor_id,omitempty"`
	RemoteInbox        string    `json:"remote_inbox,omitempty"`
	RemoteCurrentAcct  string    `json:"remote_current_acct,omitempty"`
	State              string    `json:"state,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type TransferFollowerPayload struct {
	RemoteActorID     string    `json:"remote_actor_id"`
	RemoteInbox       string    `json:"remote_inbox"`
	RemoteCurrentAcct string    `json:"remote_current_acct,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type TransferBookmarkPayload struct {
	Kind             string    `json:"kind"`
	OriginalObjectID string    `json:"original_object_id,omitempty"`
	RemoteObjectIRI  string    `json:"remote_object_iri,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type IdentityTransferImportJobInsert struct {
	UserID               uuid.UUID
	SourceOrigin         string
	TargetOrigin         string
	SourceSessionID      uuid.UUID
	SourceTokenEncrypted string
	IncludePrivate       bool
	IncludeGated         bool
}

type IdentityTransferImportJob struct {
	ID                   uuid.UUID             `json:"id"`
	UserID               uuid.UUID             `json:"-"`
	SourceOrigin         string                `json:"source_origin"`
	TargetOrigin         string                `json:"target_origin"`
	SourceSessionID      uuid.UUID             `json:"source_session_id"`
	SourceTokenEncrypted string                `json:"-"`
	Status               string                `json:"status"`
	TotalPosts           int                   `json:"total_posts"`
	ImportedPosts        int                   `json:"imported_posts"`
	FailedPosts          int                   `json:"failed_posts"`
	TotalItems           int                   `json:"total_items"`
	ImportedItems        int                   `json:"imported_items"`
	Stats                IdentityTransferStats `json:"stats"`
	NextCursor           string                `json:"next_cursor"`
	AttemptCount         int                   `json:"attempt_count"`
	NextAttemptAt        time.Time             `json:"next_attempt_at"`
	LastError            string                `json:"last_error"`
	IncludePrivate       bool                  `json:"include_private"`
	IncludeGated         bool                  `json:"include_gated"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

func (p *Pool) CreateIdentityTransferSession(ctx context.Context, in IdentityTransferSessionInsert) (IdentityTransferSession, error) {
	if in.UserID == uuid.Nil || strings.TrimSpace(in.TokenHash) == "" || in.ExpiresAt.IsZero() {
		return IdentityTransferSession{}, fmt.Errorf("invalid transfer session")
	}
	var row IdentityTransferSession
	err := p.db.QueryRow(ctx, `
		INSERT INTO identity_transfer_sessions (
			user_id, portable_id, token_hash, token_nonce, allowed_target_origin,
			include_private, include_gated, expires_at, created_ip_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, portable_id, token_hash, token_nonce, allowed_target_origin,
			include_private, include_gated, expires_at, used_at, revoked_at, attempt_count, created_at
	`, in.UserID, strings.TrimSpace(in.PortableID), strings.TrimSpace(in.TokenHash), strings.TrimSpace(in.TokenNonce),
		strings.TrimSpace(in.AllowedTargetOrigin), in.IncludePrivate, in.IncludeGated, in.ExpiresAt.UTC(), strings.TrimSpace(in.CreatedIPHash)).
		Scan(&row.ID, &row.UserID, &row.PortableID, &row.TokenHash, &row.TokenNonce, &row.AllowedTargetOrigin,
			&row.IncludePrivate, &row.IncludeGated, &row.ExpiresAt, &row.UsedAt, &row.RevokedAt, &row.AttemptCount, &row.CreatedAt)
	return row, err
}

func (p *Pool) IdentityTransferSessionByID(ctx context.Context, id uuid.UUID) (IdentityTransferSession, error) {
	var row IdentityTransferSession
	err := p.db.QueryRow(ctx, `
		SELECT id, user_id, portable_id, token_hash, token_nonce, allowed_target_origin,
			include_private, include_gated, expires_at, used_at, revoked_at, attempt_count, created_at
		FROM identity_transfer_sessions WHERE id = $1
	`, id).Scan(&row.ID, &row.UserID, &row.PortableID, &row.TokenHash, &row.TokenNonce, &row.AllowedTargetOrigin,
		&row.IncludePrivate, &row.IncludeGated, &row.ExpiresAt, &row.UsedAt, &row.RevokedAt, &row.AttemptCount, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdentityTransferSession{}, ErrNotFound
	}
	return row, err
}

func (p *Pool) RevokeIdentityTransferSession(ctx context.Context, userID, id uuid.UUID) error {
	ct, err := p.db.Exec(ctx, `
		UPDATE identity_transfer_sessions
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, id, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) RecordIdentityTransferTokenFailure(ctx context.Context, id uuid.UUID, ipHash string) error {
	_, err := p.db.Exec(ctx, `
		UPDATE identity_transfer_sessions
		SET attempt_count = attempt_count + 1, last_used_ip_hash = $2, updated_at = NOW()
		WHERE id = $1
	`, id, strings.TrimSpace(ipHash))
	return err
}

func (p *Pool) MarkIdentityTransferSessionUsed(ctx context.Context, id uuid.UUID, ipHash string) error {
	_, err := p.db.Exec(ctx, `
		UPDATE identity_transfer_sessions
		SET used_at = COALESCE(used_at, NOW()), last_used_ip_hash = $2, updated_at = NOW()
		WHERE id = $1
	`, id, strings.TrimSpace(ipHash))
	return err
}

func (p *Pool) IdentityTransferManifest(ctx context.Context, userID uuid.UUID, includePrivate, includeGated bool) (IdentityTransferManifest, error) {
	where := identityTransferPostWhere(includePrivate, includeGated)
	var out IdentityTransferManifest
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint, COALESCE(SUM(cardinality(object_keys)), 0)::bigint
		FROM posts
		WHERE user_id = $1 `+where, userID).Scan(&out.TotalPosts, &out.MediaItems)
	if err != nil {
		return out, err
	}
	out.ProfileItems = 1
	err = p.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM user_follows WHERE follower_id = $1) +
			(SELECT COUNT(*)::bigint FROM federation_remote_follows WHERE local_user_id = $1),
			(SELECT COUNT(*)::bigint FROM glipz_protocol_remote_followers WHERE local_user_id = $1),
			(SELECT COUNT(*)::bigint FROM post_bookmarks WHERE user_id = $1) +
			(SELECT COUNT(*)::bigint FROM federation_incoming_post_bookmarks WHERE user_id = $1)
	`, userID).Scan(&out.FollowingItems, &out.FollowerItems, &out.BookmarkItems)
	if err != nil {
		return out, err
	}
	out.TotalItems = out.ProfileItems + out.TotalPosts + out.FollowingItems + out.FollowerItems + out.BookmarkItems
	return out, nil
}

func identityTransferPostWhere(includePrivate, includeGated bool) string {
	parts := []string{"AND group_id IS NULL"}
	if !includePrivate {
		parts = append(parts, "AND visibility = 'public'")
	}
	if !includeGated {
		parts = append(parts, "AND view_password_hash IS NULL AND COALESCE(btrim(membership_provider), '') = ''")
	}
	return " " + strings.Join(parts, " ")
}

func (p *Pool) ListIdentityTransferPosts(ctx context.Context, userID uuid.UUID, includePrivate, includeGated bool, offset, limit int) ([]TransferPostPayload, int, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	where := identityTransferPostWhere(includePrivate, includeGated)
	rows, err := p.db.Query(ctx, `
		SELECT id, COALESCE(caption, ''), media_type, object_keys, is_nsfw, visibility,
			COALESCE(view_password_hash, ''), view_password_scope, COALESCE(view_password_text_ranges::text, '[]'),
			visible_at, created_at, reply_to_id, COALESCE(membership_provider, ''), COALESCE(membership_creator_id, ''), COALESCE(membership_tier_id, '')
		FROM posts
		WHERE user_id = $1 `+where+`
		ORDER BY visible_at ASC, id ASC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, offset, err
	}
	defer rows.Close()
	out := []TransferPostPayload{}
	for rows.Next() {
		var row TransferPostPayload
		var id uuid.UUID
		var replyTo *uuid.UUID
		var rangesJSON string
		if err := rows.Scan(&id, &row.Caption, &row.MediaType, &row.ObjectKeys, &row.IsNSFW, &row.Visibility,
			&row.ViewPasswordHash, &row.ViewPasswordScope, &rangesJSON, &row.VisibleAt, &row.CreatedAt, &replyTo,
			&row.MembershipProvider, &row.MembershipCreatorID, &row.MembershipTierID); err != nil {
			return nil, offset, err
		}
		row.ID = id.String()
		row.ObjectID = "glipz://" + id.String()
		row.ViewPasswordTextRanges = unmarshalTransferViewPasswordTextRanges(rangesJSON)
		if replyTo != nil {
			row.ReplyToObjectID = "glipz://" + replyTo.String()
		}
		poll, err := p.transferPollForPost(ctx, id)
		if err != nil {
			return nil, offset, err
		}
		row.Poll = poll
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, offset, err
	}
	return out, offset + len(out), nil
}

func (p *Pool) transferPollForPost(ctx context.Context, postID uuid.UUID) (*TransferPollPayload, error) {
	var endsAt time.Time
	err := p.db.QueryRow(ctx, `SELECT ends_at FROM post_polls WHERE post_id = $1`, postID).Scan(&endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `SELECT label FROM post_poll_options WHERE post_id = $1 ORDER BY position ASC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return &TransferPollPayload{EndsAt: endsAt, Options: labels}, rows.Err()
}

func (p *Pool) TransferObjectKeyAllowed(ctx context.Context, userID uuid.UUID, objectKey string, includePrivate, includeGated bool) (bool, error) {
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return false, nil
	}
	where := identityTransferPostWhere(includePrivate, includeGated)
	var ok bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM posts
			WHERE user_id = $1 AND $2 = ANY(object_keys) `+where+`
		) OR EXISTS (
			SELECT 1
			FROM users
			WHERE id = $1 AND (avatar_object_key = $2 OR header_object_key = $2)
		)
		LIMIT 1
	`, userID, objectKey).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

func (p *Pool) IdentityTransferProfile(ctx context.Context, userID uuid.UUID) (TransferProfilePayload, error) {
	u, err := p.UserByID(ctx, userID)
	if err != nil {
		return TransferProfilePayload{}, err
	}
	out := TransferProfilePayload{
		Handle:      u.Handle,
		DisplayName: u.DisplayName,
		Bio:         u.Bio,
		AlsoKnownAs: append([]string(nil), u.AlsoKnownAs...),
	}
	if u.AvatarObjectKey != nil {
		out.AvatarObjectKey = strings.TrimSpace(*u.AvatarObjectKey)
	}
	if u.HeaderObjectKey != nil {
		out.HeaderObjectKey = strings.TrimSpace(*u.HeaderObjectKey)
	}
	return out, nil
}

func (p *Pool) UpdateUserTransferProfile(ctx context.Context, userID uuid.UUID, in TransferProfilePayload, avatarKey, headerKey string) error {
	if len([]rune(in.Bio)) > 500 {
		return fmt.Errorf("bio too long")
	}
	if len([]rune(in.DisplayName)) > 50 {
		return fmt.Errorf("display name too long")
	}
	_, err := p.db.Exec(ctx, `
		UPDATE users
		SET display_name = $2,
			bio = $3,
			also_known_as = COALESCE($4, '{}'::text[]),
			avatar_object_key = NULLIF(trim($5), ''),
			header_object_key = NULLIF(trim($6), '')
		WHERE id = $1
	`, userID, in.DisplayName, in.Bio, in.AlsoKnownAs, avatarKey, headerKey)
	return err
}

func (p *Pool) ListIdentityTransferFollowing(ctx context.Context, userID uuid.UUID, offset, limit int) ([]TransferFollowingPayload, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := p.db.Query(ctx, `
		SELECT kind, followee_portable_id, followee_handle, remote_actor_id, remote_inbox, remote_current_acct, state, created_at
		FROM (
			SELECT 'local' AS kind, COALESCE(u.portable_id, '') AS followee_portable_id, u.handle AS followee_handle,
				'' AS remote_actor_id, '' AS remote_inbox, '' AS remote_current_acct, 'accepted' AS state, f.created_at
			FROM user_follows f
			JOIN users u ON u.id = f.followee_id
			WHERE f.follower_id = $1
			UNION ALL
			SELECT 'remote' AS kind, '' AS followee_portable_id, '' AS followee_handle,
				remote_actor_id, remote_inbox, COALESCE(remote_current_acct, ''), state, created_at
			FROM federation_remote_follows
			WHERE local_user_id = $1
		) x
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, offset, err
	}
	defer rows.Close()
	out := []TransferFollowingPayload{}
	for rows.Next() {
		var r TransferFollowingPayload
		if err := rows.Scan(&r.Kind, &r.FolloweePortableID, &r.FolloweeHandle, &r.RemoteActorID, &r.RemoteInbox, &r.RemoteCurrentAcct, &r.State, &r.CreatedAt); err != nil {
			return nil, offset, err
		}
		out = append(out, r)
	}
	return out, offset + len(out), rows.Err()
}

func (p *Pool) ListIdentityTransferFollowers(ctx context.Context, userID uuid.UUID, offset, limit int) ([]TransferFollowerPayload, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := p.db.Query(ctx, `
		SELECT remote_actor_id, remote_inbox, COALESCE(remote_current_acct, ''), created_at
		FROM glipz_protocol_remote_followers
		WHERE local_user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, offset, err
	}
	defer rows.Close()
	out := []TransferFollowerPayload{}
	for rows.Next() {
		var r TransferFollowerPayload
		if err := rows.Scan(&r.RemoteActorID, &r.RemoteInbox, &r.RemoteCurrentAcct, &r.CreatedAt); err != nil {
			return nil, offset, err
		}
		out = append(out, r)
	}
	return out, offset + len(out), rows.Err()
}

func (p *Pool) ListIdentityTransferBookmarks(ctx context.Context, userID uuid.UUID, offset, limit int) ([]TransferBookmarkPayload, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := p.db.Query(ctx, `
		SELECT kind, original_object_id, remote_object_iri, created_at
		FROM (
			SELECT 'local' AS kind, 'glipz://' || post_id::text AS original_object_id, '' AS remote_object_iri, created_at
			FROM post_bookmarks
			WHERE user_id = $1
			UNION ALL
			SELECT 'remote' AS kind, '' AS original_object_id,
				COALESCE(NULLIF(f.object_id, ''), f.object_iri) AS remote_object_iri, fb.created_at
			FROM federation_incoming_post_bookmarks fb
			JOIN federation_incoming_posts f ON f.id = fb.federation_incoming_post_id
			WHERE fb.user_id = $1 AND f.deleted_at IS NULL
		) x
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, offset, err
	}
	defer rows.Close()
	out := []TransferBookmarkPayload{}
	for rows.Next() {
		var r TransferBookmarkPayload
		if err := rows.Scan(&r.Kind, &r.OriginalObjectID, &r.RemoteObjectIRI, &r.CreatedAt); err != nil {
			return nil, offset, err
		}
		out = append(out, r)
	}
	return out, offset + len(out), rows.Err()
}

func (p *Pool) CreateIdentityTransferImportJob(ctx context.Context, in IdentityTransferImportJobInsert) (IdentityTransferImportJob, error) {
	if in.UserID == uuid.Nil || strings.TrimSpace(in.SourceOrigin) == "" || in.SourceSessionID == uuid.Nil {
		return IdentityTransferImportJob{}, fmt.Errorf("invalid import job")
	}
	var row IdentityTransferImportJob
	err := p.db.QueryRow(ctx, `
		INSERT INTO identity_transfer_import_jobs (
			user_id, source_origin, target_origin, source_session_id, source_token_encrypted, include_private, include_gated
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, source_origin, target_origin, source_session_id, source_token_encrypted, status,
			total_posts, imported_posts, failed_posts, total_items, imported_items,
			COALESCE(stats, '{}'::jsonb)::text, next_cursor, attempt_count, next_attempt_at,
			last_error, include_private, include_gated, created_at, updated_at
	`, in.UserID, strings.TrimRight(strings.TrimSpace(in.SourceOrigin), "/"), strings.TrimRight(strings.TrimSpace(in.TargetOrigin), "/"),
		in.SourceSessionID, strings.TrimSpace(in.SourceTokenEncrypted), in.IncludePrivate, in.IncludeGated).Scan(&row.ID, &row.UserID,
		&row.SourceOrigin, &row.TargetOrigin, &row.SourceSessionID, &row.SourceTokenEncrypted,
		&row.Status, &row.TotalPosts, &row.ImportedPosts, &row.FailedPosts, &row.TotalItems, &row.ImportedItems,
		statsScanner(&row.Stats), &row.NextCursor, &row.AttemptCount,
		&row.NextAttemptAt, &row.LastError, &row.IncludePrivate, &row.IncludeGated, &row.CreatedAt, &row.UpdatedAt)
	return row, err
}

func (p *Pool) IdentityTransferImportJobByID(ctx context.Context, userID, id uuid.UUID) (IdentityTransferImportJob, error) {
	var row IdentityTransferImportJob
	err := p.db.QueryRow(ctx, `
		SELECT id, user_id, source_origin, target_origin, source_session_id, source_token_encrypted, status,
			total_posts, imported_posts, failed_posts, total_items, imported_items,
			COALESCE(stats, '{}'::jsonb)::text, next_cursor, attempt_count, next_attempt_at,
			last_error, include_private, include_gated, created_at, updated_at
		FROM identity_transfer_import_jobs
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&row.ID, &row.UserID, &row.SourceOrigin, &row.TargetOrigin, &row.SourceSessionID, &row.SourceTokenEncrypted,
		&row.Status, &row.TotalPosts, &row.ImportedPosts, &row.FailedPosts, &row.TotalItems, &row.ImportedItems,
		statsScanner(&row.Stats), &row.NextCursor, &row.AttemptCount,
		&row.NextAttemptAt, &row.LastError, &row.IncludePrivate, &row.IncludeGated, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdentityTransferImportJob{}, ErrNotFound
	}
	return row, err
}

func (p *Pool) ClaimIdentityTransferImportJobs(ctx context.Context, limit int) ([]IdentityTransferImportJob, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := p.db.Query(ctx, `
		WITH cte AS (
			SELECT id
			FROM identity_transfer_import_jobs
			WHERE status IN ('pending', 'running')
				AND next_attempt_at <= NOW()
				AND (locked_until IS NULL OR locked_until < NOW())
				AND attempt_count < $1
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE identity_transfer_import_jobs j
		SET status = 'running', locked_until = NOW() + INTERVAL '3 minutes', updated_at = NOW()
		FROM cte
		WHERE j.id = cte.id
		RETURNING j.id, j.user_id, j.source_origin, j.target_origin, j.source_session_id, j.source_token_encrypted, j.status,
			j.total_posts, j.imported_posts, j.failed_posts, j.total_items, j.imported_items,
			COALESCE(j.stats, '{}'::jsonb)::text, j.next_cursor, j.attempt_count, j.next_attempt_at,
			j.last_error, j.include_private, j.include_gated, j.created_at, j.updated_at
	`, IdentityTransferJobMaxAttempts, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IdentityTransferImportJob
	for rows.Next() {
		var row IdentityTransferImportJob
		if err := rows.Scan(&row.ID, &row.UserID, &row.SourceOrigin, &row.TargetOrigin, &row.SourceSessionID, &row.SourceTokenEncrypted,
			&row.Status, &row.TotalPosts, &row.ImportedPosts, &row.FailedPosts, &row.TotalItems, &row.ImportedItems,
			statsScanner(&row.Stats), &row.NextCursor, &row.AttemptCount,
			&row.NextAttemptAt, &row.LastError, &row.IncludePrivate, &row.IncludeGated, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) CompleteIdentityTransferImportJob(ctx context.Context, id uuid.UUID, totalPosts, importedPosts, totalItems, importedItems int, stats IdentityTransferStats) error {
	statsJSON, err := marshalIdentityTransferStats(stats)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `
		UPDATE identity_transfer_import_jobs
		SET status = 'completed', total_posts = $2, imported_posts = $3,
			total_items = $4, imported_items = $5, stats = $6::jsonb,
			locked_until = NULL, last_error = '', updated_at = NOW()
		WHERE id = $1
	`, id, totalPosts, importedPosts, totalItems, importedItems, string(statsJSON))
	return err
}

func (p *Pool) ProgressIdentityTransferImportJob(ctx context.Context, id uuid.UUID, totalPosts, importedPosts, totalItems, importedItems int, nextCursor string, stats IdentityTransferStats) error {
	statsJSON, err := marshalIdentityTransferStats(stats)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `
		UPDATE identity_transfer_import_jobs
		SET total_posts = $2, imported_posts = $3, total_items = $4, imported_items = $5,
			next_cursor = $6, stats = $7::jsonb, locked_until = NOW() + INTERVAL '3 minutes', updated_at = NOW()
		WHERE id = $1
	`, id, totalPosts, importedPosts, totalItems, importedItems, strings.TrimSpace(nextCursor), string(statsJSON))
	return err
}

func (p *Pool) FailIdentityTransferImportJob(ctx context.Context, id uuid.UUID, attempt int, lastErr string, nextAt time.Time, dead bool) error {
	status := "running"
	if dead {
		status = "failed"
	}
	_, err := p.db.Exec(ctx, `
		UPDATE identity_transfer_import_jobs
		SET status = $2, attempt_count = $3, next_attempt_at = $4, locked_until = NULL,
			last_error = $5, failed_posts = failed_posts + 1, updated_at = NOW()
		WHERE id = $1
	`, id, status, attempt, nextAt.UTC(), truncateString(strings.TrimSpace(lastErr), 2000))
	return err
}

func (p *Pool) CancelIdentityTransferImportJob(ctx context.Context, userID, id uuid.UUID) error {
	ct, err := p.db.Exec(ctx, `
		UPDATE identity_transfer_import_jobs
		SET status = 'cancelled', locked_until = NULL, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status IN ('pending', 'running', 'failed')
	`, id, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) RetryIdentityTransferImportJob(ctx context.Context, userID, id uuid.UUID) error {
	ct, err := p.db.Exec(ctx, `
		UPDATE identity_transfer_import_jobs
		SET status = 'pending', next_attempt_at = NOW(), locked_until = NULL, last_error = '', updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status = 'failed'
	`, id, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) ImportTransferFollowing(ctx context.Context, userID uuid.UUID, in TransferFollowingPayload) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(in.Kind)) {
	case "local":
		portableID := NormalizePortableID(in.FolloweePortableID)
		if portableID == "" {
			return false, nil
		}
		var followeeID uuid.UUID
		err := p.db.QueryRow(ctx, `SELECT id FROM users WHERE portable_id = $1`, portableID).Scan(&followeeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if followeeID == userID {
			return false, nil
		}
		_, err = p.db.Exec(ctx, `
			INSERT INTO user_follows (follower_id, followee_id, created_at)
			VALUES ($1, $2, COALESCE($3::timestamptz, NOW()))
			ON CONFLICT (follower_id, followee_id) DO NOTHING
		`, userID, followeeID, safeTransferTime(in.CreatedAt))
		return true, err
	case "remote":
		actorID := strings.TrimSpace(in.RemoteActorID)
		inbox := strings.TrimSpace(in.RemoteInbox)
		if actorID == "" || inbox == "" {
			return false, nil
		}
		state := strings.ToLower(strings.TrimSpace(in.State))
		if state != "pending" && state != "accepted" {
			state = "accepted"
		}
		_, err := p.db.Exec(ctx, `
			INSERT INTO federation_remote_follows (local_user_id, remote_actor_id, remote_inbox, remote_current_acct, state, created_at)
			VALUES ($1, $2, $3, $4, $5, COALESCE($6::timestamptz, NOW()))
			ON CONFLICT (local_user_id, remote_actor_id) DO UPDATE SET
				remote_inbox = EXCLUDED.remote_inbox,
				remote_current_acct = COALESCE(NULLIF(EXCLUDED.remote_current_acct, ''), federation_remote_follows.remote_current_acct),
				state = CASE
					WHEN federation_remote_follows.state = 'accepted' THEN 'accepted'
					ELSE EXCLUDED.state
				END,
				updated_at = NOW()
		`, userID, actorID, inbox, NormalizeFederationTargetAcct(in.RemoteCurrentAcct), state, safeTransferTime(in.CreatedAt))
		return true, err
	default:
		return false, nil
	}
}

func (p *Pool) ImportTransferFollower(ctx context.Context, userID uuid.UUID, in TransferFollowerPayload) (bool, error) {
	actorID := strings.TrimSpace(in.RemoteActorID)
	inbox := strings.TrimSpace(in.RemoteInbox)
	if actorID == "" || inbox == "" {
		return false, nil
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO glipz_protocol_remote_followers (local_user_id, remote_actor_id, remote_inbox, remote_current_acct, created_at)
		VALUES ($1, $2, $3, $4, COALESCE($5::timestamptz, NOW()))
		ON CONFLICT (local_user_id, remote_actor_id) DO UPDATE SET
			remote_inbox = EXCLUDED.remote_inbox,
			remote_current_acct = COALESCE(NULLIF(EXCLUDED.remote_current_acct, ''), glipz_protocol_remote_followers.remote_current_acct)
	`, userID, actorID, inbox, NormalizeFederationTargetAcct(in.RemoteCurrentAcct), safeTransferTime(in.CreatedAt))
	return true, err
}

func (p *Pool) ImportTransferBookmark(ctx context.Context, userID, jobID uuid.UUID, in TransferBookmarkPayload) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(in.Kind)) {
	case "local":
		mapped, err := p.MigratedPostIDByOriginal(ctx, jobID, in.OriginalObjectID)
		if err != nil || mapped == nil {
			return false, err
		}
		_, err = p.db.Exec(ctx, `
			INSERT INTO post_bookmarks (user_id, post_id, created_at)
			VALUES ($1, $2, COALESCE($3::timestamptz, NOW()))
			ON CONFLICT (user_id, post_id) DO NOTHING
		`, userID, *mapped, safeTransferTime(in.CreatedAt))
		return true, err
	case "remote":
		iri := strings.TrimSpace(in.RemoteObjectIRI)
		if iri == "" {
			return false, nil
		}
		var incomingID uuid.UUID
		err := p.db.QueryRow(ctx, `
			SELECT id
			FROM federation_incoming_posts
			WHERE deleted_at IS NULL AND (object_iri = $1 OR object_id = $1)
			LIMIT 1
		`, iri).Scan(&incomingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		_, err = p.db.Exec(ctx, `
			INSERT INTO federation_incoming_post_bookmarks (user_id, federation_incoming_post_id, created_at)
			VALUES ($1, $2, COALESCE($3::timestamptz, NOW()))
			ON CONFLICT (user_id, federation_incoming_post_id) DO NOTHING
		`, userID, incomingID, safeTransferTime(in.CreatedAt))
		return true, err
	default:
		return false, nil
	}
}

func (p *Pool) InsertMigratedPost(ctx context.Context, userID, jobID uuid.UUID, payload TransferPostPayload, objectKeys []string, replyTo *uuid.UUID) (uuid.UUID, bool, error) {
	original := strings.TrimSpace(payload.ObjectID)
	if original == "" {
		original = strings.TrimSpace(payload.ID)
	}
	if original == "" {
		return uuid.Nil, false, fmt.Errorf("missing original object id")
	}
	var existing uuid.UUID
	err := p.db.QueryRow(ctx, `
		SELECT new_post_id FROM identity_transfer_post_mappings
		WHERE job_id = $1 AND original_object_id = $2 AND new_post_id IS NOT NULL
	`, jobID, original).Scan(&existing)
	if err == nil && existing != uuid.Nil {
		return existing, false, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, err
	}

	var pollIn *PollCreateInput
	if payload.Poll != nil && len(payload.Poll.Options) >= 2 {
		pollIn = &PollCreateInput{EndsAt: payload.Poll.EndsAt, Labels: payload.Poll.Options}
	}
	var hash *string
	if strings.TrimSpace(payload.ViewPasswordHash) != "" {
		h := strings.TrimSpace(payload.ViewPasswordHash)
		hash = &h
	}
	id, err := p.CreatePost(ctx, userID, payload.Caption, payload.MediaType, objectKeys, replyTo, "", payload.IsNSFW,
		payload.Visibility, hash, payload.ViewPasswordScope, payload.ViewPasswordTextRanges, payload.VisibleAt,
		pollIn, payload.MembershipProvider, payload.MembershipCreatorID, payload.MembershipTierID)
	if err != nil {
		_, _ = p.db.Exec(ctx, `
			INSERT INTO identity_transfer_post_mappings (job_id, user_id, source_post_id, original_object_id, status, last_error)
			VALUES ($1, $2, $3, $4, 'failed', $5)
			ON CONFLICT (job_id, original_object_id) DO UPDATE SET status = 'failed', last_error = EXCLUDED.last_error, updated_at = NOW()
		`, jobID, userID, payload.ID, original, truncateString(err.Error(), 2000))
		return uuid.Nil, false, err
	}
	// Imported history should not fan out as fresh timeline/federation activity.
	if _, err := p.db.Exec(ctx, `UPDATE posts SET feed_broadcast_done = TRUE WHERE id = $1`, id); err != nil {
		return uuid.Nil, false, err
	}
	_, err = p.db.Exec(ctx, `
		INSERT INTO identity_transfer_post_mappings (job_id, user_id, source_post_id, original_object_id, new_post_id, status)
		VALUES ($1, $2, $3, $4, $5, 'imported')
		ON CONFLICT (job_id, original_object_id) DO UPDATE SET new_post_id = EXCLUDED.new_post_id, status = 'imported', last_error = '', updated_at = NOW()
	`, jobID, userID, payload.ID, original, id)
	return id, true, err
}

func (p *Pool) MigratedPostIDByOriginal(ctx context.Context, jobID uuid.UUID, originalObjectID string) (*uuid.UUID, error) {
	originalObjectID = strings.TrimSpace(originalObjectID)
	if originalObjectID == "" {
		return nil, nil
	}
	var id uuid.UUID
	err := p.db.QueryRow(ctx, `
		SELECT new_post_id FROM identity_transfer_post_mappings
		WHERE job_id = $1 AND original_object_id = $2 AND new_post_id IS NOT NULL
	`, jobID, originalObjectID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

type identityTransferStatsScanner struct {
	stats *IdentityTransferStats
}

func statsScanner(stats *IdentityTransferStats) identityTransferStatsScanner {
	return identityTransferStatsScanner{stats: stats}
}

func (s identityTransferStatsScanner) Scan(src any) error {
	if s.stats == nil {
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case nil:
		*s.stats = IdentityTransferStats{}
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("unsupported identity transfer stats type %T", src)
	}
	if len(raw) == 0 {
		*s.stats = IdentityTransferStats{}
		return nil
	}
	if err := json.Unmarshal(raw, s.stats); err != nil {
		return err
	}
	return nil
}

func marshalIdentityTransferStats(stats IdentityTransferStats) ([]byte, error) {
	b, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func safeTransferTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func unmarshalTransferViewPasswordTextRanges(raw string) []ViewPasswordTextRange {
	var out []ViewPasswordTextRange
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return nil
	}
	return out
}
