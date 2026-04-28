package httpserver

import (
	"testing"

	"glipz.io/backend/internal/config"
)

func TestServerAppVersion(t *testing.T) {
	if got := (*Server)(nil).appVersion(); got != glipzAppVersion {
		t.Fatalf("nil server appVersion() = %q, want %q", got, glipzAppVersion)
	}

	s := &Server{}
	if got := s.appVersion(); got != glipzAppVersion {
		t.Fatalf("default appVersion() = %q, want %q", got, glipzAppVersion)
	}

	s.cfg = config.Config{GlipzVersion: "v1.2.3+build.4"}
	if got := s.appVersion(); got != "v1.2.3+build.4" {
		t.Fatalf("configured appVersion() = %q, want override", got)
	}
}
