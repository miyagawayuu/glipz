package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Note represents a row from the notes table.
type Note struct {
	ID                          uuid.UUID
	UserID                      uuid.UUID
	Title                       string
	BodyMd                      string
	BodyPremiumMd               string
	PatreonCampaignID           *string
	PatreonRequiredRewardTierID *string
	EditorMode                  string
	Status                      string
	Visibility                  string
	ViewPasswordHash            *string
	ViewPasswordHint            *string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

// NoteWithAuthor combines a note with the author's public profile projection.
type NoteWithAuthor struct {
	Note
	AuthorEmail       string
	AuthorHandle      string
	AuthorDisplayName string
	AuthorAvatarKey   *string
}

var (
	ErrEmptyNote    = errors.New("empty_note")
	ErrTitleTooLong = errors.New("title_too_long")
	ErrBodyTooLong  = errors.New("body_too_long")
)

func normalizeEditorMode(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "richtext" {
		return "richtext"
	}
	return "markdown"
}

func normalizeNoteStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "draft" {
		return "draft"
	}
	return "published"
}

func normalizeNoteVisibility(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "followers":
		return "followers"
	case "private":
		return "private"
	default:
		return "public"
	}
}

func validateNoteLengths(title, bodyMd, bodyPremiumMd string) error {
	if len(title) > 500 {
		return ErrTitleTooLong
	}
	if len(bodyMd) > 2_000_000 || len(bodyPremiumMd) > 2_000_000 {
		return ErrBodyTooLong
	}
	return nil
}

func noteHasAnyContent(title, bodyMd, bodyPremiumMd string) bool {
	return strings.TrimSpace(title) != "" || strings.TrimSpace(bodyMd) != "" || strings.TrimSpace(bodyPremiumMd) != ""
}

// CreateNote creates a new note.
// Published notes must have either a title or body content.
func (p *Pool) CreateNote(ctx context.Context, userID uuid.UUID, title, bodyMd, bodyPremiumMd, editorMode, status, visibility, viewPasswordHash, viewPasswordHint, patreonCampaignID, patreonRequiredRewardTierID string) (uuid.UUID, error) {
	title = strings.TrimSpace(title)
	bodyMd = strings.TrimSpace(bodyMd)
	bodyPremiumMd = strings.TrimSpace(bodyPremiumMd)
	viewPasswordHash = strings.TrimSpace(viewPasswordHash)
	viewPasswordHint = strings.TrimSpace(viewPasswordHint)
	patreonCampaignID = strings.TrimSpace(patreonCampaignID)
	patreonRequiredRewardTierID = strings.TrimSpace(patreonRequiredRewardTierID)
	st := normalizeNoteStatus(status)
	vis := normalizeNoteVisibility(visibility)
	if err := validateNoteLengths(title, bodyMd, bodyPremiumMd); err != nil {
		return uuid.Nil, err
	}
	if st == "published" && !noteHasAnyContent(title, bodyMd, bodyPremiumMd) {
		return uuid.Nil, ErrEmptyNote
	}
	em := normalizeEditorMode(editorMode)
	var id uuid.UUID
	err := p.db.QueryRow(ctx, `
		INSERT INTO notes (
			user_id, title, body_md, body_premium_md, editor_mode, status, visibility,
			view_password_hash, view_password_hint,
			patreon_campaign_id, patreon_required_reward_tier_id
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			NULLIF($8, ''), NULLIF($9, ''),
			NULLIF($10, ''), NULLIF($11, '')
		)
		RETURNING id
	`, userID, title, bodyMd, bodyPremiumMd, em, st, vis, viewPasswordHash, viewPasswordHint, patreonCampaignID, patreonRequiredRewardTierID).Scan(&id)
	return id, err
}

// NoteByID returns one note together with its author information.
func (p *Pool) NoteByID(ctx context.Context, noteID uuid.UUID) (NoteWithAuthor, error) {
	var out NoteWithAuthor
	var av pgtype.Text
	var pwHash, pwHint pgtype.Text
	var pc, pt pgtype.Text
	err := p.db.QueryRow(ctx, `
		SELECT n.id, n.user_id, n.title, n.body_md, n.body_premium_md, n.editor_mode, n.status, n.visibility,
			n.view_password_hash, n.view_password_hint,
			n.patreon_campaign_id, n.patreon_required_reward_tier_id,
			n.created_at, n.updated_at,
			u.email, u.handle, u.display_name, u.avatar_object_key
		FROM notes n
		JOIN users u ON u.id = n.user_id
		WHERE n.id = $1
	`, noteID).Scan(
		&out.ID, &out.UserID, &out.Title, &out.BodyMd, &out.BodyPremiumMd, &out.EditorMode, &out.Status, &out.Visibility,
		&pwHash, &pwHint, &pc, &pt,
		&out.CreatedAt, &out.UpdatedAt,
		&out.AuthorEmail, &out.AuthorHandle, &out.AuthorDisplayName, &av,
	)
	if pwHash.Valid && strings.TrimSpace(pwHash.String) != "" {
		s := strings.TrimSpace(pwHash.String)
		out.ViewPasswordHash = &s
	}
	if pwHint.Valid && strings.TrimSpace(pwHint.String) != "" {
		s := strings.TrimSpace(pwHint.String)
		out.ViewPasswordHint = &s
	}
	if pc.Valid && strings.TrimSpace(pc.String) != "" {
		s := strings.TrimSpace(pc.String)
		out.PatreonCampaignID = &s
	}
	if pt.Valid && strings.TrimSpace(pt.String) != "" {
		s := strings.TrimSpace(pt.String)
		out.PatreonRequiredRewardTierID = &s
	}
	if av.Valid && strings.TrimSpace(av.String) != "" {
		s := strings.TrimSpace(av.String)
		out.AuthorAvatarKey = &s
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return NoteWithAuthor{}, ErrNotFound
	}
	if err != nil {
		return NoteWithAuthor{}, err
	}
	return out, nil
}

// UpdateNote lets only the author update a note.
func (p *Pool) UpdateNote(ctx context.Context, userID, noteID uuid.UUID, title, bodyMd, bodyPremiumMd, editorMode, status, visibility, viewPasswordHash, viewPasswordHint, patreonCampaignID, patreonRequiredRewardTierID string) error {
	title = strings.TrimSpace(title)
	bodyMd = strings.TrimSpace(bodyMd)
	bodyPremiumMd = strings.TrimSpace(bodyPremiumMd)
	viewPasswordHash = strings.TrimSpace(viewPasswordHash)
	viewPasswordHint = strings.TrimSpace(viewPasswordHint)
	patreonCampaignID = strings.TrimSpace(patreonCampaignID)
	patreonRequiredRewardTierID = strings.TrimSpace(patreonRequiredRewardTierID)
	st := normalizeNoteStatus(status)
	vis := normalizeNoteVisibility(visibility)
	if err := validateNoteLengths(title, bodyMd, bodyPremiumMd); err != nil {
		return err
	}
	if st == "published" && !noteHasAnyContent(title, bodyMd, bodyPremiumMd) {
		return ErrEmptyNote
	}
	em := normalizeEditorMode(editorMode)
	tag, err := p.db.Exec(ctx, `
		UPDATE notes SET title = $1, body_md = $2, body_premium_md = $3, editor_mode = $4, status = $5, visibility = $6,
			view_password_hash = NULLIF($7, ''),
			view_password_hint = NULLIF($8, ''),
			patreon_campaign_id = NULLIF($9, ''),
			patreon_required_reward_tier_id = NULLIF($10, ''),
			updated_at = NOW()
		WHERE id = $11 AND user_id = $12
	`, title, bodyMd, bodyPremiumMd, em, st, vis, viewPasswordHash, viewPasswordHint, patreonCampaignID, patreonRequiredRewardTierID, noteID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteNote lets only the author delete a note.
func (p *Pool) DeleteNote(ctx context.Context, userID, noteID uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM notes WHERE id = $1 AND user_id = $2`, noteID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
