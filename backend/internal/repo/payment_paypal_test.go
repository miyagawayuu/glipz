package repo

import "testing"

func TestIsActivePaymentSubscriptionStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "ACTIVE", want: true},
		{status: " active ", want: true},
		{status: "APPROVAL_PENDING", want: false},
		{status: "CANCELLED", want: false},
		{status: "SUSPENDED", want: false},
		{status: "EXPIRED", want: false},
		{status: "", want: false},
	}

	for _, tc := range cases {
		if got := IsActivePaymentSubscriptionStatus(tc.status); got != tc.want {
			t.Fatalf("IsActivePaymentSubscriptionStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
