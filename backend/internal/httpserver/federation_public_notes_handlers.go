package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"glipz.io/backend/internal/repo"
)

func federatedIncomingNoteToJSON(row repo.FederatedIncomingNote, unlockedBodyPremiumMd string) map[string]any {
	icon := ""
	if row.ActorIconURL != nil {
		icon = strings.TrimSpace(*row.ActorIconURL)
	}
	profile := ""
	if row.ActorProfileURL != nil {
		profile = strings.TrimSpace(*row.ActorProfileURL)
	}
	premiumLocked := row.HasPremium && strings.TrimSpace(unlockedBodyPremiumMd) == ""
	bodyPremium := ""
	if !premiumLocked {
		bodyPremium = unlockedBodyPremiumMd
	}
	return map[string]any{
		"id":                  row.ID.String(),
		"object_iri":          row.ObjectIRI,
		"actor_iri":           row.ActorIRI,
		"actor_acct":          row.ActorAcct,
		"actor_name":          row.ActorName,
		"actor_icon_url":      icon,
		"actor_profile_url":   profile,
		"title":               row.Title,
		"body_md":             row.BodyMd,
		"body_premium_md":     bodyPremium,
		"premium_locked":      premiumLocked,
		"visibility":          row.Visibility,
		"published_at":        row.PublishedAt.UTC().Format(time.RFC3339),
		"updated_at":          row.UpdatedAt.UTC().Format(time.RFC3339),
		"has_premium":         row.HasPremium,
		"paywall_provider":    row.PaywallProvider,
		"patreon_campaign_id": strings.TrimSpace(row.PatreonCampaignID),
		"patreon_required_reward_tier_id": strings.TrimSpace(row.PatreonRequiredRewardTierID),
		"unlock_url": strings.TrimSpace(row.UnlockURL),
	}
}

func (s *Server) handlePublicFederatedIncomingNotesByActor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("actor"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("acct"))
	}
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_acct_or_actor"})
		return
	}
	resolved, err := ResolveRemoteActor(r.Context(), raw)
	if err != nil {
		st := resolveErrorHTTPStatus(err)
		writeJSON(w, st, map[string]string{"error": ResolveFailureAPIError(err)})
		return
	}
	rows, err := s.db.ListFederatedIncomingNotesPublicByActorIRI(r.Context(), resolved.ActorID, 50)
	if err != nil {
		writeServerError(w, "ListFederatedIncomingNotesPublicByActorIRI", err)
		return
	}
	viewerID, ok := userIDFrom(r.Context())
	unlocks := map[uuid.UUID]string{}
	if ok && len(rows) > 0 {
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		unlocks, _ = s.db.ListFederatedIncomingNoteUnlockBodiesForUser(r.Context(), viewerID, ids)
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, federatedIncomingNoteToJSON(row, unlocks[row.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePublicFederatedIncomingNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	idRaw := strings.TrimSpace(chi.URLParam(r, "id"))
	if idRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	parsed, err := uuid.Parse(idRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	row, err := s.db.GetFederatedIncomingNoteByID(r.Context(), parsed)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		default:
			writeServerError(w, "GetFederatedIncomingNoteByID", err)
		}
		return
	}
	if row.DeletedAt != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	viewerID, ok := userIDFrom(r.Context())
	unlocked := ""
	if ok {
		if m, err := s.db.ListFederatedIncomingNoteUnlockBodiesForUser(r.Context(), viewerID, []uuid.UUID{row.ID}); err == nil {
			unlocked = m[row.ID]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": federatedIncomingNoteToJSON(row, unlocked)})
}

