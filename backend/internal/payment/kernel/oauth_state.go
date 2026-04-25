package kernel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

// RandomOAuthState returns a URL-safe state string for OAuth / Connect flows.
func RandomOAuthState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// SaveOAuthState stores arbitrary JSON (or any payload) for the given provider/state.
func SaveOAuthState(ctx context.Context, rdb *redis.Client, provider, state, payload string, ttl time.Duration) error {
	return rdb.Set(ctx, OAuthStateKey(provider, state), payload, ttl).Err()
}

// GetDelOAuthState returns the payload and removes the key.
func GetDelOAuthState(ctx context.Context, rdb *redis.Client, provider, state string) (string, error) {
	return rdb.GetDel(ctx, OAuthStateKey(provider, state)).Result()
}
