// Package gumroad implements Gumroad license-key checks for fan club locks.
package gumroad

import "glipz.io/backend/internal/fanclub/kernel"

// ProviderID is the stable membership provider string stored on posts.
const ProviderID = "gumroad"

// EntitledCacheKey matches the shared fanclub kernel layout for Gumroad.
func EntitledCacheKey(viewerID, authorID, productID, tierID string) string {
	return kernel.EntitledCacheKey(ProviderID, viewerID, authorID, productID, tierID)
}
