package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

func userBadgesJSON(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return append([]string(nil), in...)
}

func toStringSlice(v any) []string {
	raw, ok := v.([]any)
	if ok {
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if raw, ok := v.([]string); ok {
		return append([]string(nil), raw...)
	}
	return nil
}

func (s *Server) visibleUserBadges(userID uuid.UUID, stored []string) []string {
	return repo.VisibleUserBadges(stored, s.isSiteAdmin(userID))
}

func (s *Server) userBadgeMap(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]string, error) {
	storedMap, err := s.db.ListUserBadgesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]string, len(storedMap))
	for id, stored := range storedMap {
		out[id] = s.visibleUserBadges(id, stored)
	}
	return out, nil
}

type adminUpdateUserBadgesReq struct {
	Badges []string `json:"badges"`
}

func (s *Server) handleAdminGetUserBadges(w http.ResponseWriter, r *http.Request) {
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
		writeServerError(w, "PublicProfileByHandle admin badges", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available_badges": repo.AvailableUserBadges(),
		"user": map[string]any{
			"id":             pfl.ID.String(),
			"handle":         pfl.Handle,
			"display_name":   resolvedDisplayName(pfl.DisplayName, pfl.Email),
			"badges":         userBadgesJSON(repo.AdminSelectableUserBadges(pfl.Badges)),
			"visible_badges": userBadgesJSON(s.visibleUserBadges(pfl.ID, pfl.Badges)),
		},
	})
}

func (s *Server) handleAdminPutUserBadges(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if handle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	var req adminUpdateUserBadgesReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), handle)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle admin badges update", err)
		return
	}
	badges := repo.AdminManagedUserBadges(pfl.Badges, req.Badges)
	if err := s.db.UpdateUserBadges(r.Context(), pfl.ID, badges); err != nil {
		writeServerError(w, "UpdateUserBadges", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":             pfl.ID.String(),
			"handle":         pfl.Handle,
			"display_name":   resolvedDisplayName(pfl.DisplayName, pfl.Email),
			"badges":         userBadgesJSON(repo.AdminSelectableUserBadges(badges)),
			"visible_badges": userBadgesJSON(s.visibleUserBadges(pfl.ID, badges)),
		},
	})
}
