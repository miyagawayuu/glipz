package paypal

import "testing"

func TestExtractSubscription_OK(t *testing.T) {
	ev := WebhookEvent{
		ID:         "WH-1",
		EventType:  "BILLING.SUBSCRIPTION.UPDATED",
		CreateTime: "2026-01-01T00:00:00Z",
		Resource:   []byte(`{"id":"I-SUB123","plan_id":"P-PLAN","status":"ACTIVE"}`),
	}
	subID, planID, status, ok := ExtractSubscription(ev)
	if !ok {
		t.Fatalf("expected ok")
	}
	if subID != "I-SUB123" || planID != "P-PLAN" || status != "ACTIVE" {
		t.Fatalf("got sub=%q plan=%q status=%q", subID, planID, status)
	}
}

func TestExtractSubscription_Invalid(t *testing.T) {
	ev := WebhookEvent{Resource: []byte(`{"nope":true}`)}
	_, _, _, ok := ExtractSubscription(ev)
	if ok {
		t.Fatalf("expected not ok")
	}
}

func TestExtractSubscription_Statuses(t *testing.T) {
	for _, status := range []string{"APPROVAL_PENDING", "ACTIVE", "SUSPENDED", "CANCELLED", "EXPIRED"} {
		ev := WebhookEvent{
			Resource: []byte(`{"id":"I-SUB123","plan_id":"P-PLAN","status":"` + status + `"}`),
		}
		_, _, got, ok := ExtractSubscription(ev)
		if !ok {
			t.Fatalf("expected ok for %s", status)
		}
		if got != status {
			t.Fatalf("got status %q, want %q", got, status)
		}
	}
}
