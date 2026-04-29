package repo

import "testing"

func TestNormalizeMinimumRegistrationAge(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "negative disables", in: -1, want: 0},
		{name: "default", in: 13, want: 13},
		{name: "caps high value", in: 121, want: 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeMinimumRegistrationAge(tt.in); got != tt.want {
				t.Fatalf("NormalizeMinimumRegistrationAge(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeReportCategory(t *testing.T) {
	if got := NormalizeReportCategory(" LEGAL "); got != "legal" {
		t.Fatalf("NormalizeReportCategory legal = %q", got)
	}
	if got := NormalizeReportCategory("unknown"); got != "other" {
		t.Fatalf("NormalizeReportCategory unknown = %q", got)
	}
}
