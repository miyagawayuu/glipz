package repo

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	PortableIDPrefix     = "glipz:id:"
	LegacyPortablePrefix = "legacy:"
)

type PortableIdentity struct {
	PortableID                 string
	AccountPublicKey           string
	AccountPrivateKeyEncrypted string
}

type RemoteAccount struct {
	ID             uuid.UUID
	PortableID     string
	CurrentAcct    string
	ProfileURL     string
	PostsURL       string
	InboxURL       string
	PublicKey      string
	MovedTo        string
	MovedFrom      string
	AlsoKnownAs    []string
	LastVerifiedAt *time.Time
}

type RemoteAccountUpsert struct {
	PortableID  string
	CurrentAcct string
	ProfileURL  string
	PostsURL    string
	InboxURL    string
	PublicKey   string
	MovedTo     string
	MovedFrom   string
	AlsoKnownAs []string
}

func NormalizePortableID(raw string) string {
	return strings.TrimSpace(raw)
}

func LegacyPortableIDForAcct(acct string) string {
	acct = NormalizeFederationTargetAcct(acct)
	if acct == "" {
		return ""
	}
	return LegacyPortablePrefix + acct
}

func PortableIDForRemote(acct, portableID string) string {
	if id := NormalizePortableID(portableID); id != "" {
		return id
	}
	return LegacyPortableIDForAcct(acct)
}

func portableIDForPublicKey(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return PortableIDPrefix + base64.RawURLEncoding.EncodeToString(sum[:])
}

func isLocalPlaceholderPortableID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), PortableIDPrefix+"local-")
}

func newPortableIdentity() (PortableIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PortableIdentity{}, err
	}
	enc := base64.RawURLEncoding
	return PortableIdentity{
		PortableID:                 portableIDForPublicKey(pub),
		AccountPublicKey:           enc.EncodeToString(pub),
		AccountPrivateKeyEncrypted: enc.EncodeToString(priv),
	}, nil
}

func (p *Pool) EnsureUserPortableIdentity(ctx context.Context, userID uuid.UUID) (PortableIdentity, error) {
	var out PortableIdentity
	err := p.db.QueryRow(ctx, `
		SELECT COALESCE(portable_id, ''), COALESCE(account_public_key, ''), COALESCE(account_private_key_encrypted, '')
		FROM users WHERE id = $1
	`, userID).Scan(&out.PortableID, &out.AccountPublicKey, &out.AccountPrivateKeyEncrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return PortableIdentity{}, ErrNotFound
	}
	if err != nil {
		return PortableIdentity{}, err
	}
	if strings.TrimSpace(out.PortableID) != "" && strings.TrimSpace(out.AccountPublicKey) != "" && strings.TrimSpace(out.AccountPrivateKeyEncrypted) != "" {
		pub, errPub := base64.RawURLEncoding.DecodeString(strings.TrimSpace(out.AccountPublicKey))
		priv, errPriv := base64.RawURLEncoding.DecodeString(strings.TrimSpace(out.AccountPrivateKeyEncrypted))
		if errPub == nil && errPriv == nil && len(pub) == ed25519.PublicKeySize && len(priv) == ed25519.PrivateKeySize &&
			bytes.Equal(ed25519.PrivateKey(priv).Public().(ed25519.PublicKey), ed25519.PublicKey(pub)) {
			expectedID := portableIDForPublicKey(ed25519.PublicKey(pub))
			if out.PortableID != expectedID && isLocalPlaceholderPortableID(out.PortableID) {
				if _, err := p.db.Exec(ctx, `UPDATE users SET portable_id = $2 WHERE id = $1`, userID, expectedID); err != nil {
					return PortableIdentity{}, err
				}
				out.PortableID = expectedID
			}
		}
		return out, nil
	}
	next, err := newPortableIdentity()
	if err != nil {
		return PortableIdentity{}, err
	}
	_, err = p.db.Exec(ctx, `
		UPDATE users
		SET portable_id = $2,
			account_public_key = $3,
			account_private_key_encrypted = $4
		WHERE id = $1
	`, userID, next.PortableID, next.AccountPublicKey, next.AccountPrivateKeyEncrypted)
	if err != nil {
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "23505" {
			return PortableIdentity{}, err
		}
	}
	err = p.db.QueryRow(ctx, `
		SELECT COALESCE(portable_id, ''), COALESCE(account_public_key, ''), COALESCE(account_private_key_encrypted, '')
		FROM users WHERE id = $1
	`, userID).Scan(&out.PortableID, &out.AccountPublicKey, &out.AccountPrivateKeyEncrypted)
	return out, err
}

func (p *Pool) SetUserPortableIdentity(ctx context.Context, userID uuid.UUID, identity PortableIdentity) error {
	identity.PortableID = NormalizePortableID(identity.PortableID)
	identity.AccountPublicKey = strings.TrimSpace(identity.AccountPublicKey)
	identity.AccountPrivateKeyEncrypted = strings.TrimSpace(identity.AccountPrivateKeyEncrypted)
	if userID == uuid.Nil || identity.AccountPublicKey == "" || identity.AccountPrivateKeyEncrypted == "" {
		return fmt.Errorf("invalid portable identity")
	}
	pub, err := base64.RawURLEncoding.DecodeString(identity.AccountPublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid portable identity")
	}
	expectedID := portableIDForPublicKey(ed25519.PublicKey(pub))
	if identity.PortableID == "" || isLocalPlaceholderPortableID(identity.PortableID) {
		identity.PortableID = expectedID
	}
	if identity.PortableID != expectedID {
		return fmt.Errorf("invalid portable identity")
	}
	priv, err := base64.RawURLEncoding.DecodeString(identity.AccountPrivateKeyEncrypted)
	if err != nil || len(priv) != ed25519.PrivateKeySize || !bytes.Equal(ed25519.PrivateKey(priv).Public().(ed25519.PublicKey), ed25519.PublicKey(pub)) {
		return fmt.Errorf("invalid portable identity")
	}
	_, err = p.db.Exec(ctx, `
		UPDATE users
		SET portable_id = $2,
			account_public_key = $3,
			account_private_key_encrypted = $4
		WHERE id = $1
	`, userID, identity.PortableID, identity.AccountPublicKey, identity.AccountPrivateKeyEncrypted)
	return err
}

func (p *Pool) MarkUserMoved(ctx context.Context, userID uuid.UUID, movedToAcct string) error {
	movedToAcct = NormalizeFederationTargetAcct(movedToAcct)
	if userID == uuid.Nil || movedToAcct == "" || !strings.Contains(movedToAcct, "@") {
		return fmt.Errorf("invalid moved_to_acct")
	}
	_, err := p.db.Exec(ctx, `
		UPDATE users SET moved_to_acct = $2, moved_at = NOW()
		WHERE id = $1
	`, userID, movedToAcct)
	return err
}

func (p *Pool) UpsertRemoteAccount(ctx context.Context, in RemoteAccountUpsert) (RemoteAccount, error) {
	in.PortableID = PortableIDForRemote(in.CurrentAcct, in.PortableID)
	in.CurrentAcct = NormalizeFederationTargetAcct(in.CurrentAcct)
	if in.PortableID == "" {
		return RemoteAccount{}, fmt.Errorf("invalid remote portable id")
	}
	var row RemoteAccount
	var last pgtype.Timestamptz
	err := p.db.QueryRow(ctx, `
		INSERT INTO federation_remote_accounts (
			portable_id, current_acct, profile_url, posts_url, inbox_url, public_key,
			moved_to, moved_from, also_known_as, last_verified_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, '{}'::text[]), NOW())
		ON CONFLICT (portable_id) DO UPDATE SET
			current_acct = COALESCE(NULLIF(EXCLUDED.current_acct, ''), federation_remote_accounts.current_acct),
			profile_url = COALESCE(NULLIF(EXCLUDED.profile_url, ''), federation_remote_accounts.profile_url),
			posts_url = COALESCE(NULLIF(EXCLUDED.posts_url, ''), federation_remote_accounts.posts_url),
			inbox_url = COALESCE(NULLIF(EXCLUDED.inbox_url, ''), federation_remote_accounts.inbox_url),
			public_key = COALESCE(NULLIF(EXCLUDED.public_key, ''), federation_remote_accounts.public_key),
			moved_to = COALESCE(NULLIF(EXCLUDED.moved_to, ''), federation_remote_accounts.moved_to),
			moved_from = COALESCE(NULLIF(EXCLUDED.moved_from, ''), federation_remote_accounts.moved_from),
			also_known_as = CASE WHEN cardinality(EXCLUDED.also_known_as) > 0 THEN EXCLUDED.also_known_as ELSE federation_remote_accounts.also_known_as END,
			last_verified_at = NOW(),
			updated_at = NOW()
		RETURNING id, portable_id, current_acct, profile_url, posts_url, inbox_url, public_key,
			moved_to, moved_from, also_known_as, last_verified_at
	`, strings.TrimSpace(in.PortableID), in.CurrentAcct, strings.TrimSpace(in.ProfileURL), strings.TrimSpace(in.PostsURL),
		strings.TrimSpace(in.InboxURL), strings.TrimSpace(in.PublicKey), NormalizeFederationTargetAcct(in.MovedTo),
		NormalizeFederationTargetAcct(in.MovedFrom), in.AlsoKnownAs).Scan(&row.ID, &row.PortableID, &row.CurrentAcct, &row.ProfileURL,
		&row.PostsURL, &row.InboxURL, &row.PublicKey, &row.MovedTo, &row.MovedFrom, &row.AlsoKnownAs, &last)
	if err != nil {
		return RemoteAccount{}, err
	}
	if last.Valid {
		t := last.Time.UTC()
		row.LastVerifiedAt = &t
	}
	return row, nil
}

func (p *Pool) RemoteAccountByPortableID(ctx context.Context, portableID string) (RemoteAccount, error) {
	portableID = NormalizePortableID(portableID)
	var row RemoteAccount
	var last pgtype.Timestamptz
	err := p.db.QueryRow(ctx, `
		SELECT id, portable_id, current_acct, profile_url, posts_url, inbox_url, public_key,
			moved_to, moved_from, also_known_as, last_verified_at
		FROM federation_remote_accounts WHERE portable_id = $1
	`, portableID).Scan(&row.ID, &row.PortableID, &row.CurrentAcct, &row.ProfileURL, &row.PostsURL, &row.InboxURL,
		&row.PublicKey, &row.MovedTo, &row.MovedFrom, &row.AlsoKnownAs, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		return RemoteAccount{}, ErrNotFound
	}
	if err != nil {
		return RemoteAccount{}, err
	}
	if last.Valid {
		t := last.Time.UTC()
		row.LastVerifiedAt = &t
	}
	return row, nil
}
