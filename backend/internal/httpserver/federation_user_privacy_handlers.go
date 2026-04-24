package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

func federatedActorKey(row repo.FederatedIncomingPost) string {
	k := strings.TrimSpace(row.ActorAcct)
	if k == "" {
		k = strings.TrimSpace(row.ActorIRI)
	}
	return k
}

func (s *Server) federatedIncomingHiddenFromViewer(ctx context.Context, viewer uuid.UUID, row repo.FederatedIncomingPost) (bool, error) {
	if viewer == uuid.Nil {
		return false, nil
	}
	key := repo.NormalizeFederationTargetAcct(federatedActorKey(row))
	if key == "" {
		return false, nil
	}
	b, err := s.db.HasFederationUserBlock(ctx, viewer, key)
	if err != nil || b {
		return b, err
	}
	return s.db.HasFederationUserMute(ctx, viewer, key)
}

// rejectIfFederatedIncomingHidden writes not_found and returns true when the viewer has blocked or muted the remote author.
func (s *Server) rejectIfFederatedIncomingHidden(w http.ResponseWriter, r *http.Request, uid uuid.UUID, row repo.FederatedIncomingPost) bool {
	hidden, err := s.federatedIncomingHiddenFromViewer(r.Context(), uid, row)
	if err != nil {
		writeServerError(w, "federatedIncomingHiddenFromViewer", err)
		return true
	}
	if hidden {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return true
	}
	return false
}

type federationPrivacyAcctReq struct {
	TargetAcct string `json:"target_acct"`
}

func (s *Server) handleMeFederationBlocksList(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.ListFederationUserBlocks(r.Context(), uid, 300)
	if err != nil {
		writeServerError(w, "ListFederationUserBlocks", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleMeFederationMutesList(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.ListFederationUserMutes(r.Context(), uid, 300)
	if err != nil {
		writeServerError(w, "ListFederationUserMutes", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleMeFederationBlocksAdd(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<10))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	var req federationPrivacyAcctReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := s.db.AddFederationUserBlock(r.Context(), uid, req.TargetAcct); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_acct"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeFederationBlocksRemove(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	acct := repo.NormalizeFederationTargetAcct(r.URL.Query().Get("target_acct"))
	if acct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_target_acct"})
		return
	}
	if err := s.db.RemoveFederationUserBlock(r.Context(), uid, acct); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "RemoveFederationUserBlock", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeFederationMutesAdd(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<10))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	var req federationPrivacyAcctReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := s.db.AddFederationUserMute(r.Context(), uid, req.TargetAcct); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_acct"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeFederationMutesRemove(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	acct := repo.NormalizeFederationTargetAcct(r.URL.Query().Get("target_acct"))
	if acct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_target_acct"})
		return
	}
	if err := s.db.RemoveFederationUserMute(r.Context(), uid, acct); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "RemoveFederationUserMute", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeFederationRelationshipGET(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	acct := repo.NormalizeFederationTargetAcct(r.URL.Query().Get("target_acct"))
	if acct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_target_acct"})
		return
	}
	blocked, muted, err := s.db.FederationUserPrivacyRelationship(r.Context(), uid, acct)
	if err != nil {
		writeServerError(w, "FederationUserPrivacyRelationship", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocked": blocked, "muted": muted})
}
