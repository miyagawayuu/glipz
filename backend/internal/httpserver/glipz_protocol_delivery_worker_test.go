package httpserver

import (
	"testing"
	"time"
)

func TestGlipzProtocolOutboxBackoff(t *testing.T) {
	if d := glipzProtocolOutboxBackoff(1); d != 30*time.Second {
		t.Fatalf("attempt 1: got %v want 30s", d)
	}
	if d := glipzProtocolOutboxBackoff(2); d != 60*time.Second {
		t.Fatalf("attempt 2: got %v want 60s", d)
	}
	if d := glipzProtocolOutboxBackoff(10); d != time.Hour {
		t.Fatalf("attempt 10: got %v want 1h cap", d)
	}
}
