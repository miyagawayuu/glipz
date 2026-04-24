package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type createNoteReq struct {
	Title                       string `json:"title"`
	BodyMd                      string `json:"body_md"`
	BodyPremiumMd               string `json:"body_premium_md"`
	EditorMode                  string `json:"editor_mode"`
	Status                      string `json:"status"`
	Visibility                  string `json:"visibility"`
	PatreonCampaignID           string `json:"patreon_campaign_id"`
	PatreonRequiredRewardTierID string `json:"patreon_required_reward_tier_id"`
}

type patchNoteReq struct {
	Title                       string `json:"title"`
	BodyMd                      string `json:"body_md"`
	BodyPremiumMd               string `json:"body_premium_md"`
	EditorMode                  string `json:"editor_mode"`
	Status                      string `json:"status"`
	Visibility                  string `json:"visibility"`
	PatreonCampaignID           string `json:"patreon_campaign_id"`
	PatreonRequiredRewardTierID string `json:"patreon_required_reward_tier_id"`
}

func (s *Server) noteToJSON(row repo.NoteWithAuthor, viewer uuid.UUID, bodyPremiumOut string, premiumLocked bool) map[string]any {
	avatarURL := ""
	if row.AuthorAvatarKey != nil {
		if k := strings.TrimSpace(*row.AuthorAvatarKey); k != "" {
			avatarURL = s.glipzProtocolPublicMediaURL(k)
		}
	}
	out := map[string]any{
		"id":                row.ID.String(),
		"title":             row.Title,
		"body_md":           row.BodyMd,
		"body_premium_md":   bodyPremiumOut,
		"editor_mode":       row.EditorMode,
		"status":            row.Status,
		"visibility":        row.Visibility,
		"created_at":        row.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":        row.UpdatedAt.UTC().Format(time.RFC3339),
		"user_id":           row.UserID.String(),
		"user_handle":       row.AuthorHandle,
		"user_display_name": resolvedDisplayName(row.AuthorDisplayName, row.AuthorEmail),
		"user_avatar_url":   avatarURL,
		"is_owner":          row.UserID == viewer,
		"premium_locked":    premiumLocked,
	}
	if row.PatreonCampaignID != nil && strings.TrimSpace(*row.PatreonCampaignID) != "" {
		out["patreon_campaign_id"] = strings.TrimSpace(*row.PatreonCampaignID)
	} else {
		out["patreon_campaign_id"] = nil
	}
	if row.PatreonRequiredRewardTierID != nil && strings.TrimSpace(*row.PatreonRequiredRewardTierID) != "" {
		out["patreon_required_reward_tier_id"] = strings.TrimSpace(*row.PatreonRequiredRewardTierID)
	} else {
		out["patreon_required_reward_tier_id"] = nil
	}
	return out
}

func (s *Server) handleNoteCreate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req createNoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	id, err := s.db.CreateNote(r.Context(), uid, req.Title, req.BodyMd, req.BodyPremiumMd, req.EditorMode, req.Status, req.Visibility, req.PatreonCampaignID, req.PatreonRequiredRewardTierID)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrEmptyNote):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_note"})
		case errors.Is(err, repo.ErrTitleTooLong):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title_too_long"})
		case errors.Is(err, repo.ErrBodyTooLong):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body_too_long"})
		default:
			writeServerError(w, "CreateNote", err)
		}
		return
	}
	row, err := s.db.NoteByID(r.Context(), id)
	if err != nil {
		writeServerError(w, "NoteByID after create", err)
		return
	}
	premOut, locked := s.notePremiumProjection(r.Context(), row, uid)
	writeJSON(w, http.StatusCreated, map[string]any{"note": s.noteToJSON(row, uid, premOut, locked)})
}

func (s *Server) handleNoteGet(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	noteID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "noteID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_note_id"})
		return
	}
	row, err := s.db.NoteByID(r.Context(), noteID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "NoteByID", err)
		return
	}
	if err := s.assertNoteReadable(r.Context(), row, uid); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	premOut, locked := s.notePremiumProjection(r.Context(), row, uid)
	writeJSON(w, http.StatusOK, map[string]any{"note": s.noteToJSON(row, uid, premOut, locked)})
}

func (s *Server) assertNoteReadable(ctx context.Context, row repo.NoteWithAuthor, viewer uuid.UUID) error {
	if row.UserID == viewer {
		return nil
	}
	if row.Status != "published" {
		return repo.ErrNotFound
	}
	switch row.Visibility {
	case "private":
		return repo.ErrNotFound
	case "followers":
		ok, err := s.db.IsFollowing(ctx, viewer, row.UserID)
		if err != nil || !ok {
			return repo.ErrNotFound
		}
	}
	return nil
}

func (s *Server) notePremiumProjection(ctx context.Context, row repo.NoteWithAuthor, viewer uuid.UUID) (bodyPremiumOut string, premiumLocked bool) {
	premium := strings.TrimSpace(row.BodyPremiumMd)
	if premium == "" {
		return "", false
	}
	if row.UserID == viewer {
		return row.BodyPremiumMd, false
	}
	author, err := s.db.UserByID(ctx, row.UserID)
	if err != nil {
		return "", true
	}
	camp := ""
	tier := ""
	if row.PatreonCampaignID != nil {
		camp = strings.TrimSpace(*row.PatreonCampaignID)
	}
	if camp == "" && author.PatreonCampaignID != nil {
		camp = strings.TrimSpace(*author.PatreonCampaignID)
	}
	if row.PatreonRequiredRewardTierID != nil {
		tier = strings.TrimSpace(*row.PatreonRequiredRewardTierID)
	}
	if tier == "" && author.PatreonRequiredRewardTierID != nil {
		tier = strings.TrimSpace(*author.PatreonRequiredRewardTierID)
	}
	if camp == "" || tier == "" {
		return "", true
	}
	// Note paywall: Patreon (fanclub). Additional providers can branch here using row/author paywall fields.
	ok, err := s.viewerEntitledToAuthorPatreonTier(ctx, viewer, row.UserID, camp, tier)
	if err != nil || !ok {
		return "", true
	}
	return row.BodyPremiumMd, false
}

func (s *Server) handleNotePatch(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	noteID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "noteID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_note_id"})
		return
	}
	var req patchNoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	err = s.db.UpdateNote(r.Context(), uid, noteID, req.Title, req.BodyMd, req.BodyPremiumMd, req.EditorMode, req.Status, req.Visibility, req.PatreonCampaignID, req.PatreonRequiredRewardTierID)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		case errors.Is(err, repo.ErrEmptyNote):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_note"})
		case errors.Is(err, repo.ErrTitleTooLong):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title_too_long"})
		case errors.Is(err, repo.ErrBodyTooLong):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body_too_long"})
		default:
			writeServerError(w, "UpdateNote", err)
		}
		return
	}
	row, err := s.db.NoteByID(r.Context(), noteID)
	if err != nil {
		writeServerError(w, "NoteByID after patch", err)
		return
	}
	premOut, locked := s.notePremiumProjection(r.Context(), row, uid)
	writeJSON(w, http.StatusOK, map[string]any{"note": s.noteToJSON(row, uid, premOut, locked)})
}

func (s *Server) handleNoteDelete(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	noteID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "noteID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_note_id"})
		return
	}
	err = s.db.DeleteNote(r.Context(), uid, noteID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DeleteNote", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
