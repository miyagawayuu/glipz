package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

const (
	legalRequestMaxBodyBytes = 1 << 16
	legalStringMax           = 2000
	dmReportPlaintextMax     = 12000
)

type createLegalRequestReq struct {
	RequestType      string   `json:"request_type"`
	AgencyName       string   `json:"agency_name"`
	Jurisdiction     string   `json:"jurisdiction"`
	LegalBasis       string   `json:"legal_basis"`
	ExternalRef      string   `json:"external_reference"`
	TargetUserID     string   `json:"target_user_id"`
	TargetHandle     string   `json:"target_handle"`
	TargetFromAt     string   `json:"target_from_at"`
	TargetUntilAt    string   `json:"target_until_at"`
	DataTypes        []string `json:"data_types"`
	Emergency        bool     `json:"emergency"`
	DueAt            string   `json:"due_at"`
	ResponseSummary  string   `json:"response_summary"`
	UserNoticeStatus string   `json:"user_notice_status"`
}

type updateLegalRequestReq struct {
	Status           string `json:"status"`
	ResponseSummary  string `json:"response_summary"`
	UserNoticeStatus string `json:"user_notice_status"`
}

type createLegalHoldReq struct {
	TargetUserID string `json:"target_user_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Reason       string `json:"reason"`
	ExpiresAt    string `json:"expires_at"`
}

type createDMReportReq struct {
	Reason                     string `json:"reason"`
	IncludePlaintext           bool   `json:"include_plaintext"`
	ReporterSubmittedPlaintext string `json:"reporter_submitted_plaintext"`
	AttachmentsNote            string `json:"attachments_note"`
}

func limitedTrim(raw string, max int) (string, bool) {
	v := strings.TrimSpace(raw)
	return v, utf8.RuneCountInString(v) <= max
}

func parseOptionalUUID(raw string) (*uuid.UUID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return nil, false
	}
	return &id, true
}

func parseOptionalRFC3339(raw string) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, false
	}
	utc := t.UTC()
	return &utc, true
}

func legalRequestToJSON(row repo.LawEnforcementRequest) map[string]any {
	out := map[string]any{
		"id":                 row.ID.String(),
		"request_type":       row.RequestType,
		"agency_name":        row.AgencyName,
		"jurisdiction":       row.Jurisdiction,
		"legal_basis":        row.LegalBasis,
		"external_reference": row.ExternalRef,
		"target_handle":      row.TargetHandle,
		"data_types":         row.DataTypes,
		"emergency":          row.Emergency,
		"status":             row.Status,
		"response_summary":   row.ResponseSummary,
		"user_notice_status": row.UserNoticeStatus,
		"created_at":         row.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if row.TargetUserID != nil {
		out["target_user_id"] = row.TargetUserID.String()
	}
	if row.TargetFromAt != nil {
		out["target_from_at"] = row.TargetFromAt.UTC().Format(time.RFC3339)
	}
	if row.TargetUntilAt != nil {
		out["target_until_at"] = row.TargetUntilAt.UTC().Format(time.RFC3339)
	}
	if row.DueAt != nil {
		out["due_at"] = row.DueAt.UTC().Format(time.RFC3339)
	}
	if row.AssignedAdminID != nil {
		out["assigned_admin_id"] = row.AssignedAdminID.String()
	}
	return out
}

func dmReportToJSON(row repo.DMReport) map[string]any {
	out := map[string]any{
		"id":                           row.ID.String(),
		"reporter_user_id":             row.ReporterUserID.String(),
		"reporter_handle":              row.ReporterHandle,
		"reporter_display_name":        row.ReporterDisplayName,
		"thread_id":                    row.ThreadID.String(),
		"message_id":                   row.MessageID.String(),
		"message_sender_id":            row.MessageSenderID.String(),
		"message_sender_handle":        row.MessageSenderHandle,
		"message_sender_display_name":  row.MessageSenderDisplayName,
		"reason":                       row.Reason,
		"include_plaintext":            row.IncludePlaintext,
		"reporter_submitted_plaintext": row.ReporterSubmittedPlaintext,
		"attachments_note":             row.AttachmentsNote,
		"status":                       row.Status,
		"created_at":                   row.CreatedAt.UTC().Format(time.RFC3339),
	}
	if row.ResolvedAt != nil {
		out["resolved_at"] = row.ResolvedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) auditAdminAction(r *http.Request, action, targetType string, targetID, requestID *uuid.UUID, metadata map[string]any) {
	adminID, ok := userIDFrom(r.Context())
	if !ok {
		return
	}
	if err := s.db.InsertAdminAuditEvent(r.Context(), repo.AdminAuditInput{
		AdminUserID: adminID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		RequestID:   requestID,
		IP:          s.clientIPForAuthRateLimit(r),
		UserAgent:   r.UserAgent(),
		Metadata:    metadata,
	}); err != nil {
		// Audit failures should be visible to operators; callers still return their primary result.
		fmt.Printf("admin audit %s: %v\n", action, err)
	}
}

func (s *Server) handleAdminLegalRequests(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListLawEnforcementRequests(r.Context(), 100)
	if err != nil {
		writeServerError(w, "ListLawEnforcementRequests", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, legalRequestToJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleAdminCreateLegalRequest(w http.ResponseWriter, r *http.Request) {
	if s.sensitiveActionRateLimitExceeded(r.Context(), r, "legal_request_create") {
		writeSensitiveActionRateLimited(w)
		return
	}
	adminID, _ := userIDFrom(r.Context())
	var req createLegalRequestReq
	if err := json.NewDecoder(io.LimitReader(r.Body, legalRequestMaxBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	agencyName, ok := limitedTrim(req.AgencyName, 200)
	if !ok || agencyName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_agency_name"})
		return
	}
	jurisdiction, ok := limitedTrim(req.Jurisdiction, 200)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_jurisdiction"})
		return
	}
	legalBasis, ok := limitedTrim(req.LegalBasis, legalStringMax)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_legal_basis"})
		return
	}
	externalRef, ok := limitedTrim(req.ExternalRef, 200)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_external_reference"})
		return
	}
	targetHandle, ok := limitedTrim(req.TargetHandle, 120)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_handle"})
		return
	}
	targetUserID, ok := parseOptionalUUID(req.TargetUserID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_user_id"})
		return
	}
	fromAt, ok := parseOptionalRFC3339(req.TargetFromAt)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_from_at"})
		return
	}
	untilAt, ok := parseOptionalRFC3339(req.TargetUntilAt)
	if !ok || (fromAt != nil && untilAt != nil && untilAt.Before(*fromAt)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_until_at"})
		return
	}
	dueAt, ok := parseOptionalRFC3339(req.DueAt)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_due_at"})
		return
	}
	summary, ok := limitedTrim(req.ResponseSummary, legalStringMax)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_response_summary"})
		return
	}
	row, err := s.db.CreateLawEnforcementRequest(r.Context(), repo.CreateLawEnforcementRequestInput{
		RequestType:      req.RequestType,
		AgencyName:       agencyName,
		Jurisdiction:     jurisdiction,
		LegalBasis:       legalBasis,
		ExternalRef:      externalRef,
		TargetUserID:     targetUserID,
		TargetHandle:     targetHandle,
		TargetFromAt:     fromAt,
		TargetUntilAt:    untilAt,
		DataTypes:        req.DataTypes,
		Emergency:        req.Emergency,
		DueAt:            dueAt,
		AssignedAdminID:  &adminID,
		ResponseSummary:  summary,
		UserNoticeStatus: req.UserNoticeStatus,
	})
	if err != nil {
		writeServerError(w, "CreateLawEnforcementRequest", err)
		return
	}
	s.auditAdminAction(r, "legal_request.create", "law_enforcement_request", &row.ID, &row.ID, map[string]any{"status": row.Status})
	writeJSON(w, http.StatusCreated, map[string]any{"item": legalRequestToJSON(row)})
}

func (s *Server) handleAdminUpdateLegalRequest(w http.ResponseWriter, r *http.Request) {
	requestID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "requestID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request_id"})
		return
	}
	var req updateLegalRequestReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	summary, ok := limitedTrim(req.ResponseSummary, legalStringMax)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_response_summary"})
		return
	}
	row, err := s.db.UpdateLawEnforcementRequestStatus(r.Context(), requestID, req.Status, summary, req.UserNoticeStatus)
	if errors.Is(err, repo.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeServerError(w, "UpdateLawEnforcementRequestStatus", err)
		return
	}
	s.auditAdminAction(r, "legal_request.update", "law_enforcement_request", &row.ID, &row.ID, map[string]any{"status": row.Status})
	writeJSON(w, http.StatusOK, map[string]any{"item": legalRequestToJSON(row)})
}

func (s *Server) handleAdminCreateLegalHold(w http.ResponseWriter, r *http.Request) {
	requestID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "requestID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request_id"})
		return
	}
	var req createLegalHoldReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	targetUserID, ok := parseOptionalUUID(req.TargetUserID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_user_id"})
		return
	}
	resourceID, ok := parseOptionalUUID(req.ResourceID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_resource_id"})
		return
	}
	resourceType, ok := limitedTrim(req.ResourceType, 80)
	if !ok || resourceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_resource_type"})
		return
	}
	reason, ok := limitedTrim(req.Reason, legalStringMax)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_reason"})
		return
	}
	expiresAt, ok := parseOptionalRFC3339(req.ExpiresAt)
	if !ok || expiresAt == nil || expiresAt.Before(time.Now().UTC()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_expires_at"})
		return
	}
	hold, err := s.db.CreateLegalPreservationHold(r.Context(), repo.CreateLegalPreservationHoldInput{
		RequestID: requestID, TargetUserID: targetUserID, ResourceType: resourceType, ResourceID: resourceID, Reason: reason, ExpiresAt: *expiresAt,
	})
	if err != nil {
		writeServerError(w, "CreateLegalPreservationHold", err)
		return
	}
	s.auditAdminAction(r, "legal_hold.create", "legal_preservation_hold", &hold.ID, &requestID, map[string]any{"resource_type": hold.ResourceType})
	writeJSON(w, http.StatusCreated, map[string]any{"id": hold.ID.String(), "status": "ok"})
}

func (s *Server) handleAdminExportLegalRequest(w http.ResponseWriter, r *http.Request) {
	if s.sensitiveActionRateLimitExceeded(r.Context(), r, "legal_request_export") {
		writeSensitiveActionRateLimited(w)
		return
	}
	requestID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "requestID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request_id"})
		return
	}
	pkg, err := s.db.BuildLegalDisclosurePackage(r.Context(), requestID)
	if errors.Is(err, repo.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeServerError(w, "BuildLegalDisclosurePackage", err)
		return
	}
	s.auditAdminAction(r, "legal_request.export", "law_enforcement_request", &requestID, &requestID, map[string]any{"format": "json"})
	b, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		writeServerError(w, "legal package marshal", err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="glipz-legal-request-%s.json"`, requestID.String()))
	writeJSONBytes(w, http.StatusOK, b)
}

func (s *Server) handleCreateDMReport(w http.ResponseWriter, r *http.Request) {
	if s.sensitiveActionRateLimitExceeded(r.Context(), r, "dm_report_create") {
		writeSensitiveActionRateLimited(w)
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "messageID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_message_id"})
		return
	}
	var req createDMReportReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	reason, code := normalizeReportReason(req.Reason)
	if code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
		return
	}
	plaintext, ok := limitedTrim(req.ReporterSubmittedPlaintext, dmReportPlaintextMax)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plaintext_too_long"})
		return
	}
	if !req.IncludePlaintext {
		plaintext = ""
	}
	note, ok := limitedTrim(req.AttachmentsNote, 2000)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "attachments_note_too_long"})
		return
	}
	row, err := s.db.CreateDMReport(r.Context(), repo.CreateDMReportInput{
		ReporterUserID: uid, ThreadID: threadID, MessageID: messageID, Reason: reason,
		IncludePlaintext: req.IncludePlaintext, ReporterSubmittedPlaintext: plaintext, AttachmentsNote: note,
	})
	if errors.Is(err, repo.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeServerError(w, "CreateDMReport", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": dmReportToJSON(row)})
}

func (s *Server) handleAdminDMReports(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListAdminDMReports(r.Context(), 100)
	if err != nil {
		writeServerError(w, "ListAdminDMReports", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, dmReportToJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleAdminUpdateDMReportStatus(w http.ResponseWriter, r *http.Request) {
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
	if err := s.db.UpdateDMReportStatus(r.Context(), reportID, req.Status); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "UpdateDMReportStatus", err)
		return
	}
	s.auditAdminAction(r, "dm_report.update", "dm_report", &reportID, nil, map[string]any{"status": req.Status})
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(strings.ToLower(req.Status))})
}
