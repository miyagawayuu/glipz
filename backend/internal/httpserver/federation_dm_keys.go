package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"glipz.io/backend/internal/repo"
)

func dmKeyKID(algorithm string, publicJWK []byte) string {
	algorithm = strings.TrimSpace(strings.ToUpper(algorithm))
	sum := sha256.Sum256(append([]byte(algorithm+"|"), publicJWK...))
	return "dm-" + hex.EncodeToString(sum[:12])
}

func (s *Server) handleFederationDMKeysByHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	handle := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if handle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), handle)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle dm-keys", err)
		return
	}
	algorithm, publicJWK, _, err := s.db.DMIdentityKeyForUser(r.Context(), pfl.ID)
	if err != nil {
		// Do not expose whether a user exists beyond 404.
		if errors.Is(err, repo.ErrDMIdentityUnavailable) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DMIdentityKeyForUser dm-keys", err)
		return
	}
	if len(publicJWK) == 0 || !json.Valid(publicJWK) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"v":         1,
		"acct":      s.localFullAcct(pfl.Handle),
		"kid":       dmKeyKID(algorithm, publicJWK),
		"algorithm": strings.TrimSpace(algorithm),
		"public_jwk": decodeJSONValue(publicJWK),
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

