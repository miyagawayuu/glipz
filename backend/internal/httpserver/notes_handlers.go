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
	"golang.org/x/crypto/bcrypt"

	"glipz.io/backend/internal/repo"
)

type createNoteReq struct {
	Title                       string `json:"title"`
	BodyMd                      string `json:"body_md"`
	BodyPremiumMd               string `json:"body_premium_md"`
	PaywallProvider             string `json:"paywall_provider"`
	PatreonCampaignID           string `json:"patreon_campaign_id"`
	PatreonRequiredRewardTierID string `json:"patreon_required_reward_tier_id"`
	EditorMode                  string `json:"editor_mode"`
	Status                      string `json:"status"`
	Visibility                  string `json:"visibility"`
	ViewPassword                string `json:"view_password"`
	ViewPasswordHint            string `json:"view_password_hint"`
}

type patchNoteReq struct {
	Title                       string `json:"title"`
	BodyMd                      string `json:"body_md"`
	BodyPremiumMd               string `json:"body_premium_md"`
	PaywallProvider             *string `json:"paywall_provider"`
	PatreonCampaignID           *string `json:"patreon_campaign_id"`
	PatreonRequiredRewardTierID *string `json:"patreon_required_reward_tier_id"`
	EditorMode                  string `json:"editor_mode"`
	Status                      string `json:"status"`
	Visibility                  string `json:"visibility"`
	ViewPassword                *string `json:"view_password"`
	ClearViewPassword           bool    `json:"clear_view_password"`
	ViewPasswordHint            *string `json:"view_password_hint"`
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
	if row.ViewPasswordHash != nil && strings.TrimSpace(*row.ViewPasswordHash) != "" {
		out["has_view_password"] = true
	} else {
		out["has_view_password"] = false
	}
	if row.ViewPasswordHint != nil && strings.TrimSpace(*row.ViewPasswordHint) != "" {
		out["view_password_hint"] = strings.TrimSpace(*row.ViewPasswordHint)
	} else {
		out["view_password_hint"] = nil
	}
	pp := ""
	if row.PatreonCampaignID != nil && strings.TrimSpace(*row.PatreonCampaignID) != "" &&
		row.PatreonRequiredRewardTierID != nil && strings.TrimSpace(*row.PatreonRequiredRewardTierID) != "" {
		pp = "patreon"
	}
	out["paywall_provider"] = pp
	if row.PatreonCampaignID != nil && strings.TrimSpace(*row.PatreonCampaignID) != "" {
		out["patreon_campaign_id"] = strings.TrimSpace(*row.PatreonCampaignID)
	} else {
		out["patreon_campaign_id"] = ""
	}
	if row.PatreonRequiredRewardTierID != nil && strings.TrimSpace(*row.PatreonRequiredRewardTierID) != "" {
		out["patreon_required_reward_tier_id"] = strings.TrimSpace(*row.PatreonRequiredRewardTierID)
	} else {
		out["patreon_required_reward_tier_id"] = ""
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
	pw := strings.TrimSpace(req.ViewPassword)
	pwh := strings.TrimSpace(req.ViewPasswordHint)
	hash := ""
	if pw != "" {
		hb, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			writeServerError(w, "note password bcrypt", err)
			return
		}
		hash = string(hb)
	}
	paywallProvider := strings.ToLower(strings.TrimSpace(req.PaywallProvider))
	pc := ""
	pt := ""
	if paywallProvider == "patreon" {
		pc = strings.TrimSpace(req.PatreonCampaignID)
		pt = strings.TrimSpace(req.PatreonRequiredRewardTierID)
	}
	id, err := s.db.CreateNote(r.Context(), uid, req.Title, req.BodyMd, req.BodyPremiumMd, req.EditorMode, req.Status, req.Visibility, hash, pwh, pc, pt)
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
	if row.Status == "published" && row.Visibility != "private" {
		s.deliverFederationNoteCreate(r.Context(), uid, row.ID)
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
	// Patreon (fanclub) entitlement for local viewers: if configured for the note (or author default), unlocked without password.
	camp := ""
	tier := ""
	if row.PatreonCampaignID != nil {
		camp = strings.TrimSpace(*row.PatreonCampaignID)
	}
	if row.PatreonRequiredRewardTierID != nil {
		tier = strings.TrimSpace(*row.PatreonRequiredRewardTierID)
	}
	if camp == "" || tier == "" {
		au, err := s.db.UserByID(ctx, row.UserID)
		if err == nil {
			if camp == "" && au.PatreonCampaignID != nil {
				camp = strings.TrimSpace(*au.PatreonCampaignID)
			}
			if tier == "" && au.PatreonRequiredRewardTierID != nil {
				tier = strings.TrimSpace(*au.PatreonRequiredRewardTierID)
			}
		}
	}
	if camp != "" && tier != "" {
		ok, err := s.viewerEntitledToAuthorPatreonTier(ctx, viewer, row.UserID, camp, tier)
		if err == nil && ok {
			return row.BodyPremiumMd, false
		}
	}
	if row.ViewPasswordHash == nil || strings.TrimSpace(*row.ViewPasswordHash) == "" {
		return "", true
	}
	rk := noteUnlockRedisKey(viewer, row.ID)
	n, err := s.rdb.Exists(ctx, rk).Result()
	if err != nil || n == 0 {
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
	before, err := s.db.NoteByID(r.Context(), noteID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		writeServerError(w, "NoteByID before patch", err)
		return
	}
	beforeWasFederated := err == nil && before.UserID == uid && before.Status == "published" && before.Visibility != "private"
	var req patchNoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	// Compute new password hash/hint values based on optional patch semantics.
	newHash := ""
	if before.ViewPasswordHash != nil {
		newHash = strings.TrimSpace(*before.ViewPasswordHash)
	}
	newHint := ""
	if before.ViewPasswordHint != nil {
		newHint = strings.TrimSpace(*before.ViewPasswordHint)
	}
	if req.ClearViewPassword && req.ViewPassword != nil && strings.TrimSpace(*req.ViewPassword) != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_conflict"})
		return
	}
	if req.ClearViewPassword {
		newHash = ""
	}
	if req.ViewPassword != nil {
		pw := strings.TrimSpace(*req.ViewPassword)
		if pw != "" {
			hb, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
			if err != nil {
				writeServerError(w, "note password bcrypt", err)
				return
			}
			newHash = string(hb)
		}
	}
	if req.ViewPasswordHint != nil {
		newHint = strings.TrimSpace(*req.ViewPasswordHint)
	}
	// Paywall (Patreon) patch semantics.
	pc := ""
	pt := ""
	if before.PatreonCampaignID != nil {
		pc = strings.TrimSpace(*before.PatreonCampaignID)
	}
	if before.PatreonRequiredRewardTierID != nil {
		pt = strings.TrimSpace(*before.PatreonRequiredRewardTierID)
	}
	pp := ""
	if pc != "" && pt != "" {
		pp = "patreon"
	}
	if req.PaywallProvider != nil {
		pp = strings.ToLower(strings.TrimSpace(*req.PaywallProvider))
		if pp != "patreon" {
			pp = ""
			pc = ""
			pt = ""
		}
	}
	if req.PatreonCampaignID != nil {
		pc = strings.TrimSpace(*req.PatreonCampaignID)
	}
	if req.PatreonRequiredRewardTierID != nil {
		pt = strings.TrimSpace(*req.PatreonRequiredRewardTierID)
	}
	if pp != "patreon" {
		pc = ""
		pt = ""
	}
	err = s.db.UpdateNote(r.Context(), uid, noteID, req.Title, req.BodyMd, req.BodyPremiumMd, req.EditorMode, req.Status, req.Visibility, newHash, newHint, pc, pt)
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
	afterIsFederated := row.Status == "published" && row.Visibility != "private"
	switch {
	case afterIsFederated:
		s.deliverFederationNoteUpdate(r.Context(), uid, row)
	case beforeWasFederated && !afterIsFederated:
		s.deliverFederationNoteDelete(r.Context(), uid, row.ID)
	}
	premOut, locked := s.notePremiumProjection(r.Context(), row, uid)
	writeJSON(w, http.StatusOK, map[string]any{"note": s.noteToJSON(row, uid, premOut, locked)})
}

func noteUnlockRedisKey(viewerID, noteID uuid.UUID) string {
	return "noteunlock:v1:" + viewerID.String() + ":" + noteID.String()
}

type unlockNoteReq struct {
	Password string `json:"password"`
}

func (s *Server) handleNoteUnlock(w http.ResponseWriter, r *http.Request) {
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
	var req unlockNoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	if req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_password"})
		return
	}
	row, err := s.db.NoteByID(r.Context(), noteID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "NoteByID unlock", err)
		return
	}
	if err := s.assertNoteReadable(r.Context(), row, uid); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if row.UserID == uid {
		premOut, locked := s.notePremiumProjection(r.Context(), row, uid)
		writeJSON(w, http.StatusOK, map[string]any{"note": s.noteToJSON(row, uid, premOut, locked)})
		return
	}
	if row.ViewPasswordHash == nil || strings.TrimSpace(*row.ViewPasswordHash) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_password"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*row.ViewPasswordHash), []byte(req.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong_password"})
		return
	}
	rk := noteUnlockRedisKey(uid, noteID)
	if err := s.rdb.Set(r.Context(), rk, "1", postUnlockRedisTTL).Err(); err != nil {
		writeServerError(w, "note unlock redis Set", err)
		return
	}
	// Return the note with premium projection now unlocked.
	updated, err := s.db.NoteByID(r.Context(), noteID)
	if err != nil {
		writeServerError(w, "NoteByID after unlock", err)
		return
	}
	premOut, locked := s.notePremiumProjection(r.Context(), updated, uid)
	writeJSON(w, http.StatusOK, map[string]any{"note": s.noteToJSON(updated, uid, premOut, locked)})
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
	row, err := s.db.NoteByID(r.Context(), noteID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		writeServerError(w, "NoteByID before delete", err)
		return
	}
	wasFederated := err == nil && row.UserID == uid && row.Status == "published" && row.Visibility != "private"
	err = s.db.DeleteNote(r.Context(), uid, noteID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DeleteNote", err)
		return
	}
	if wasFederated {
		s.deliverFederationNoteDelete(r.Context(), uid, noteID)
	}
	w.WriteHeader(http.StatusNoContent)
}
