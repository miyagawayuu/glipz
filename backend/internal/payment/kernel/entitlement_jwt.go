package kernel

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const PurposeEntitlement = "payment_entitlement"

// EntitlementClaims are used to unlock payment-locked posts without leaking provider logic
// into the generic unlock handler.
type EntitlementClaims struct {
	Purpose  string `json:"purpose"`
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	jwt.RegisteredClaims
}

func SignEntitlement(secret []byte, provider string, viewerUserID uuid.UUID, scope string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := EntitlementClaims{
		Purpose:  PurposeEntitlement,
		Provider: strings.TrimSpace(provider),
		Scope:    strings.TrimSpace(scope),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   viewerUserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(secret)
}

func ParseEntitlement(secret []byte, token string) (*EntitlementClaims, error) {
	parsed, err := jwt.ParseWithClaims(strings.TrimSpace(token), &EntitlementClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*EntitlementClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if strings.TrimSpace(claims.Purpose) != PurposeEntitlement {
		return nil, errors.New("invalid purpose")
	}
	return claims, nil
}

