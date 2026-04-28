package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	CommunityRoleOwner  = "owner"
	CommunityRoleMember = "member"

	CommunityMemberPending  = "pending"
	CommunityMemberApproved = "approved"
	CommunityMemberRejected = "rejected"
)

type Community struct {
	ID                  uuid.UUID
	Name                string
	Description         string
	Details             string
	IconObjectKey       *string
	HeaderObjectKey     *string
	CreatorUserID       uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ApprovedMemberCount int64
	ViewerRole          string
	ViewerStatus        string
	PendingRequestCount int64
	CanManage           bool
}

type CommunityJoinRequest struct {
	UserID      uuid.UUID
	Handle      string
	DisplayName string
	AvatarKey   *string
	CreatedAt   time.Time
}

type CommunityMemberPreview struct {
	UserID      uuid.UUID
	Handle      string
	DisplayName string
	AvatarKey   *string
}

func (p *Pool) CreateCommunity(ctx context.Context, creatorID uuid.UUID, name, description, details string, iconObjectKey, headerObjectKey *string) (Community, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	details = strings.TrimSpace(details)
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Community{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var row Community
	err = tx.QueryRow(ctx, `
		INSERT INTO communities (name, description, details, icon_object_key, header_object_key, creator_user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, description, details, icon_object_key, header_object_key, creator_user_id, created_at, updated_at
	`, name, description, details, iconObjectKey, headerObjectKey, creatorID).Scan(
		&row.ID, &row.Name, &row.Description, &row.Details, &row.IconObjectKey, &row.HeaderObjectKey, &row.CreatorUserID, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return Community{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO community_members (community_id, user_id, role, status)
		VALUES ($1, $2, 'owner', 'approved')
	`, row.ID, creatorID); err != nil {
		return Community{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Community{}, err
	}
	row.ApprovedMemberCount = 1
	row.ViewerRole = CommunityRoleOwner
	row.ViewerStatus = CommunityMemberApproved
	return row, nil
}

func (p *Pool) ListCommunities(ctx context.Context, viewerID uuid.UUID, query string, limit int) ([]Community, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	rows, err := p.db.Query(ctx, `
		SELECT c.id, c.name, c.description, c.details, c.icon_object_key, c.header_object_key, c.creator_user_id, c.created_at, c.updated_at,
			COALESCE(mem.member_count, 0)::bigint,
			COALESCE(cm.role, ''),
			COALESCE(cm.status, ''),
			0::bigint
		FROM communities c
		LEFT JOIN (
			SELECT community_id, COUNT(*)::bigint AS member_count
			FROM community_members
			WHERE status = 'approved'
			GROUP BY community_id
		) mem ON mem.community_id = c.id
		LEFT JOIN community_members cm ON cm.community_id = c.id AND cm.user_id = $1
		WHERE $3 = ''
		   OR c.name ILIKE '%' || $3 || '%'
		   OR c.description ILIKE '%' || $3 || '%'
		   OR c.id::text ILIKE '%' || $3 || '%'
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT $2
	`, viewerID, limit, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommunities(rows)
}

func (p *Pool) CommunityByID(ctx context.Context, viewerID, communityID uuid.UUID) (Community, error) {
	var row Community
	err := p.db.QueryRow(ctx, `
		SELECT c.id, c.name, c.description, c.details, c.icon_object_key, c.header_object_key, c.creator_user_id, c.created_at, c.updated_at,
			COALESCE(mem.member_count, 0)::bigint,
			COALESCE(cm.role, ''),
			COALESCE(cm.status, ''),
			CASE WHEN COALESCE(cm.role, '') = 'owner' AND COALESCE(cm.status, '') = 'approved'
				THEN COALESCE(req.pending_count, 0)::bigint
				ELSE 0::bigint
			END
		FROM communities c
		LEFT JOIN (
			SELECT community_id, COUNT(*)::bigint AS member_count
			FROM community_members
			WHERE status = 'approved'
			GROUP BY community_id
		) mem ON mem.community_id = c.id
		LEFT JOIN community_members cm ON cm.community_id = c.id AND cm.user_id = $1
		LEFT JOIN (
			SELECT community_id, COUNT(*)::bigint AS pending_count
			FROM community_members
			WHERE status = 'pending'
			GROUP BY community_id
		) req ON req.community_id = c.id
		WHERE c.id = $2
	`, viewerID, communityID).Scan(
		&row.ID, &row.Name, &row.Description, &row.Details, &row.IconObjectKey, &row.HeaderObjectKey, &row.CreatorUserID, &row.CreatedAt, &row.UpdatedAt,
		&row.ApprovedMemberCount, &row.ViewerRole, &row.ViewerStatus, &row.PendingRequestCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Community{}, ErrNotFound
	}
	if err != nil {
		return Community{}, err
	}
	return row, nil
}

func scanCommunities(rows pgx.Rows) ([]Community, error) {
	var out []Community
	for rows.Next() {
		var row Community
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Description, &row.Details, &row.IconObjectKey, &row.HeaderObjectKey, &row.CreatorUserID, &row.CreatedAt, &row.UpdatedAt,
			&row.ApprovedMemberCount, &row.ViewerRole, &row.ViewerStatus, &row.PendingRequestCount,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) RequestCommunityJoin(ctx context.Context, userID, communityID uuid.UUID) (Community, error) {
	c, err := p.CommunityByID(ctx, userID, communityID)
	if err != nil {
		return Community{}, err
	}
	_, err = p.db.Exec(ctx, `
		INSERT INTO community_members (community_id, user_id, role, status, created_at, updated_at)
		VALUES ($1, $2, 'member', 'pending', NOW(), NOW())
		ON CONFLICT (community_id, user_id) DO UPDATE
		SET status = CASE
				WHEN community_members.status = 'rejected' THEN 'pending'
				ELSE community_members.status
			END,
			updated_at = NOW()
	`, c.ID, userID)
	if err != nil {
		return Community{}, err
	}
	return p.CommunityByID(ctx, userID, communityID)
}

func (p *Pool) ApprovedCommunityMember(ctx context.Context, communityID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.QueryRow(ctx, `
		SELECT true
		FROM community_members
		WHERE community_id = $1 AND user_id = $2 AND status = 'approved'
	`, communityID, userID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (p *Pool) EnsureCommunityApprovedMember(ctx context.Context, communityID, userID uuid.UUID) error {
	var approved bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
				SELECT 1 FROM community_members cm
				WHERE cm.community_id = c.id AND cm.user_id = $2 AND cm.status = 'approved'
			)
		FROM communities c
		WHERE c.id = $1
	`, communityID, userID).Scan(&approved)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !approved {
		return ErrForbidden
	}
	return nil
}

func (p *Pool) communityOwnerApproved(ctx context.Context, communityID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.QueryRow(ctx, `
		SELECT true
		FROM community_members
		WHERE community_id = $1 AND user_id = $2 AND role = 'owner' AND status = 'approved'
	`, communityID, userID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (p *Pool) UpdateCommunity(ctx context.Context, actorID, communityID uuid.UUID, allowAdmin bool, name, description string, details *string, iconObjectKey, headerObjectKey *string) (Community, error) {
	if !allowAdmin {
		ok, err := p.communityOwnerApproved(ctx, communityID, actorID)
		if err != nil {
			return Community{}, err
		}
		if !ok {
			return Community{}, ErrForbidden
		}
	}
	ct, err := p.db.Exec(ctx, `
		UPDATE communities
		SET name = $3,
			description = $4,
			details = COALESCE($5::text, details),
			icon_object_key = COALESCE($6::text, icon_object_key),
			header_object_key = COALESCE($7::text, header_object_key),
			updated_at = NOW()
		WHERE id = $1
		  AND ($2::boolean OR EXISTS (
			SELECT 1 FROM community_members cm
			WHERE cm.community_id = communities.id
			  AND cm.user_id = $8
			  AND cm.role = 'owner'
			  AND cm.status = 'approved'
		  ))
	`, communityID, allowAdmin, strings.TrimSpace(name), strings.TrimSpace(description), details, iconObjectKey, headerObjectKey, actorID)
	if err != nil {
		return Community{}, err
	}
	if ct.RowsAffected() == 0 {
		ok, err := p.communityExists(ctx, communityID)
		if err != nil {
			return Community{}, err
		}
		if !ok {
			return Community{}, ErrNotFound
		}
		return Community{}, ErrForbidden
	}
	return p.CommunityByID(ctx, actorID, communityID)
}

func (p *Pool) DeleteCommunity(ctx context.Context, actorID, communityID uuid.UUID, allowAdmin bool) error {
	if !allowAdmin {
		ok, err := p.communityOwnerApproved(ctx, communityID, actorID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrForbidden
		}
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM posts WHERE group_id = $1`, communityID); err != nil {
		return err
	}
	ct, err := tx.Exec(ctx, `
		DELETE FROM communities
		WHERE id = $1
		  AND ($2::boolean OR EXISTS (
			SELECT 1 FROM community_members cm
			WHERE cm.community_id = communities.id
			  AND cm.user_id = $3
			  AND cm.role = 'owner'
			  AND cm.status = 'approved'
		  ))
	`, communityID, allowAdmin, actorID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		ok, err := p.communityExists(ctx, communityID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		return ErrForbidden
	}
	return tx.Commit(ctx)
}

func (p *Pool) communityExists(ctx context.Context, communityID uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.QueryRow(ctx, `SELECT true FROM communities WHERE id = $1`, communityID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (p *Pool) ListCommunityJoinRequests(ctx context.Context, actorID, communityID uuid.UUID, limit int) ([]CommunityJoinRequest, error) {
	c, err := p.CommunityByID(ctx, actorID, communityID)
	if err != nil {
		return nil, err
	}
	ok, err := p.communityOwnerApproved(ctx, c.ID, actorID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT u.id, u.handle, u.display_name, u.avatar_object_key, cm.created_at
		FROM community_members cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.community_id = $1 AND cm.status = 'pending'
		ORDER BY cm.created_at ASC, u.id ASC
		LIMIT $2
	`, c.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommunityJoinRequest
	for rows.Next() {
		var row CommunityJoinRequest
		var avatar *string
		if err := rows.Scan(&row.UserID, &row.Handle, &row.DisplayName, &avatar, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.AvatarKey = avatar
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) ListCommunityMemberPreviews(ctx context.Context, communityID uuid.UUID, limit int) ([]CommunityMemberPreview, error) {
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	rows, err := p.db.Query(ctx, `
		SELECT u.id, u.handle, u.display_name, u.avatar_object_key
		FROM community_members cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.community_id = $1 AND cm.status = 'approved'
		ORDER BY (cm.role = 'owner') DESC, cm.created_at ASC, u.id ASC
		LIMIT $2
	`, communityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommunityMemberPreview
	for rows.Next() {
		var row CommunityMemberPreview
		var avatar *string
		if err := rows.Scan(&row.UserID, &row.Handle, &row.DisplayName, &avatar); err != nil {
			return nil, err
		}
		row.AvatarKey = avatar
		out = append(out, row)
	}
	return out, rows.Err()
}

func (p *Pool) ReviewCommunityJoinRequest(ctx context.Context, actorID, communityID, targetUserID uuid.UUID, approve bool) error {
	c, err := p.CommunityByID(ctx, actorID, communityID)
	if err != nil {
		return err
	}
	ok, err := p.communityOwnerApproved(ctx, c.ID, actorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	status := CommunityMemberRejected
	if approve {
		status = CommunityMemberApproved
	}
	ct, err := p.db.Exec(ctx, `
		UPDATE community_members
		SET status = $3, role = 'member', updated_at = NOW()
		WHERE community_id = $1 AND user_id = $2 AND status = 'pending'
	`, c.ID, targetUserID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) ListCommunityPosts(ctx context.Context, viewerID, communityID uuid.UUID, limit int) ([]PostRow, error) {
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
		WHERE p.group_id = $2
		  AND p.reply_to_id IS NULL
		  AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
		  AND p.visible_at <= NOW()
		  AND `+postReadableByViewerSQL("p", "$1")+`
		ORDER BY p.visible_at DESC, p.id DESC
		LIMIT $3
	`, viewerID, communityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostRows(rows)
}

// ListCommunityPostMediaTiles returns recent top-level community posts for the profile-style media grid.
func (p *Pool) ListCommunityPostMediaTiles(ctx context.Context, viewerID, communityID uuid.UUID, limit int) ([]PostMediaTileRow, error) {
	if limit <= 0 || limit > 120 {
		limit = 90
	}
	rows, err := p.db.Query(ctx, `
		SELECT p.id, p.user_id, p.media_type, p.object_keys[1],
			(COALESCE(btrim(p.view_password_hash), '') <> '') AS has_pw,
			COALESCE(p.view_password_scope, 0)
		FROM posts p
		WHERE p.group_id = $2
			AND p.reply_to_id IS NULL
			AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
			AND p.visible_at <= NOW()
			AND `+postReadableByViewerSQL("p", "$1")+`
			AND p.media_type IN ('image', 'video', 'audio')
			AND cardinality(p.object_keys) > 0
		ORDER BY p.visible_at DESC, p.id DESC
		LIMIT $3
	`, viewerID, communityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostMediaTileRow
	for rows.Next() {
		var r PostMediaTileRow
		var key string
		if err := rows.Scan(&r.PostID, &r.AuthorUserID, &r.MediaType, &key, &r.HasViewPassword, &r.ViewPasswordScope); err != nil {
			return nil, err
		}
		r.FirstObjectKey = strings.TrimSpace(key)
		out = append(out, r)
	}
	return out, rows.Err()
}
