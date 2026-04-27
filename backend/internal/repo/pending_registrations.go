package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrPendingRegistrationExpired = errors.New("pending registration expired")
var ErrPendingRegistrationConsumed = errors.New("pending registration already consumed")

func (p *Pool) IsHandleAvailable(ctx context.Context, handle string) (bool, error) {
	handle = strings.TrimSpace(strings.ToLower(handle))
	if handle == "" {
		return false, ErrNotFound
	}
	var exists bool
	if err := p.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE lower(handle) = lower($1))
			OR EXISTS (
				SELECT 1 FROM pending_user_registrations
				WHERE lower(handle) = lower($1)
				  AND consumed_at IS NULL
				  AND expires_at > NOW()
			)
	`, handle).Scan(&exists); err != nil {
		return false, err
	}
	return !exists, nil
}

func (p *Pool) UpsertPendingUserRegistration(ctx context.Context, email, passwordHash, handle string, birthDate time.Time, tokenSHA256 string, expiresAt time.Time) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO pending_user_registrations (
			email, password_hash, handle, birth_date, token_sha256, expires_at, consumed_at, verified_user_id, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NULL, NULL, NOW())
		ON CONFLICT (email) DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			handle = EXCLUDED.handle,
			birth_date = EXCLUDED.birth_date,
			token_sha256 = EXCLUDED.token_sha256,
			expires_at = EXCLUDED.expires_at,
			consumed_at = NULL,
			verified_user_id = NULL,
			updated_at = NOW()
	`, email, passwordHash, handle, birthDate.UTC(), tokenSHA256, expiresAt.UTC())
	return err
}

func (p *Pool) CompletePendingUserRegistration(ctx context.Context, tokenSHA256 string) (uuid.UUID, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		pendingID    uuid.UUID
		email        string
		passwordHash string
		handle       string
		birthDate    *time.Time
		expiresAt    time.Time
		consumedAt   *time.Time
	)
	var consumedRaw pgtype.Timestamptz
	var birthDateRaw pgtype.Date
	err = tx.QueryRow(ctx, `
		SELECT id, email, password_hash, handle, birth_date, expires_at, consumed_at
		FROM pending_user_registrations
		WHERE token_sha256 = $1
		FOR UPDATE
	`, tokenSHA256).Scan(&pendingID, &email, &passwordHash, &handle, &birthDateRaw, &expiresAt, &consumedRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	if consumedRaw.Valid {
		t := consumedRaw.Time.UTC()
		consumedAt = &t
	}
	if birthDateRaw.Valid {
		t := birthDateRaw.Time.UTC()
		birthDate = &t
	}
	if consumedAt != nil {
		return uuid.Nil, ErrPendingRegistrationConsumed
	}
	now := time.Now().UTC()
	if !expiresAt.UTC().After(now) {
		return uuid.Nil, ErrPendingRegistrationExpired
	}

	handle = strings.TrimSpace(strings.ToLower(handle))
	if handle == "" {
		handle = SanitizeHandle(email)
	}
	if IsReservedHandle(handle) {
		return uuid.Nil, ErrReservedHandle
	}
	identity, err := newPortableIdentity()
	if err != nil {
		return uuid.Nil, err
	}
	var userID uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO users (
			email, password_hash, handle, birth_date, portable_id, account_public_key, account_private_key_encrypted
		) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		email, passwordHash, handle, birthDate, identity.PortableID, identity.AccountPublicKey, identity.AccountPrivateKeyEncrypted,
	).Scan(&userID)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			if strings.Contains(strings.ToLower(pe.ConstraintName), "email") {
				return uuid.Nil, fmt.Errorf("email taken: %w", err)
			}
			return uuid.Nil, ErrHandleTaken
		}
		return uuid.Nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE pending_user_registrations
		SET consumed_at = $2, verified_user_id = $3, updated_at = NOW()
		WHERE id = $1
	`, pendingID, now, userID); err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}
