package paypal

import (
	"encoding/json"
	"strings"
	"time"
)

// WebhookEvent is a minimal envelope common to PayPal webhooks.
type WebhookEvent struct {
	ID        string          `json:"id"`
	EventType string          `json:"event_type"`
	CreateTime string         `json:"create_time"`
	Resource  json.RawMessage `json:"resource"`
}

func (e WebhookEvent) ParsedCreateTime() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(e.CreateTime))
	if t.IsZero() {
		t, _ = time.Parse(time.RFC3339, strings.TrimSpace(e.CreateTime))
	}
	return t.UTC()
}

type SubscriptionResource struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	Status string `json:"status"`
}

// ExtractSubscription attempts to pull subscription_id/plan_id/status from webhook.
// This covers BILLING.SUBSCRIPTION.* events where resource is a subscription object.
func ExtractSubscription(e WebhookEvent) (subID, planID, status string, ok bool) {
	var r SubscriptionResource
	if err := json.Unmarshal(e.Resource, &r); err != nil {
		return "", "", "", false
	}
	subID = strings.TrimSpace(r.ID)
	planID = strings.TrimSpace(r.PlanID)
	status = strings.TrimSpace(r.Status)
	if subID == "" {
		return "", "", "", false
	}
	return subID, planID, status, true
}

