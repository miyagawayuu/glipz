package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"glipz.io/backend/internal/fanclub/gumroad"
	"glipz.io/backend/internal/repo"
)

type gumroadEntitlementReq struct {
	PostID     string `json:"post_id"`
	LicenseKey string `json:"license_key"`
}

// POST /api/v1/fanclub/gumroad/entitlement
func (s *Server) handleGumroadEntitlement(w http.ResponseWriter, r *http.Request) {
	if !s.gumroadFeatureEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gumroad_not_configured"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req gumroadEntitlementReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(req.PostID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	licenseKey := strings.TrimSpace(req.LicenseKey)
	if licenseKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gumroad_license_required"})
		return
	}
	row, err := s.db.PostSensitiveByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostSensitiveByID gumroad", err)
		return
	}
	if !row.HasMembershipLock || !strings.EqualFold(strings.TrimSpace(row.MembershipProvider), gumroad.ProviderID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_gumroad_post"})
		return
	}
	if row.UserID == uid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_applicable"})
		return
	}
	productID := strings.TrimSpace(row.MembershipCreatorID)
	if productID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_membership_metadata"})
		return
	}
	vr, err := (&gumroad.Client{}).VerifyLicense(r.Context(), productID, licenseKey)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gumroad_api_error"})
		return
	}
	if !vr.Entitled(productID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not_entitled"})
		return
	}
	viewer, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserByID gumroad ent", err)
		return
	}
	cacheKey := gumroad.EntitledCacheKey(uid.String(), row.UserID.String(), productID, row.MembershipTierID)
	_ = s.rdb.Set(r.Context(), cacheKey, "1", 5*time.Minute).Err()
	jws, err := s.mintFederationEntitlementJWT(r.Context(), s.localFullAcct(viewer.Handle), row, postID, nil)
	if err != nil {
		writeServerError(w, "mintFederationEntitlementJWT gumroad", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"entitlement_jwt": jws})
}
