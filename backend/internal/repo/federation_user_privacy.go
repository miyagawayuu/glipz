package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// NormalizeFederationTargetAcct normalizes acct keys for storage and comparison.
func NormalizeFederationTargetAcct(acct string) string {
	acct = strings.TrimSpace(acct)
	low := strings.ToLower(acct)
	if strings.HasPrefix(low, PortableIDPrefix) {
		return PortableIDPrefix + strings.TrimSpace(acct[len(PortableIDPrefix):])
	}
	if strings.HasPrefix(low, LegacyPortablePrefix) {
		return LegacyPortablePrefix + strings.ToLower(strings.TrimSpace(acct[len(LegacyPortablePrefix):]))
	}
	acct = low
	return acct
}

// FedIncomingActorVisibleSQL returns AND ... conditions for alias `f` (federation_incoming_posts).
// viewerParam is a placeholder like $1 (including cast if needed by caller).
func FedIncomingActorVisibleSQL(alias, viewerParam string) string {
	return ` AND NOT EXISTS (
			SELECT 1 FROM federation_user_blocks b
			WHERE b.user_id = ` + viewerParam + `
				AND b.target_acct IN (
					lower(trim(both from COALESCE(` + alias + `.actor_acct, ` + alias + `.actor_iri, ''))),
					trim(both from COALESCE(` + alias + `.actor_portable_id, ''))
				))
		AND NOT EXISTS (
			SELECT 1 FROM federation_user_mutes m
			WHERE m.user_id = ` + viewerParam + `
				AND m.target_acct IN (
					lower(trim(both from COALESCE(` + alias + `.actor_acct, ` + alias + `.actor_iri, ''))),
					trim(both from COALESCE(` + alias + `.actor_portable_id, ''))
				))`
}

type FederationUserPrivacyRow struct {
	TargetAcct string `json:"target_acct"`
	CreatedAt  string `json:"created_at"`
}

func (p *Pool) HasFederationUserBlock(ctx context.Context, userID uuid.UUID, targetAcct string) (bool, error) {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" {
		return false, nil
	}
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM federation_user_blocks
		WHERE user_id = $1 AND target_acct = $2
	`, userID, targetAcct).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (p *Pool) HasFederationUserMute(ctx context.Context, userID uuid.UUID, targetAcct string) (bool, error) {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" {
		return false, nil
	}
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM federation_user_mutes
		WHERE user_id = $1 AND target_acct = $2
	`, userID, targetAcct).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (p *Pool) AddFederationUserBlock(ctx context.Context, userID uuid.UUID, targetAcct string) error {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" || (!strings.Contains(targetAcct, "@") && !strings.HasPrefix(targetAcct, PortableIDPrefix) && !strings.HasPrefix(targetAcct, LegacyPortablePrefix)) {
		return fmt.Errorf("invalid target_acct")
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM federation_user_mutes WHERE user_id = $1 AND target_acct = $2`, userID, targetAcct); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO federation_user_blocks (user_id, target_acct) VALUES ($1, $2)
		ON CONFLICT (user_id, target_acct) DO NOTHING
	`, userID, targetAcct); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM glipz_protocol_remote_followers
		WHERE local_user_id = $1 AND lower(trim(remote_actor_id)) = $2
	`, userID, targetAcct); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Pool) RemoveFederationUserBlock(ctx context.Context, userID uuid.UUID, targetAcct string) error {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" {
		return fmt.Errorf("invalid target_acct")
	}
	tag, err := p.db.Exec(ctx, `DELETE FROM federation_user_blocks WHERE user_id = $1 AND target_acct = $2`, userID, targetAcct)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) AddFederationUserMute(ctx context.Context, userID uuid.UUID, targetAcct string) error {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" || (!strings.Contains(targetAcct, "@") && !strings.HasPrefix(targetAcct, PortableIDPrefix) && !strings.HasPrefix(targetAcct, LegacyPortablePrefix)) {
		return fmt.Errorf("invalid target_acct")
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_user_mutes (user_id, target_acct) VALUES ($1, $2)
		ON CONFLICT (user_id, target_acct) DO NOTHING
	`, userID, targetAcct)
	return err
}

func (p *Pool) RemoveFederationUserMute(ctx context.Context, userID uuid.UUID, targetAcct string) error {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" {
		return fmt.Errorf("invalid target_acct")
	}
	tag, err := p.db.Exec(ctx, `DELETE FROM federation_user_mutes WHERE user_id = $1 AND target_acct = $2`, userID, targetAcct)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) ListFederationUserBlocks(ctx context.Context, userID uuid.UUID, limit int) ([]FederationUserPrivacyRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := p.db.Query(ctx, `
		SELECT target_acct, to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM federation_user_blocks
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederationUserPrivacyRow
	for rows.Next() {
		var r FederationUserPrivacyRow
		if err := rows.Scan(&r.TargetAcct, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) ListFederationUserMutes(ctx context.Context, userID uuid.UUID, limit int) ([]FederationUserPrivacyRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := p.db.Query(ctx, `
		SELECT target_acct, to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM federation_user_mutes
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederationUserPrivacyRow
	for rows.Next() {
		var r FederationUserPrivacyRow
		if err := rows.Scan(&r.TargetAcct, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) FederationUserPrivacyRelationship(ctx context.Context, userID uuid.UUID, targetAcct string) (blocked, muted bool, err error) {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" {
		return false, false, nil
	}
	err = p.db.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM federation_user_blocks WHERE user_id = $1 AND target_acct = $2),
			EXISTS (SELECT 1 FROM federation_user_mutes WHERE user_id = $1 AND target_acct = $2)
	`, userID, targetAcct).Scan(&blocked, &muted)
	if err != nil {
		return false, false, err
	}
	return blocked, muted, nil
}

// PostAuthorHasFederationBlock returns true when post owner has blocked targetAcct (for inbound interaction stealth).
func (p *Pool) PostAuthorHasFederationBlock(ctx context.Context, postID uuid.UUID, targetAcct string) (bool, error) {
	targetAcct = NormalizeFederationTargetAcct(targetAcct)
	if targetAcct == "" {
		return false, nil
	}
	var owner uuid.UUID
	err := p.db.QueryRow(ctx, `SELECT user_id FROM posts WHERE id = $1`, postID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	return p.HasFederationUserBlock(ctx, owner, targetAcct)
}
