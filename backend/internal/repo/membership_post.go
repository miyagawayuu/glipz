package repo

import (
	"errors"
	"strings"
)

var (
	// ErrInvalidMembership is returned when membership lock fields are inconsistent or provider is not allowed.
	ErrInvalidMembership = errors.New("invalid membership")
)

// AllowedMembershipProviders lists provider IDs (lowercase) that may be set on a post. Extend as new integrations ship.
var AllowedMembershipProviders = map[string]struct{}{
	"patreon": {},
	"gumroad": {},
}

// NormalizePostMembership validates optional membership lock metadata for local posts.
// All empty means no membership lock. All non-empty and consistent means lock is enabled.
func NormalizePostMembership(provider, creatorID, tierID string) (p, c, t string, err error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	creatorID = strings.TrimSpace(creatorID)
	tierID = strings.TrimSpace(tierID)
	if provider == "" && creatorID == "" && tierID == "" {
		return "", "", "", nil
	}
	if provider == "" || creatorID == "" || tierID == "" {
		return "", "", "", ErrInvalidMembership
	}
	if _, ok := AllowedMembershipProviders[provider]; !ok {
		return "", "", "", ErrInvalidMembership
	}
	return provider, creatorID, tierID, nil
}
