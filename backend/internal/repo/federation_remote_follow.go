package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RemoteFollowRow represents a row where a Glipz user follows a remote actor.
type RemoteFollowRow struct {
	ID                 uuid.UUID
	LocalUserID        uuid.UUID
	RemoteActorID      string
	RemoteInbox        string
	State              string
	FollowActivityID   string
}

// UpsertRemoteFollowPending records a follow as pending immediately after sending it.
// Existing accepted rows remain accepted instead of returning a conflict-style error.
func (p *Pool) UpsertRemoteFollowPending(ctx context.Context, localUserID uuid.UUID, remoteActorID, remoteInbox, followActivityID string) error {
	remoteActorID = strings.TrimSpace(remoteActorID)
	remoteInbox = strings.TrimSpace(remoteInbox)
	if remoteActorID == "" || remoteInbox == "" {
		return fmt.Errorf("empty remote actor or inbox")
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_remote_follows (local_user_id, remote_actor_id, remote_inbox, state, follow_activity_id)
		VALUES ($1, $2, $3, 'pending', $4)
		ON CONFLICT (local_user_id, remote_actor_id) DO UPDATE SET
			remote_inbox = EXCLUDED.remote_inbox,
			follow_activity_id = EXCLUDED.follow_activity_id,
			state = CASE
				WHEN federation_remote_follows.state = 'accepted' THEN federation_remote_follows.state
				ELSE 'pending'
			END
	`, localUserID, remoteActorID, remoteInbox, strings.TrimSpace(followActivityID))
	return err
}

// MarkRemoteFollowAccepted marks a pending follow as accepted after an Accept activity.
// It is idempotent for already accepted rows and returns ErrNotFound when the row is missing.
func (p *Pool) MarkRemoteFollowAccepted(ctx context.Context, localUserID uuid.UUID, remoteActorID string) error {
	remoteActorID = strings.TrimSpace(remoteActorID)
	tag, err := p.db.Exec(ctx, `
		UPDATE federation_remote_follows SET state = 'accepted'
		WHERE local_user_id = $1 AND remote_actor_id = $2 AND state IN ('pending', 'accepted')
	`, localUserID, remoteActorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RepointRemoteFollowRemoteActor rewrites remote_actor_id from old to new during Move handling.
func (p *Pool) RepointRemoteFollowRemoteActor(ctx context.Context, oldIRI, newIRI string) error {
	oldIRI = strings.TrimSpace(oldIRI)
	newIRI = strings.TrimSpace(newIRI)
	if oldIRI == "" || newIRI == "" || strings.EqualFold(oldIRI, newIRI) {
		return nil
	}
	_, _ = p.db.Exec(ctx, `
		DELETE FROM federation_remote_follows a
		USING federation_remote_follows b
		WHERE a.remote_actor_id = $1 AND b.local_user_id = a.local_user_id AND b.remote_actor_id = $2
	`, oldIRI, newIRI)
	_, err := p.db.Exec(ctx, `
		UPDATE federation_remote_follows SET remote_actor_id = $2 WHERE remote_actor_id = $1
	`, oldIRI, newIRI)
	return err
}

// DeleteRemoteFollow removes a follow row during unfollow.
func (p *Pool) DeleteRemoteFollow(ctx context.Context, localUserID uuid.UUID, remoteActorID string) error {
	remoteActorID = strings.TrimSpace(remoteActorID)
	tag, err := p.db.Exec(ctx, `
		DELETE FROM federation_remote_follows WHERE local_user_id = $1 AND remote_actor_id = $2
	`, localUserID, remoteActorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetRemoteFollow returns one follow row.
func (p *Pool) GetRemoteFollow(ctx context.Context, localUserID uuid.UUID, remoteActorID string) (RemoteFollowRow, error) {
	remoteActorID = strings.TrimSpace(remoteActorID)
	var r RemoteFollowRow
	err := p.db.QueryRow(ctx, `
		SELECT id, local_user_id, remote_actor_id, remote_inbox, state, COALESCE(follow_activity_id, '')
		FROM federation_remote_follows
		WHERE local_user_id = $1 AND remote_actor_id = $2
	`, localUserID, remoteActorID).Scan(&r.ID, &r.LocalUserID, &r.RemoteActorID, &r.RemoteInbox, &r.State, &r.FollowActivityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RemoteFollowRow{}, ErrNotFound
	}
	if err != nil {
		return RemoteFollowRow{}, err
	}
	return r, nil
}

// ListRemoteFollowsForUser returns remote follows in reverse chronological order.
func (p *Pool) ListRemoteFollowsForUser(ctx context.Context, localUserID uuid.UUID) ([]RemoteFollowRow, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, local_user_id, remote_actor_id, remote_inbox, state, COALESCE(follow_activity_id, '')
		FROM federation_remote_follows
		WHERE local_user_id = $1
		ORDER BY created_at DESC
	`, localUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RemoteFollowRow
	for rows.Next() {
		var r RemoteFollowRow
		if err := rows.Scan(&r.ID, &r.LocalUserID, &r.RemoteActorID, &r.RemoteInbox, &r.State, &r.FollowActivityID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAcceptedRemoteFollowsForUser returns accepted remote follows (remote actors) for the local user.
func (p *Pool) ListAcceptedRemoteFollowsForUser(ctx context.Context, localUserID uuid.UUID, limit, offset int) ([]RemoteFollowRow, error) {
	limit = clampListLimit(limit)
	offset = clampListOffset(offset)
	rows, err := p.db.Query(ctx, `
		SELECT id, local_user_id, remote_actor_id, remote_inbox, state, COALESCE(follow_activity_id, '')
		FROM federation_remote_follows
		WHERE local_user_id = $1 AND state = 'accepted'
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, localUserID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RemoteFollowRow
	for rows.Next() {
		var r RemoteFollowRow
		if err := rows.Scan(&r.ID, &r.LocalUserID, &r.RemoteActorID, &r.RemoteInbox, &r.State, &r.FollowActivityID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAcceptedRemoteFollowLocalUserIDs returns local user IDs that follow the remote actor in accepted state.
func (p *Pool) ListAcceptedRemoteFollowLocalUserIDs(ctx context.Context, remoteActorID string) ([]uuid.UUID, error) {
	remoteActorID = strings.TrimSpace(remoteActorID)
	if remoteActorID == "" {
		return nil, nil
	}
	rows, err := p.db.Query(ctx, `
		SELECT local_user_id
		FROM federation_remote_follows
		WHERE remote_actor_id = $1 AND state = 'accepted'
		ORDER BY local_user_id
	`, remoteActorID)
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

// CountAcceptedRemoteFollowsForUser returns the number of accepted remote follows for the local user.
func (p *Pool) CountAcceptedRemoteFollowsForUser(ctx context.Context, localUserID uuid.UUID) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM federation_remote_follows
		WHERE local_user_id = $1 AND state = 'accepted'
	`, localUserID).Scan(&n)
	return n, err
}
