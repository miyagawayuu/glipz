package authjwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const PurposeAccess = "access"
const PurposeMFA = "mfa"

const TokenUseUser = "user"
const TokenUseOAuth = "oauth"

const Issuer = "glipz"
const AudienceAPI = "glipz-api"

type Claims struct {
	Purpose  string `json:"purpose"`
	TokenUse string `json:"token_use,omitempty"`
	ClientID string `json:"client_id,omitempty"`
	Scope    string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

func SignAccess(secret []byte, userID uuid.UUID, ttl time.Duration) (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Purpose:  PurposeAccess,
		TokenUse: TokenUseUser,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    Issuer,
			Audience:  []string{AudienceAPI},
		},
	})
	return t.SignedString(secret)
}

func SignOAuthAccess(secret []byte, userID, clientID uuid.UUID, scope string, ttl time.Duration) (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Purpose:  PurposeAccess,
		TokenUse: TokenUseOAuth,
		ClientID: clientID.String(),
		Scope:    scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    Issuer,
			Audience:  []string{AudienceAPI},
		},
	})
	return t.SignedString(secret)
}

func SignMFA(secret []byte, userID uuid.UUID, ttl time.Duration) (string, string, error) {
	now := time.Now()
	jti := uuid.NewString()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Purpose: PurposeMFA,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    Issuer,
			Audience:  []string{AudienceAPI},
		},
	})
	signed, err := t.SignedString(secret)
	return signed, jti, err
}

func Parse(secret []byte, token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	}, jwt.WithIssuer(Issuer), jwt.WithAudience(AudienceAPI))
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
