package httpserver

import (
	"net/http/httptest"
	"testing"

	"glipz.io/backend/internal/config"
	"glipz.io/backend/internal/s3client"
)

func TestFederationRemoteMediaURL(t *testing.T) {
	s := &Server{cfg: config.Config{GlipzProtocolPublicOrigin: "https://local.example"}}

	got := s.federationRemoteMediaURL("https://remote.example/media/avatar.png#frag")
	want := "https://local.example/api/v1/media/remote?url=https%3A%2F%2Fremote.example%2Fmedia%2Favatar.png"
	if got != want {
		t.Fatalf("proxied remote media URL = %q, want %q", got, want)
	}

	own := "https://local.example/api/v1/media/object/users/u/avatar.png"
	if got := s.federationRemoteMediaURL(own); got != own {
		t.Fatalf("own media URL = %q, want unchanged %q", got, own)
	}

	localhost := "https://localhost/media/avatar.png"
	if got := s.federationRemoteMediaURL(localhost); got != localhost {
		t.Fatalf("unsafe localhost URL = %q, want unchanged %q", got, localhost)
	}
}

func TestLocalMediaDirectURLUsesConfiguredPublicBase(t *testing.T) {
	s := &Server{cfg: config.Config{GlipzProtocolMediaPublicBase: "https://media.example.com/"}}

	got := s.localMediaDirectURL("/posts/abc/image.png")
	want := "https://media.example.com/posts/abc/image.png"
	if got != want {
		t.Fatalf("direct media URL = %q, want %q", got, want)
	}
}

func TestWriteMediaProxyHeadersForcesDangerousContentToAttachment(t *testing.T) {
	rec := httptest.NewRecorder()

	writeMediaProxyHeaders(rec, s3client.ObjectMeta{
		ContentType:   "text/html; charset=utf-8",
		ContentLength: 42,
	})

	if got := rec.Header().Get("Content-Type"); got != fallbackDownloadContentType {
		t.Fatalf("Content-Type = %q, want %q", got, fallbackDownloadContentType)
	}
	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
		t.Fatalf("Cross-Origin-Resource-Policy = %q, want same-origin", got)
	}
}

func TestWriteMediaProxyHeadersKeepsSafeMediaInline(t *testing.T) {
	rec := httptest.NewRecorder()

	writeMediaProxyHeaders(rec, s3client.ObjectMeta{
		ContentType:   "image/png",
		ContentLength: 42,
	})

	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("Content-Disposition = %q, want empty", got)
	}
}

func TestMediaUploadContentTypeAllowlist(t *testing.T) {
	allowed := []string{
		"image/jpeg",
		"image/png; charset=binary",
		"image/webp",
		"video/mp4",
		"video/webm",
		"audio/mpeg",
		"audio/ogg",
	}
	for _, ct := range allowed {
		if !isAllowedUploadMediaContentType(ct) {
			t.Fatalf("isAllowedUploadMediaContentType(%q) = false, want true", ct)
		}
		if !isAllowedPresignedMediaContentType(ct) {
			t.Fatalf("isAllowedPresignedMediaContentType(%q) = false, want true", ct)
		}
	}
}

func TestMediaUploadContentTypeRejectsActiveContent(t *testing.T) {
	rejected := []string{
		"",
		"application/octet-stream",
		"image/svg+xml",
		"text/html; charset=utf-8",
		"application/xhtml+xml",
		"application/javascript",
		"text/css",
	}
	for _, ct := range rejected {
		if isAllowedUploadMediaContentType(ct) {
			t.Fatalf("isAllowedUploadMediaContentType(%q) = true, want false", ct)
		}
		if isAllowedPresignedMediaContentType(ct) {
			t.Fatalf("isAllowedPresignedMediaContentType(%q) = true, want false", ct)
		}
	}
}
