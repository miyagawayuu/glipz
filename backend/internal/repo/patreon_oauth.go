package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PatreonTokenRow is stored per Glipz user. Tokens must not appear in API responses.
type PatreonTokenRow struct {
	UserID         uuid.UUID
	PatreonUserID  string
	AccessToken    string
	RefreshToken   string
	TokenExpiresAt *time.Time
}

func (p *Pool) UpsertPatreonOAuth(ctx context.Context, userID uuid.UUID, patreonUserID, access, refresh string, expires *time.Time) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO user_patreon_oauth (user_id, patreon_user_id, access_token, refresh_token, token_expires_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (user_id) DO UPDATE SET
			patreon_user_id = EXCLUDED.patreon_user_id,
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			token_expires_at = EXCLUDED.token_expires_at,
			updated_at = now()
	`, userID, patreonUserID, access, refresh, expires)
	return err
}

func (p *Pool) PatreonOAuthByUserID(ctx context.Context, userID uuid.UUID) (PatreonTokenRow, error) {
	var row PatreonTokenRow
	err := p.db.QueryRow(ctx, `
		SELECT user_id, patreon_user_id, access_token, refresh_token, token_expires_at
		FROM user_patreon_oauth WHERE user_id = $1
	`, userID).Scan(&row.UserID, &row.PatreonUserID, &row.AccessToken, &row.RefreshToken, &row.TokenExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PatreonTokenRow{}, ErrNotFound
	}
	if err != nil {
		return PatreonTokenRow{}, err
	}
	return row, nil
}

// DeletePatreonOAuth removes the connection if present. Missing row is not an error.
func (p *Pool) DeletePatreonOAuth(ctx context.Context, userID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `DELETE FROM user_patreon_oauth WHERE user_id = $1`, userID)
	return err
}

func (p *Pool) UserHasPatreonConnection(ctx context.Context, userID uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_patreon_oauth WHERE user_id = $1)`, userID).Scan(&ok)
	return ok, err
}
