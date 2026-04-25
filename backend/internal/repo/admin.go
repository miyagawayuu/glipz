package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AdminUserRow struct {
	ID          uuid.UUID
	Email       string
	Handle      string
	DisplayName string
	Badges      []string
	SuspendedAt *time.Time
	CreatedAt   time.Time
}

type AdminUserFilter struct {
	Query  string
	Status string
	Limit  int
	Offset int
}

func (p *Pool) ListAdminUsers(ctx context.Context, filter AdminUserFilter) ([]AdminUserRow, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query := strings.TrimSpace(filter.Query)
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	where := []string{"TRUE"}
	args := []any{}
	if query != "" {
		args = append(args, "%"+strings.ToLower(query)+"%")
		where = append(where, "(lower(email) LIKE $1 OR lower(handle) LIKE $1 OR lower(display_name) LIKE $1)")
	}
	if status == "suspended" {
		where = append(where, "suspended_at IS NOT NULL")
	} else if status == "active" {
		where = append(where, "suspended_at IS NULL")
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := p.db.QueryRow(ctx, "SELECT COUNT(*)::int FROM users WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitParam := len(args) + 1
	offsetParam := len(args) + 2
	args = append(args, limit, offset)
	rows, err := p.db.Query(ctx, `
		SELECT id, email, handle, display_name, COALESCE(badges, '{}'::text[]), suspended_at, created_at
		FROM users
		WHERE `+whereSQL+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+itoa(limitParam)+` OFFSET $`+itoa(offsetParam), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]AdminUserRow, 0, limit)
	for rows.Next() {
		var row AdminUserRow
		var suspendedAt pgtype.Timestamptz
		var badges []string
		if err := rows.Scan(&row.ID, &row.Email, &row.Handle, &row.DisplayName, &badges, &suspendedAt, &row.CreatedAt); err != nil {
			return nil, 0, err
		}
		row.Badges = NormalizeUserBadges(badges)
		row.SuspendedAt = ptrTimestamptz(suspendedAt)
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (p *Pool) AdminUserByID(ctx context.Context, id uuid.UUID) (AdminUserRow, error) {
	var row AdminUserRow
	var suspendedAt pgtype.Timestamptz
	var badges []string
	err := p.db.QueryRow(ctx, `
		SELECT id, email, handle, display_name, COALESCE(badges, '{}'::text[]), suspended_at, created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&row.ID, &row.Email, &row.Handle, &row.DisplayName, &badges, &suspendedAt, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminUserRow{}, ErrNotFound
	}
	if err != nil {
		return AdminUserRow{}, err
	}
	row.Badges = NormalizeUserBadges(badges)
	row.SuspendedAt = ptrTimestamptz(suspendedAt)
	return row, nil
}

func (p *Pool) UnsuspendUser(ctx context.Context, userID uuid.UUID) error {
	ct, err := p.db.Exec(ctx, `UPDATE users SET suspended_at = NULL WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) CountSuspendedUsers(ctx context.Context) (int64, error) {
	var n int64
	err := p.db.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM users WHERE suspended_at IS NOT NULL`).Scan(&n)
	return n, err
}

func (p *Pool) CountOpenReports(ctx context.Context) (int64, int64, error) {
	var local, federated int64
	if err := p.db.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM post_reports WHERE status = 'open'`).Scan(&local); err != nil {
		return 0, 0, err
	}
	if err := p.db.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM federation_incoming_post_reports WHERE status = 'open'`).Scan(&federated); err != nil {
		return 0, 0, err
	}
	return local, federated, nil
}

type OperatorAnnouncement struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Date  string `json:"date"`
}

type SiteSettings struct {
	RegistrationsEnabled    bool                   `json:"registrations_enabled"`
	ServerName              string                 `json:"server_name"`
	ServerDescription       string                 `json:"server_description"`
	AdminName               string                 `json:"admin_name"`
	AdminEmail              string                 `json:"admin_email"`
	TermsURL                string                 `json:"terms_url"`
	PrivacyPolicyURL        string                 `json:"privacy_policy_url"`
	NSFWGuidelinesURL       string                 `json:"nsfw_guidelines_url"`
	FederationPolicySummary string                 `json:"federation_policy_summary"`
	OperatorAnnouncements   []OperatorAnnouncement `json:"operator_announcements"`
}

func DefaultSiteSettings() SiteSettings {
	return SiteSettings{
		RegistrationsEnabled:  true,
		OperatorAnnouncements: []OperatorAnnouncement{},
	}
}

func (p *Pool) GetSiteSettings(ctx context.Context) (SiteSettings, error) {
	settings := DefaultSiteSettings()
	rows, err := p.db.Query(ctx, `SELECT key, value FROM site_settings`)
	if err != nil {
		return settings, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return settings, err
		}
		switch key {
		case "registrations_enabled":
			_ = json.Unmarshal(raw, &settings.RegistrationsEnabled)
		case "server_name":
			_ = json.Unmarshal(raw, &settings.ServerName)
		case "server_description":
			_ = json.Unmarshal(raw, &settings.ServerDescription)
		case "admin_name":
			_ = json.Unmarshal(raw, &settings.AdminName)
		case "admin_email":
			_ = json.Unmarshal(raw, &settings.AdminEmail)
		case "terms_url":
			_ = json.Unmarshal(raw, &settings.TermsURL)
		case "privacy_policy_url":
			_ = json.Unmarshal(raw, &settings.PrivacyPolicyURL)
		case "nsfw_guidelines_url":
			_ = json.Unmarshal(raw, &settings.NSFWGuidelinesURL)
		case "federation_policy_summary":
			_ = json.Unmarshal(raw, &settings.FederationPolicySummary)
		case "operator_announcements":
			_ = json.Unmarshal(raw, &settings.OperatorAnnouncements)
		}
	}
	return settings, rows.Err()
}

func (p *Pool) UpdateSiteSettings(ctx context.Context, settings SiteSettings) (SiteSettings, error) {
	if settings.OperatorAnnouncements == nil {
		settings.OperatorAnnouncements = []OperatorAnnouncement{}
	}
	values := map[string]any{
		"registrations_enabled":     settings.RegistrationsEnabled,
		"server_name":               strings.TrimSpace(settings.ServerName),
		"server_description":        strings.TrimSpace(settings.ServerDescription),
		"admin_name":                strings.TrimSpace(settings.AdminName),
		"admin_email":               strings.TrimSpace(settings.AdminEmail),
		"terms_url":                 strings.TrimSpace(settings.TermsURL),
		"privacy_policy_url":        strings.TrimSpace(settings.PrivacyPolicyURL),
		"nsfw_guidelines_url":       strings.TrimSpace(settings.NSFWGuidelinesURL),
		"federation_policy_summary": strings.TrimSpace(settings.FederationPolicySummary),
		"operator_announcements":    settings.OperatorAnnouncements,
	}
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return SiteSettings{}, err
		}
		if _, err := p.db.Exec(ctx, `
			INSERT INTO site_settings (key, value, updated_at)
			VALUES ($1, $2::jsonb, NOW())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		`, key, string(raw)); err != nil {
			return SiteSettings{}, err
		}
	}
	return p.GetSiteSettings(ctx)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
