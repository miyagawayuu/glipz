// Package patreon implements Patreon OAuth and API calls for fan club / membership checks.
// Provider business rules live here; only kernel key helpers are shared across sites.
package patreon

import "glipz.io/backend/internal/fanclub/kernel"

// ProviderID is the stable membership provider string stored on posts and in kernel Redis keys.
const ProviderID = "patreon"

// OAuthStateKey is the fanclub kernel prefix for this provider.
func OAuthStateKey(state string) string {
	return kernel.OAuthStateKey(ProviderID, state)
}

// EntitledCacheKey matches kernel layout for Patreon.
func EntitledCacheKey(viewerID, authorID, campaignID, tierID string) string {
	return kernel.EntitledCacheKey(ProviderID, viewerID, authorID, campaignID, tierID)
}
