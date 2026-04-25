// Package kernel holds user-to-user (non-custodial) payment infrastructure:
// stable Redis key layout and OAuth state helpers. No PSP business logic.
package kernel

import "strings"

const (
	// KeyPrefix is the top-level namespace for all payment keys (distinct from fanclub:).
	KeyPrefix = "payment"
)

// OAuthStateKey is the Redis key for a single OAuth state parameter (GETDEL after use),
// e.g. Stripe Connect or PayPal merchant onboarding. Payload shape is provider-owned.
func OAuthStateKey(provider, state string) string {
	return KeyPrefix + ":oauth:" + strings.TrimSpace(provider) + ":" + strings.TrimSpace(state)
}

// WebhookEventDedupKey is used to mark a PSP webhook event as seen (SET NX + TTL) so
// idempotent processing does not double-fulfill. eventID must be the provider’s unique id.
func WebhookEventDedupKey(provider, eventID string) string {
	return KeyPrefix + ":webhook:processed:" + strings.TrimSpace(provider) + ":" + strings.TrimSpace(eventID)
}

// IdempotencyKey namespaces client- or server-generated idempotency keys per user and provider.
func IdempotencyKey(provider, userID, idempotencyKey string) string {
	return KeyPrefix + ":idempo:" + strings.TrimSpace(provider) + ":" +
		strings.TrimSpace(userID) + ":" + strings.TrimSpace(idempotencyKey)
}
