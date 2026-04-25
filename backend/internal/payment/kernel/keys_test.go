package kernel

import "testing"

func TestKeyLayouts(t *testing.T) {
	if got := OAuthStateKey("stripe", "abc"); got != "payment:oauth:stripe:abc" {
		t.Errorf("OAuthStateKey: got %q", got)
	}
	if got := WebhookEventDedupKey("paypal", "evt_1"); got != "payment:webhook:processed:paypal:evt_1" {
		t.Errorf("WebhookEventDedupKey: got %q", got)
	}
	if got := IdempotencyKey("stripe", "u1", "k"); got != "payment:idempo:stripe:u1:k" {
		t.Errorf("IdempotencyKey: got %q", got)
	}
}
