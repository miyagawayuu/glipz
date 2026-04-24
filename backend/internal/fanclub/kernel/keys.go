// Package kernel holds cross-site fanclub infrastructure (key naming, no provider business logic).
package kernel

import "strings"

// OAuthStateKey is the Redis key for a single OAuth state parameter (GETDEL after use).
func OAuthStateKey(provider, state string) string {
	return "fanclub:oauth:" + strings.TrimSpace(provider) + ":" + strings.TrimSpace(state)
}

// EntitledCacheKey is the Redis key for a cached paywall decision (0/1).
func EntitledCacheKey(provider, viewerID, authorID, scopeID, tierID string) string {
	// scope/tier are opaque per provider; keep order stable.
	return "fanclub:entitled:" + strings.TrimSpace(provider) + ":" +
		strings.TrimSpace(viewerID) + ":" + strings.TrimSpace(authorID) + ":" +
		strings.TrimSpace(scopeID) + ":" + strings.TrimSpace(tierID)
}
