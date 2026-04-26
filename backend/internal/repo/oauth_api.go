package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type OAuthClientRow struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Name         string
	RedirectURIs string
	CreatedAt    time.Time
	ClientIDStr  string // same as ID
}

func (p *Pool) OAuthClientCreate(ctx context.Context, userID uuid.UUID, name, redirectURIs string, secretHash string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return uuid.Nil, errors.New("oauth client: empty name")
	}
	var id uuid.UUID
	err := p.db.QueryRow(ctx, `
		INSERT INTO oauth_clients (user_id, name, client_secret_hash, redirect_uris)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, name, secretHash, strings.TrimSpace(redirectURIs)).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (p *Pool) OAuthClientList(ctx context.Context, userID uuid.UUID) ([]OAuthClientRow, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, user_id, name, redirect_uris, created_at
		FROM oauth_clients WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OAuthClientRow
	for rows.Next() {
		var r OAuthClientRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.RedirectURIs, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.ClientIDStr = r.ID.String()
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) OAuthClientDelete(ctx context.Context, ownerUserID, clientID uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM oauth_clients WHERE id = $1 AND user_id = $2`, clientID, ownerUserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type oauthClientCredRow struct {
	UserID       uuid.UUID
	SecretHash   string
	RedirectURIs string
}

// OAuthClientByIDPublic returns a client's redirect URIs for the authorization screen without exposing secrets.
func (p *Pool) OAuthClientByIDPublic(ctx context.Context, clientID uuid.UUID) (OAuthClientRow, error) {
	var r OAuthClientRow
	err := p.db.QueryRow(ctx, `
		SELECT id, user_id, name, redirect_uris, created_at
		FROM oauth_clients WHERE id = $1
	`, clientID).Scan(&r.ID, &r.UserID, &r.Name, &r.RedirectURIs, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.ClientIDStr = r.ID.String()
	return r, nil
}

func (p *Pool) oauthClientByID(ctx context.Context, clientID uuid.UUID) (oauthClientCredRow, error) {
	var r oauthClientCredRow
	err := p.db.QueryRow(ctx, `
		SELECT user_id, client_secret_hash, redirect_uris FROM oauth_clients WHERE id = $1
	`, clientID).Scan(&r.UserID, &r.SecretHash, &r.RedirectURIs)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// OAuthClientCredentialsValid validates client_id and client_secret, then returns the owner user_id.
func (p *Pool) OAuthClientCredentialsValid(ctx context.Context, clientID uuid.UUID, clientSecret string) (uuid.UUID, error) {
	r, err := p.oauthClientByID(ctx, clientID)
	if err != nil {
		return uuid.Nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(r.SecretHash), []byte(clientSecret)) != nil {
		return uuid.Nil, ErrNotFound
	}
	return r.UserID, nil
}

// OAuthRedirectURIAllowed checks whether uri appears in the registered newline-delimited redirect URI list.
func OAuthRedirectURIAllowed(redirectURIsBlock, uri string) bool {
	want := strings.TrimSpace(uri)
	if want == "" {
		return false
	}
	for _, line := range strings.Split(redirectURIsBlock, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func (p *Pool) OAuthAuthorizationCodeInsert(ctx context.Context, clientID, userID uuid.UUID, redirectURI, scope, codePlain string, ttl time.Duration) error {
	sum := sha256.Sum256([]byte(codePlain))
	hash := hex.EncodeToString(sum[:])
	exp := time.Now().UTC().Add(ttl)
	_, err := p.db.Exec(ctx, `
		INSERT INTO oauth_authorization_codes (code_sha256, client_id, user_id, redirect_uri, scope, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, hash, clientID, userID, strings.TrimSpace(redirectURI), strings.TrimSpace(scope), exp)
	return err
}

// OAuthAuthorizationCodeExchange consumes an authorization code and returns the user_id and scope authorized for the token.
func (p *Pool) OAuthAuthorizationCodeExchange(ctx context.Context, clientID uuid.UUID, codePlain, redirectURI string) (uuid.UUID, string, error) {
	sum := sha256.Sum256([]byte(codePlain))
	hash := hex.EncodeToString(sum[:])
	red := strings.TrimSpace(redirectURI)
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var uid uuid.UUID
	var scope string
	err = tx.QueryRow(ctx, `
		SELECT user_id, scope FROM oauth_authorization_codes
		WHERE code_sha256 = $1 AND client_id = $2 AND redirect_uri = $3
		  AND used_at IS NULL AND expires_at > NOW()
		FOR UPDATE
	`, hash, clientID, red).Scan(&uid, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrNotFound
	}
	if err != nil {
		return uuid.Nil, "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE oauth_authorization_codes SET used_at = NOW() WHERE code_sha256 = $1
	`, hash); err != nil {
		return uuid.Nil, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, "", err
	}
	return uid, strings.TrimSpace(scope), nil
}

// --- Personal access tokens (glpat_<uuid>_<secret>) ---

func (p *Pool) PersonalAccessTokenCreate(ctx context.Context, userID uuid.UUID, label, secretHash, prefix string) (uuid.UUID, error) {
	var id uuid.UUID
	err := p.db.QueryRow(ctx, `
		INSERT INTO api_personal_access_tokens (user_id, label, token_prefix, secret_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, strings.TrimSpace(label), prefix, secretHash).Scan(&id)
	return id, err
}

type PersonalAccessTokenListRow struct {
	ID          uuid.UUID
	Label       string
	TokenPrefix string
	CreatedAt   time.Time
}

func (p *Pool) PersonalAccessTokenList(ctx context.Context, userID uuid.UUID) ([]PersonalAccessTokenListRow, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, label, token_prefix, created_at
		FROM api_personal_access_tokens WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PersonalAccessTokenListRow
	for rows.Next() {
		var r PersonalAccessTokenListRow
		if err := rows.Scan(&r.ID, &r.Label, &r.TokenPrefix, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Pool) PersonalAccessTokenDelete(ctx context.Context, userID, tokenID uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM api_personal_access_tokens WHERE id = $1 AND user_id = $2`, tokenID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UserIDFromPersonalAccessToken validates a bearer token string and returns its user_id.
func (p *Pool) UserIDFromPersonalAccessToken(ctx context.Context, raw string) (uuid.UUID, error) {
	const prefix = "glpat_"
	if !strings.HasPrefix(raw, prefix) {
		return uuid.Nil, ErrNotFound
	}
	rest := strings.TrimPrefix(raw, prefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return uuid.Nil, ErrNotFound
	}
	tid, perr := uuid.Parse(parts[0])
	if perr != nil {
		return uuid.Nil, ErrNotFound
	}
	secret := parts[1]
	if len(secret) < 16 {
		return uuid.Nil, ErrNotFound
	}
	var uid uuid.UUID
	var hash string
	err := p.db.QueryRow(ctx, `SELECT user_id, secret_hash FROM api_personal_access_tokens WHERE id = $1`, tid).Scan(&uid, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) != nil {
		return uuid.Nil, ErrNotFound
	}
	_, _ = p.db.Exec(ctx, `UPDATE api_personal_access_tokens SET last_used_at = NOW() WHERE id = $1`, tid)
	return uid, nil
}
