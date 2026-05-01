package httpserver

import (
	"strings"
	"testing"
	"time"
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

func TestParseOptionalRFC3339(t *testing.T) {
	if got, ok := parseOptionalRFC3339(""); !ok || got != nil {
		t.Fatalf("empty time parse = %v %v, want nil true", got, ok)
	}
	got, ok := parseOptionalRFC3339("2026-05-01T02:30:00+09:00")
	if !ok || got == nil {
		t.Fatalf("valid time parse = %v %v, want value true", got, ok)
	}
	want := time.Date(2026, 4, 30, 17, 30, 0, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("valid time parse = %v (%v), want %v UTC", got, got.Location(), want)
	}
	if _, ok := parseOptionalRFC3339("2026-05-01"); ok {
		t.Fatal("invalid RFC3339 time accepted")
	}
}
