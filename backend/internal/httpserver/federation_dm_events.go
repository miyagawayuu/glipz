package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

func (s *Server) handleFederationDMEventInbound(ctx context.Context, verified verifiedFederationRequest, eventID string, ev federationEventEnvelope) error {
	kind := strings.TrimSpace(ev.Kind)
	dm := ev.DM
	if dm == nil {
		return errors.New("missing_dm")
	}
	threadID, err := uuid.Parse(strings.TrimSpace(dm.ThreadID))
	if err != nil || threadID == uuid.Nil {
		return errors.New("invalid_thread_id")
	}
	toUser, toHost, err := splitAcct(dm.ToAcct)
	if err != nil || !strings.EqualFold(toHost, s.federationDisplayHost()) || strings.TrimSpace(toUser) == "" {
		return errors.New("invalid_to_acct")
	}
	fromUser, fromHost, err := splitAcct(dm.FromAcct)
	if err != nil || !strings.EqualFold(fromHost, verified.InstanceHost) || strings.TrimSpace(fromUser) == "" {
		return errors.New("invalid_from_acct")
	}
	pfl, err := s.db.PublicProfileByHandle(ctx, toUser)
	if err != nil {
		return err
	}
	if bl, errB := s.db.HasFederationUserBlock(ctx, pfl.ID, strings.TrimSpace(dm.FromAcct)); errB != nil {
		return errB
	} else if bl {
		return nil
	}

	// Mutual follow enforcement (minimum viable):
	// - local -> remote must be accepted
	// - remote -> local must exist as follower
	okOut, err := s.db.HasAcceptedRemoteFollowForUser(ctx, pfl.ID, dm.FromAcct)
	if err != nil {
		return err
	}
	okIn, err := s.db.HasGlipzProtocolRemoteFollower(ctx, pfl.ID, dm.FromAcct)
	if err != nil {
		return err
	}
	if !okOut || !okIn {
		return repo.ErrForbidden
	}

	switch kind {
	case "dm_invite":
		if strings.TrimSpace(dm.ToAcct) == "" || strings.TrimSpace(dm.FromAcct) == "" {
			return errors.New("invalid_dm")
		}
		if err := s.db.UpsertFederationDMThread(ctx, threadID, pfl.ID, strings.TrimSpace(dm.FromAcct), "invited_inbound"); err != nil {
			return err
		}
		s.publishFederationNotifyEvent(ctx, pfl.ID, "dm_invite", strings.TrimSpace(dm.FromAcct))
		s.publishFederationDMStreamEvent(ctx, pfl.ID, "federation_dm_invite", threadID, strings.TrimSpace(dm.FromAcct))
		return nil
	case "dm_accept":
		// At this stage we only flip the thread state; thread key material stays end-to-end.
		if dm.KeyBoxForInviter == nil || strings.TrimSpace(dm.KeyBoxForInviter.IV) == "" || strings.TrimSpace(dm.KeyBoxForInviter.Data) == "" {
			return errors.New("invalid_key_box")
		}
		if err := s.db.UpsertFederationDMThread(ctx, threadID, pfl.ID, strings.TrimSpace(dm.FromAcct), "accepted"); err != nil {
			return err
		}
		s.publishFederationDMStreamEvent(ctx, pfl.ID, "federation_dm_accept", threadID, strings.TrimSpace(dm.FromAcct))
		return nil
	case "dm_reject":
		if err := s.db.UpsertFederationDMThread(ctx, threadID, pfl.ID, strings.TrimSpace(dm.FromAcct), "rejected"); err != nil {
			return err
		}
		s.publishFederationDMStreamEvent(ctx, pfl.ID, "federation_dm_reject", threadID, strings.TrimSpace(dm.FromAcct))
		return nil
	case "dm_message":
		if dm.RecipientPayload == nil || strings.TrimSpace(dm.RecipientPayload.IV) == "" || strings.TrimSpace(dm.RecipientPayload.Data) == "" {
			return errors.New("invalid_recipient_payload")
		}
		msgID, err := uuid.Parse(strings.TrimSpace(dm.MessageID))
		if err != nil || msgID == uuid.Nil {
			return errors.New("invalid_message_id")
		}
		var sentAt *time.Time
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(dm.SentAt)); err == nil {
			tt := t.UTC()
			sentAt = &tt
		}
		payloadJSON, _ := json.Marshal(dm.RecipientPayload)
		attachmentsJSON, _ := json.Marshal(dm.Attachments)
		if err := s.db.UpsertFederationDMThread(ctx, threadID, pfl.ID, strings.TrimSpace(dm.FromAcct), "accepted"); err != nil {
			return err
		}
		if err := s.db.InsertFederationDMMessage(ctx, msgID, threadID, strings.TrimSpace(dm.FromAcct), nil, payloadJSON, attachmentsJSON, sentAt); err != nil {
			return err
		}
		s.publishFederationNotifyEvent(ctx, pfl.ID, "dm_message", strings.TrimSpace(dm.FromAcct))
		s.publishFederationDMStreamEvent(ctx, pfl.ID, "federation_dm_message", threadID, strings.TrimSpace(dm.FromAcct))
		return nil
	default:
		return errors.New("unsupported_dm_kind")
	}
}

