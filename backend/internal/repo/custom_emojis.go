package repo

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrCustomEmojiConflict = errors.New("custom emoji conflict")
	ErrInvalidCustomEmoji  = errors.New("invalid custom emoji")
)

var customEmojiNameRE = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

type CustomEmoji struct {
	ID            uuid.UUID
	ShortcodeName string
	OwnerUserID   *uuid.UUID
	OwnerHandle   string
	Domain        string
	ObjectKey     string
	IsEnabled     bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NormalizeCustomEmojiName(raw string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if !customEmojiNameRE.MatchString(name) {
		return "", false
	}
	return name, true
}

func MakeCustomEmojiShortcode(name, ownerHandle, domain string) string {
	name = strings.TrimSpace(name)
	ownerHandle = strings.TrimSpace(ownerHandle)
	domain = strings.TrimSpace(domain)
	switch {
	case name == "":
		return ""
	case domain != "":
		return ":" + name + "@" + domain + ":"
	case ownerHandle != "":
		return ":" + name + "@" + ownerHandle + ":"
	default:
		return ":" + name + ":"
	}
}

type EmojiReference struct {
	Raw         string
	IsShortcode bool
	Name        string
	OwnerHandle string
	Domain      string
}

func ParseEmojiReference(raw string) (EmojiReference, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return EmojiReference{}, false
	}
	ref := EmojiReference{Raw: trimmed}
	if !strings.HasPrefix(trimmed, ":") || !strings.HasSuffix(trimmed, ":") || len(trimmed) < 3 {
		return ref, true
	}
	body := strings.TrimSuffix(strings.TrimPrefix(trimmed, ":"), ":")
	if strings.Contains(body, ":") {
		return EmojiReference{}, false
	}
	namePart := body
	scopePart := ""
	if idx := strings.LastIndex(body, "@"); idx >= 0 {
		namePart = body[:idx]
		scopePart = body[idx+1:]
	}
	name, ok := NormalizeCustomEmojiName(namePart)
	if !ok {
		return EmojiReference{}, false
	}
	ref.IsShortcode = true
	ref.Name = name
	if scopePart == "" {
		return ref, true
	}
	scopePart = strings.TrimSpace(strings.ToLower(scopePart))
	if scopePart == "" {
		return EmojiReference{}, false
	}
	if strings.Contains(scopePart, ".") {
		ref.Domain = scopePart
		return ref, true
	}
	if !customEmojiNameRE.MatchString(scopePart) {
		return EmojiReference{}, false
	}
	ref.OwnerHandle = scopePart
	return ref, true
}

func scanCustomEmojiRow(rows pgx.Rows) ([]CustomEmoji, error) {
	var out []CustomEmoji
	for rows.Next() {
		var item CustomEmoji
		var ownerID pgtype.UUID
		var ownerHandle pgtype.Text
		if err := rows.Scan(
			&item.ID,
			&item.ShortcodeName,
			&ownerID,
			&ownerHandle,
			&item.Domain,
			&item.ObjectKey,
			&item.IsEnabled,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if ownerID.Valid {
			id := uuid.UUID(ownerID.Bytes)
			item.OwnerUserID = &id
		}
		if ownerHandle.Valid {
			item.OwnerHandle = strings.TrimSpace(ownerHandle.String)
		}
		item.Domain = strings.TrimSpace(item.Domain)
		item.ShortcodeName = strings.TrimSpace(strings.ToLower(item.ShortcodeName))
		item.ObjectKey = strings.TrimSpace(item.ObjectKey)
		out = append(out, item)
	}
	return out, rows.Err()
}

func customEmojiConflict(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ErrCustomEmojiConflict
	}
	return err
}

func (p *Pool) ListEnabledCustomEmojis(ctx context.Context) ([]CustomEmoji, error) {
	rows, err := p.db.Query(ctx, `
		SELECT ce.id, ce.shortcode_name, ce.owner_user_id, u.handle, COALESCE(ce.domain, ''), ce.object_key, ce.is_enabled, ce.created_at, ce.updated_at
		FROM custom_emojis ce
		LEFT JOIN users u ON u.id = ce.owner_user_id
		WHERE ce.is_enabled = TRUE
		ORDER BY ce.owner_user_id NULLS FIRST, ce.created_at ASC, ce.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCustomEmojiRow(rows)
}

func (p *Pool) ListUserCustomEmojis(ctx context.Context, ownerID uuid.UUID) ([]CustomEmoji, error) {
	rows, err := p.db.Query(ctx, `
		SELECT ce.id, ce.shortcode_name, ce.owner_user_id, u.handle, COALESCE(ce.domain, ''), ce.object_key, ce.is_enabled, ce.created_at, ce.updated_at
		FROM custom_emojis ce
		LEFT JOIN users u ON u.id = ce.owner_user_id
		WHERE ce.owner_user_id = $1 AND COALESCE(btrim(ce.domain), '') = ''
		ORDER BY ce.created_at ASC, ce.id ASC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCustomEmojiRow(rows)
}

func (p *Pool) ListSiteCustomEmojis(ctx context.Context) ([]CustomEmoji, error) {
	rows, err := p.db.Query(ctx, `
		SELECT ce.id, ce.shortcode_name, ce.owner_user_id, u.handle, COALESCE(ce.domain, ''), ce.object_key, ce.is_enabled, ce.created_at, ce.updated_at
		FROM custom_emojis ce
		LEFT JOIN users u ON u.id = ce.owner_user_id
		WHERE ce.owner_user_id IS NULL AND COALESCE(btrim(ce.domain), '') = ''
		ORDER BY ce.created_at ASC, ce.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCustomEmojiRow(rows)
}

func (p *Pool) CreateUserCustomEmoji(ctx context.Context, ownerID uuid.UUID, shortcodeName, objectKey string, enabled bool) (CustomEmoji, error) {
	name, ok := NormalizeCustomEmojiName(shortcodeName)
	if !ok || strings.TrimSpace(objectKey) == "" {
		return CustomEmoji{}, ErrInvalidCustomEmoji
	}
	var item CustomEmoji
	var ownerHandle pgtype.Text
	err := p.db.QueryRow(ctx, `
		INSERT INTO custom_emojis (shortcode_name, owner_user_id, object_key, is_enabled)
		VALUES ($1, $2, $3, $4)
		RETURNING id, shortcode_name, owner_user_id, ''::text, COALESCE(domain, ''), object_key, is_enabled, created_at, updated_at
	`, name, ownerID, strings.TrimSpace(objectKey), enabled).Scan(
		&item.ID, &item.ShortcodeName, new(pgtype.UUID), &ownerHandle, &item.Domain, &item.ObjectKey, &item.IsEnabled, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return CustomEmoji{}, customEmojiConflict(err)
	}
	user, err := p.UserByID(ctx, ownerID)
	if err != nil {
		return CustomEmoji{}, err
	}
	item.ShortcodeName = name
	item.OwnerUserID = &ownerID
	item.OwnerHandle = user.Handle
	return item, nil
}

func (p *Pool) CreateSiteCustomEmoji(ctx context.Context, shortcodeName, objectKey string, enabled bool) (CustomEmoji, error) {
	name, ok := NormalizeCustomEmojiName(shortcodeName)
	if !ok || strings.TrimSpace(objectKey) == "" {
		return CustomEmoji{}, ErrInvalidCustomEmoji
	}
	var item CustomEmoji
	var ownerID pgtype.UUID
	var ownerHandle pgtype.Text
	err := p.db.QueryRow(ctx, `
		INSERT INTO custom_emojis (shortcode_name, object_key, is_enabled)
		VALUES ($1, $2, $3)
		RETURNING id, shortcode_name, owner_user_id, ''::text, COALESCE(domain, ''), object_key, is_enabled, created_at, updated_at
	`, name, strings.TrimSpace(objectKey), enabled).Scan(
		&item.ID, &item.ShortcodeName, &ownerID, &ownerHandle, &item.Domain, &item.ObjectKey, &item.IsEnabled, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return CustomEmoji{}, customEmojiConflict(err)
	}
	item.ShortcodeName = name
	return item, nil
}

func (p *Pool) UpdateUserCustomEmoji(ctx context.Context, ownerID, emojiID uuid.UUID, shortcodeName string, objectKey *string, enabled bool) (CustomEmoji, error) {
	name, ok := NormalizeCustomEmojiName(shortcodeName)
	if !ok {
		return CustomEmoji{}, ErrInvalidCustomEmoji
	}
	var keyArg any
	if objectKey != nil {
		trimmed := strings.TrimSpace(*objectKey)
		if trimmed == "" {
			return CustomEmoji{}, ErrInvalidCustomEmoji
		}
		keyArg = trimmed
	}
	var item CustomEmoji
	var itemOwnerID pgtype.UUID
	var ownerHandle pgtype.Text
	err := p.db.QueryRow(ctx, `
		UPDATE custom_emojis
		SET shortcode_name = $3,
			object_key = COALESCE($4, object_key),
			is_enabled = $5,
			updated_at = NOW()
		WHERE id = $1 AND owner_user_id = $2 AND COALESCE(btrim(domain), '') = ''
		RETURNING id, shortcode_name, owner_user_id, ''::text, COALESCE(domain, ''), object_key, is_enabled, created_at, updated_at
	`, emojiID, ownerID, name, keyArg, enabled).Scan(
		&item.ID, &item.ShortcodeName, &itemOwnerID, &ownerHandle, &item.Domain, &item.ObjectKey, &item.IsEnabled, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomEmoji{}, ErrNotFound
	}
	if err != nil {
		return CustomEmoji{}, customEmojiConflict(err)
	}
	user, err := p.UserByID(ctx, ownerID)
	if err != nil {
		return CustomEmoji{}, err
	}
	item.OwnerUserID = &ownerID
	item.OwnerHandle = user.Handle
	item.ShortcodeName = name
	return item, nil
}

func (p *Pool) UpdateSiteCustomEmoji(ctx context.Context, emojiID uuid.UUID, shortcodeName string, objectKey *string, enabled bool) (CustomEmoji, error) {
	name, ok := NormalizeCustomEmojiName(shortcodeName)
	if !ok {
		return CustomEmoji{}, ErrInvalidCustomEmoji
	}
	var keyArg any
	if objectKey != nil {
		trimmed := strings.TrimSpace(*objectKey)
		if trimmed == "" {
			return CustomEmoji{}, ErrInvalidCustomEmoji
		}
		keyArg = trimmed
	}
	var item CustomEmoji
	var ownerID pgtype.UUID
	var ownerHandle pgtype.Text
	err := p.db.QueryRow(ctx, `
		UPDATE custom_emojis
		SET shortcode_name = $2,
			object_key = COALESCE($3, object_key),
			is_enabled = $4,
			updated_at = NOW()
		WHERE id = $1 AND owner_user_id IS NULL AND COALESCE(btrim(domain), '') = ''
		RETURNING id, shortcode_name, owner_user_id, ''::text, COALESCE(domain, ''), object_key, is_enabled, created_at, updated_at
	`, emojiID, name, keyArg, enabled).Scan(
		&item.ID, &item.ShortcodeName, &ownerID, &ownerHandle, &item.Domain, &item.ObjectKey, &item.IsEnabled, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomEmoji{}, ErrNotFound
	}
	if err != nil {
		return CustomEmoji{}, customEmojiConflict(err)
	}
	item.ShortcodeName = name
	return item, nil
}

func (p *Pool) DeleteUserCustomEmoji(ctx context.Context, ownerID, emojiID uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM custom_emojis WHERE id = $1 AND owner_user_id = $2 AND COALESCE(btrim(domain), '') = ''`, emojiID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) DeleteSiteCustomEmoji(ctx context.Context, emojiID uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM custom_emojis WHERE id = $1 AND owner_user_id IS NULL AND COALESCE(btrim(domain), '') = ''`, emojiID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Pool) FindEnabledCustomEmojiByReference(ctx context.Context, ref EmojiReference) (CustomEmoji, error) {
	if !ref.IsShortcode || ref.Name == "" {
		return CustomEmoji{}, ErrNotFound
	}
	var (
		row      CustomEmoji
		ownerID  pgtype.UUID
		handleTx pgtype.Text
		err      error
	)
	switch {
	case ref.OwnerHandle != "":
		err = p.db.QueryRow(ctx, `
			SELECT ce.id, ce.shortcode_name, ce.owner_user_id, u.handle, COALESCE(ce.domain, ''), ce.object_key, ce.is_enabled, ce.created_at, ce.updated_at
			FROM custom_emojis ce
			JOIN users u ON u.id = ce.owner_user_id
			WHERE ce.is_enabled = TRUE
			  AND COALESCE(btrim(ce.domain), '') = ''
			  AND lower(ce.shortcode_name) = lower($1)
			  AND lower(u.handle) = lower($2)
			LIMIT 1
		`, ref.Name, ref.OwnerHandle).Scan(
			&row.ID, &row.ShortcodeName, &ownerID, &handleTx, &row.Domain, &row.ObjectKey, &row.IsEnabled, &row.CreatedAt, &row.UpdatedAt,
		)
	case ref.Domain != "":
		err = p.db.QueryRow(ctx, `
			SELECT ce.id, ce.shortcode_name, ce.owner_user_id, u.handle, COALESCE(ce.domain, ''), ce.object_key, ce.is_enabled, ce.created_at, ce.updated_at
			FROM custom_emojis ce
			LEFT JOIN users u ON u.id = ce.owner_user_id
			WHERE ce.is_enabled = TRUE
			  AND lower(ce.shortcode_name) = lower($1)
			  AND lower(COALESCE(ce.domain, '')) = lower($2)
			LIMIT 1
		`, ref.Name, ref.Domain).Scan(
			&row.ID, &row.ShortcodeName, &ownerID, &handleTx, &row.Domain, &row.ObjectKey, &row.IsEnabled, &row.CreatedAt, &row.UpdatedAt,
		)
	default:
		err = p.db.QueryRow(ctx, `
			SELECT ce.id, ce.shortcode_name, ce.owner_user_id, u.handle, COALESCE(ce.domain, ''), ce.object_key, ce.is_enabled, ce.created_at, ce.updated_at
			FROM custom_emojis ce
			LEFT JOIN users u ON u.id = ce.owner_user_id
			WHERE ce.is_enabled = TRUE
			  AND ce.owner_user_id IS NULL
			  AND COALESCE(btrim(ce.domain), '') = ''
			  AND lower(ce.shortcode_name) = lower($1)
			LIMIT 1
		`, ref.Name).Scan(
			&row.ID, &row.ShortcodeName, &ownerID, &handleTx, &row.Domain, &row.ObjectKey, &row.IsEnabled, &row.CreatedAt, &row.UpdatedAt,
		)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomEmoji{}, ErrNotFound
	}
	if err != nil {
		return CustomEmoji{}, err
	}
	if ownerID.Valid {
		id := uuid.UUID(ownerID.Bytes)
		row.OwnerUserID = &id
	}
	if handleTx.Valid {
		row.OwnerHandle = strings.TrimSpace(handleTx.String)
	}
	row.ShortcodeName = strings.TrimSpace(strings.ToLower(row.ShortcodeName))
	row.Domain = strings.TrimSpace(row.Domain)
	return row, nil
}
