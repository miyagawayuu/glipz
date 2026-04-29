package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type adminUserJSON struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Handle      string   `json:"handle"`
	DisplayName string   `json:"display_name"`
	Badges      []string `json:"badges"`
	IsSiteAdmin bool     `json:"is_site_admin"`
	SuspendedAt string   `json:"suspended_at,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

func (s *Server) adminUserJSON(row repo.AdminUserRow) adminUserJSON {
	suspendedAt := ""
	if row.SuspendedAt != nil {
		suspendedAt = row.SuspendedAt.UTC().Format(time.RFC3339)
	}
	return adminUserJSON{
		ID:          row.ID.String(),
		Email:       row.Email,
		Handle:      row.Handle,
		DisplayName: row.DisplayName,
		Badges:      row.Badges,
		IsSiteAdmin: s.isSiteAdmin(row.ID),
		SuspendedAt: suspendedAt,
		CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	totalUsers, err := s.db.CountUsersTotal(ctx)
	if err != nil {
		writeServerError(w, "admin overview CountUsersTotal", err)
		return
	}
	suspendedUsers, err := s.db.CountSuspendedUsers(ctx)
	if err != nil {
		writeServerError(w, "admin overview CountSuspendedUsers", err)
		return
	}
	openLocal, openFederated, err := s.db.CountOpenReports(ctx)
	if err != nil {
		writeServerError(w, "admin overview CountOpenReports", err)
		return
	}
	pendingDeliveries, err := s.db.CountFederationDeliveriesByStatus(ctx, "pending")
	if err != nil {
		writeServerError(w, "admin overview pending deliveries", err)
		return
	}
	deadDeliveries, err := s.db.CountFederationDeliveriesByStatus(ctx, "dead")
	if err != nil {
		writeServerError(w, "admin overview dead deliveries", err)
		return
	}
	settings, err := s.db.GetSiteSettings(ctx)
	if err != nil {
		writeServerError(w, "admin overview settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users_total":                  totalUsers,
		"users_suspended":              suspendedUsers,
		"reports_open_local":           openLocal,
		"reports_open_federated":       openFederated,
		"federation_pending":           pendingDeliveries,
		"federation_dead":              deadDeliveries,
		"registrations_enabled":        settings.RegistrationsEnabled,
		"federation_policy_summary":    settings.FederationPolicySummary,
		"operator_announcements_count": len(settings.OperatorAnnouncements),
	})
}

func (s *Server) handleAdminUsersList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	rows, total, err := s.db.ListAdminUsers(r.Context(), repo.AdminUserFilter{
		Query:  q.Get("query"),
		Status: q.Get("status"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeServerError(w, "admin ListAdminUsers", err)
		return
	}
	items := make([]adminUserJSON, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.adminUserJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleAdminUserGet(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "userID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	row, err := s.db.AdminUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "admin AdminUserByID", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": s.adminUserJSON(row)})
}

type adminUserSuspensionReq struct {
	Suspended bool `json:"suspended"`
}

func (s *Server) handleAdminUserSuspensionPatch(w http.ResponseWriter, r *http.Request) {
	adminID, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	userID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "userID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	var req adminUserSuspensionReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if req.Suspended {
		if userID == adminID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_suspend_self"})
			return
		}
		if s.isSiteAdmin(userID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_suspend_site_admin"})
			return
		}
		err = s.db.SuspendUser(r.Context(), userID)
	} else {
		err = s.db.UnsuspendUser(r.Context(), userID)
	}
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "admin user suspension", err)
		return
	}
	row, err := s.db.AdminUserByID(r.Context(), userID)
	if err != nil {
		writeServerError(w, "admin user suspension reload", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": s.adminUserJSON(row)})
}

func (s *Server) handleAdminInstanceSettingsGet(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.GetSiteSettings(r.Context())
	if err != nil {
		writeServerError(w, "admin GetSiteSettings", err)
		return
	}
	if strings.TrimSpace(settings.FederationPolicySummary) == "" {
		settings.FederationPolicySummary = strings.TrimSpace(s.cfg.FederationPolicySummary)
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handleAdminInstanceSettingsPatch(w http.ResponseWriter, r *http.Request) {
	current, err := s.db.GetSiteSettings(r.Context())
	if err != nil {
		writeServerError(w, "admin GetSiteSettings", err)
		return
	}
	var req repo.SiteSettings
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if !validOptionalHTTPURL(req.TermsURL) || !validOptionalHTTPURL(req.PrivacyPolicyURL) || !validOptionalHTTPURL(req.NSFWGuidelinesURL) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url"})
		return
	}
	if req.MinimumRegistrationAge < 0 || req.MinimumRegistrationAge > 120 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_minimum_registration_age"})
		return
	}
	current.RegistrationsEnabled = req.RegistrationsEnabled
	current.MinimumRegistrationAge = req.MinimumRegistrationAge
	current.ServerName = strings.TrimSpace(req.ServerName)
	current.ServerDescription = strings.TrimSpace(req.ServerDescription)
	current.AdminName = strings.TrimSpace(req.AdminName)
	current.AdminEmail = strings.TrimSpace(req.AdminEmail)
	current.TermsURL = strings.TrimSpace(req.TermsURL)
	current.PrivacyPolicyURL = strings.TrimSpace(req.PrivacyPolicyURL)
	current.NSFWGuidelinesURL = strings.TrimSpace(req.NSFWGuidelinesURL)
	current.FederationPolicySummary = strings.TrimSpace(req.FederationPolicySummary)
	current.OperatorAnnouncements = req.OperatorAnnouncements
	updated, err := s.db.UpdateSiteSettings(r.Context(), current)
	if err != nil {
		writeServerError(w, "admin UpdateSiteSettings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": updated})
}

func (s *Server) handlePublicInstanceSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.GetSiteSettings(r.Context())
	if err != nil {
		writeServerError(w, "public GetSiteSettings", err)
		return
	}
	if strings.TrimSpace(settings.FederationPolicySummary) == "" {
		settings.FederationPolicySummary = strings.TrimSpace(s.cfg.FederationPolicySummary)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"registrations_enabled":     settings.RegistrationsEnabled,
		"minimum_registration_age":  settings.MinimumRegistrationAge,
		"server_name":               settings.ServerName,
		"server_description":        settings.ServerDescription,
		"admin_name":                settings.AdminName,
		"admin_email":               settings.AdminEmail,
		"terms_url":                 settings.TermsURL,
		"privacy_policy_url":        settings.PrivacyPolicyURL,
		"nsfw_guidelines_url":       settings.NSFWGuidelinesURL,
		"federation_policy_summary": settings.FederationPolicySummary,
		"operator_announcements":    settings.OperatorAnnouncements,
	})
}

func validOptionalHTTPURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
