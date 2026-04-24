package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"glipz.io/backend/internal/fanclub/kernel"
	"glipz.io/backend/internal/fanclub/patreon"
	"glipz.io/backend/internal/repo"
)

type patreonOAuthState struct {
	UserID   string `json:"user_id"`
	ReturnTo string `json:"return_to"`
}

func (s *Server) patreonIntegrationAvailable() bool {
	return strings.TrimSpace(s.cfg.PatreonClientID) != "" && strings.TrimSpace(s.cfg.PatreonClientSecret) != ""
}

func (s *Server) patreonRedirectURI() (string, bool) {
	if u := strings.TrimSpace(s.cfg.PatreonRedirectURI); u != "" {
		return u, true
	}
	if o := strings.TrimSpace(s.federationPublicOrigin()); o != "" {
		return strings.TrimSuffix(o, "/") + "/api/v1/fanclub/patreon/callback", true
	}
	return "", false
}

func (s *Server) patreonClient() *patreon.Client {
	cfg := s.patreonClientConfig()
	return &patreon.Client{Config: cfg}
}

func (s *Server) patreonClientConfig() patreon.Config {
	redir, _ := s.patreonRedirectURI()
	return patreon.Config{
		ClientID:     strings.TrimSpace(s.cfg.PatreonClientID),
		ClientSecret: strings.TrimSpace(s.cfg.PatreonClientSecret),
		RedirectURI:  redir,
	}
}

func (s *Server) patreonReturnURL(pathOrURL string) string {
	pathOrURL = strings.TrimSpace(pathOrURL)
	if pathOrURL == "" {
		return s.cfg.FrontendOrigin
	}
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		u, err := url.Parse(pathOrURL)
		if err != nil {
			return s.cfg.FrontendOrigin
		}
		got := strings.TrimSuffix(strings.ToLower(u.Scheme+"://"+u.Host), "/")
		for _, o := range s.cfg.FrontendOrigins {
			o = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(o)), "/")
			if o == got {
				return pathOrURL
			}
		}
		return s.cfg.FrontendOrigin
	}
	if !strings.HasPrefix(pathOrURL, "/") || strings.HasPrefix(pathOrURL, "//") {
		return s.cfg.FrontendOrigin
	}
	return strings.TrimSuffix(s.cfg.FrontendOrigin, "/") + pathOrURL
}

// GET /api/v1/fanclub/patreon/authorize?return_to=/settings
func (s *Server) handlePatreonAuthorize(w http.ResponseWriter, r *http.Request) {
	if !s.patreonIntegrationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patreon_not_configured"})
		return
	}
	if _, ok := s.patreonRedirectURI(); !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patreon_missing_redirect_uri"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	returnTo := s.patreonReturnURL(r.URL.Query().Get("return_to"))
	st, err := kernel.RandomOAuthState()
	if err != nil {
		writeServerError(w, "RandomOAuthState", err)
		return
	}
	payload, err := json.Marshal(patreonOAuthState{UserID: uid.String(), ReturnTo: returnTo})
	if err != nil {
		writeServerError(w, "patreon oauth json", err)
		return
	}
	if err := kernel.SaveOAuthState(r.Context(), s.rdb, patreon.ProviderID, st, string(payload), 15*time.Minute); err != nil {
		writeServerError(w, "SaveOAuthState patreon", err)
		return
	}
	u, _ := url.Parse("https://www.patreon.com/oauth2/authorize")
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", s.cfg.PatreonClientID)
	q.Set("redirect_uri", s.patreonClientConfig().RedirectURI)
	q.Set("scope", "identity identity[email] campaigns")
	q.Set("state", st)
	u.RawQuery = q.Encode()
	// Browsers do not send Authorization on <a href> navigation; the SPA fetches with Accept: JSON.
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeJSON(w, http.StatusOK, map[string]string{"redirect": u.String()})
		return
	}
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// GET /api/v1/fanclub/patreon/callback?code&state
func (s *Server) handlePatreonCallback(w http.ResponseWriter, r *http.Request) {
	if !s.patreonIntegrationAvailable() {
		http.Redirect(w, r, s.patreonReturnURL(""), http.StatusFound)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	st := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || st == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_callback"})
		return
	}
	raw, err := kernel.GetDelOAuthState(r.Context(), s.rdb, patreon.ProviderID, st)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
			return
		}
		writeServerError(w, "GetDelOAuthState patreon", err)
		return
	}
	if strings.TrimSpace(raw) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	var payload patreonOAuthState
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	userID, err := uuid.Parse(strings.TrimSpace(payload.UserID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	cl := s.patreonClient()
	tr, err := cl.ExchangeCode(r.Context(), code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token_exchange_failed"})
		return
	}
	pID, err := cl.PatreonUserID(r.Context(), tr.AccessToken)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patreon_identity_failed"})
		return
	}
	ref := tr.RefreshToken
	exp := patreon.ExpiresAt(tr)
	if err := s.db.UpsertPatreonOAuth(r.Context(), userID, pID, tr.AccessToken, ref, exp); err != nil {
		writeServerError(w, "UpsertPatreonOAuth", err)
		return
	}
	ret := s.patreonReturnURL(payload.ReturnTo)
	http.Redirect(w, r, ret, http.StatusFound)
}

// GET /api/v1/fanclub/patreon/status
func (s *Server) handlePatreonStatus(w http.ResponseWriter, r *http.Request) {
	if !s.patreonIntegrationAvailable() {
		writeJSON(w, http.StatusOK, map[string]any{"patreon": map[string]any{"available": false}})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	connected, err := s.db.UserHasPatreonConnection(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserHasPatreonConnection", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"patreon": map[string]any{"available": true, "connected": connected}})
}

// DELETE /api/v1/fanclub/patreon/connection
func (s *Server) handlePatreonDeleteConnection(w http.ResponseWriter, r *http.Request) {
	if !s.patreonIntegrationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patreon_not_configured"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	_ = s.db.DeletePatreonOAuth(r.Context(), uid)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "ok"})
}

type patreonEntitlementReq struct {
	PostID string `json:"post_id"`
}

// POST /api/v1/fanclub/patreon/entitlement
func (s *Server) handlePatreonEntitlement(w http.ResponseWriter, r *http.Request) {
	if !s.patreonIntegrationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patreon_not_configured"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req patreonEntitlementReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(req.PostID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	row, err := s.db.PostSensitiveByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostSensitiveByID patreon", err)
		return
	}
	if !row.HasMembershipLock || strings.ToLower(strings.TrimSpace(row.MembershipProvider)) != patreon.ProviderID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_patreon_post"})
		return
	}
	if row.UserID == uid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_applicable"})
		return
	}
	access, err := s.patreonAccessToken(r.Context(), uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patreon_not_connected"})
			return
		}
		writeServerError(w, "patreonAccessToken", err)
		return
	}
	cl := s.patreonClient()
	_, match, err := cl.EntitlementMatch(r.Context(), access, patreon.EntitlementArgs{
		CampaignID: row.MembershipCreatorID,
		TierID:     row.MembershipTierID,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "patreon_api_error"})
		return
	}
	if !match {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not_entitled"})
		return
	}
	viewer, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserByID patreon ent", err)
		return
	}
	authorID := row.UserID
	cacheKey := patreon.EntitledCacheKey(uid.String(), authorID.String(), row.MembershipCreatorID, row.MembershipTierID)
	_ = s.rdb.Set(r.Context(), cacheKey, "1", 5*time.Minute).Err()
	viewerAcct := s.localFullAcct(viewer.Handle)
	jws, err := s.mintFederationEntitlementJWT(viewerAcct, row, postID)
	if err != nil {
		writeServerError(w, "mintFederationEntitlementJWT patreon", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"entitlement_jwt": jws})
}

func (s *Server) patreonAccessToken(ctx context.Context, userID uuid.UUID) (string, error) {
	row, err := s.db.PatreonOAuthByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	if s.patreonShouldRefresh(row) {
		cl := s.patreonClient()
		tr, err := cl.Refresh(ctx, row.RefreshToken)
		if err != nil {
			return "", err
		}
		ref := tr.RefreshToken
		if ref == "" {
			ref = row.RefreshToken
		}
		exp := patreon.ExpiresAt(tr)
		pID := row.PatreonUserID
		if err := s.db.UpsertPatreonOAuth(ctx, userID, pID, tr.AccessToken, ref, exp); err != nil {
			return "", err
		}
		return tr.AccessToken, nil
	}
	return row.AccessToken, nil
}

func (s *Server) patreonShouldRefresh(row repo.PatreonTokenRow) bool {
	if strings.TrimSpace(row.RefreshToken) == "" {
		return false
	}
	if row.TokenExpiresAt == nil {
		return true
	}
	return time.Until(*row.TokenExpiresAt) < 2*time.Minute
}

// GET /api/v1/fanclub/patreon/campaigns
func (s *Server) handlePatreonCampaigns(w http.ResponseWriter, r *http.Request) {
	if !s.patreonIntegrationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patreon_not_configured"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	access, err := s.patreonAccessToken(r.Context(), uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patreon_not_connected"})
			return
		}
		writeServerError(w, "patreonAccessToken campaigns", err)
		return
	}
	camps, err := s.patreonClient().ListCreatorCampaigns(r.Context(), access)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "patreon_api_error"})
		return
	}
	out := make([]map[string]any, 0, len(camps))
	for _, c := range camps {
		tiers := make([]map[string]any, 0, len(c.Tiers))
		for _, t := range c.Tiers {
			tiers = append(tiers, map[string]any{
				"id":   t.ID,
				"name": t.Name,
			})
		}
		out = append(out, map[string]any{
			"id":    c.ID,
			"title": c.Title,
			"tiers": tiers,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaigns": out})
}