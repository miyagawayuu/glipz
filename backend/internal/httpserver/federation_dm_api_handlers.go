package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type federationDMInviteReq struct {
	ToAcct   string `json:"to_acct"`
	ThreadID string `json:"thread_id,omitempty"`
	// Optional invite expiry for the receiver UX only.
	ExpiresAt string `json:"expires_at,omitempty"`
}

type federationDMInviteResp struct {
	ThreadID string `json:"thread_id"`
}

type federationDMAcceptReq struct {
	ThreadID string            `json:"thread_id"`
	ToAcct   string            `json:"to_acct"`
	KeyBox   federationSealedBox `json:"key_box_for_inviter"`
}

type federationDMRejectReq struct {
	ThreadID string `json:"thread_id"`
	ToAcct   string `json:"to_acct"`
}

type federationDMMessageReq struct {
	ThreadID         string                     `json:"thread_id"`
	MessageID        string                     `json:"message_id"`
	ToAcct           string                     `json:"to_acct"`
	SenderPayload    *federationSealedBox         `json:"sender_payload,omitempty"`
	RecipientPayload federationSealedBox         `json:"recipient_payload"`
	SentAt           string                     `json:"sent_at"`
	Attachments      []federationEventDMAttachment `json:"attachments,omitempty"`
}

func (s *Server) ensureMutualFollowForFederationDM(ctx context.Context, localUserID uuid.UUID, remoteAcct string) error {
	okOut, err := s.db.HasAcceptedRemoteFollowForUser(ctx, localUserID, remoteAcct)
	if err != nil {
		return err
	}
	okIn, err := s.db.HasGlipzProtocolRemoteFollower(ctx, localUserID, remoteAcct)
	if err != nil {
		return err
	}
	if !okOut || !okIn {
		return repo.ErrForbidden
	}
	return nil
}

func (s *Server) federationDMRemoteInbox(ctx context.Context, toAcct string) (host string, inboxURL string, err error) {
	_, host, err = splitAcct(toAcct)
	if err != nil {
		return "", "", err
	}
	disc, err := fetchRemoteFederationDiscovery(ctx, host)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(disc.Server.DMKeysURL) == "" {
		return "", "", errors.New("unsupported_peer_dm")
	}
	inbox := strings.TrimSpace(disc.Server.EventsURL)
	if inbox == "" {
		return "", "", errors.New("missing_peer_inbox")
	}
	return host, inbox, nil
}

func writeFederationDMOutboundRemoteInboxError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	// Normalize common remote/discovery failures to 4xx so the UI can show a meaningful message.
	if strings.Contains(err.Error(), "unsupported_peer_dm") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_peer"})
		return true
	}
	if strings.Contains(err.Error(), "missing_peer_inbox") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer_inbox"})
		return true
	}
	if code := ResolveFailureAPIError(err); code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
		return true
	}
	return false
}

func (s *Server) handleFederationDMInviteOutbound(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req federationDMInviteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.ToAcct = strings.TrimSpace(req.ToAcct)
	if req.ToAcct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_to_acct"})
		return
	}
	threadID := uuid.New()
	if strings.TrimSpace(req.ThreadID) != "" {
		if parsed, err := uuid.Parse(strings.TrimSpace(req.ThreadID)); err == nil && parsed != uuid.Nil {
			threadID = parsed
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
			return
		}
	}
	if err := s.ensureMutualFollowForFederationDM(r.Context(), uid, req.ToAcct); err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeServerError(w, "ensureMutualFollowForFederationDM invite", err)
		return
	}
	_, inboxURL, err := s.federationDMRemoteInbox(r.Context(), req.ToAcct)
	if err != nil {
		if writeFederationDMOutboundRemoteInboxError(w, err) {
			return
		}
		writeServerError(w, "federationDMRemoteInbox invite", err)
		return
	}
	author, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload invite", err)
		return
	}
	alg, pub, _, err := s.db.DMIdentityKeyForUser(r.Context(), uid)
	if err != nil {
		if errors.Is(err, repo.ErrDMIdentityUnavailable) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dm_not_configured"})
			return
		}
		writeServerError(w, "DMIdentityKeyForUser invite", err)
		return
	}
	fromKID := dmKeyKID(alg, pub)
	if err := s.db.UpsertFederationDMThread(r.Context(), threadID, uid, req.ToAcct, "invited_outbound"); err != nil {
		writeServerError(w, "UpsertFederationDMThread invite", err)
		return
	}
	ev := federationEventEnvelope{
		Kind:   "dm_invite",
		Author: author,
		DM: &federationEventDM{
			ThreadID:     threadID.String(),
			ToAcct:       req.ToAcct,
			FromAcct:     author.Acct,
			FromKID:      fromKID,
			Capabilities: map[string]any{"attachments": true},
			ExpiresAt:    strings.TrimSpace(req.ExpiresAt),
		},
	}
	if err := s.queueDirectedFederationEvent(r.Context(), uid, threadID, inboxURL, ev); err != nil {
		writeServerError(w, "queueDirectedFederationEvent dm_invite", err)
		return
	}
	writeJSON(w, http.StatusOK, federationDMInviteResp{ThreadID: threadID.String()})
}

func (s *Server) handleFederationDMAcceptOutbound(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req federationDMAcceptReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(req.ThreadID))
	if err != nil || threadID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	req.ToAcct = strings.TrimSpace(req.ToAcct)
	if req.ToAcct == "" || strings.TrimSpace(req.KeyBox.IV) == "" || strings.TrimSpace(req.KeyBox.Data) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := s.ensureMutualFollowForFederationDM(r.Context(), uid, req.ToAcct); err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeServerError(w, "ensureMutualFollowForFederationDM accept", err)
		return
	}
	_, inboxURL, err := s.federationDMRemoteInbox(r.Context(), req.ToAcct)
	if err != nil {
		if writeFederationDMOutboundRemoteInboxError(w, err) {
			return
		}
		writeServerError(w, "federationDMRemoteInbox accept", err)
		return
	}
	author, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload accept", err)
		return
	}
	ev := federationEventEnvelope{
		Kind:   "dm_accept",
		Author: author,
		DM: &federationEventDM{
			ThreadID:         threadID.String(),
			ToAcct:           req.ToAcct,
			FromAcct:         author.Acct,
			KeyBoxForInviter: &req.KeyBox,
		},
	}
	if err := s.db.UpsertFederationDMThread(r.Context(), threadID, uid, req.ToAcct, "accepted"); err != nil {
		writeServerError(w, "UpsertFederationDMThread accept", err)
		return
	}
	if err := s.queueDirectedFederationEvent(r.Context(), uid, threadID, inboxURL, ev); err != nil {
		writeServerError(w, "queueDirectedFederationEvent dm_accept", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleFederationDMRejectOutbound(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req federationDMRejectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(req.ThreadID))
	if err != nil || threadID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	req.ToAcct = strings.TrimSpace(req.ToAcct)
	if req.ToAcct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_to_acct"})
		return
	}
	if err := s.ensureMutualFollowForFederationDM(r.Context(), uid, req.ToAcct); err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeServerError(w, "ensureMutualFollowForFederationDM reject", err)
		return
	}
	_, inboxURL, err := s.federationDMRemoteInbox(r.Context(), req.ToAcct)
	if err != nil {
		if writeFederationDMOutboundRemoteInboxError(w, err) {
			return
		}
		writeServerError(w, "federationDMRemoteInbox reject", err)
		return
	}
	author, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload reject", err)
		return
	}
	ev := federationEventEnvelope{
		Kind:   "dm_reject",
		Author: author,
		DM: &federationEventDM{
			ThreadID: threadID.String(),
			ToAcct:   req.ToAcct,
			FromAcct: author.Acct,
		},
	}
	if err := s.db.UpsertFederationDMThread(r.Context(), threadID, uid, req.ToAcct, "rejected"); err != nil {
		writeServerError(w, "UpsertFederationDMThread reject", err)
		return
	}
	if err := s.queueDirectedFederationEvent(r.Context(), uid, threadID, inboxURL, ev); err != nil {
		writeServerError(w, "queueDirectedFederationEvent dm_reject", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleFederationDMMessageOutbound(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req federationDMMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(req.ThreadID))
	if err != nil || threadID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(req.MessageID))
	if err != nil || messageID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_message_id"})
		return
	}
	req.ToAcct = strings.TrimSpace(req.ToAcct)
	if req.ToAcct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_to_acct"})
		return
	}
	if strings.TrimSpace(req.RecipientPayload.IV) == "" || strings.TrimSpace(req.RecipientPayload.Data) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_recipient_payload"})
		return
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(req.SentAt)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_sent_at"})
		return
	}
	if err := s.ensureMutualFollowForFederationDM(r.Context(), uid, req.ToAcct); err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeServerError(w, "ensureMutualFollowForFederationDM message", err)
		return
	}
	_, inboxURL, err := s.federationDMRemoteInbox(r.Context(), req.ToAcct)
	if err != nil {
		if writeFederationDMOutboundRemoteInboxError(w, err) {
			return
		}
		writeServerError(w, "federationDMRemoteInbox message", err)
		return
	}
	author, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload message", err)
		return
	}
	sentAtParsed, _ := time.Parse(time.RFC3339, strings.TrimSpace(req.SentAt))
	recipientPayloadJSON, _ := json.Marshal(req.RecipientPayload)
	var senderPayloadJSON []byte
	if req.SenderPayload != nil {
		if strings.TrimSpace(req.SenderPayload.IV) == "" || strings.TrimSpace(req.SenderPayload.Data) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_sender_payload"})
			return
		}
		senderPayloadJSON, _ = json.Marshal(req.SenderPayload)
	}
	attachmentsJSON, _ := json.Marshal(req.Attachments)
	ev := federationEventEnvelope{
		Kind:   "dm_message",
		Author: author,
		DM: &federationEventDM{
			ThreadID:         threadID.String(),
			MessageID:        messageID.String(),
			ToAcct:           req.ToAcct,
			FromAcct:         author.Acct,
			RecipientPayload: &req.RecipientPayload,
			SentAt:           strings.TrimSpace(req.SentAt),
			Attachments:      req.Attachments,
		},
	}
	if err := s.db.UpsertFederationDMThread(r.Context(), threadID, uid, req.ToAcct, "accepted"); err != nil {
		writeServerError(w, "UpsertFederationDMThread message", err)
		return
	}
	// Store the outbound ciphertext locally so the sender can render it in the thread view.
	var sentAt *time.Time
	if !sentAtParsed.IsZero() {
		t := sentAtParsed.UTC()
		sentAt = &t
	}
	if err := s.db.InsertFederationDMMessage(r.Context(), messageID, threadID, author.Acct, senderPayloadJSON, recipientPayloadJSON, attachmentsJSON, sentAt); err != nil {
		writeServerError(w, "InsertFederationDMMessage outbound", err)
		return
	}
	if err := s.queueDirectedFederationEvent(r.Context(), uid, threadID, inboxURL, ev); err != nil {
		writeServerError(w, "queueDirectedFederationEvent dm_message", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

