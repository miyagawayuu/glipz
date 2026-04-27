package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type identityBundle struct {
	V                          int                      `json:"v"`
	PortableID                 string                   `json:"portable_id"`
	AccountPublicKey           string                   `json:"account_public_key"`
	AccountPrivateKeyEncrypted string                   `json:"account_private_key_encrypted,omitempty"`
	PrivateKey                 *encryptedIdentitySecret `json:"private_key,omitempty"`
	Handle                     string                   `json:"handle"`
	DisplayName                string                   `json:"display_name"`
	Bio                        string                   `json:"bio"`
	AlsoKnownAs                []string                 `json:"also_known_as,omitempty"`
	CreatedForOrigin           string                   `json:"created_for_origin,omitempty"`
	ExportedAt                 string                   `json:"exported_at"`
}

func (s *Server) handleMeIdentityExport(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserByID identity export", err)
		return
	}
	identity, err := s.db.EnsureUserPortableIdentity(r.Context(), uid)
	if err != nil {
		writeServerError(w, "EnsureUserPortableIdentity export", err)
		return
	}
	writeJSON(w, http.StatusOK, identityBundle{
		V:                          1,
		PortableID:                 identity.PortableID,
		AccountPublicKey:           identity.AccountPublicKey,
		AccountPrivateKeyEncrypted: identity.AccountPrivateKeyEncrypted,
		Handle:                     u.Handle,
		DisplayName:                u.DisplayName,
		Bio:                        u.Bio,
		AlsoKnownAs:                append([]string(nil), u.AlsoKnownAs...),
		ExportedAt:                 time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleMeIdentityImport(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req identityBundle
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := s.db.SetUserPortableIdentity(r.Context(), uid, repo.PortableIdentity{
		PortableID:                 req.PortableID,
		AccountPublicKey:           req.AccountPublicKey,
		AccountPrivateKeyEncrypted: req.AccountPrivateKeyEncrypted,
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_identity_bundle"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeIdentityMove(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req struct {
		MovedToAcct string `json:"moved_to_acct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	movedTo := repo.NormalizeFederationTargetAcct(req.MovedToAcct)
	_, movedHost, err := splitAcct(movedTo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_moved_to_acct"})
		return
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserByID identity move", err)
		return
	}
	identity, err := s.db.EnsureUserPortableIdentity(r.Context(), uid)
	if err != nil {
		writeServerError(w, "EnsureUserPortableIdentity move", err)
		return
	}
	if !strings.EqualFold(movedHost, s.federationDisplayHost()) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		doc, err := fetchRemoteFederationAccount(ctx, movedTo)
		if err != nil || doc.Account == nil ||
			strings.TrimSpace(doc.Account.ID) != strings.TrimSpace(identity.PortableID) ||
			strings.TrimSpace(doc.Account.PublicKey) != strings.TrimSpace(identity.AccountPublicKey) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_identity_mismatch"})
			return
		}
	}
	if err := s.db.MarkUserMoved(r.Context(), uid, movedTo); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_moved_to_acct"})
		return
	}
	oldAcct := s.localFullAcct(u.Handle)
	inboxes, _ := s.db.ListFederationSubscriberInboxes(r.Context(), uid)
	if len(inboxes) > 0 {
		ev := federationEventEnvelope{
			V:    federationEventSchemaVersion,
			Kind: "account_moved",
			Author: federationEventAuthor{
				ID:          identity.PortableID,
				Acct:        oldAcct,
				Handle:      u.Handle,
				Domain:      s.federationDisplayHost(),
				DisplayName: resolvedDisplayName(u.DisplayName, u.Email),
				ProfileURL:  s.localProfileURL(u.Handle),
				PublicKey:   identity.AccountPublicKey,
			},
			Move: &federationAccountMove{
				PortableID: identity.PortableID,
				OldAcct:    oldAcct,
				NewAcct:    movedTo,
				InboxURL:   strings.TrimSuffix(s.federationPublicOrigin(), "/") + "/federation/events",
				PublicKey:  identity.AccountPublicKey,
			},
		}
		s.enqueueFederationPayload(r.Context(), uid, uuid.New(), inboxes, ev)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "moved_to_acct": movedTo})
}
