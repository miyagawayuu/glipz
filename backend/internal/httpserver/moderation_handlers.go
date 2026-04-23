package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type createReportReq struct {
	Reason string `json:"reason"`
}

type updateReportStatusReq struct {
	Status string `json:"status"`
}

func isValidModerationStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "open", "resolved", "dismissed", "spam":
		return true
	default:
		return false
	}
}

func normalizeReportReason(raw string) (string, string) {
	reason := strings.TrimSpace(raw)
	if reason == "" {
		return "", "report_reason_required"
	}
	if utf8.RuneCountInString(reason) > 1000 {
		return "", "report_reason_too_long"
	}
	return reason, ""
}

func (s *Server) handleCreatePostReport(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	readable, err := s.db.CanViewerReadPost(r.Context(), uid, postID)
	if err != nil {
		writeServerError(w, "report CanViewerReadPost", err)
		return
	}
	if !readable {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	authorID, _, err := s.db.PostFeedMeta(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "report PostFeedMeta", err)
		return
	}
	if authorID == uid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_report_own_post"})
		return
	}
	var req createReportReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	reason, code := normalizeReportReason(req.Reason)
	if code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
		return
	}
	if err := s.db.InsertPostReport(r.Context(), uid, postID, reason); err != nil {
		writeServerError(w, "report InsertPostReport", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateFederatedIncomingPostReport(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	incomingID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "incomingID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	row, err := s.loadFederatedIncomingForAction(r.Context(), incomingID, r.URL.Query().Get("object_url"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "report loadFederatedIncomingForAction", err)
		return
	}
	if row.RecipientUserID != nil && *row.RecipientUserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	var req createReportReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	reason, code := normalizeReportReason(req.Reason)
	if code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
		return
	}
	if err := s.db.InsertFederatedIncomingPostReport(r.Context(), uid, row.ID, reason); err != nil {
		writeServerError(w, "report InsertFederatedIncomingPostReport", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminSuspendPostAuthor(w http.ResponseWriter, r *http.Request) {
	adminID, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	authorID, _, err := s.db.PostFeedMeta(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "admin suspend PostFeedMeta", err)
		return
	}
	if authorID == adminID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_suspend_self"})
		return
	}
	if s.isSiteAdmin(authorID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_suspend_site_admin"})
		return
	}
	if err := s.db.SuspendUser(r.Context(), authorID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "admin suspend SuspendUser", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminPostReports(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListAdminPostReports(r.Context(), 100)
	if err != nil {
		writeServerError(w, "admin ListAdminPostReports", err)
		return
	}
	type item struct {
		ID                    string `json:"id"`
		CreatedAt             string `json:"created_at"`
		PostID                string `json:"post_id"`
		PostCaption           string `json:"post_caption"`
		Reason                string `json:"reason"`
		Status                string `json:"status"`
		ResolvedAt            string `json:"resolved_at,omitempty"`
		PostAuthorUserID      string `json:"post_author_user_id"`
		PostAuthorHandle      string `json:"post_author_handle"`
		PostAuthorDisplayName string `json:"post_author_display_name"`
		ReporterUserID        string `json:"reporter_user_id"`
		ReporterHandle        string `json:"reporter_handle"`
		ReporterDisplayName   string `json:"reporter_display_name"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		resolvedAt := ""
		if row.ResolvedAt != nil {
			resolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item{
			ID:                    row.ID.String(),
			CreatedAt:             row.CreatedAt.UTC().Format(time.RFC3339),
			PostID:                row.PostID.String(),
			PostCaption:           row.PostCaption,
			Reason:                row.Reason,
			Status:                row.Status,
			ResolvedAt:            resolvedAt,
			PostAuthorUserID:      row.PostAuthorUserID.String(),
			PostAuthorHandle:      row.PostAuthorHandle,
			PostAuthorDisplayName: row.PostAuthorDisplayName,
			ReporterUserID:        row.ReporterUserID.String(),
			ReporterHandle:        row.ReporterHandle,
			ReporterDisplayName:   row.ReporterDisplayName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleAdminFederatedPostReports(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListAdminFederatedIncomingPostReports(r.Context(), 100)
	if err != nil {
		writeServerError(w, "admin ListAdminFederatedIncomingPostReports", err)
		return
	}
	type item struct {
		ID                  string `json:"id"`
		CreatedAt           string `json:"created_at"`
		IncomingPostID      string `json:"incoming_post_id"`
		ObjectIRI           string `json:"object_iri"`
		CaptionText         string `json:"caption_text"`
		Reason              string `json:"reason"`
		Status              string `json:"status"`
		ResolvedAt          string `json:"resolved_at,omitempty"`
		ActorAcct           string `json:"actor_acct"`
		ActorName           string `json:"actor_name"`
		ReporterUserID      string `json:"reporter_user_id"`
		ReporterHandle      string `json:"reporter_handle"`
		ReporterDisplayName string `json:"reporter_display_name"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		resolvedAt := ""
		if row.ResolvedAt != nil {
			resolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item{
			ID:                  row.ID.String(),
			CreatedAt:           row.CreatedAt.UTC().Format(time.RFC3339),
			IncomingPostID:      row.IncomingPostID.String(),
			ObjectIRI:           row.ObjectIRI,
			CaptionText:         row.CaptionText,
			Reason:              row.Reason,
			Status:              row.Status,
			ResolvedAt:          resolvedAt,
			ActorAcct:           row.ActorAcct,
			ActorName:           row.ActorName,
			ReporterUserID:      row.ReporterUserID.String(),
			ReporterHandle:      row.ReporterHandle,
			ReporterDisplayName: row.ReporterDisplayName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleAdminUpdatePostReportStatus(w http.ResponseWriter, r *http.Request) {
	reportID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "reportID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_report_id"})
		return
	}
	var req updateReportStatusReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if !isValidModerationStatus(req.Status) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_status"})
		return
	}
	if err := s.db.UpdatePostReportStatus(r.Context(), reportID, req.Status); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "admin UpdatePostReportStatus", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(strings.ToLower(req.Status))})
}

func (s *Server) handleAdminUpdateFederatedPostReportStatus(w http.ResponseWriter, r *http.Request) {
	reportID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "reportID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_report_id"})
		return
	}
	var req updateReportStatusReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if !isValidModerationStatus(req.Status) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_status"})
		return
	}
	if err := s.db.UpdateFederatedIncomingPostReportStatus(r.Context(), reportID, req.Status); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "admin UpdateFederatedIncomingPostReportStatus", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(strings.ToLower(req.Status))})
}
