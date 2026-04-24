package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/authjwt"
	"glipz.io/backend/internal/repo"
)

const redisDMUserPrefix = "glipz:dm:user:"

type putDMIdentityReq struct {
	Algorithm           string          `json:"algorithm"`
	PublicJWK           json.RawMessage `json:"public_jwk"`
	EncryptedPrivateJWK json.RawMessage `json:"encrypted_private_jwk"`
}

type createDMThreadReq struct {
	PeerHandle string `json:"peer_handle"`
}

type createDMMessageReq struct {
	SenderPayload    json.RawMessage `json:"sender_payload"`
	RecipientPayload json.RawMessage `json:"recipient_payload"`
	Attachments      json.RawMessage `json:"attachments"`
}

type dmCallInviteReq struct {
	Mode string `json:"mode"`
}

type dmCallHistoryReq struct {
	Mode string `json:"mode"`
}

type dmAttachmentRow struct {
	ObjectKey       string          `json:"object_key"`
	PublicURL       string          `json:"public_url"`
	FileName        string          `json:"file_name"`
	ContentType     string          `json:"content_type"`
	SizeBytes       int64           `json:"size_bytes"`
	EncryptedBytes  int64           `json:"encrypted_bytes"`
	FileIV          string          `json:"file_iv"`
	SenderKeyBox    json.RawMessage `json:"sender_key_box"`
	RecipientKeyBox json.RawMessage `json:"recipient_key_box"`
}

func redisDMUserChannel(uid uuid.UUID) string {
	return redisDMUserPrefix + uid.String()
}

func (s *Server) publishDMUserEvent(ctx context.Context, recipientID, messageID uuid.UUID) {
	m, err := s.db.DMStreamEventForRecipient(ctx, recipientID, messageID)
	if err != nil {
		log.Printf("DMStreamEventForRecipient: %v", err)
		return
	}
	if rawID, _ := m["sender_user_id"].(string); rawID != "" {
		if senderID, err := uuid.Parse(rawID); err == nil {
			m["sender_badges"] = userBadgesJSON(s.visibleUserBadges(senderID, toStringSlice(m["sender_badges"])))
		}
	}
	delete(m, "sender_user_id")
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	ch := redisDMUserChannel(recipientID)
	if err := s.rdb.Publish(ctx, ch, string(b)).Err(); err != nil {
		log.Printf("redis dm Publish %s: %v", ch, err)
	}
	if payload, ok := s.webPushNotificationFromDM(m); ok {
		s.queueWebPush(recipientID, payload)
	}
}

func (s *Server) publishDMCallEvent(ctx context.Context, recipientID, eventID uuid.UUID) {
	m, err := s.db.DMCallEventStreamForRecipient(ctx, recipientID, eventID)
	if err != nil {
		log.Printf("DMCallEventStreamForRecipient: %v", err)
		return
	}
	if rawID, _ := m["sender_user_id"].(string); rawID != "" {
		if senderID, err := uuid.Parse(rawID); err == nil {
			m["sender_badges"] = userBadgesJSON(s.visibleUserBadges(senderID, toStringSlice(m["sender_badges"])))
		}
	}
	delete(m, "sender_user_id")
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	ch := redisDMUserChannel(recipientID)
	if err := s.rdb.Publish(ctx, ch, string(b)).Err(); err != nil {
		log.Printf("redis dm call Publish %s: %v", ch, err)
	}
	if payload, ok := s.webPushNotificationFromDM(m); ok {
		s.queueWebPush(recipientID, payload)
	}
}

func dmAvatarURL(srv *Server, objectKey *string) any {
	if objectKey == nil || strings.TrimSpace(*objectKey) == "" {
		return nil
	}
	return srv.glipzProtocolPublicMediaURL(*objectKey)
}

func decodeJSONValue(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func (s *Server) dmThreadToJSON(row repo.DMThreadSummary, canCall bool, badges []string) map[string]any {
	out := map[string]any{
		"id":                row.ID.String(),
		"peer_id":           row.PeerID.String(),
		"peer_handle":       row.PeerHandle,
		"peer_display_name": row.PeerDisplayName,
		"peer_badges":       userBadgesJSON(badges),
		"peer_avatar_url":   dmAvatarURL(s, row.PeerAvatarKey),
		"peer_algorithm":    row.PeerAlgorithm,
		"peer_public_jwk":   decodeJSONValue(row.PeerPublicJWK),
		"unread_count":      row.UnreadCount,
		"created_at":        row.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":        row.UpdatedAt.UTC().Format(time.RFC3339),
		"last_message_at":   nil,
		"can_call":          canCall,
	}
	if row.LastMessageAt != nil {
		out["last_message_at"] = row.LastMessageAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) dmMessageToJSON(row repo.DMMessageViewer, badges []string) map[string]any {
	return map[string]any{
		"id":                  row.ID.String(),
		"sent_by_me":          row.SentByMe,
		"sender_handle":       row.SenderHandle,
		"sender_display_name": row.SenderDisplayName,
		"sender_badges":       userBadgesJSON(badges),
		"sender_avatar_url":   dmAvatarURL(s, row.SenderAvatarKey),
		"ciphertext":          decodeJSONValue(row.Ciphertext),
		"attachments":         decodeJSONValue(row.Attachments),
		"created_at":          row.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) dmCallEventToJSON(row repo.DMCallEvent, badges []string) map[string]any {
	return map[string]any{
		"id":                 row.ID.String(),
		"event_type":         row.EventType,
		"call_mode":          row.CallMode,
		"sent_by_me":         row.SentByMe,
		"actor_handle":       row.ActorHandle,
		"actor_display_name": row.ActorDisplayName,
		"actor_badges":       userBadgesJSON(badges),
		"actor_avatar_url":   dmAvatarURL(s, row.ActorAvatarKey),
		"created_at":         row.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func isAllowedDMAttachmentContentType(ct string) bool {
	ct = strings.TrimSpace(strings.ToLower(ct))
	switch {
	case ct == "":
		return false
	case strings.HasPrefix(ct, "image/"),
		strings.HasPrefix(ct, "video/"),
		strings.HasPrefix(ct, "audio/"),
		strings.HasPrefix(ct, "text/"),
		strings.HasPrefix(ct, "application/"):
		return true
	default:
		return false
	}
}

func validateDMAttachments(userID uuid.UUID, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("[]"), nil
	}
	if !json.Valid(raw) {
		return nil, errors.New("invalid_attachments")
	}
	var items []dmAttachmentRow
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("invalid_attachments")
	}
	if len(items) > 8 {
		return nil, errors.New("too_many_attachments")
	}
	prefix := "uploads/" + userID.String() + "/"
	for _, it := range items {
		if strings.TrimSpace(it.ObjectKey) == "" || !strings.HasPrefix(it.ObjectKey, prefix) {
			return nil, errors.New("invalid_object_key")
		}
		if !isAllowedDMAttachmentContentType(it.ContentType) {
			return nil, errors.New("unsupported_type")
		}
		if len(it.SenderKeyBox) == 0 || !json.Valid(it.SenderKeyBox) || len(it.RecipientKeyBox) == 0 || !json.Valid(it.RecipientKeyBox) {
			return nil, errors.New("invalid_attachment_key_box")
		}
		if strings.TrimSpace(it.FileIV) == "" {
			return nil, errors.New("invalid_attachment_iv")
		}
	}
	return raw, nil
}

func (s *Server) handleGetDMIdentity(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	algorithm, publicJWK, encryptedPrivateJWK, err := s.db.DMIdentityKeyForUser(r.Context(), uid)
	if err != nil {
		if errors.Is(err, repo.ErrDMIdentityUnavailable) {
			writeJSON(w, http.StatusOK, map[string]any{"configured": false})
			return
		}
		writeServerError(w, "DMIdentityKeyForUser", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":            true,
		"algorithm":             algorithm,
		"public_jwk":            decodeJSONValue(publicJWK),
		"encrypted_private_jwk": decodeJSONValue(encryptedPrivateJWK),
	})
}

func (s *Server) handlePutDMIdentity(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req putDMIdentityReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if len(req.PublicJWK) == 0 || !json.Valid(req.PublicJWK) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_public_jwk"})
		return
	}
	if len(req.EncryptedPrivateJWK) == 0 || !json.Valid(req.EncryptedPrivateJWK) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_encrypted_private_jwk"})
		return
	}
	alg := strings.TrimSpace(req.Algorithm)
	if alg == "" {
		alg = "ECDH-P256"
	}
	if err := s.db.UpsertDMIdentityKey(r.Context(), uid, alg, req.PublicJWK, req.EncryptedPrivateJWK); err != nil {
		writeServerError(w, "UpsertDMIdentityKey", err)
		return
	}
	if err := s.db.ProcessDMAutoAcceptPendingInvitesForInvitee(r.Context(), uid); err != nil {
		log.Printf("ProcessDMAutoAcceptPendingInvitesForInvitee: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "algorithm": alg})
}

func (s *Server) handleListDMThreads(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := s.db.ListDMThreads(r.Context(), uid, 50)
	if err != nil {
		writeServerError(w, "ListDMThreads", err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	badgeMap, err := s.userBadgeMap(r.Context(), func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.PeerID)
		}
		return ids
	}())
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs dm threads", err)
		return
	}
	for _, it := range items {
		canCall, err := s.canReceiveDMCall(r.Context(), uid, it.PeerID)
		if err != nil {
			writeServerError(w, "canReceiveDMCall list", err)
			return
		}
		out = append(out, s.dmThreadToJSON(it, canCall, badgeMap[it.PeerID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleCreateDMThread(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if _, _, _, err := s.db.DMIdentityKeyForUser(r.Context(), uid); err != nil {
		if errors.Is(err, repo.ErrDMIdentityUnavailable) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identity_required"})
			return
		}
		writeServerError(w, "DMIdentityKeyForUser sender", err)
		return
	}
	var req createDMThreadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	handle := strings.TrimPrefix(strings.TrimSpace(req.PeerHandle), "@")
	if handle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer_handle"})
		return
	}
	peer, err := s.db.DMContactByHandle(r.Context(), handle)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		case errors.Is(err, repo.ErrDMIdentityUnavailable):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer_identity_required"})
		default:
			writeServerError(w, "DMContactByHandle", err)
		}
		return
	}
	if peer.UserID == uid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_message_self"})
		return
	}
	row, err := s.db.EnsureDMThread(r.Context(), uid, peer.UserID)
	if err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_message_self"})
			return
		}
		writeServerError(w, "EnsureDMThread", err)
		return
	}
	canCall, err := s.canReceiveDMCall(r.Context(), uid, peer.UserID)
	if err != nil {
		writeServerError(w, "canReceiveDMCall create", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), []uuid.UUID{row.PeerID})
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs dm thread create", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"thread": s.dmThreadToJSON(row, canCall, badgeMap[row.PeerID])})
}

// handleInviteDMPeer sends a "join DM" notification when the peer has not set up DM keys yet.
func (s *Server) handleInviteDMPeer(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if _, _, _, err := s.db.DMIdentityKeyForUser(r.Context(), uid); err != nil {
		if errors.Is(err, repo.ErrDMIdentityUnavailable) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identity_required"})
			return
		}
		writeServerError(w, "DMIdentityKeyForUser invite peer", err)
		return
	}
	var req createDMThreadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	handle := strings.TrimPrefix(strings.TrimSpace(req.PeerHandle), "@")
	if handle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), handle)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle invite peer", err)
		return
	}
	if pfl.ID == uid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_message_self"})
		return
	}
	_, _, _, errPeerKey := s.db.DMIdentityKeyForUser(r.Context(), pfl.ID)
	if errPeerKey == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "peer_ready"})
		return
	}
	if !errors.Is(errPeerKey, repo.ErrDMIdentityUnavailable) {
		writeServerError(w, "DMIdentityKeyForUser peer check", errPeerKey)
		return
	}
	autoAccept, errAuto := s.db.UserDMInviteAutoAccept(r.Context(), pfl.ID)
	if errAuto != nil {
		writeServerError(w, "UserDMInviteAutoAccept invite peer", errAuto)
		return
	}
	if autoAccept {
		if err := s.db.UpsertDMAutoAcceptPendingInvite(r.Context(), uid, pfl.ID); err != nil {
			writeServerError(w, "UpsertDMAutoAcceptPendingInvite", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "invited_auto"})
		return
	}
	nid, errN := s.db.InsertNotification(r.Context(), pfl.ID, uid, "dm_invite", nil, nil)
	if errN != nil {
		writeServerError(w, "InsertNotification dm_invite", errN)
		return
	}
	s.publishNotifyUserEvent(r.Context(), pfl.ID, nid)
	writeJSON(w, http.StatusOK, map[string]any{"status": "invited"})
}

func (s *Server) handleGetDMThread(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	row, err := s.db.DMThreadByIDForUser(r.Context(), uid, threadID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DMThreadByIDForUser", err)
		return
	}
	canCall, err := s.canReceiveDMCall(r.Context(), uid, row.PeerID)
	if err != nil {
		writeServerError(w, "canReceiveDMCall get", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), []uuid.UUID{row.PeerID})
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs dm thread get", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": s.dmThreadToJSON(row, canCall, badgeMap[row.PeerID])})
}

func (s *Server) handleListDMMessages(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	var before *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_before"})
			return
		}
		before = &t
	}
	items, err := s.db.ListDMMessages(r.Context(), uid, threadID, before, 50)
	if err != nil {
		writeServerError(w, "ListDMMessages", err)
		return
	}
	if before == nil && len(items) > 0 {
		_ = s.db.MarkDMThreadRead(r.Context(), uid, threadID, items[0].CreatedAt)
	}
	out := make([]map[string]any, 0, len(items))
	badgeMap, err := s.userBadgeMap(r.Context(), func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.SenderID)
		}
		return ids
	}())
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs dm messages", err)
		return
	}
	for _, it := range items {
		out = append(out, s.dmMessageToJSON(it, badgeMap[it.SenderID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleListDMCallHistory(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	items, err := s.db.ListDMCallEvents(r.Context(), uid, threadID, 30)
	if err != nil {
		writeServerError(w, "ListDMCallEvents", err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	badgeMap, err := s.userBadgeMap(r.Context(), func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.ActorID)
		}
		return ids
	}())
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs dm call history", err)
		return
	}
	for _, it := range items {
		out = append(out, s.dmCallEventToJSON(it, badgeMap[it.ActorID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleCreateDMMessage(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	var req createDMMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if len(req.SenderPayload) == 0 || !json.Valid(req.SenderPayload) || len(req.RecipientPayload) == 0 || !json.Valid(req.RecipientPayload) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
		return
	}
	attachments, err := validateDMAttachments(uid, req.Attachments)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.db.CreateDMMessage(r.Context(), repo.CreateDMMessageInput{
		ThreadID:         threadID,
		SenderID:         uid,
		SenderPayload:    req.SenderPayload,
		RecipientPayload: req.RecipientPayload,
		Attachments:      attachments,
	})
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		case errors.Is(err, repo.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		default:
			writeServerError(w, "CreateDMMessage", err)
		}
		return
	}
	msg, err := s.db.DMMessageByIDForViewer(r.Context(), uid, created.ID)
	if err != nil {
		writeServerError(w, "DMMessageByIDForViewer", err)
		return
	}
	s.publishDMUserEvent(r.Context(), created.RecipientID, created.ID)
	badgeMap, err := s.userBadgeMap(r.Context(), []uuid.UUID{msg.SenderID})
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs dm message create", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"message": s.dmMessageToJSON(msg, badgeMap[msg.SenderID])})
}

func (s *Server) handleMarkDMThreadRead(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	if err := s.db.MarkDMThreadRead(r.Context(), uid, threadID, time.Now().UTC()); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "MarkDMThreadRead", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDMUnreadCount(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	n, err := s.db.DMUnreadCount(r.Context(), uid)
	if err != nil {
		writeServerError(w, "DMUnreadCount", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

func (s *Server) handleDMStream(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	flusher, okFlush := w.(http.Flusher)
	if !okFlush {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming_unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	chName := redisDMUserChannel(uid)
	pubsub := s.rdb.Subscribe(ctx, chName)
	defer func() { _ = pubsub.Close() }()
	if _, err := fmt.Fprintf(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()
	msgCh := pubsub.Channel()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			if msg == nil || msg.Payload == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: dm\ndata: %s\n\n", msg.Payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleIssueDMCallToken(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	thread, err := s.db.DMThreadByIDForUser(r.Context(), uid, threadID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DMThreadByIDForUser", err)
		return
	}
	if !s.turnConfigured() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "turn_not_configured"})
		return
	}
	allowedOutgoing, err := s.canReceiveDMCall(r.Context(), uid, thread.PeerID)
	if err != nil {
		writeServerError(w, "canReceiveDMCall", err)
		return
	}
	allowedIncoming, err := s.canReceiveDMCall(r.Context(), thread.PeerID, uid)
	if err != nil {
		writeServerError(w, "canReceiveDMCall reverse", err)
		return
	}
	if !allowedOutgoing && !allowedIncoming {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dm_call_not_allowed"})
		return
	}
	mode := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "audio"
	}
	if mode != "audio" && mode != "video" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_mode"})
		return
	}
	sess, role, err := s.getOrCreateCallSession(r.Context(), threadID, uid, thread.PeerID, mode)
	if err != nil {
		writeServerError(w, "getOrCreateCallSession", err)
		return
	}
	now := time.Now().UTC()
	iceServers := s.dmCallIceServers(sess.CallID, now)
	wsTok, err := authjwt.SignDMCallWS(s.secret, uid, time.Until(sess.ExpiresAt))
	if err != nil {
		writeServerError(w, "SignDMCallWS", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"call_id":        sess.CallID,
		"role":           role,
		"signaling_url":  "/api/v1/dm/calls/" + sess.CallID + "/signaling",
		"ws_token":       wsTok,
		"ice_servers":    iceServers,
		"expires_at":     sess.ExpiresAt.UTC().Format(time.RFC3339),
		"thread_id":      threadID.String(),
		"peer_id":        thread.PeerID.String(),
		"call_mode":      mode,
	})
}

func (s *Server) handleInviteDMCall(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	var req dmCallInviteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	mode := strings.TrimSpace(strings.ToLower(req.Mode))
	if mode != "audio" && mode != "video" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_mode"})
		return
	}
	thread, err := s.db.DMThreadByIDForUser(r.Context(), uid, threadID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DMThreadByIDForUser", err)
		return
	}
	allowed, err := s.canReceiveDMCall(r.Context(), uid, thread.PeerID)
	if err != nil {
		writeServerError(w, "canReceiveDMCall", err)
		return
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dm_call_not_allowed"})
		return
	}
	created, err := s.db.CreateDMCallEvent(r.Context(), threadID, uid, "invite", mode)
	if err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeServerError(w, "CreateDMCallEvent invite", err)
		return
	}
	s.publishDMCallEvent(r.Context(), created.RecipientID, created.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCancelDMCall(w http.ResponseWriter, r *http.Request) {
	s.handleRecordDMCallEvent(w, r, "cancel")
}

func (s *Server) handleEndDMCall(w http.ResponseWriter, r *http.Request) {
	s.handleRecordDMCallEvent(w, r, "end")
}

func (s *Server) handleMissedDMCall(w http.ResponseWriter, r *http.Request) {
	s.handleRecordDMCallEvent(w, r, "missed")
}

func (s *Server) handleRecordDMCallEvent(w http.ResponseWriter, r *http.Request, eventType string) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	thread, err := s.db.DMThreadByIDForUser(r.Context(), uid, threadID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DMThreadByIDForUser", err)
		return
	}
	var req dmCallHistoryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	mode := strings.TrimSpace(strings.ToLower(req.Mode))
	if mode != "audio" && mode != "video" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_mode"})
		return
	}
	created, err := s.db.CreateDMCallEvent(r.Context(), threadID, uid, eventType, mode)
	if err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeServerError(w, "CreateDMCallEvent "+eventType, err)
		return
	}
	s.publishDMCallEvent(r.Context(), thread.PeerID, created.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDMUpload(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := r.ParseMultipartForm(mediaUploadMaxBytes + (2 << 20)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_multipart"})
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_file"})
		return
	}
	defer file.Close()

	ct := strings.TrimSpace(strings.ToLower(hdr.Header.Get("Content-Type")))
	if ct == "" {
		ct = "application/octet-stream"
	}
	if !isAllowedDMAttachmentContentType(ct) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_type"})
		return
	}
	filename := strings.TrimSpace(hdr.Filename)
	ext := ""
	if i := strings.LastIndex(filename, "."); i >= 0 {
		ext = strings.ToLower(filename[i:])
		if len(ext) > 12 {
			ext = ""
		}
	}
	objectKey := "uploads/" + uid.String() + "/" + uuid.NewString() + ext

	tmp, err := os.CreateTemp("", "glipz-dm-*")
	if err != nil {
		writeServerError(w, "dm upload tempfile", err)
		return
	}
	tmpPath := tmp.Name()
	closed := false
	closeTmp := func() {
		if !closed {
			_ = tmp.Close()
			closed = true
		}
		_ = os.Remove(tmpPath)
	}
	defer closeTmp()
	n, err := io.Copy(tmp, io.LimitReader(file, mediaUploadMaxBytes+1))
	if err != nil {
		writeServerError(w, "dm upload read", err)
		return
	}
	if n > mediaUploadMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file_too_large"})
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_file"})
		return
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		writeServerError(w, "dm upload seek", err)
		return
	}
	if err := s.s3.PutObject(r.Context(), objectKey, ct, tmp, n); err != nil {
		writeServerError(w, "dm s3 put", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object_key":   objectKey,
		"public_url":   s.glipzProtocolPublicMediaURL(objectKey),
		"content_type": ct,
		"size_bytes":   n,
		"file_name":    filename,
	})
}
