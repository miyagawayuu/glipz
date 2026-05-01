package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type customEmojiJSON struct {
	ID            string `json:"id"`
	Shortcode     string `json:"shortcode"`
	ShortcodeName string `json:"shortcode_name"`
	OwnerHandle   string `json:"owner_handle,omitempty"`
	Domain        string `json:"domain,omitempty"`
	ImageURL      string `json:"image_url"`
	IsEnabled     bool   `json:"is_enabled"`
	Scope         string `json:"scope"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type customEmojiUpsertReq struct {
	ShortcodeName string  `json:"shortcode_name"`
	ObjectKey     *string `json:"object_key"`
	IsEnabled     *bool   `json:"is_enabled"`
}

func (s *Server) customEmojiJSON(row repo.CustomEmoji) customEmojiJSON {
	scope := "site"
	if row.OwnerUserID != nil {
		scope = "user"
	}
	if strings.TrimSpace(row.Domain) != "" {
		scope = "remote"
	}
	return customEmojiJSON{
		ID:            row.ID.String(),
		Shortcode:     repo.MakeCustomEmojiShortcode(row.ShortcodeName, row.OwnerHandle, row.Domain),
		ShortcodeName: row.ShortcodeName,
		OwnerHandle:   row.OwnerHandle,
		Domain:        row.Domain,
		ImageURL:      s.glipzProtocolPublicMediaURL(row.ObjectKey),
		IsEnabled:     row.IsEnabled,
		Scope:         scope,
		CreatedAt:     row.CreatedAt.UTC().Format(http.TimeFormat),
		UpdatedAt:     row.UpdatedAt.UTC().Format(http.TimeFormat),
	}
}

func (s *Server) customEmojiListJSON(rows []repo.CustomEmoji) []customEmojiJSON {
	out := make([]customEmojiJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.customEmojiJSON(row))
	}
	return out
}

func validateOwnedObjectKey(uid uuid.UUID, objectKey string) bool {
	prefix := "uploads/" + uid.String() + "/"
	key, ok := normalizePublicMediaObjectKey(objectKey)
	return ok && strings.HasPrefix(key, prefix)
}

func (s *Server) handleListEnabledCustomEmojis(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListEnabledCustomEmojis(r.Context())
	if err != nil {
		writeServerError(w, "ListEnabledCustomEmojis", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.customEmojiListJSON(rows)})
}

func (s *Server) handleMeListCustomEmojis(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.ListUserCustomEmojis(r.Context(), uid)
	if err != nil {
		writeServerError(w, "ListUserCustomEmojis", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.customEmojiListJSON(rows)})
}

func (s *Server) handleAdminListSiteCustomEmojis(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListSiteCustomEmojis(r.Context())
	if err != nil {
		writeServerError(w, "ListSiteCustomEmojis", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.customEmojiListJSON(rows)})
}

func readCustomEmojiUpsertReq(w http.ResponseWriter, r *http.Request) (customEmojiUpsertReq, bool) {
	var req customEmojiUpsertReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return customEmojiUpsertReq{}, false
	}
	return req, true
}

func parseCustomEmojiID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	emojiID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "emojiID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_emoji_id"})
		return uuid.Nil, false
	}
	return emojiID, true
}

func writeCustomEmojiRepoError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.Is(err, repo.ErrCustomEmojiConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "shortcode_conflict"})
	case errors.Is(err, repo.ErrInvalidCustomEmoji):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_custom_emoji"})
	default:
		writeServerError(w, action, err)
	}
}

func (s *Server) handleMeCreateCustomEmoji(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	req, ok := readCustomEmojiUpsertReq(w, r)
	if !ok {
		return
	}
	if req.ObjectKey == nil || !validateOwnedObjectKey(uid, *req.ObjectKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_object_key"})
		return
	}
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	row, err := s.db.CreateUserCustomEmoji(r.Context(), uid, req.ShortcodeName, *req.ObjectKey, enabled)
	if err != nil {
		writeCustomEmojiRepoError(w, "CreateUserCustomEmoji", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.customEmojiJSON(row)})
}

func (s *Server) handleMePatchCustomEmoji(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	emojiID, ok := parseCustomEmojiID(w, r)
	if !ok {
		return
	}
	req, ok := readCustomEmojiUpsertReq(w, r)
	if !ok {
		return
	}
	if req.ObjectKey != nil && !validateOwnedObjectKey(uid, *req.ObjectKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_object_key"})
		return
	}
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	row, err := s.db.UpdateUserCustomEmoji(r.Context(), uid, emojiID, req.ShortcodeName, req.ObjectKey, enabled)
	if err != nil {
		writeCustomEmojiRepoError(w, "UpdateUserCustomEmoji", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.customEmojiJSON(row)})
}

func (s *Server) handleMeDeleteCustomEmoji(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	emojiID, ok := parseCustomEmojiID(w, r)
	if !ok {
		return
	}
	if err := s.db.DeleteUserCustomEmoji(r.Context(), uid, emojiID); err != nil {
		writeCustomEmojiRepoError(w, "DeleteUserCustomEmoji", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminCreateSiteCustomEmoji(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	req, ok := readCustomEmojiUpsertReq(w, r)
	if !ok {
		return
	}
	if req.ObjectKey == nil || !validateOwnedObjectKey(uid, *req.ObjectKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_object_key"})
		return
	}
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	row, err := s.db.CreateSiteCustomEmoji(r.Context(), req.ShortcodeName, *req.ObjectKey, enabled)
	if err != nil {
		writeCustomEmojiRepoError(w, "CreateSiteCustomEmoji", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.customEmojiJSON(row)})
}

func (s *Server) handleAdminPatchSiteCustomEmoji(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	emojiID, ok := parseCustomEmojiID(w, r)
	if !ok {
		return
	}
	req, ok := readCustomEmojiUpsertReq(w, r)
	if !ok {
		return
	}
	if req.ObjectKey != nil && !validateOwnedObjectKey(uid, *req.ObjectKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_object_key"})
		return
	}
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	row, err := s.db.UpdateSiteCustomEmoji(r.Context(), emojiID, req.ShortcodeName, req.ObjectKey, enabled)
	if err != nil {
		writeCustomEmojiRepoError(w, "UpdateSiteCustomEmoji", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.customEmojiJSON(row)})
}

func (s *Server) handleAdminDeleteSiteCustomEmoji(w http.ResponseWriter, r *http.Request) {
	emojiID, ok := parseCustomEmojiID(w, r)
	if !ok {
		return
	}
	if err := s.db.DeleteSiteCustomEmoji(r.Context(), emojiID); err != nil {
		writeCustomEmojiRepoError(w, "DeleteSiteCustomEmoji", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
