package httpserver

import (
	"strings"
	"testing"
)

func TestIsValidModerationStatus(t *testing.T) {
	for _, status := range []string{"open", " resolved ", "DISMISSED", "spam"} {
		if !isValidModerationStatus(status) {
			t.Fatalf("status %q should be valid", status)
		}
	}
	for _, status := range []string{"", "pending", "closed"} {
		if isValidModerationStatus(status) {
			t.Fatalf("status %q should be invalid", status)
		}
	}
}

func TestNormalizeReportReason(t *testing.T) {
	if reason, code := normalizeReportReason("  harmful content  "); reason != "harmful content" || code != "" {
		t.Fatalf("normalizeReportReason valid = %q %q, want trimmed reason and empty code", reason, code)
	}
	if _, code := normalizeReportReason("  "); code != "report_reason_required" {
		t.Fatalf("blank reason code = %q, want report_reason_required", code)
	}
	if _, code := normalizeReportReason(strings.Repeat("あ", 1001)); code != "report_reason_too_long" {
		t.Fatalf("oversized reason code = %q, want report_reason_too_long", code)
	}
}
