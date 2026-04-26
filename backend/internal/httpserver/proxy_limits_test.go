package httpserver

import (
	"bytes"
	"strings"
	"testing"
)

func TestCopyWithMaxBytesDoesNotWriteProbeByte(t *testing.T) {
	var dst bytes.Buffer
	written, exceeded, err := copyWithMaxBytes(&dst, strings.NewReader("abcdef"), 5)
	if err != nil {
		t.Fatalf("copyWithMaxBytes error = %v", err)
	}
	if !exceeded {
		t.Fatalf("copyWithMaxBytes exceeded = false, want true")
	}
	if written != 5 {
		t.Fatalf("copyWithMaxBytes written = %d, want 5", written)
	}
	if got := dst.String(); got != "abcde" {
		t.Fatalf("copied body = %q, want %q", got, "abcde")
	}
}

func TestCopyWithMaxBytesExactLimit(t *testing.T) {
	var dst bytes.Buffer
	written, exceeded, err := copyWithMaxBytes(&dst, strings.NewReader("abcde"), 5)
	if err != nil {
		t.Fatalf("copyWithMaxBytes error = %v", err)
	}
	if exceeded {
		t.Fatalf("copyWithMaxBytes exceeded = true, want false")
	}
	if written != 5 || dst.String() != "abcde" {
		t.Fatalf("written/body = %d/%q, want 5/%q", written, dst.String(), "abcde")
	}
}
