package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (p *Pool) GetUserTimelineSettings(ctx context.Context, userID uuid.UUID) ([]byte, bool, error) {
	var raw []byte
	err := p.db.QueryRow(ctx, `
		SELECT settings
		FROM user_timeline_settings
		WHERE user_id = $1
	`, userID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (p *Pool) UpsertUserTimelineSettings(ctx context.Context, userID uuid.UUID, settingsJSON []byte) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO user_timeline_settings (user_id, settings, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET settings = EXCLUDED.settings,
			updated_at = NOW()
	`, userID, string(settingsJSON))
	return err
}

func (p *Pool) DeleteUserTimelineSettings(ctx context.Context, userID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `DELETE FROM user_timeline_settings WHERE user_id = $1`, userID)
	return err
}
