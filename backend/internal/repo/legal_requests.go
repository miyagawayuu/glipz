package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type LawEnforcementRequest struct {
	ID               uuid.UUID
	RequestType      string
	AgencyName       string
	Jurisdiction     string
	LegalBasis       string
	ExternalRef      string
	TargetUserID     *uuid.UUID
	TargetHandle     string
	TargetFromAt     *time.Time
	TargetUntilAt    *time.Time
	DataTypes        []string
	Emergency        bool
	Status           string
	DueAt            *time.Time
	AssignedAdminID  *uuid.UUID
	ResponseSummary  string
	UserNoticeStatus string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreateLawEnforcementRequestInput struct {
	RequestType      string
	AgencyName       string
	Jurisdiction     string
	LegalBasis       string
	ExternalRef      string
	TargetUserID     *uuid.UUID
	TargetHandle     string
	TargetFromAt     *time.Time
	TargetUntilAt    *time.Time
	DataTypes        []string
	Emergency        bool
	DueAt            *time.Time
	AssignedAdminID  *uuid.UUID
	ResponseSummary  string
	UserNoticeStatus string
}

type LegalPreservationHold struct {
	ID           uuid.UUID
	RequestID    uuid.UUID
	TargetUserID *uuid.UUID
	ResourceType string
	ResourceID   *uuid.UUID
	Reason       string
	ExpiresAt    time.Time
	ReleasedAt   *time.Time
	CreatedAt    time.Time
}

type CreateLegalPreservationHoldInput struct {
	RequestID    uuid.UUID
	TargetUserID *uuid.UUID
	ResourceType string
	ResourceID   *uuid.UUID
	Reason       string
	ExpiresAt    time.Time
}

type AdminAuditInput struct {
	AdminUserID uuid.UUID
	Action      string
	TargetType  string
	TargetID    *uuid.UUID
	RequestID   *uuid.UUID
	IP          string
	UserAgent   string
	Metadata    map[string]any
}

type DMReport struct {
	ID                         uuid.UUID
	ReporterUserID             uuid.UUID
	ReporterHandle             string
	ReporterDisplayName        string
	ThreadID                   uuid.UUID
	MessageID                  uuid.UUID
	MessageSenderID            uuid.UUID
	MessageSenderHandle        string
	MessageSenderDisplayName   string
	Category                   string
	Reason                     string
	IncludePlaintext           bool
	ReporterSubmittedPlaintext string
	AttachmentsNote            string
	Status                     string
	ResolvedAt                 *time.Time
	CreatedAt                  time.Time
}

type CreateDMReportInput struct {
	ReporterUserID             uuid.UUID
	ThreadID                   uuid.UUID
	MessageID                  uuid.UUID
	Category                   string
	Reason                     string
	IncludePlaintext           bool
	ReporterSubmittedPlaintext string
	AttachmentsNote            string
}

type LegalDisclosurePackage struct {
	Manifest     map[string]any        `json:"manifest"`
	Request      LawEnforcementRequest `json:"request"`
	Account      map[string]any        `json:"account,omitempty"`
	DM           []map[string]any      `json:"direct_messages,omitempty"`
	Reports      []map[string]any      `json:"reports,omitempty"`
	AccessEvents []map[string]any      `json:"access_events,omitempty"`
	Audit        []map[string]any      `json:"audit_events,omitempty"`
	Notes        map[string]string     `json:"notes"`
}

func normalizeLegalRequestStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "incoming", "reviewing", "preserved", "responded", "rejected", "closed":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "incoming"
	}
}

func normalizeLegalRequestType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "legal_process", "preservation", "emergency", "user_notice", "other":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "legal_process"
	}
}

func normalizeUserNoticeStatus(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "not_applicable", "pending", "sent", "delayed", "prohibited":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "not_applicable"
	}
}

func NormalizeReportCategory(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "spam", "abuse", "legal", "safety":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "other"
	}
}

func uuidPtrFromPg(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id, err := uuid.FromBytes(v.Bytes[:])
	if err != nil {
		return nil
	}
	return &id
}

func scanLawEnforcementRequest(row pgx.Row) (LawEnforcementRequest, error) {
	var out LawEnforcementRequest
	var targetUserID, assignedAdminID pgtype.UUID
	var targetFromAt, targetUntilAt, dueAt pgtype.Timestamptz
	err := row.Scan(
		&out.ID,
		&out.RequestType,
		&out.AgencyName,
		&out.Jurisdiction,
		&out.LegalBasis,
		&out.ExternalRef,
		&targetUserID,
		&out.TargetHandle,
		&targetFromAt,
		&targetUntilAt,
		&out.DataTypes,
		&out.Emergency,
		&out.Status,
		&dueAt,
		&assignedAdminID,
		&out.ResponseSummary,
		&out.UserNoticeStatus,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	out.TargetUserID = uuidPtrFromPg(targetUserID)
	out.AssignedAdminID = uuidPtrFromPg(assignedAdminID)
	out.TargetFromAt = ptrTimestamptz(targetFromAt)
	out.TargetUntilAt = ptrTimestamptz(targetUntilAt)
	out.DueAt = ptrTimestamptz(dueAt)
	return out, err
}

const lawRequestSelect = `
	SELECT id, request_type, agency_name, jurisdiction, legal_basis, external_reference,
		target_user_id, target_handle, target_from_at, target_until_at, data_types, emergency,
		status, due_at, assigned_admin_id, response_summary, user_notice_status, created_at, updated_at
	FROM law_enforcement_requests`

func (p *Pool) ListLawEnforcementRequests(ctx context.Context, limit int) ([]LawEnforcementRequest, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, lawRequestSelect+` ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LawEnforcementRequest, 0, limit)
	for rows.Next() {
		item, err := scanLawEnforcementRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (p *Pool) LawEnforcementRequestByID(ctx context.Context, id uuid.UUID) (LawEnforcementRequest, error) {
	row, err := scanLawEnforcementRequest(p.db.QueryRow(ctx, lawRequestSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return LawEnforcementRequest{}, ErrNotFound
	}
	return row, err
}

func (p *Pool) CreateLawEnforcementRequest(ctx context.Context, in CreateLawEnforcementRequestInput) (LawEnforcementRequest, error) {
	requestType := normalizeLegalRequestType(in.RequestType)
	status := normalizeLegalRequestStatus("")
	noticeStatus := normalizeUserNoticeStatus(in.UserNoticeStatus)
	return scanLawEnforcementRequest(p.db.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO law_enforcement_requests (
				request_type, agency_name, jurisdiction, legal_basis, external_reference,
				target_user_id, target_handle, target_from_at, target_until_at, data_types,
				emergency, status, due_at, assigned_admin_id, response_summary, user_notice_status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			RETURNING id
		)
		`+lawRequestSelect+` WHERE id = (SELECT id FROM ins)`,
		requestType,
		strings.TrimSpace(in.AgencyName),
		strings.TrimSpace(in.Jurisdiction),
		strings.TrimSpace(in.LegalBasis),
		strings.TrimSpace(in.ExternalRef),
		in.TargetUserID,
		strings.TrimSpace(in.TargetHandle),
		in.TargetFromAt,
		in.TargetUntilAt,
		normalizeStringSlice(in.DataTypes, 12),
		in.Emergency,
		status,
		in.DueAt,
		in.AssignedAdminID,
		strings.TrimSpace(in.ResponseSummary),
		noticeStatus,
	))
}

func (p *Pool) UpdateLawEnforcementRequestStatus(ctx context.Context, id uuid.UUID, status, responseSummary, noticeStatus string) (LawEnforcementRequest, error) {
	status = normalizeLegalRequestStatus(status)
	noticeStatus = normalizeUserNoticeStatus(noticeStatus)
	row, err := scanLawEnforcementRequest(p.db.QueryRow(ctx, `
		WITH upd AS (
			UPDATE law_enforcement_requests
			SET status = $2,
				response_summary = $3,
				user_notice_status = $4,
				updated_at = NOW()
			WHERE id = $1
			RETURNING id
		)
		`+lawRequestSelect+` WHERE id = (SELECT id FROM upd)`,
		id, status, strings.TrimSpace(responseSummary), noticeStatus,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return LawEnforcementRequest{}, ErrNotFound
	}
	return row, err
}

func (p *Pool) CreateLegalPreservationHold(ctx context.Context, in CreateLegalPreservationHoldInput) (LegalPreservationHold, error) {
	var out LegalPreservationHold
	var targetUserID, resourceID pgtype.UUID
	var releasedAt pgtype.Timestamptz
	err := p.db.QueryRow(ctx, `
		INSERT INTO legal_preservation_holds (request_id, target_user_id, resource_type, resource_id, reason, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, request_id, target_user_id, resource_type, resource_id, reason, expires_at, released_at, created_at
	`, in.RequestID, in.TargetUserID, strings.TrimSpace(in.ResourceType), in.ResourceID, strings.TrimSpace(in.Reason), in.ExpiresAt.UTC()).Scan(
		&out.ID, &out.RequestID, &targetUserID, &out.ResourceType, &resourceID, &out.Reason, &out.ExpiresAt, &releasedAt, &out.CreatedAt,
	)
	if err != nil {
		return LegalPreservationHold{}, err
	}
	out.TargetUserID = uuidPtrFromPg(targetUserID)
	out.ResourceID = uuidPtrFromPg(resourceID)
	out.ReleasedAt = ptrTimestamptz(releasedAt)
	return out, nil
}

func (p *Pool) HasActiveLegalHold(ctx context.Context, targetUserID uuid.UUID, resourceType string, resourceID *uuid.UUID) (bool, error) {
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	var count int
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM legal_preservation_holds
		WHERE released_at IS NULL
			AND expires_at > NOW()
			AND (
				($1::uuid IS NOT NULL AND target_user_id = $1 AND (resource_type IN ('account', 'user') OR $2 = '' OR resource_type = $2))
				OR ($3::uuid IS NOT NULL AND resource_id = $3 AND ($2 = '' OR resource_type = $2))
			)
	`, nullableUUID(targetUserID), resourceType, resourceID).Scan(&count)
	return count > 0, err
}

func nullableUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func (p *Pool) RecordUserAccessEvent(ctx context.Context, userID uuid.UUID, eventType, ip, userAgent string) error {
	if userID == uuid.Nil {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO user_access_events (user_id, event_type, ip, user_agent)
		VALUES ($1, $2, $3, $4)
	`, userID, strings.ToLower(strings.TrimSpace(eventType)), strings.TrimSpace(ip), strings.TrimSpace(userAgent))
	return err
}

func (p *Pool) InsertAdminAuditEvent(ctx context.Context, in AdminAuditInput) error {
	metadata := in.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	var prevHash string
	_ = p.db.QueryRow(ctx, `SELECT event_hash FROM admin_audit_events ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&prevHash)
	body := map[string]any{
		"admin_user_id": in.AdminUserID.String(),
		"action":        strings.TrimSpace(in.Action),
		"target_type":   strings.TrimSpace(in.TargetType),
		"ip":            strings.TrimSpace(in.IP),
		"user_agent":    strings.TrimSpace(in.UserAgent),
		"metadata":      json.RawMessage(raw),
		"prev_hash":     prevHash,
	}
	if in.TargetID != nil {
		body["target_id"] = in.TargetID.String()
	}
	if in.RequestID != nil {
		body["request_id"] = in.RequestID.String()
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(append([]byte(prevHash), payload...))
	_, err = p.db.Exec(ctx, `
		INSERT INTO admin_audit_events (admin_user_id, action, target_type, target_id, request_id, ip, user_agent, metadata, prev_hash, event_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10)
	`, in.AdminUserID, strings.TrimSpace(in.Action), strings.TrimSpace(in.TargetType), in.TargetID, in.RequestID, strings.TrimSpace(in.IP), strings.TrimSpace(in.UserAgent), raw, prevHash, hex.EncodeToString(sum[:]))
	return err
}

func normalizeStringSlice(items []string, max int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		v := strings.ToLower(strings.TrimSpace(item))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if len(out) >= max {
			break
		}
	}
	return out
}

func (p *Pool) CreateDMReport(ctx context.Context, in CreateDMReportInput) (DMReport, error) {
	if in.ReporterUserID == uuid.Nil || in.ThreadID == uuid.Nil || in.MessageID == uuid.Nil {
		return DMReport{}, ErrNotFound
	}
	var exists bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM dm_messages m
			JOIN dm_threads t ON t.id = m.thread_id
			WHERE m.id = $1 AND m.thread_id = $2 AND ($3 = t.user_low_id OR $3 = t.user_high_id)
		)
	`, in.MessageID, in.ThreadID, in.ReporterUserID).Scan(&exists)
	if err != nil {
		return DMReport{}, err
	}
	if !exists {
		return DMReport{}, ErrNotFound
	}
	var id uuid.UUID
	category := NormalizeReportCategory(in.Category)
	err = p.db.QueryRow(ctx, `
		INSERT INTO dm_reports (reporter_user_id, thread_id, message_id, category, reason, include_plaintext, reporter_submitted_plaintext, attachments_note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (reporter_user_id, message_id) DO UPDATE
		SET category = EXCLUDED.category,
			reason = EXCLUDED.reason,
			include_plaintext = EXCLUDED.include_plaintext,
			reporter_submitted_plaintext = EXCLUDED.reporter_submitted_plaintext,
			attachments_note = EXCLUDED.attachments_note,
			status = 'open',
			resolved_at = NULL
		RETURNING id
	`, in.ReporterUserID, in.ThreadID, in.MessageID, category, strings.TrimSpace(in.Reason), in.IncludePlaintext, strings.TrimSpace(in.ReporterSubmittedPlaintext), strings.TrimSpace(in.AttachmentsNote)).Scan(&id)
	if err != nil {
		return DMReport{}, err
	}
	return p.DMReportByID(ctx, id)
}

func (p *Pool) DMReportByID(ctx context.Context, id uuid.UUID) (DMReport, error) {
	rows, err := p.listDMReports(ctx, `WHERE r.id = $1`, id)
	if err != nil {
		return DMReport{}, err
	}
	if len(rows) == 0 {
		return DMReport{}, ErrNotFound
	}
	return rows[0], nil
}

func (p *Pool) ListAdminDMReports(ctx context.Context, limit int) ([]DMReport, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return p.listDMReports(ctx, `ORDER BY CASE COALESCE(r.category, 'other') WHEN 'legal' THEN 0 WHEN 'safety' THEN 1 ELSE 2 END, r.created_at DESC LIMIT $1`, limit)
}

func (p *Pool) listDMReports(ctx context.Context, suffix string, args ...any) ([]DMReport, error) {
	rows, err := p.db.Query(ctx, `
		SELECT r.id, r.reporter_user_id,
			ru.handle,
			COALESCE(NULLIF(trim(ru.display_name), ''), trim(split_part(ru.email, '@', 1))),
			r.thread_id, r.message_id, m.sender_id,
			su.handle,
			COALESCE(NULLIF(trim(su.display_name), ''), trim(split_part(su.email, '@', 1))),
			r.category, r.reason, r.include_plaintext, r.reporter_submitted_plaintext, r.attachments_note,
			r.status, r.resolved_at, r.created_at
		FROM dm_reports r
		JOIN users ru ON ru.id = r.reporter_user_id
		JOIN dm_messages m ON m.id = r.message_id
		JOIN users su ON su.id = m.sender_id
		`+suffix, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DMReport{}
	for rows.Next() {
		var r DMReport
		var resolvedAt pgtype.Timestamptz
		if err := rows.Scan(
			&r.ID, &r.ReporterUserID, &r.ReporterHandle, &r.ReporterDisplayName,
			&r.ThreadID, &r.MessageID, &r.MessageSenderID, &r.MessageSenderHandle, &r.MessageSenderDisplayName,
			&r.Category, &r.Reason, &r.IncludePlaintext, &r.ReporterSubmittedPlaintext, &r.AttachmentsNote,
			&r.Status, &resolvedAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.ResolvedAt = ptrTimestamptz(resolvedAt)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) UpdateDMReportStatus(ctx context.Context, id uuid.UUID, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	tag, err := p.db.Exec(ctx, `
		UPDATE dm_reports
		SET status = $2,
			resolved_at = CASE WHEN $2 = 'open' THEN NULL ELSE NOW() END
		WHERE id = $1
	`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) BuildLegalDisclosurePackage(ctx context.Context, requestID uuid.UUID, generatedBy *uuid.UUID) (LegalDisclosurePackage, error) {
	req, err := p.LawEnforcementRequestByID(ctx, requestID)
	if err != nil {
		return LegalDisclosurePackage{}, err
	}
	dataTypes := legalDataTypeSet(req.DataTypes)
	pkg := LegalDisclosurePackage{
		Request: req,
		Notes: map[string]string{
			"dm_plaintext": "Direct message bodies and attachments are client-side encrypted. This package includes encrypted payloads and metadata stored by the server, not decrypted plaintext.",
		},
	}
	if req.TargetUserID == nil {
		pkg.Manifest = buildLegalDisclosureManifest(pkg, generatedBy, dataTypes)
		return pkg, nil
	}
	if dataTypes["account"] {
		u, err := p.UserByID(ctx, *req.TargetUserID)
		if err == nil {
			pkg.Account = map[string]any{
				"id":           u.ID.String(),
				"email":        u.Email,
				"handle":       u.Handle,
				"display_name": u.DisplayName,
				"suspended":    u.SuspendedAt != nil,
			}
		} else if !errors.Is(err, ErrNotFound) {
			return LegalDisclosurePackage{}, err
		}
	}
	if dataTypes["dm_metadata"] || dataTypes["encrypted_dm_payloads"] {
		pkg.DM, err = p.legalDMExport(ctx, *req.TargetUserID, req.TargetFromAt, req.TargetUntilAt, dataTypes["encrypted_dm_payloads"], 500)
		if err != nil {
			return LegalDisclosurePackage{}, err
		}
	}
	if dataTypes["dm_reports"] || dataTypes["reports"] {
		pkg.Reports, err = p.legalDMReportExport(ctx, *req.TargetUserID, 200)
		if err != nil {
			return LegalDisclosurePackage{}, err
		}
	}
	if dataTypes["access_events"] {
		pkg.AccessEvents, err = p.legalAccessEventExport(ctx, *req.TargetUserID, req.TargetFromAt, req.TargetUntilAt, 200)
		if err != nil {
			return LegalDisclosurePackage{}, err
		}
	}
	if dataTypes["audit_events"] {
		pkg.Audit, err = p.legalAuditExport(ctx, requestID, 200)
		if err != nil {
			return LegalDisclosurePackage{}, err
		}
	}
	pkg.Manifest = buildLegalDisclosureManifest(pkg, generatedBy, dataTypes)
	return pkg, nil
}

func legalDataTypeSet(items []string) map[string]bool {
	allowed := map[string]bool{
		"account":               true,
		"dm_metadata":           true,
		"encrypted_dm_payloads": true,
		"dm_reports":            true,
		"reports":               true,
		"access_events":         true,
		"audit_events":          true,
	}
	out := map[string]bool{}
	if len(items) == 0 {
		for k := range allowed {
			out[k] = true
		}
		return out
	}
	for _, item := range items {
		v := strings.ToLower(strings.TrimSpace(item))
		if allowed[v] {
			out[v] = true
		}
	}
	return out
}

func buildLegalDisclosureManifest(pkg LegalDisclosurePackage, generatedBy *uuid.UUID, dataTypes map[string]bool) map[string]any {
	sectionHashes := map[string]string{}
	counts := map[string]int{}
	addSection := func(name string, value any, count int) {
		if count <= 0 {
			return
		}
		raw, _ := json.Marshal(value)
		sum := sha256.Sum256(raw)
		sectionHashes[name] = hex.EncodeToString(sum[:])
		counts[name] = count
	}
	if pkg.Account != nil {
		addSection("account", pkg.Account, 1)
	}
	addSection("direct_messages", pkg.DM, len(pkg.DM))
	addSection("reports", pkg.Reports, len(pkg.Reports))
	addSection("access_events", pkg.AccessEvents, len(pkg.AccessEvents))
	addSection("audit_events", pkg.Audit, len(pkg.Audit))
	includedTypes := make([]string, 0, len(dataTypes))
	for k, v := range dataTypes {
		if v {
			includedTypes = append(includedTypes, k)
		}
	}
	manifest := map[string]any{
		"version":        1,
		"request_id":     pkg.Request.ID.String(),
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"included_types": includedTypes,
		"counts":         counts,
		"section_sha256": sectionHashes,
	}
	if generatedBy != nil && *generatedBy != uuid.Nil {
		manifest["generated_by_admin_id"] = generatedBy.String()
	}
	eventHashes := make([]string, 0, len(pkg.Audit))
	for _, row := range pkg.Audit {
		if v, ok := row["event_hash"].(string); ok && strings.TrimSpace(v) != "" {
			eventHashes = append(eventHashes, v)
		}
	}
	if len(eventHashes) > 0 {
		manifest["audit_event_hashes"] = eventHashes
	}
	tmp := pkg
	tmp.Manifest = manifest
	raw, _ := json.Marshal(tmp)
	sum := sha256.Sum256(raw)
	manifest["package_sha256"] = hex.EncodeToString(sum[:])
	return manifest
}

func (p *Pool) legalDMExport(ctx context.Context, userID uuid.UUID, fromAt, untilAt *time.Time, includePayloads bool, limit int) ([]map[string]any, error) {
	rows, err := p.db.Query(ctx, `
		SELECT m.id, m.thread_id, m.sender_id, m.sender_payload, m.recipient_payload, m.attachments, m.created_at,
			t.user_low_id, t.user_high_id
		FROM dm_messages m
		JOIN dm_threads t ON t.id = m.thread_id
		WHERE ($1 = t.user_low_id OR $1 = t.user_high_id)
			AND ($2::timestamptz IS NULL OR m.created_at >= $2)
			AND ($3::timestamptz IS NULL OR m.created_at <= $3)
		ORDER BY m.created_at DESC
		LIMIT $4
	`, userID, fromAt, untilAt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, threadID, senderID, lowID, highID uuid.UUID
		var senderPayload, recipientPayload, attachments []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &threadID, &senderID, &senderPayload, &recipientPayload, &attachments, &createdAt, &lowID, &highID); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":           id.String(),
			"thread_id":    threadID.String(),
			"sender_id":    senderID.String(),
			"user_low_id":  lowID.String(),
			"user_high_id": highID.String(),
			"created_at":   createdAt.UTC().Format(time.RFC3339),
		}
		if includePayloads {
			item["sender_payload"] = json.RawMessage(senderPayload)
			item["recipient_payload"] = json.RawMessage(recipientPayload)
			item["attachments"] = json.RawMessage(attachments)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (p *Pool) legalDMReportExport(ctx context.Context, userID uuid.UUID, limit int) ([]map[string]any, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, thread_id, message_id, category, reason, include_plaintext, reporter_submitted_plaintext, attachments_note, status, created_at
		FROM dm_reports
		WHERE reporter_user_id = $1
			OR message_id IN (SELECT id FROM dm_messages WHERE sender_id = $1)
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, threadID, messageID uuid.UUID
		var category, reason, plaintext, note, status string
		var includePlaintext bool
		var createdAt time.Time
		if err := rows.Scan(&id, &threadID, &messageID, &category, &reason, &includePlaintext, &plaintext, &note, &status, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":                           id.String(),
			"thread_id":                    threadID.String(),
			"message_id":                   messageID.String(),
			"category":                     category,
			"reason":                       reason,
			"include_plaintext":            includePlaintext,
			"reporter_submitted_plaintext": plaintext,
			"attachments_note":             note,
			"status":                       status,
			"created_at":                   createdAt.UTC().Format(time.RFC3339),
		})
	}
	return out, rows.Err()
}

func (p *Pool) legalAccessEventExport(ctx context.Context, userID uuid.UUID, fromAt, untilAt *time.Time, limit int) ([]map[string]any, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, event_type, ip, user_agent, created_at
		FROM user_access_events
		WHERE user_id = $1
			AND ($2::timestamptz IS NULL OR created_at >= $2)
			AND ($3::timestamptz IS NULL OR created_at <= $3)
		ORDER BY created_at DESC
		LIMIT $4
	`, userID, fromAt, untilAt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var eventType, ip, userAgent string
		var createdAt time.Time
		if err := rows.Scan(&id, &eventType, &ip, &userAgent, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":         id.String(),
			"event_type": eventType,
			"ip":         ip,
			"user_agent": userAgent,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	return out, rows.Err()
}

func (p *Pool) legalAuditExport(ctx context.Context, requestID uuid.UUID, limit int) ([]map[string]any, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, admin_user_id, action, target_type, target_id, ip, user_agent, metadata, prev_hash, event_hash, created_at
		FROM admin_audit_events
		WHERE request_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, requestID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var adminID, targetID pgtype.UUID
		var action, targetType, ip, ua, prevHash, eventHash string
		var metadata []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &adminID, &action, &targetType, &targetID, &ip, &ua, &metadata, &prevHash, &eventHash, &createdAt); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":          id.String(),
			"action":      action,
			"target_type": targetType,
			"ip":          ip,
			"user_agent":  ua,
			"metadata":    json.RawMessage(metadata),
			"prev_hash":   prevHash,
			"event_hash":  eventHash,
			"created_at":  createdAt.UTC().Format(time.RFC3339),
		}
		if v := uuidPtrFromPg(adminID); v != nil {
			item["admin_user_id"] = v.String()
		}
		if v := uuidPtrFromPg(targetID); v != nil {
			item["target_id"] = v.String()
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
