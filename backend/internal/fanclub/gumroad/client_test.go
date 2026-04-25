package gumroad

import "testing"

func TestVerifyResultEntitled(t *testing.T) {
	ok := VerifyResult{
		Success: true,
		Purchase: Purchase{
			ProductID: "prod_123",
		},
	}
	if !ok.Entitled("prod_123") {
		t.Fatal("expected active purchase to be entitled")
	}

	refunded := VerifyResult{
		Success: true,
		Purchase: Purchase{
			ProductID: "prod_123",
			Refunded:  true,
		},
	}
	if refunded.Entitled("prod_123") {
		t.Fatal("expected refunded purchase to be ineligible")
	}

	cancelled := VerifyResult{
		Success: true,
		Purchase: Purchase{
			ProductID:               "prod_123",
			SubscriptionCancelledAt: "2026-01-01T00:00:00Z",
		},
	}
	if cancelled.Entitled("prod_123") {
		t.Fatal("expected cancelled subscription to be ineligible")
	}
}
