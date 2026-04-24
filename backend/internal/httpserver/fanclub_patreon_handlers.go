package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"glipz.io/backend/internal/fanclub/kernel"
	"glipz.io/backend/internal/fanclub/patreon"
	"glipz.io/backend/internal/repo"
)

type patreonOAuthState struct {
	UserID uuid.UUID `json:"user_id"`
	Kind   string    `json:"kind"` // "member" | "creator"
}

func (s *Server) patreonOAuthConfig() patreon.Config {
	return patreon.Config{
		ClientID:     s.cfg.PatreonClientID,
		ClientSecret: s.cfg.PatreonClientSecret,
		RedirectURI:  s.cfg.PatreonRedirectURI,
	}
}

func (s *Server) requirePatreonConfigured(w http.ResponseWriter) bool {
	if !s.patreonOAuthConfig().Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patreon_not_configured"})
		return false
	}
	return true
}

func (s *Server) handleFanclubPatreonMemberAuthorizeURL(w http.ResponseWriter, r *http.Request) {
	if !s.requirePatreonConfigured(w) {
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	state, err := kernel.RandomOAuthState()
	if err != nil {
		writeServerError(w, "patreon state", err)
		return
	}
	payload, _ := json.Marshal(patreonOAuthState{UserID: uid, Kind: "member"})
	if err := kernel.SaveOAuthState(r.Context(), s.rdb, patreon.ProviderID, state, string(payload), 15*time.Minute); err != nil {
		writeServerError(w, "patreon redis set", err)
		return
	}
	url := patreon.AuthorizeURL(s.patreonOAuthConfig(), []string{"identity", "identity.memberships"}, state)
	writeJSON(w, http.StatusOK, map[string]any{"authorize_url": url})
}

func (s *Server) handleFanclubPatreonCreatorAuthorizeURL(w http.ResponseWriter, r *http.Request) {
	if !s.requirePatreonConfigured(w) {
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	state, err := kernel.RandomOAuthState()
	if err != nil {
		writeServerError(w, "patreon state", err)
		return
	}
	payload, _ := json.Marshal(patreonOAuthState{UserID: uid, Kind: "creator"})
	if err := kernel.SaveOAuthState(r.Context(), s.rdb, patreon.ProviderID, state, string(payload), 15*time.Minute); err != nil {
		writeServerError(w, "patreon redis set", err)
		return
	}
	url := patreon.AuthorizeURL(s.patreonOAuthConfig(), []string{"identity", "campaigns"}, state)
	writeJSON(w, http.StatusOK, map[string]any{"authorize_url": url})
}

func (s *Server) handleFanclubPatreonOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.patreonOAuthConfig().Enabled() {
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=error", http.StatusFound)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=missing_code", http.StatusFound)
		return
	}
	raw, err := kernel.GetDelOAuthState(r.Context(), s.rdb, patreon.ProviderID, state)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=bad_state", http.StatusFound)
			return
		}
		writeServerError(w, "patreon redis getdel", err)
		return
	}
	if raw == "" {
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=bad_state", http.StatusFound)
		return
	}
	var st patreonOAuthState
	if err := json.Unmarshal([]byte(raw), &st); err != nil || st.UserID == uuid.Nil {
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=bad_state", http.StatusFound)
		return
	}
	pc := s.patreonOAuthConfig()
	access, refresh, exp, err := patreon.Exchange(pc, code)
	if err != nil {
		log.Printf("patreon exchange: %v", err)
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=exchange_failed", http.StatusFound)
		return
	}
	ctx := r.Context()
	switch st.Kind {
	case "member":
		pid, err := patreon.FetchIdentityUserID(access)
		if err != nil {
			log.Printf("patreon identity member: %v", err)
			http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=identity_failed", http.StatusFound)
			return
		}
		if err := s.db.SetUserPatreonMemberTokens(ctx, st.UserID, access, refresh, exp, pid); err != nil {
			writeServerError(w, "SetUserPatreonMemberTokens", err)
			return
		}
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=member_ok", http.StatusFound)
	case "creator":
		if err := s.db.SetUserPatreonCreatorTokens(ctx, st.UserID, access, refresh, exp); err != nil {
			writeServerError(w, "SetUserPatreonCreatorTokens", err)
			return
		}
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=creator_ok", http.StatusFound)
	default:
		http.Redirect(w, r, s.cfg.FrontendOrigin+"/settings?fanclub=patreon&result=bad_kind", http.StatusFound)
	}
}

func (s *Server) handleFanclubPatreonMemberDisconnect(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := s.db.ClearUserPatreonMember(r.Context(), uid); err != nil {
		writeServerError(w, "ClearUserPatreonMember", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFanclubPatreonCreatorDisconnect(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := s.db.ClearUserPatreonCreator(r.Context(), uid); err != nil {
		writeServerError(w, "ClearUserPatreonCreator", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFanclubPatreonCreatorCampaigns(w http.ResponseWriter, r *http.Request) {
	if !s.requirePatreonConfigured(w) {
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserByID patreon campaigns", err)
		return
	}
	access, err := s.patreonCreatorAccessToken(r.Context(), u)
	if err != nil {
		writeServerError(w, "patreon creator token", err)
		return
	}
	if access == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patreon_creator_not_linked"})
		return
	}
	camps, err := patreon.FetchCreatorCampaigns(access)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "patreon_campaigns_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaigns": camps})
}

func (s *Server) handleFanclubPatreonCreatorTiers(w http.ResponseWriter, r *http.Request) {
	if !s.requirePatreonConfigured(w) {
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserByID patreon tiers", err)
		return
	}
	access, err := s.patreonCreatorAccessToken(r.Context(), u)
	if err != nil {
		writeServerError(w, "patreon creator token", err)
		return
	}
	if access == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patreon_creator_not_linked"})
		return
	}
	camp := strings.TrimSpace(r.URL.Query().Get("campaign_id"))
	if camp == "" && u.PatreonCampaignID != nil {
		camp = strings.TrimSpace(*u.PatreonCampaignID)
	}
	if camp == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patreon_campaign_missing"})
		return
	}
	tiers, err := patreon.FetchCampaignTiers(access, camp)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "patreon_tiers_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tiers": tiers})
}

// patreonMemberAccessToken returns the viewer's Patreon member token and refreshes it when close to expiry.
func (s *Server) patreonMemberAccessToken(ctx context.Context, u repo.User) (string, error) {
	if u.PatreonMemberAccessToken == nil || strings.TrimSpace(*u.PatreonMemberAccessToken) == "" {
		return "", nil
	}
	tok := strings.TrimSpace(*u.PatreonMemberAccessToken)
	if !s.patreonOAuthConfig().Enabled() {
		return tok, nil
	}
	exp := u.PatreonMemberTokenExpiresAt
	if exp != nil && time.Until(*exp) > 5*time.Minute {
		return tok, nil
	}
	ref := u.PatreonMemberRefreshToken
	if ref == nil || strings.TrimSpace(*ref) == "" {
		return tok, nil
	}
	access, newRef, newExp, err := patreon.Refresh(s.patreonOAuthConfig(), *ref)
	if err != nil {
		return "", err
	}
	rf := strings.TrimSpace(newRef)
	if rf == "" {
		rf = strings.TrimSpace(*ref)
	}
	pid := ""
	if u.PatreonMemberUserID != nil {
		pid = strings.TrimSpace(*u.PatreonMemberUserID)
	}
	if err := s.db.SetUserPatreonMemberTokens(ctx, u.ID, access, rf, newExp, pid); err != nil {
		return "", err
	}
	return access, nil
}

// patreonCreatorAccessToken returns the creator Patreon token and refreshes it when close to expiry.
func (s *Server) patreonCreatorAccessToken(ctx context.Context, u repo.User) (string, error) {
	if u.PatreonCreatorAccessToken == nil || strings.TrimSpace(*u.PatreonCreatorAccessToken) == "" {
		return "", nil
	}
	tok := strings.TrimSpace(*u.PatreonCreatorAccessToken)
	if !s.patreonOAuthConfig().Enabled() {
		return tok, nil
	}
	exp := u.PatreonCreatorTokenExpiresAt
	if exp != nil && time.Until(*exp) > 5*time.Minute {
		return tok, nil
	}
	ref := u.PatreonCreatorRefreshToken
	if ref == nil || strings.TrimSpace(*ref) == "" {
		return tok, nil
	}
	access, newRef, newExp, err := patreon.Refresh(s.patreonOAuthConfig(), *ref)
	if err != nil {
		return "", err
	}
	rf := strings.TrimSpace(newRef)
	if rf == "" {
		rf = strings.TrimSpace(*ref)
	}
	if err := s.db.SetUserPatreonCreatorTokens(ctx, u.ID, access, rf, newExp); err != nil {
		return "", err
	}
	return access, nil
}

// viewerEntitledToAuthorPatreonTier checks entitlement using the viewer's member token.
func (s *Server) viewerEntitledToAuthorPatreonTier(ctx context.Context, viewerID, authorID uuid.UUID, campaignID, requiredTierID string) (bool, error) {
	campaignID = strings.TrimSpace(campaignID)
	requiredTierID = strings.TrimSpace(requiredTierID)
	if campaignID == "" || requiredTierID == "" {
		return false, nil
	}
	cacheKey := kernel.EntitledCacheKey(patreon.ProviderID, viewerID.String(), authorID.String(), campaignID, requiredTierID)
	if v, err := s.rdb.Get(ctx, cacheKey).Result(); err == nil {
		return v == "1", nil
	}
	vu, err := s.db.UserByID(ctx, viewerID)
	if err != nil {
		return false, err
	}
	access, err := s.patreonMemberAccessToken(ctx, vu)
	if err != nil {
		return false, err
	}
	if access == "" {
		_ = s.rdb.Set(ctx, cacheKey, "0", 2*time.Minute).Err()
		return false, nil
	}
	ok, err := patreon.MemberEntitledToReward(access, campaignID, requiredTierID)
	if err != nil {
		return false, err
	}
	val := "0"
	if ok {
		val = "1"
	}
	_ = s.rdb.Set(ctx, cacheKey, val, 8*time.Minute).Err()
	return ok, nil
}

