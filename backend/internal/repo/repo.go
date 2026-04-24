package repo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var ErrNotFound = errors.New("not found")

// ErrCannotFollowSelf is returned when a user tries to follow themselves.
var ErrCannotFollowSelf = errors.New("cannot follow self")

// ErrForbidden is returned for operations without sufficient permission.
var ErrForbidden = errors.New("forbidden")

// ErrInvalidViewPassword is returned when the view password length is invalid.
var ErrInvalidViewPassword = errors.New("invalid view password")

// ErrInvalidViewPasswordScope is returned when the protected scope for a view password is invalid.
var ErrInvalidViewPasswordScope = errors.New("invalid view password scope")

// ErrInvalidViewPasswordTextRanges is returned when protected text ranges are invalid.
var ErrInvalidViewPasswordTextRanges = errors.New("invalid view password text ranges")

// ErrMembershipWithPassword is returned when a membership-locked post cannot also get a view password.
var ErrMembershipWithPassword = errors.New("membership with view password")

// ErrHandleTaken is returned when a handle is already in use.
var ErrHandleTaken = errors.New("handle taken")

// ErrReservedHandle is returned for reserved or blocked handles.
var ErrReservedHandle = errors.New("reserved handle")

const (
	ViewPasswordScopeNone = 0
	ViewPasswordScopeText = 1 << iota
	ViewPasswordScopeMedia
	ViewPasswordScopeAll
)

const viewPasswordScopeMask = ViewPasswordScopeText | ViewPasswordScopeMedia | ViewPasswordScopeAll

type ViewPasswordTextRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func EffectiveViewPasswordScope(hasPassword bool, scope int) int {
	if !hasPassword {
		return ViewPasswordScopeNone
	}
	scope &= viewPasswordScopeMask
	if scope == ViewPasswordScopeNone {
		return ViewPasswordScopeAll
	}
	if scope&ViewPasswordScopeAll != 0 {
		return ViewPasswordScopeAll
	}
	return scope
}

func ScopeProtectsText(scope int) bool {
	scope = EffectiveViewPasswordScope(scope != ViewPasswordScopeNone, scope)
	return scope&ViewPasswordScopeAll != 0 || scope&ViewPasswordScopeText != 0
}

func ScopeProtectsMedia(scope int) bool {
	scope = EffectiveViewPasswordScope(scope != ViewPasswordScopeNone, scope)
	return scope&ViewPasswordScopeAll != 0 || scope&ViewPasswordScopeMedia != 0
}

func NormalizeViewPasswordTextRanges(caption string, ranges []ViewPasswordTextRange) ([]ViewPasswordTextRange, error) {
	runes := []rune(caption)
	max := len(runes)
	if len(ranges) == 0 {
		return nil, nil
	}
	out := make([]ViewPasswordTextRange, 0, len(ranges))
	for _, rg := range ranges {
		if rg.Start < 0 || rg.End < 0 || rg.Start >= rg.End || rg.End > max {
			return nil, ErrInvalidViewPasswordTextRanges
		}
		out = append(out, ViewPasswordTextRange{Start: rg.Start, End: rg.End})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].End < out[j].End
		}
		return out[i].Start < out[j].Start
	})
	merged := out[:0]
	for _, rg := range out {
		if len(merged) == 0 {
			merged = append(merged, rg)
			continue
		}
		last := &merged[len(merged)-1]
		if rg.Start <= last.End {
			if rg.End > last.End {
				last.End = rg.End
			}
			continue
		}
		merged = append(merged, rg)
	}
	return merged, nil
}

func NormalizeViewPasswordProtection(caption string, scope int, ranges []ViewPasswordTextRange) (int, []ViewPasswordTextRange, error) {
	scope &= viewPasswordScopeMask
	if scope == ViewPasswordScopeNone {
		return ViewPasswordScopeNone, nil, ErrInvalidViewPasswordScope
	}
	if scope&ViewPasswordScopeAll != 0 {
		return ViewPasswordScopeAll, nil, nil
	}
	if scope&ViewPasswordScopeText == 0 {
		return scope, nil, nil
	}
	norm, err := NormalizeViewPasswordTextRanges(caption, ranges)
	if err != nil {
		return ViewPasswordScopeNone, nil, err
	}
	if len(norm) == 0 {
		return ViewPasswordScopeNone, nil, ErrInvalidViewPasswordTextRanges
	}
	return scope, norm, nil
}

func MarshalViewPasswordTextRanges(ranges []ViewPasswordTextRange) string {
	if len(ranges) == 0 {
		return "[]"
	}
	b, err := json.Marshal(ranges)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func ParseViewPasswordTextRanges(raw string) ([]ViewPasswordTextRange, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []ViewPasswordTextRange
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, ErrInvalidViewPasswordTextRanges
	}
	return out, nil
}

func decodeStoredViewPasswordProtection(hasPassword bool, caption string, storedScope int, rawRanges string) (int, []ViewPasswordTextRange, error) {
	scope := EffectiveViewPasswordScope(hasPassword, storedScope)
	if scope == ViewPasswordScopeNone {
		return ViewPasswordScopeNone, nil, nil
	}
	ranges, err := ParseViewPasswordTextRanges(rawRanges)
	if err != nil {
		return ViewPasswordScopeNone, nil, err
	}
	if scope == ViewPasswordScopeAll {
		return ViewPasswordScopeAll, nil, nil
	}
	norm, err := NormalizeViewPasswordTextRanges(caption, ranges)
	if err != nil {
		return ViewPasswordScopeNone, nil, err
	}
	if scope&ViewPasswordScopeText != 0 && len(norm) == 0 {
		return ViewPasswordScopeAll, nil, nil
	}
	if scope&ViewPasswordScopeText == 0 {
		norm = nil
	}
	return scope, norm, nil
}

type User struct {
	ID                   uuid.UUID
	Email                string
	PasswordHash         string
	Handle               string
	DisplayName          string
	Bio                  string
	Badges               []string
	SuspendedAt          *time.Time
	DMCallTimeoutSeconds int
	DMCallEnabled        bool
	DMCallScope          string
	DMCallAllowedUserIDs []uuid.UUID
	DMInviteAutoAccept   bool
	AvatarObjectKey      *string
	HeaderObjectKey      *string
	TOTPSecret           *string
	TOTPEnabled          bool
}

// PublicProfile contains public user data without password fields.
type PublicProfile struct {
	ID                  uuid.UUID
	Email               string
	Handle              string
	DisplayName         string
	Bio                 string
	Badges              []string
	ProfileExternalURLs []string
	AvatarObjectKey     *string
	HeaderObjectKey     *string
}

// FollowListUser represents a user row for follower and following lists.
// followed_by_me and follows_you are meaningful only when a viewer ID is provided.
type FollowListUser struct {
	ID              uuid.UUID
	Email           string
	Handle          string
	DisplayName     string
	Bio             string
	Badges          []string
	AvatarObjectKey *string
	FollowedByMe    bool
	FollowsYou      bool
}

// SanitizeHandle derives a URL-safe handle from the email local part using lowercase [a-z0-9_] and a 30-character limit.
func SanitizeHandle(email string) string {
	at := strings.LastIndex(email, "@")
	local := ""
	if at >= 0 {
		local = strings.ToLower(strings.TrimSpace(email[:at]))
	} else {
		local = strings.ToLower(strings.TrimSpace(email))
	}
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		s = "user"
	}
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

// NormalizeHandle trims, lowercases, and validates a user handle as 1 to 30 characters of [a-z0-9_].
func NormalizeHandle(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", fmt.Errorf("empty handle")
	}
	if len(s) > 30 {
		return "", fmt.Errorf("handle too long")
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return "", fmt.Errorf("invalid handle")
		}
	}
	return s, nil
}

var reservedHandles = map[string]struct{}{
	"admin": {}, "administrator": {}, "api": {}, "auth": {}, "feed": {}, "feeds": {}, "login": {}, "logout": {},
	"register": {}, "signup": {}, "settings": {}, "security": {}, "search": {}, "notifications": {}, "bookmarks": {},
	"notes": {}, "note": {}, "posts": {}, "post": {}, "live": {}, "oauth": {}, "legal": {}, "privacy": {},
	"terms": {}, "about": {}, "help": {}, "support": {}, "root": {}, "system": {}, "glipz": {}, "me": {},
	"webmaster": {}, "owner": {}, "null": {}, "undefined": {}, "index": {},
	"staff": {}, "moderator": {}, "mod": {}, "administrator_jp": {}, "admin_jp": {}, "support_jp": {},
	"help_jp": {}, "official": {}, "team": {}, "dev": {}, "developer": {}, "test": {},
}

func IsReservedHandle(handle string) bool {
	_, ok := reservedHandles[strings.TrimSpace(strings.ToLower(handle))]
	return ok
}

// RandomSuffix returns a short random suffix for handle collisions.
func RandomSuffix() string {
	return randomHandleSuffix()
}

func randomHandleSuffix() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "x"
	}
	return hex.EncodeToString(buf)
}

const (
	PostVisibilityPublic    = "public"
	PostVisibilityLoggedIn  = "logged_in"
	PostVisibilityFollowers = "followers"
	PostVisibilityPrivate   = "private"
)

func normalizePostVisibility(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case PostVisibilityLoggedIn:
		return PostVisibilityLoggedIn
	case PostVisibilityFollowers:
		return PostVisibilityFollowers
	case PostVisibilityPrivate:
		return PostVisibilityPrivate
	default:
		return PostVisibilityPublic
	}
}

func postVisibilityExpr(alias string) string {
	return fmt.Sprintf("COALESCE(NULLIF(btrim(%s.visibility), ''), 'public')", alias)
}

func postReadableByViewerSQL(alias, viewerParam string) string {
	vis := postVisibilityExpr(alias)
	loggedInViewer := fmt.Sprintf("%s <> '00000000-0000-0000-0000-000000000000'::uuid", viewerParam)
	return fmt.Sprintf(`(
		%s.user_id = %s
		OR %s = 'public'
		OR (%s = 'logged_in' AND %s)
		OR (
			%s = 'followers'
			AND EXISTS (
				SELECT 1 FROM user_follows f
				WHERE f.follower_id = %s AND f.followee_id = %s.user_id
			)
		)
	)`, alias, viewerParam, vis, vis, loggedInViewer, vis, viewerParam, alias)
}

type Post struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Caption    string
	MediaType  string
	ObjectKeys []string
	Visibility string
}

type Pool struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Pool {
	return &Pool{db: db}
}

func (p *Pool) CreateUser(ctx context.Context, email, passwordHash, handle string) (uuid.UUID, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return uuid.Nil, fmt.Errorf("empty handle")
	}
	var id uuid.UUID
	err := p.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, handle) VALUES ($1, $2, $3) RETURNING id`,
		email, passwordHash, handle,
	).Scan(&id)
	return id, err
}

func ptrText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := strings.TrimSpace(t.String)
	if s == "" {
		return nil
	}
	return &s
}

func ptrTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tm := t.Time.UTC()
	return &tm
}

func (p *Pool) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	var totp pgtype.Text
	var totpEn pgtype.Bool
	var av pgtype.Text
	var hdr pgtype.Text
	var suspendedAt pgtype.Timestamptz
	var allowedUserIDs []uuid.UUID
	var badges []string
	err := p.db.QueryRow(ctx,
		`SELECT id, email, password_hash, handle, display_name, bio,
			suspended_at,
			COALESCE(badges, '{}'::text[]),
			COALESCE(dm_call_timeout_seconds, 30),
			COALESCE(dm_call_enabled, false),
			COALESCE(dm_call_scope, 'none'),
			COALESCE(dm_call_allowed_user_ids, '{}'::uuid[]),
			COALESCE(dm_invite_auto_accept, false),
			avatar_object_key, header_object_key, totp_secret, totp_enabled
		FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Handle, &u.DisplayName, &u.Bio, &suspendedAt, &badges,
		&u.DMCallTimeoutSeconds, &u.DMCallEnabled, &u.DMCallScope, &allowedUserIDs,
		&u.DMInviteAutoAccept,
		&av, &hdr, &totp, &totpEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if totp.Valid {
		s := totp.String
		u.TOTPSecret = &s
	}
	if totpEn.Valid {
		u.TOTPEnabled = totpEn.Bool
	}
	u.SuspendedAt = ptrTimestamptz(suspendedAt)
	u.Badges = NormalizeUserBadges(badges)
	if av.Valid {
		s := av.String
		u.AvatarObjectKey = &s
	}
	if hdr.Valid {
		s := hdr.String
		u.HeaderObjectKey = &s
	}
	u.DMCallAllowedUserIDs = allowedUserIDs
	return u, nil
}

func (p *Pool) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	var totp pgtype.Text
	var totpEn pgtype.Bool
	var av pgtype.Text
	var hdr pgtype.Text
	var suspendedAt pgtype.Timestamptz
	var allowedUserIDs []uuid.UUID
	var badges []string
	err := p.db.QueryRow(ctx,
		`SELECT id, email, password_hash, handle, display_name, bio,
			suspended_at,
			COALESCE(badges, '{}'::text[]),
			COALESCE(dm_call_timeout_seconds, 30),
			COALESCE(dm_call_enabled, false),
			COALESCE(dm_call_scope, 'none'),
			COALESCE(dm_call_allowed_user_ids, '{}'::uuid[]),
			COALESCE(dm_invite_auto_accept, false),
			avatar_object_key, header_object_key, totp_secret, totp_enabled
		FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Handle, &u.DisplayName, &u.Bio, &suspendedAt, &badges,
		&u.DMCallTimeoutSeconds, &u.DMCallEnabled, &u.DMCallScope, &allowedUserIDs,
		&u.DMInviteAutoAccept,
		&av, &hdr, &totp, &totpEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if totp.Valid {
		s := totp.String
		u.TOTPSecret = &s
	}
	if totpEn.Valid {
		u.TOTPEnabled = totpEn.Bool
	}
	u.SuspendedAt = ptrTimestamptz(suspendedAt)
	u.Badges = NormalizeUserBadges(badges)
	if av.Valid {
		s := av.String
		u.AvatarObjectKey = &s
	}
	if hdr.Valid {
		s := hdr.String
		u.HeaderObjectKey = &s
	}
	u.DMCallAllowedUserIDs = allowedUserIDs
	return u, nil
}

func (p *Pool) PublicProfileByHandle(ctx context.Context, handle string) (PublicProfile, error) {
	handle = strings.TrimSpace(handle)
	var pfl PublicProfile
	var av pgtype.Text
	var hdr pgtype.Text
	var badges []string
	var urlRaw []byte
	err := p.db.QueryRow(ctx, `
		SELECT id, email, handle, display_name, bio, COALESCE(badges, '{}'::text[]), avatar_object_key, header_object_key,
			COALESCE(profile_external_urls, '[]'::jsonb)::text
		FROM users WHERE lower(handle) = lower($1)
	`, handle).Scan(&pfl.ID, &pfl.Email, &pfl.Handle, &pfl.DisplayName, &pfl.Bio, &badges, &av, &hdr, &urlRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicProfile{}, ErrNotFound
	}
	if err != nil {
		return PublicProfile{}, err
	}
	if av.Valid {
		s := av.String
		pfl.AvatarObjectKey = &s
	}
	if hdr.Valid {
		s := hdr.String
		pfl.HeaderObjectKey = &s
	}
	pfl.Badges = NormalizeUserBadges(badges)
	pfl.ProfileExternalURLs = unmarshalProfileExternalURLsJSON(urlRaw)
	return pfl, nil
}

func (p *Pool) UpdateUserProfile(ctx context.Context, userID uuid.UUID, bio, displayName, handle, avatarKey, headerKey string, profileExternalURLs []string, isBot, isAI bool) error {
	if len([]rune(bio)) > 500 {
		return fmt.Errorf("bio too long")
	}
	if len([]rune(displayName)) > 50 {
		return fmt.Errorf("display name too long")
	}
	urlsJSON, err := json.Marshal(profileExternalURLs)
	if err != nil {
		return err
	}
	current, err := p.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	nextBadges := ProfileManagedUserBadges(current.Badges, isBot, isAI)
	_, err = p.db.Exec(ctx, `
		UPDATE users SET bio = $2,
			display_name = $3,
			handle = $4,
			avatar_object_key = NULLIF(trim($5), ''),
			header_object_key = NULLIF(trim($6), ''),
			profile_external_urls = $7::jsonb,
			badges = $8
		WHERE id = $1
	`, userID, bio, displayName, handle, avatarKey, headerKey, urlsJSON, nextBadges)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return ErrHandleTaken
		}
		return err
	}
	return nil
}

func (p *Pool) SetDMCallTimeoutSeconds(ctx context.Context, userID uuid.UUID, seconds int) error {
	if seconds < 5 || seconds > 300 {
		return fmt.Errorf("dm call timeout out of range")
	}
	_, err := p.db.Exec(ctx, `
		UPDATE users SET dm_call_timeout_seconds = $2 WHERE id = $1
	`, userID, seconds)
	return err
}

func (p *Pool) SetDMInviteAutoAccept(ctx context.Context, userID uuid.UUID, v bool) error {
	_, err := p.db.Exec(ctx, `
		UPDATE users SET dm_invite_auto_accept = $2 WHERE id = $1
	`, userID, v)
	return err
}

func (p *Pool) SetDMCallPolicy(ctx context.Context, userID uuid.UUID, enabled bool, scope string, allowedUserIDs []uuid.UUID) error {
	scope = strings.TrimSpace(strings.ToLower(scope))
	switch scope {
	case "all", "followers", "specific_users":
	default:
		scope = "none"
	}
	if !enabled {
		scope = "none"
	}
	if scope != "specific_users" {
		allowedUserIDs = []uuid.UUID{}
	}
	_, err := p.db.Exec(ctx, `
		UPDATE users
		SET dm_call_enabled = $2,
			dm_call_scope = $3,
			dm_call_allowed_user_ids = $4,
			dm_call_allowed_group_ids = '{}'::uuid[]
		WHERE id = $1
	`, userID, enabled, scope, allowedUserIDs)
	return err
}

func (p *Pool) SetTOTPSecret(ctx context.Context, userID uuid.UUID, secret string, enabled bool) error {
	_, err := p.db.Exec(ctx,
		`UPDATE users SET totp_secret = $2, totp_enabled = $3 WHERE id = $1`,
		userID, secret, enabled,
	)
	return err
}

func (p *Pool) IsUserSuspended(ctx context.Context, userID uuid.UUID) (bool, error) {
	var suspended bool
	err := p.db.QueryRow(ctx, `SELECT suspended_at IS NOT NULL FROM users WHERE id = $1`, userID).Scan(&suspended)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	return suspended, nil
}

func (p *Pool) SuspendUser(ctx context.Context, userID uuid.UUID) error {
	ct, err := p.db.Exec(ctx, `UPDATE users SET suspended_at = COALESCE(suspended_at, NOW()) WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func normalizeModerationReportStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "resolved":
		return "resolved"
	case "dismissed":
		return "dismissed"
	case "spam":
		return "spam"
	default:
		return "open"
	}
}

func (p *Pool) InsertPostReport(ctx context.Context, reporterUserID, postID uuid.UUID, reason string) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO post_reports (reporter_user_id, post_id, reason, status, resolved_at, created_at)
		VALUES ($1, $2, $3, 'open', NULL, NOW())
		ON CONFLICT (reporter_user_id, post_id) DO UPDATE
		SET reason = EXCLUDED.reason,
			status = 'open',
			resolved_at = NULL,
			created_at = NOW()
	`, reporterUserID, postID, strings.TrimSpace(reason))
	return err
}

func (p *Pool) InsertFederatedIncomingPostReport(ctx context.Context, reporterUserID, incomingPostID uuid.UUID, reason string) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_incoming_post_reports (reporter_user_id, federation_incoming_post_id, reason, status, resolved_at, created_at)
		VALUES ($1, $2, $3, 'open', NULL, NOW())
		ON CONFLICT (reporter_user_id, federation_incoming_post_id) DO UPDATE
		SET reason = EXCLUDED.reason,
			status = 'open',
			resolved_at = NULL,
			created_at = NOW()
	`, reporterUserID, incomingPostID, strings.TrimSpace(reason))
	return err
}

type AdminPostReportRow struct {
	ID                    uuid.UUID
	CreatedAt             time.Time
	PostID                uuid.UUID
	PostCaption           string
	Reason                string
	Status                string
	ResolvedAt            *time.Time
	PostAuthorUserID      uuid.UUID
	PostAuthorHandle      string
	PostAuthorDisplayName string
	ReporterUserID        uuid.UUID
	ReporterHandle        string
	ReporterDisplayName   string
}

func (p *Pool) ListAdminPostReports(ctx context.Context, limit int) ([]AdminPostReportRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := p.db.Query(ctx, `
		SELECT
			pr.id,
			pr.created_at,
			p.id,
			p.caption,
			COALESCE(pr.reason, ''),
			COALESCE(pr.status, 'open'),
			pr.resolved_at,
			author.id,
			author.handle,
			COALESCE(NULLIF(btrim(author.display_name), ''), split_part(author.email, '@', 1)) AS author_display_name,
			reporter.id,
			reporter.handle,
			COALESCE(NULLIF(btrim(reporter.display_name), ''), split_part(reporter.email, '@', 1)) AS reporter_display_name
		FROM post_reports pr
		JOIN posts p ON p.id = pr.post_id
		JOIN users author ON author.id = p.user_id
		JOIN users reporter ON reporter.id = pr.reporter_user_id
		ORDER BY pr.created_at DESC, pr.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminPostReportRow
	for rows.Next() {
		var row AdminPostReportRow
		if err := rows.Scan(
			&row.ID,
			&row.CreatedAt,
			&row.PostID,
			&row.PostCaption,
			&row.Reason,
			&row.Status,
			&row.ResolvedAt,
			&row.PostAuthorUserID,
			&row.PostAuthorHandle,
			&row.PostAuthorDisplayName,
			&row.ReporterUserID,
			&row.ReporterHandle,
			&row.ReporterDisplayName,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type AdminFederatedIncomingPostReportRow struct {
	ID                  uuid.UUID
	CreatedAt           time.Time
	IncomingPostID      uuid.UUID
	ObjectIRI           string
	CaptionText         string
	Reason              string
	Status              string
	ResolvedAt          *time.Time
	ActorAcct           string
	ActorName           string
	ReporterUserID      uuid.UUID
	ReporterHandle      string
	ReporterDisplayName string
}

func (p *Pool) ListAdminFederatedIncomingPostReports(ctx context.Context, limit int) ([]AdminFederatedIncomingPostReportRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := p.db.Query(ctx, `
		SELECT
			fr.id,
			fr.created_at,
			fp.id,
			fp.object_iri,
			fp.caption_text,
			COALESCE(fr.reason, ''),
			COALESCE(fr.status, 'open'),
			fr.resolved_at,
			fp.actor_acct,
			fp.actor_name,
			reporter.id,
			reporter.handle,
			COALESCE(NULLIF(btrim(reporter.display_name), ''), split_part(reporter.email, '@', 1)) AS reporter_display_name
		FROM federation_incoming_post_reports fr
		JOIN federation_incoming_posts fp ON fp.id = fr.federation_incoming_post_id
		JOIN users reporter ON reporter.id = fr.reporter_user_id
		WHERE fp.deleted_at IS NULL
		ORDER BY fr.created_at DESC, fr.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminFederatedIncomingPostReportRow
	for rows.Next() {
		var row AdminFederatedIncomingPostReportRow
		if err := rows.Scan(
			&row.ID,
			&row.CreatedAt,
			&row.IncomingPostID,
			&row.ObjectIRI,
			&row.CaptionText,
			&row.Reason,
			&row.Status,
			&row.ResolvedAt,
			&row.ActorAcct,
			&row.ActorName,
			&row.ReporterUserID,
			&row.ReporterHandle,
			&row.ReporterDisplayName,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) UpdatePostReportStatus(ctx context.Context, reportID uuid.UUID, status string) error {
	status = normalizeModerationReportStatus(status)
	ct, err := p.db.Exec(ctx, `
		UPDATE post_reports
		SET status = $2,
			resolved_at = CASE WHEN $2 <> 'open' THEN NOW() ELSE NULL END
		WHERE id = $1
	`, reportID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) UpdateFederatedIncomingPostReportStatus(ctx context.Context, reportID uuid.UUID, status string) error {
	status = normalizeModerationReportStatus(status)
	ct, err := p.db.Exec(ctx, `
		UPDATE federation_incoming_post_reports
		SET status = $2,
			resolved_at = CASE WHEN $2 <> 'open' THEN NOW() ELSE NULL END
		WHERE id = $1
	`, reportID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) PostExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.QueryRow(ctx, `SELECT true FROM posts WHERE id = $1`, id).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (p *Pool) CreatePost(ctx context.Context, userID uuid.UUID, caption, mediaType string, objectKeys []string, replyTo *uuid.UUID, replyToRemoteObjectIRI string, isNSFW bool, visibility string, viewPasswordHash *string, viewPasswordScope int, viewPasswordTextRanges []ViewPasswordTextRange, visibleAt time.Time, pollIn *PollCreateInput, membershipProvider, membershipCreatorID, membershipTierID string) (uuid.UUID, error) {
	if objectKeys == nil {
		objectKeys = []string{}
	}
	n := len(objectKeys)
	switch mediaType {
	case "none":
		if n != 0 {
			return uuid.Nil, fmt.Errorf("objectKeys: none media wants 0 keys, got %d", n)
		}
	case "image":
		if n < 1 || n > 4 {
			return uuid.Nil, fmt.Errorf("objectKeys: image wants 1..4, got %d", n)
		}
	case "video":
		if n != 1 {
			return uuid.Nil, fmt.Errorf("objectKeys: video wants 1 key, got %d", n)
		}
	default:
		return uuid.Nil, fmt.Errorf("media_type: invalid %q", mediaType)
	}
	if replyTo != nil {
		ok, err := p.PostExists(ctx, *replyTo)
		if err != nil {
			return uuid.Nil, err
		}
		if !ok {
			return uuid.Nil, ErrNotFound
		}
	}
	if pollIn != nil {
		if len(pollIn.Labels) < 2 || len(pollIn.Labels) > 4 {
			return uuid.Nil, fmt.Errorf("poll: want 2..4 options")
		}
		if !pollIn.EndsAt.After(visibleAt) {
			return uuid.Nil, fmt.Errorf("poll: ends_at must be after visible_at")
		}
	}
	scope := ViewPasswordScopeNone
	rangesJSON := "[]"
	if viewPasswordHash != nil && strings.TrimSpace(*viewPasswordHash) != "" {
		var err error
		scope, viewPasswordTextRanges, err = NormalizeViewPasswordProtection(caption, viewPasswordScope, viewPasswordTextRanges)
		if err != nil {
			return uuid.Nil, err
		}
		rangesJSON = MarshalViewPasswordTextRanges(viewPasswordTextRanges)
	}

	replyToRemoteObjectIRI = strings.TrimSpace(replyToRemoteObjectIRI)
	visibility = normalizePostVisibility(visibility)
	feedDone := replyTo != nil || replyToRemoteObjectIRI != ""

	tx, err := p.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO posts (
			user_id, caption, media_type, object_keys, reply_to_id, reply_to_remote_object_iri,
			is_nsfw, visibility, view_password_hash, view_password_scope, view_password_text_ranges, visible_at, feed_broadcast_done, group_id,
			membership_provider, membership_creator_id, membership_tier_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, $13, NULL, $14, $15, $16)
		RETURNING id
	`, userID, caption, mediaType, objectKeys, replyTo, replyToRemoteObjectIRI, isNSFW, visibility, viewPasswordHash, scope, rangesJSON, visibleAt.UTC(), feedDone, membershipProvider, membershipCreatorID, membershipTierID).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}

	if pollIn != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO post_polls (post_id, ends_at) VALUES ($1, $2)`, id, pollIn.EndsAt.UTC()); err != nil {
			return uuid.Nil, err
		}
		for i, lab := range pollIn.Labels {
			if _, err := tx.Exec(ctx, `
				INSERT INTO post_poll_options (post_id, position, label) VALUES ($1, $2, $3)
			`, id, i, lab); err != nil {
				return uuid.Nil, err
			}
		}
	}
	if err := syncPostHashtags(ctx, tx, id, caption); err != nil {
		return uuid.Nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

type PostRow struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	Email                  string
	UserHandle             string
	DisplayName            string
	AvatarObjectKey        *string
	Caption                string
	MediaType              string
	ObjectKeys             []string
	IsNSFW                 bool
	Visibility             string
	HasViewPassword        bool
	ViewPasswordScope      int
	ViewPasswordTextRanges []ViewPasswordTextRange
	HasMembershipLock      bool
	MembershipProvider     string
	MembershipCreatorID    string
	MembershipTierID       string
	CreatedAt              time.Time
	VisibleAt              time.Time
	Poll                   *PostPoll
	Reactions              []PostReaction
	ReplyCount             int64
	LikeCount              int64
	RepostCount            int64
	LikedByMe              bool
	RepostedByMe           bool
	BookmarkedByMe         bool
}

// PostSensitive holds post data for unlock APIs, including hashed protection fields.
type PostSensitive struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	Caption                string
	MediaType              string
	ObjectKeys             []string
	IsNSFW                 bool
	ViewPasswordHash       *string
	ViewPasswordScope      int
	ViewPasswordTextRanges []ViewPasswordTextRange
	HasMembershipLock      bool
	MembershipProvider     string
	MembershipCreatorID    string
	MembershipTierID       string
}

func (p *Pool) PostSensitiveByID(ctx context.Context, postID uuid.UUID) (PostSensitive, error) {
	var row PostSensitive
	var hash pgtype.Text
	var scope int
	var textRanges string
	err := p.db.QueryRow(ctx, `
		SELECT id, user_id, caption, media_type, object_keys, is_nsfw, view_password_hash,
			COALESCE(view_password_scope, 0),
			COALESCE(view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(membership_provider, ''), COALESCE(membership_creator_id, ''), COALESCE(membership_tier_id, '')
		FROM posts WHERE id = $1
	`, postID).Scan(&row.ID, &row.UserID, &row.Caption, &row.MediaType, &row.ObjectKeys, &row.IsNSFW, &hash, &scope, &textRanges,
		&row.HasMembershipLock, &row.MembershipProvider, &row.MembershipCreatorID, &row.MembershipTierID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PostSensitive{}, ErrNotFound
	}
	if err != nil {
		return PostSensitive{}, err
	}
	if hash.Valid && strings.TrimSpace(hash.String) != "" {
		s := strings.TrimSpace(hash.String)
		row.ViewPasswordHash = &s
	}
	row.ViewPasswordScope, row.ViewPasswordTextRanges, err = decodeStoredViewPasswordProtection(row.ViewPasswordHash != nil, row.Caption, scope, textRanges)
	if err != nil {
		return PostSensitive{}, err
	}
	return row, nil
}

// PostRowForViewer returns a single visible post projected the same way as the feed.
func (p *Pool) PostRowForViewer(ctx context.Context, viewerID, postID uuid.UUID) (PostRow, error) {
	var r PostRow
	var av pgtype.Text
	var scope int
	var textRanges string
	err := p.db.QueryRow(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
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
		WHERE p.id = $2
		  AND p.visible_at <= NOW()
		  AND `+postReadableByViewerSQL("p", "$1")+`
	`, viewerID, postID).Scan(
		&r.ID, &r.UserID, &r.Email, &r.UserHandle, &r.DisplayName, &av, &r.Caption, &r.MediaType, &r.ObjectKeys,
		&r.IsNSFW, &r.Visibility, &r.HasViewPassword, &scope, &textRanges,
		&r.HasMembershipLock, &r.MembershipProvider, &r.MembershipCreatorID, &r.MembershipTierID,
		&r.CreatedAt, &r.VisibleAt,
		&r.ReplyCount, &r.LikeCount, &r.RepostCount, &r.LikedByMe, &r.RepostedByMe, &r.BookmarkedByMe,
	)
	if av.Valid && strings.TrimSpace(av.String) != "" {
		s := strings.TrimSpace(av.String)
		r.AvatarObjectKey = &s
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return PostRow{}, ErrNotFound
	}
	if err != nil {
		return PostRow{}, err
	}
	r.ViewPasswordScope, r.ViewPasswordTextRanges, err = decodeStoredViewPasswordProtection(r.HasViewPassword, r.Caption, scope, textRanges)
	if err != nil {
		return PostRow{}, err
	}
	return r, nil
}

// PostRowWithReplyTo represents a reply row within a root-post thread, including the parent post ID.
type PostRowWithReplyTo struct {
	PostRow
	ReplyToID              *uuid.UUID
	ReplyToRemoteObjectIRI string
}

// ListThreadDescendants returns visible replies that belong to a root-post thread.
// Results are chronological, using ID as a tie-breaker, and never include the root post itself.
func (p *Pool) ListThreadDescendants(ctx context.Context, viewerID, rootPostID uuid.UUID) ([]PostRowWithReplyTo, error) {
	rows, err := p.db.Query(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, reply_to_id FROM posts WHERE id = $2
			UNION ALL
			SELECT child.id, child.reply_to_id FROM posts child
			INNER JOIN chain c ON child.reply_to_id = c.id
		),
		root_ok AS (
			SELECT 1 FROM posts r
			WHERE r.id = $2
			  AND r.reply_to_id IS NULL
			  AND COALESCE(btrim(r.reply_to_remote_object_iri), '') = ''
			  AND r.visible_at <= NOW()
			  AND r.group_id IS NULL
			  AND `+postReadableByViewerSQL("r", "$1")+`
		)
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
			p.created_at, p.visible_at,
			(COALESCE(rpl.reply_count, 0) + COALESCE(frpl.reply_count, 0))::bigint,
			COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
			COALESCE(rp.repost_count, 0)::bigint,
			EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
			EXISTS (SELECT 1 FROM post_reposts r WHERE r.post_id = p.id AND r.user_id = $1),
			EXISTS (SELECT 1 FROM post_bookmarks b WHERE b.post_id = p.id AND b.user_id = $1),
			p.reply_to_id
		FROM posts p
		INNER JOIN chain d ON p.id = d.id
		INNER JOIN root_ok ON TRUE
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
		WHERE p.id <> $2
		AND p.visible_at <= NOW()
		AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY p.visible_at ASC, p.id ASC
	`, viewerID, rootPostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostRowWithReplyTo
	for rows.Next() {
		var r PostRow
		var av pgtype.Text
		var replyTo pgtype.UUID
		var scope int
		var textRanges string
		var hasMembershipLock bool
		var membershipProvider, membershipCreatorID, membershipTierID string
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Email, &r.UserHandle, &r.DisplayName, &av, &r.Caption, &r.MediaType, &r.ObjectKeys,
			&r.IsNSFW, &r.Visibility, &r.HasViewPassword, &scope, &textRanges,
			&hasMembershipLock, &membershipProvider, &membershipCreatorID, &membershipTierID,
			&r.CreatedAt, &r.VisibleAt,
			&r.ReplyCount, &r.LikeCount, &r.RepostCount, &r.LikedByMe, &r.RepostedByMe, &r.BookmarkedByMe,
			&replyTo,
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
		r.HasMembershipLock = hasMembershipLock
		r.MembershipProvider = strings.TrimSpace(membershipProvider)
		r.MembershipCreatorID = strings.TrimSpace(membershipCreatorID)
		r.MembershipTierID = strings.TrimSpace(membershipTierID)
		var replyToID *uuid.UUID
		if replyTo.Valid {
			x := uuid.UUID(replyTo.Bytes)
			replyToID = &x
		}
		out = append(out, PostRowWithReplyTo{PostRow: r, ReplyToID: replyToID})
	}
	return out, rows.Err()
}

// PostMembershipUpdate when non-nil updates membership lock columns; empty strings clear the lock.
type PostMembershipUpdate struct {
	Provider, CreatorID, TierID string
}

// UpdatePost lets only the post owner update caption, NSFW state, visibility, view-password settings, and optional membership lock.
func (p *Pool) UpdatePost(ctx context.Context, ownerID, postID uuid.UUID, caption string, isNSFW bool, visibility string, clearViewPassword bool, newPassword *string, viewPasswordScope int, viewPasswordTextRanges []ViewPasswordTextRange, memUpdate *PostMembershipUpdate) error {
	row, err := p.PostSensitiveByID(ctx, postID)
	if err != nil {
		return err
	}
	if row.UserID != ownerID {
		return ErrForbidden
	}
	if row.HasMembershipLock && newPassword != nil && strings.TrimSpace(*newPassword) != "" {
		return ErrMembershipWithPassword
	}
	var hash *string
	scope := row.ViewPasswordScope
	ranges := row.ViewPasswordTextRanges
	switch {
	case clearViewPassword:
		hash = nil
		scope = ViewPasswordScopeNone
		ranges = nil
	case newPassword != nil:
		pw := strings.TrimSpace(*newPassword)
		if pw == "" {
			hash = row.ViewPasswordHash
			break
		}
		if len(pw) < 4 || len(pw) > 72 {
			return ErrInvalidViewPassword
		}
		b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		s := string(b)
		hash = &s
	default:
		hash = row.ViewPasswordHash
	}
	if hash != nil && strings.TrimSpace(*hash) != "" {
		scope, ranges, err = NormalizeViewPasswordProtection(caption, viewPasswordScope, viewPasswordTextRanges)
		if err != nil {
			return err
		}
	} else {
		scope = ViewPasswordScopeNone
		ranges = nil
	}
	visibility = normalizePostVisibility(visibility)
	memProv, memCre, memTier := "", "", ""
	if memUpdate != nil {
		memProv = memUpdate.Provider
		memCre = memUpdate.CreatorID
		memTier = memUpdate.TierID
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if memUpdate == nil {
		if _, err := tx.Exec(ctx, `
			UPDATE posts SET caption = $1, is_nsfw = $2, visibility = $3, view_password_hash = $4, view_password_scope = $5, view_password_text_ranges = $6::jsonb
			WHERE id = $7 AND user_id = $8
		`, caption, isNSFW, visibility, hash, scope, MarshalViewPasswordTextRanges(ranges), postID, ownerID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE posts SET caption = $1, is_nsfw = $2, visibility = $3, view_password_hash = $4, view_password_scope = $5, view_password_text_ranges = $6::jsonb,
			membership_provider = $9, membership_creator_id = $10, membership_tier_id = $11
			WHERE id = $7 AND user_id = $8
		`, caption, isNSFW, visibility, hash, scope, MarshalViewPasswordTextRanges(ranges), postID, ownerID, memProv, memCre, memTier); err != nil {
			return err
		}
	}
	if err := syncPostHashtags(ctx, tx, postID, caption); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListFollowerIDs returns the IDs of users following followeeID.
func (p *Pool) ListFollowerIDs(ctx context.Context, followeeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := p.db.Query(ctx, `SELECT follower_id FROM user_follows WHERE followee_id = $1`, followeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PostFeedMeta returns the author ID and whether the post is a top-level timeline entry.
func (p *Pool) PostFeedMeta(ctx context.Context, postID uuid.UUID) (authorID uuid.UUID, isRoot bool, err error) {
	var reply pgtype.UUID
	var remoteReply string
	err = p.db.QueryRow(ctx, `SELECT user_id, reply_to_id, COALESCE(reply_to_remote_object_iri, '') FROM posts WHERE id = $1`, postID).Scan(&authorID, &reply, &remoteReply)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	isRoot = !reply.Valid && strings.TrimSpace(remoteReply) == ""
	return authorID, isRoot, nil
}

func (p *Pool) DeletePostByActor(ctx context.Context, actorID, postID uuid.UUID, allowForeign bool) error {
	var owner uuid.UUID
	err := p.db.QueryRow(ctx, `SELECT user_id FROM posts WHERE id = $1`, postID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner != actorID && !allowForeign {
		return ErrForbidden
	}
	_, err = p.db.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
	return err
}

func (p *Pool) ListFeed(ctx context.Context, viewerID uuid.UUID, limit int) ([]PostRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
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
		WHERE p.reply_to_id IS NULL
		  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
		  AND p.visible_at <= NOW()
		  AND p.group_id IS NULL
		  AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY p.visible_at DESC, p.id DESC
		LIMIT $2
	`, viewerID, limit)
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
		var hasMembershipLock bool
		var membershipProvider, membershipCreatorID, membershipTierID string
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Email, &r.UserHandle, &r.DisplayName, &av, &r.Caption, &r.MediaType, &r.ObjectKeys,
			&r.IsNSFW, &r.Visibility, &r.HasViewPassword, &scope, &textRanges,
			&hasMembershipLock, &membershipProvider, &membershipCreatorID, &membershipTierID,
			&r.CreatedAt, &r.VisibleAt,
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
		r.HasMembershipLock = hasMembershipLock
		r.MembershipProvider = strings.TrimSpace(membershipProvider)
		r.MembershipCreatorID = strings.TrimSpace(membershipCreatorID)
		r.MembershipTierID = strings.TrimSpace(membershipTierID)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) ListUserPosts(ctx context.Context, viewerID, authorID uuid.UUID, limit int) ([]PostRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
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
		WHERE p.user_id = $2
		  AND p.reply_to_id IS NULL
		  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
		  AND p.visible_at <= NOW()
		  AND p.group_id IS NULL
		  AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY p.visible_at DESC, p.id DESC
		LIMIT $3
	`, viewerID, authorID, limit)
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
		var hasMembershipLock bool
		var membershipProvider, membershipCreatorID, membershipTierID string
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Email, &r.UserHandle, &r.DisplayName, &av, &r.Caption, &r.MediaType, &r.ObjectKeys,
			&r.IsNSFW, &r.Visibility, &r.HasViewPassword, &scope, &textRanges,
			&hasMembershipLock, &membershipProvider, &membershipCreatorID, &membershipTierID,
			&r.CreatedAt, &r.VisibleAt,
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
		r.HasMembershipLock = hasMembershipLock
		r.MembershipProvider = strings.TrimSpace(membershipProvider)
		r.MembershipCreatorID = strings.TrimSpace(membershipCreatorID)
		r.MembershipTierID = strings.TrimSpace(membershipTierID)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListUserReplyPosts returns a user's replies in reverse chronological order.
func (p *Pool) ListUserReplyPosts(ctx context.Context, viewerID, authorID uuid.UUID, limit int) ([]PostRowWithReplyTo, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
			p.created_at, p.visible_at,
			COALESCE(rpl.reply_count, 0)::bigint,
			COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
			COALESCE(rp.repost_count, 0)::bigint,
			EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
			EXISTS (SELECT 1 FROM post_reposts r WHERE r.post_id = p.id AND r.user_id = $1),
			EXISTS (SELECT 1 FROM post_bookmarks b WHERE b.post_id = p.id AND b.user_id = $1),
			p.reply_to_id,
			COALESCE(p.reply_to_remote_object_iri, '')
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
		  AND (p.reply_to_id IS NOT NULL OR COALESCE(btrim(p.reply_to_remote_object_iri), '') <> '')
		  AND p.visible_at <= NOW()
		  AND p.group_id IS NULL
		  AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY p.visible_at DESC, p.id DESC
		LIMIT $3
	`, viewerID, authorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostRowWithReplyTo
	for rows.Next() {
		var r PostRow
		var av pgtype.Text
		var replyTo pgtype.UUID
		var replyToRemote string
		var scope int
		var textRanges string
		var hasMembershipLock bool
		var membershipProvider, membershipCreatorID, membershipTierID string
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Email, &r.UserHandle, &r.DisplayName, &av, &r.Caption, &r.MediaType, &r.ObjectKeys,
			&r.IsNSFW, &r.Visibility, &r.HasViewPassword, &scope, &textRanges,
			&hasMembershipLock, &membershipProvider, &membershipCreatorID, &membershipTierID,
			&r.CreatedAt, &r.VisibleAt,
			&r.ReplyCount, &r.LikeCount, &r.RepostCount, &r.LikedByMe, &r.RepostedByMe, &r.BookmarkedByMe,
			&replyTo, &replyToRemote,
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
		r.HasMembershipLock = hasMembershipLock
		r.MembershipProvider = strings.TrimSpace(membershipProvider)
		r.MembershipCreatorID = strings.TrimSpace(membershipCreatorID)
		r.MembershipTierID = strings.TrimSpace(membershipTierID)
		var replyToID *uuid.UUID
		if replyTo.Valid {
			x := uuid.UUID(replyTo.Bytes)
			replyToID = &x
		}
		out = append(out, PostRowWithReplyTo{
			PostRow:                r,
			ReplyToID:              replyToID,
			ReplyToRemoteObjectIRI: strings.TrimSpace(replyToRemote),
		})
	}
	return out, rows.Err()
}

func (p *Pool) ListPostsByReplyToRemoteObjectIRI(ctx context.Context, viewerID uuid.UUID, objectIRI string, limit int) ([]PostRow, error) {
	objectIRI = strings.TrimSpace(objectIRI)
	if objectIRI == "" {
		return []PostRow{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
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
		WHERE COALESCE(btrim(p.reply_to_remote_object_iri), '') = $2
		  AND p.visible_at <= NOW()
		  AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY p.visible_at ASC, p.id ASC
		LIMIT $3
	`, viewerID, objectIRI, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostRows(rows)
}

func (p *Pool) ToggleLike(ctx context.Context, userID, postID uuid.UUID) (liked bool, count int64, err error) {
	ok, err := p.CanViewerReadPost(ctx, userID, postID)
	if err != nil {
		return false, 0, err
	}
	if !ok {
		return false, 0, ErrNotFound
	}
	tag, err := p.db.Exec(ctx, `DELETE FROM post_likes WHERE user_id = $1 AND post_id = $2`, userID, postID)
	if err != nil {
		return false, 0, err
	}
	if tag.RowsAffected() == 0 {
		_, err = p.db.Exec(ctx, `INSERT INTO post_likes (user_id, post_id) VALUES ($1, $2)`, userID, postID)
		if err != nil {
			return false, 0, err
		}
		liked = true
	}
	err = p.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM post_likes WHERE post_id = $1) +
			(SELECT COUNT(*)::bigint FROM post_remote_likes WHERE post_id = $1)
	`, postID).Scan(&count)
	if err != nil {
		return false, 0, err
	}
	return liked, count, nil
}

func (p *Pool) ToggleRepost(ctx context.Context, userID, postID uuid.UUID, comment *string) (reposted bool, count int64, err error) {
	ok, err := p.CanViewerReadPost(ctx, userID, postID)
	if err != nil {
		return false, 0, err
	}
	if !ok {
		return false, 0, ErrNotFound
	}
	visibility, err := p.PostVisibilityByID(ctx, postID)
	if err != nil {
		return false, 0, err
	}
	if visibility != PostVisibilityPublic {
		return false, 0, ErrForbidden
	}
	tag, err := p.db.Exec(ctx, `DELETE FROM post_reposts WHERE user_id = $1 AND post_id = $2`, userID, postID)
	if err != nil {
		return false, 0, err
	}
	if tag.RowsAffected() == 0 {
		var commentArg any
		if comment != nil {
			if t := strings.TrimSpace(*comment); t != "" {
				commentArg = t
			}
		}
		_, err = p.db.Exec(ctx, `INSERT INTO post_reposts (user_id, post_id, comment_text) VALUES ($1, $2, $3)`, userID, postID, commentArg)
		if err != nil {
			return false, 0, err
		}
		reposted = true
	}
	err = p.db.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM post_reposts WHERE post_id = $1`, postID).Scan(&count)
	if err != nil {
		return false, 0, err
	}
	return reposted, count, nil
}

func (p *Pool) ToggleBookmark(ctx context.Context, userID, postID uuid.UUID) (bookmarked bool, err error) {
	ok, err := p.CanViewerReadPost(ctx, userID, postID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	tag, err := p.db.Exec(ctx, `DELETE FROM post_bookmarks WHERE user_id = $1 AND post_id = $2`, userID, postID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := p.db.Exec(ctx, `INSERT INTO post_bookmarks (user_id, post_id) VALUES ($1, $2)`, userID, postID); err != nil {
			return false, err
		}
		bookmarked = true
	}
	return bookmarked, nil
}

type BookmarkedPostRow struct {
	PostRow
	BookmarkedAt time.Time
}

func (p *Pool) ListBookmarkedPosts(ctx context.Context, viewerID uuid.UUID, limit int) ([]BookmarkedPostRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
			p.created_at, p.visible_at,
			(COALESCE(rpl.reply_count, 0) + COALESCE(frpl.reply_count, 0))::bigint,
			COALESCE(lk.like_count, 0)::bigint + COALESCE(rlk.like_count, 0)::bigint,
			COALESCE(rp.repost_count, 0)::bigint,
			EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
			EXISTS (SELECT 1 FROM post_reposts r WHERE r.post_id = p.id AND r.user_id = $1),
			TRUE,
			pb.created_at
		FROM post_bookmarks pb
		JOIN posts p ON p.id = pb.post_id
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
		WHERE pb.user_id = $1
		  AND p.visible_at <= NOW()
		  AND p.group_id IS NULL
		  AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY pb.created_at DESC, p.id DESC
		LIMIT $2
	`, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookmarkedPostRow
	for rows.Next() {
		var row BookmarkedPostRow
		var av pgtype.Text
		var scope int
		var textRanges string
		var hasMembershipLock bool
		var membershipProvider, membershipCreatorID, membershipTierID string
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.Email, &row.UserHandle, &row.DisplayName, &av, &row.Caption, &row.MediaType, &row.ObjectKeys,
			&row.IsNSFW, &row.Visibility, &row.HasViewPassword, &scope, &textRanges,
			&hasMembershipLock, &membershipProvider, &membershipCreatorID, &membershipTierID,
			&row.CreatedAt, &row.VisibleAt,
			&row.ReplyCount, &row.LikeCount, &row.RepostCount, &row.LikedByMe, &row.RepostedByMe, &row.BookmarkedByMe,
			&row.BookmarkedAt,
		); err != nil {
			return nil, err
		}
		if av.Valid && strings.TrimSpace(av.String) != "" {
			s := strings.TrimSpace(av.String)
			row.AvatarObjectKey = &s
		}
		var err error
		row.ViewPasswordScope, row.ViewPasswordTextRanges, err = decodeStoredViewPasswordProtection(row.HasViewPassword, row.Caption, scope, textRanges)
		if err != nil {
			return nil, err
		}
		row.HasMembershipLock = hasMembershipLock
		row.MembershipProvider = strings.TrimSpace(membershipProvider)
		row.MembershipCreatorID = strings.TrimSpace(membershipCreatorID)
		row.MembershipTierID = strings.TrimSpace(membershipTierID)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) FollowCounts(ctx context.Context, userID uuid.UUID) (followers, following int64, err error) {
	err = p.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM user_follows WHERE followee_id = $1),
			(SELECT COUNT(*)::bigint FROM user_follows WHERE follower_id = $1)
	`, userID).Scan(&followers, &following)
	return followers, following, err
}

func (p *Pool) IsFollowing(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.QueryRow(ctx,
		`SELECT true FROM user_follows WHERE follower_id = $1 AND followee_id = $2`,
		followerID, followeeID,
	).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (p *Pool) CanViewerReadPost(ctx context.Context, viewerID, postID uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.QueryRow(ctx, `
		SELECT true
		FROM posts p
		WHERE p.id = $2
		  AND p.visible_at <= NOW()
		  AND `+postReadableByViewerSQL("p", "$1")+`
	`, viewerID, postID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (p *Pool) PostVisibilityByID(ctx context.Context, postID uuid.UUID) (string, error) {
	var visibility string
	err := p.db.QueryRow(ctx, `
		SELECT `+postVisibilityExpr("p")+`
		FROM posts p
		WHERE p.id = $1
	`, postID).Scan(&visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return normalizePostVisibility(visibility), nil
}

func (p *Pool) ToggleFollow(ctx context.Context, followerID, followeeID uuid.UUID) (following bool, followerCount int64, err error) {
	if followerID == followeeID {
		return false, 0, ErrCannotFollowSelf
	}
	var exists bool
	err = p.db.QueryRow(ctx, `SELECT true FROM users WHERE id = $1`, followeeID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, ErrNotFound
	}
	if err != nil {
		return false, 0, err
	}
	tag, err := p.db.Exec(ctx, `DELETE FROM user_follows WHERE follower_id = $1 AND followee_id = $2`, followerID, followeeID)
	if err != nil {
		return false, 0, err
	}
	if tag.RowsAffected() == 0 {
		_, err = p.db.Exec(ctx, `INSERT INTO user_follows (follower_id, followee_id) VALUES ($1, $2)`, followerID, followeeID)
		if err != nil {
			return false, 0, err
		}
		following = true
	}
	err = p.db.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM user_follows WHERE followee_id = $1`, followeeID).Scan(&followerCount)
	if err != nil {
		return false, 0, err
	}
	return following, followerCount, nil
}

func clampListLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 50
	}
	return limit
}

func clampListOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > 100000 {
		return 100000
	}
	return offset
}

// ListUserFollowers returns followers of userID, that is, user_follows.follower_id.
// hasNext is true only when another page is available.
func (p *Pool) ListUserFollowers(ctx context.Context, viewerID, userID uuid.UUID, limit, offset int) (items []FollowListUser, nextOffset int, hasNext bool, err error) {
	limit = clampListLimit(limit)
	offset = clampListOffset(offset)
	fetch := limit + 1

	rows, err := p.db.Query(ctx, `
		SELECT u.id, u.email, u.handle, u.display_name, COALESCE(u.bio, ''), u.avatar_object_key,
			CASE WHEN $1::uuid = '00000000-0000-0000-0000-000000000000'::uuid THEN false
				 ELSE EXISTS (SELECT 1 FROM user_follows uf WHERE uf.follower_id = $1 AND uf.followee_id = u.id)
			END AS followed_by_me,
			CASE WHEN $1::uuid = '00000000-0000-0000-0000-000000000000'::uuid THEN false
				 ELSE EXISTS (SELECT 1 FROM user_follows uf WHERE uf.follower_id = u.id AND uf.followee_id = $1)
			END AS follows_you
		FROM user_follows f
		JOIN users u ON u.id = f.follower_id
		WHERE f.followee_id = $2
		ORDER BY f.created_at DESC, u.id DESC
		LIMIT $3 OFFSET $4
	`, viewerID, userID, fetch, offset)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()

	for rows.Next() {
		var it FollowListUser
		var av pgtype.Text
		if err := rows.Scan(&it.ID, &it.Email, &it.Handle, &it.DisplayName, &it.Bio, &av, &it.FollowedByMe, &it.FollowsYou); err != nil {
			return nil, 0, false, err
		}
		if av.Valid && strings.TrimSpace(av.String) != "" {
			s := strings.TrimSpace(av.String)
			it.AvatarObjectKey = &s
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}

	if len(items) > limit {
		items = items[:limit]
		hasNext = true
		nextOffset = offset + limit
	}
	return items, nextOffset, hasNext, nil
}

// ListUserFollowing returns users followed by userID, that is, user_follows.followee_id.
func (p *Pool) ListUserFollowing(ctx context.Context, viewerID, userID uuid.UUID, limit, offset int) (items []FollowListUser, nextOffset int, hasNext bool, err error) {
	limit = clampListLimit(limit)
	offset = clampListOffset(offset)
	fetch := limit + 1

	rows, err := p.db.Query(ctx, `
		SELECT u.id, u.email, u.handle, u.display_name, COALESCE(u.bio, ''), u.avatar_object_key,
			CASE WHEN $1::uuid = '00000000-0000-0000-0000-000000000000'::uuid THEN false
				 ELSE EXISTS (SELECT 1 FROM user_follows uf WHERE uf.follower_id = $1 AND uf.followee_id = u.id)
			END AS followed_by_me,
			CASE WHEN $1::uuid = '00000000-0000-0000-0000-000000000000'::uuid THEN false
				 ELSE EXISTS (SELECT 1 FROM user_follows uf WHERE uf.follower_id = u.id AND uf.followee_id = $1)
			END AS follows_you
		FROM user_follows f
		JOIN users u ON u.id = f.followee_id
		WHERE f.follower_id = $2
		ORDER BY f.created_at DESC, u.id DESC
		LIMIT $3 OFFSET $4
	`, viewerID, userID, fetch, offset)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()

	for rows.Next() {
		var it FollowListUser
		var av pgtype.Text
		if err := rows.Scan(&it.ID, &it.Email, &it.Handle, &it.DisplayName, &it.Bio, &av, &it.FollowedByMe, &it.FollowsYou); err != nil {
			return nil, 0, false, err
		}
		if av.Valid && strings.TrimSpace(av.String) != "" {
			s := strings.TrimSpace(av.String)
			it.AvatarObjectKey = &s
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}

	if len(items) > limit {
		items = items[:limit]
		hasNext = true
		nextOffset = offset + limit
	}
	return items, nextOffset, hasNext, nil
}

func (p *Pool) ListFeedFollowing(ctx context.Context, viewerID uuid.UUID, limit int) ([]PostRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, u.email, u.handle, u.display_name, u.avatar_object_key, p.caption, p.media_type, p.object_keys,
			p.is_nsfw,
			`+postVisibilityExpr("p")+`,
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_view_password,
			COALESCE(p.view_password_scope, 0),
			COALESCE(p.view_password_text_ranges, '[]'::jsonb)::text,
			(COALESCE(btrim(p.membership_provider), '') <> '') AS has_membership_lock,
			COALESCE(p.membership_provider, ''), COALESCE(p.membership_creator_id, ''), COALESCE(p.membership_tier_id, ''),
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
		WHERE p.reply_to_id IS NULL
		  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
		  AND p.visible_at <= NOW()
		  AND p.group_id IS NULL
		  AND EXISTS (
			SELECT 1 FROM user_follows f
			WHERE f.follower_id = $1 AND f.followee_id = p.user_id
		  )
		  AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY p.visible_at DESC, p.id DESC
		LIMIT $2
	`, viewerID, limit)
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
		var hasMembershipLock bool
		var membershipProvider, membershipCreatorID, membershipTierID string
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Email, &r.UserHandle, &r.DisplayName, &av, &r.Caption, &r.MediaType, &r.ObjectKeys,
			&r.IsNSFW, &r.Visibility, &r.HasViewPassword, &scope, &textRanges,
			&hasMembershipLock, &membershipProvider, &membershipCreatorID, &membershipTierID,
			&r.CreatedAt, &r.VisibleAt,
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
		r.HasMembershipLock = hasMembershipLock
		r.MembershipProvider = strings.TrimSpace(membershipProvider)
		r.MembershipCreatorID = strings.TrimSpace(membershipCreatorID)
		r.MembershipTierID = strings.TrimSpace(membershipTierID)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountUsersTotal returns the total registered user count for NodeInfo and similar outputs.
func (p *Pool) CountUsersTotal(ctx context.Context) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM users`).Scan(&n)
	return n, err
}
