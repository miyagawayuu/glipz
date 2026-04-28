package repo

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AccountDeletionResult struct {
	ObjectKeys []string
}

func (p *Pool) DeleteUserAccount(ctx context.Context, userID uuid.UUID) (AccountDeletionResult, error) {
	if userID == uuid.Nil {
		return AccountDeletionResult{}, ErrNotFound
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AccountDeletionResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	keys, err := collectAccountObjectKeys(ctx, tx, userID)
	if err != nil {
		return AccountDeletionResult{}, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return AccountDeletionResult{}, err
	}
	if tag.RowsAffected() == 0 {
		return AccountDeletionResult{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return AccountDeletionResult{}, err
	}
	return AccountDeletionResult{ObjectKeys: keys}, nil
}

func collectAccountObjectKeys(ctx context.Context, tx pgx.Tx, userID uuid.UUID) ([]string, error) {
	seen := map[string]bool{}
	add := func(raw string) {
		key := strings.TrimSpace(raw)
		if key != "" {
			seen[key] = true
		}
	}

	var avatar, header pgtype.Text
	if err := tx.QueryRow(ctx, `
		SELECT avatar_object_key, header_object_key
		FROM users WHERE id = $1
	`, userID).Scan(&avatar, &header); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if avatar.Valid {
		add(avatar.String)
	}
	if header.Valid {
		add(header.String)
	}

	if err := collectObjectKeyRows(ctx, tx, add, `
		SELECT btrim(key)
		FROM posts p, unnest(p.object_keys) AS key
		WHERE p.user_id = $1 AND btrim(key) <> ''
	`, userID); err != nil {
		return nil, err
	}
	if err := collectObjectKeyRows(ctx, tx, add, `
		SELECT btrim(object_key)
		FROM custom_emojis
		WHERE owner_user_id = $1 AND btrim(object_key) <> ''
	`, userID); err != nil {
		return nil, err
	}
	if err := collectObjectKeyRows(ctx, tx, add, `
		SELECT btrim(item->>'object_key')
		FROM dm_messages m, jsonb_array_elements(m.attachments) AS item
		WHERE m.sender_id = $1 AND btrim(COALESCE(item->>'object_key', '')) <> ''
	`, userID); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	return out, nil
}

func collectObjectKeyRows(ctx context.Context, tx pgx.Tx, add func(string), query string, args ...any) error {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return err
		}
		add(key)
	}
	return rows.Err()
}
