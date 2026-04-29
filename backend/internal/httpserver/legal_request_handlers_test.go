package httpserver

import (
	"strings"
	"testing"
)

func TestLegalDocsAllowLawEnforcementPolicy(t *testing.T) {
	if _, ok := legalDocNames["law-enforcement"]; !ok {
		t.Fatal("law-enforcement legal document is not allowed")
	}
}

func TestLimitedTrimRejectsOversizedInput(t *testing.T) {
	got, ok := limitedTrim("  ok  ", 2)
	if !ok || got != "ok" {
		t.Fatalf("limitedTrim valid = %q %v, want ok true", got, ok)
	}
	if _, ok := limitedTrim(strings.Repeat("あ", 3), 2); ok {
		t.Fatal("limitedTrim accepted oversized unicode input")
	}
}

func TestParseOptionalUUIDRejectsInvalidAndNil(t *testing.T) {
	if id, ok := parseOptionalUUID(""); !ok || id != nil {
		t.Fatalf("empty UUID parse = %v %v, want nil true", id, ok)
	}
	if _, ok := parseOptionalUUID("00000000-0000-0000-0000-000000000000"); ok {
		t.Fatal("nil UUID accepted")
	}
	if _, ok := parseOptionalUUID("not-a-uuid"); ok {
		t.Fatal("invalid UUID accepted")
	}
}
