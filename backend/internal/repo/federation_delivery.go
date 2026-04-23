package repo

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

type FederationDeliveryRow = GlipzProtocolOutboxDeliveryRow

type FederationDeliveryInsert = GlipzProtocolOutboxDeliveryInsert

func (p *Pool) InsertFederationDeliveries(ctx context.Context, items []FederationDeliveryInsert) error {
	return p.InsertGlipzProtocolOutboxDeliveries(ctx, items)
}

func (p *Pool) ClaimFederationDeliveries(ctx context.Context, limit int) ([]FederationDeliveryRow, error) {
	return p.ClaimGlipzProtocolOutboxDeliveries(ctx, limit)
}

func (p *Pool) CompleteFederationDelivery(ctx context.Context, id uuid.UUID) error {
	return p.CompleteGlipzProtocolOutboxDelivery(ctx, id)
}

func (p *Pool) FailFederationDelivery(ctx context.Context, id uuid.UUID, attemptCount int, lastErr string, nextAttempt time.Time, dead bool) error {
	return p.FailGlipzProtocolOutboxDelivery(ctx, id, attemptCount, lastErr, nextAttempt, dead)
}

func (p *Pool) CountFederationDeliveriesByStatus(ctx context.Context, status string) (int64, error) {
	return p.CountGlipzProtocolOutboxDeliveriesByStatus(ctx, status)
}

type FederationDeliveryAdminRow = GlipzProtocolOutboxDeliveryAdminRow

func (p *Pool) ListFederationDeliveriesAdmin(ctx context.Context, status string, limit int) ([]FederationDeliveryAdminRow, error) {
	return p.ListGlipzProtocolOutboxDeliveriesAdmin(ctx, status, limit)
}

type FederationPublicPostRow = GlipzProtocolPostRow

func (p *Pool) ListFederationPublicPosts(ctx context.Context, authorID uuid.UUID, limit int, beforeVisibleAt *time.Time, beforeID *uuid.UUID) ([]FederationPublicPostRow, error) {
	return p.ListGlipzProtocolOutboxPosts(ctx, authorID, limit, beforeVisibleAt, beforeID)
}

func (p *Pool) CountFederationPublicPosts(ctx context.Context, authorID uuid.UUID) (int64, error) {
	return p.CountGlipzProtocolOutboxPosts(ctx, authorID)
}

func (p *Pool) GetFederationPublicPostForDelivery(ctx context.Context, authorID, postID uuid.UUID) (FederationPublicPostRow, error) {
	return p.GetGlipzProtocolPostForDelivery(ctx, authorID, postID)
}

func (p *Pool) UpsertFederationSubscriber(ctx context.Context, localUserID uuid.UUID, remoteAccountID, remoteInbox string) error {
	return p.UpsertGlipzProtocolRemoteFollower(ctx, localUserID, remoteAccountID, remoteInbox)
}

func (p *Pool) DeleteFederationSubscriber(ctx context.Context, localUserID uuid.UUID, remoteAccountID string) error {
	return p.DeleteGlipzProtocolRemoteFollower(ctx, localUserID, remoteAccountID)
}

func (p *Pool) CountFederationRemoteFollowers(ctx context.Context, localUserID uuid.UUID) (int64, error) {
	return p.CountGlipzProtocolRemoteFollowers(ctx, localUserID)
}

func (p *Pool) ListFederationSubscriberInboxes(ctx context.Context, localUserID uuid.UUID) ([]string, error) {
	return p.ListGlipzProtocolRemoteFollowerInboxes(ctx, localUserID)
}

func (p *Pool) UpsertRemoteFollowAccepted(ctx context.Context, localUserID uuid.UUID, remoteAccountID, remoteInbox string) error {
	remoteAccountID = strings.TrimSpace(remoteAccountID)
	remoteInbox = strings.TrimSpace(remoteInbox)
	if remoteAccountID == "" || remoteInbox == "" {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_remote_follows (local_user_id, remote_actor_id, remote_inbox, state, follow_activity_id)
		VALUES ($1, $2, $3, 'accepted', '')
		ON CONFLICT (local_user_id, remote_actor_id) DO UPDATE SET
			remote_inbox = EXCLUDED.remote_inbox,
			state = 'accepted'
	`, localUserID, remoteAccountID, remoteInbox)
	return err
}

func (p *Pool) UpdateFederationIncomingPost(ctx context.Context, in InsertFederatedIncomingInput) error {
	inserted, err := p.InsertFederatedIncomingPost(ctx, in)
	if err != nil {
		return err
	}
	if inserted {
		return nil
	}
	return p.UpdateFederatedIncomingFromNote(ctx, in.ObjectIRI, in.CaptionText, in.MediaType, in.MediaURLs, in.IsNSFW, in.PublishedAt, in.LikeCount, in.ReplyToObjectIRI, in.RepostOfObjectIRI, in.RepostComment, in.HasViewPassword, in.ViewPasswordScope, in.ViewPasswordTextRanges, in.UnlockURL)
}

func MustMarshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
