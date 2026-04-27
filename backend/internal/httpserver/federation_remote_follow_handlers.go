package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"glipz.io/backend/internal/repo"
)

func (s *Server) glipzProtocolConfigured(w http.ResponseWriter) bool {
	if strings.TrimSpace(s.federationPublicOrigin()) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "federation_disabled"})
		return false
	}
	return true
}

func resolveErrorHTTPStatus(err error) int {
	if errors.Is(err, errResolveBadInput) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func deliveryFailureAPIError(err error) string {
	if err == nil {
		return "delivery_failed"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "status 401"):
		return "delivery_unauthorized"
	case strings.Contains(s, "status 403"):
		return "delivery_forbidden"
	case strings.Contains(s, "status 404"):
		return "delivery_not_found"
	case strings.Contains(s, "status 429"):
		return "delivery_rate_limited"
	case strings.Contains(s, "status 5"):
		return "delivery_upstream_error"
	default:
		return "delivery_failed"
	}
}

func remoteFollowTargetFromRequest(r *http.Request) (raw string, err error) {
	if a := strings.TrimSpace(r.URL.Query().Get("actor")); a != "" {
		return a, nil
	}
	if a := strings.TrimSpace(r.URL.Query().Get("acct")); a != "" {
		return a, nil
	}
	var body struct {
		Acct  string `json:"acct"`
		Actor string `json:"actor"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 8192))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if a := strings.TrimSpace(body.Actor); a != "" {
		return a, nil
	}
	if a := strings.TrimSpace(body.Acct); a != "" {
		return a, nil
	}
	return "", errResolveBadInput
}

func (s *Server) handleRemoteFollowPOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.glipzProtocolConfigured(w) {
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		Acct  string `json:"acct"`
		Actor string `json:"actor"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
		return
	}
	raw := strings.TrimSpace(body.Actor)
	if raw == "" {
		raw = strings.TrimSpace(body.Acct)
	}
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_acct_or_actor"})
		return
	}
	resolved, err := ResolveRemoteActor(r.Context(), raw)
	if err != nil {
		st := resolveErrorHTTPStatus(err)
		msg := ResolveFailureAPIError(err)
		log.Printf("remote-follow resolve (POST): input=%q code=%s err=%v", raw, msg, err)
		writeJSON(w, st, map[string]string{"error": msg})
		return
	}
	remoteAccount, remoteAccountErr := s.rememberResolvedRemoteAccount(r.Context(), resolved)
	existing, err := s.db.GetRemoteFollow(r.Context(), uid, resolved.ActorID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		writeServerError(w, "GetRemoteFollow", err)
		return
	}
	if err == nil && existing.State == "accepted" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": "accepted", "remote_actor_id": resolved.ActorID})
		return
	}
	handle, err := s.db.UserHandleByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "remote follow handle", err)
		return
	}
	nh, err := repo.NormalizeHandle(handle)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_handle"})
		return
	}
	deliveryURL := resolved.DeliveryInbox()
	reqBody := federationFollowRequest{
		EventID:      federationNewEventID(),
		FollowerAcct: s.localFullAcct(nh),
		TargetAcct:   resolved.Acct,
		InboxURL:     strings.TrimSuffix(s.federationPublicOrigin(), "/") + "/federation/events",
	}
	if err := s.signedFederationPOST(r.Context(), resolved.FollowURL, reqBody); err != nil {
		code := deliveryFailureAPIError(err)
		log.Printf("remote-follow deliver: inbox=%s code=%s err=%v", deliveryURL, code, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": code})
		return
	}
	if err := s.db.UpsertRemoteFollowAccepted(r.Context(), uid, resolved.ActorID, deliveryURL); err != nil {
		writeServerError(w, "UpsertRemoteFollowAccepted", err)
		return
	}
	if remoteAccountErr == nil {
		_ = s.db.AttachRemoteAccountToRemoteFollow(r.Context(), uid, resolved.ActorID, remoteAccount.ID, resolved.Acct)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": "accepted", "remote_actor_id": resolved.ActorID})
}

func (s *Server) handleRemoteFollowGET(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.glipzProtocolConfigured(w) {
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.ListRemoteFollowsForUser(r.Context(), uid)
	if err != nil {
		writeServerError(w, "ListRemoteFollowsForUser", err)
		return
	}
	type item struct {
		RemoteActorID string `json:"remote_actor_id"`
		State         string `json:"state"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		out = append(out, item{RemoteActorID: row.RemoteActorID, State: row.State})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleRemoteFollowDELETE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.glipzProtocolConfigured(w) {
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	raw, err := remoteFollowTargetFromRequest(r)
	if err != nil {
		if errors.Is(err, errResolveBadInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_acct_or_actor"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
		return
	}
	resolved, err := ResolveRemoteActor(r.Context(), raw)
	if err != nil {
		st := resolveErrorHTTPStatus(err)
		msg := ResolveFailureAPIError(err)
		log.Printf("remote-follow resolve (DELETE): input=%q code=%s err=%v", raw, msg, err)
		writeJSON(w, st, map[string]string{"error": msg})
		return
	}
	_, _ = s.rememberResolvedRemoteAccount(r.Context(), resolved)
	if _, err := s.db.GetRemoteFollow(r.Context(), uid, resolved.ActorID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_following"})
			return
		}
		writeServerError(w, "GetRemoteFollow", err)
		return
	}
	handle, err := s.db.UserHandleByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "remote unfollow handle", err)
		return
	}
	nh, err := repo.NormalizeHandle(handle)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_handle"})
		return
	}
	reqBody := federationFollowRequest{
		EventID:      federationNewEventID(),
		FollowerAcct: s.localFullAcct(nh),
		TargetAcct:   resolved.Acct,
		InboxURL:     strings.TrimSuffix(s.federationPublicOrigin(), "/") + "/federation/events",
	}
	if err := s.signedFederationPOST(r.Context(), resolved.UnfollowURL, reqBody); err != nil {
		log.Printf("remote unfollow Undo: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "delivery_failed"})
		return
	}
	if err := s.db.DeleteRemoteFollow(r.Context(), uid, resolved.ActorID); err != nil {
		writeServerError(w, "DeleteRemoteFollow", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
