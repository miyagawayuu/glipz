package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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

func TestNormalizePublicMediaObjectKey(t *testing.T) {
	uid := uuid.NewString()
	allowed := "uploads/" + uid + "/file.png"
	if got, ok := normalizePublicMediaObjectKey(allowed); !ok || got != allowed {
		t.Fatalf("allowed object key = %q %v, want %q true", got, ok, allowed)
	}

	rejected := []string{
		"",
		"/uploads/" + uid + "/file.png",
		"uploads/" + uid + "/../secret.txt",
		"uploads/" + uid + "//file.png",
		"uploads/not-a-uuid/file.png",
		"private/" + uid + "/file.png",
		"uploads/" + uid + "/nested/file.png",
		`uploads\` + uid + `\file.png`,
	}
	for _, raw := range rejected {
		if got, ok := normalizePublicMediaObjectKey(raw); ok {
			t.Fatalf("normalizePublicMediaObjectKey(%q) = %q true, want false", raw, got)
		}
	}
}

func TestHandlePublicMediaObjectRejectsUnmanagedObjectKeys(t *testing.T) {
	store, err := s3client.NewLocal(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{s3: store}
	r := chi.NewRouter()
	r.Get("/media/object/*", s.handlePublicMediaObject)
	r.Head("/media/object/*", s.handlePublicMediaObject)

	uid := uuid.NewString()
	key := "uploads/" + uid + "/file.txt"
	if err := store.PutObject(t.Context(), key, "text/plain", strings.NewReader("hello world"), int64(len("hello world"))); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/media/object/"+key, nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD allowed key status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/media/object/"+key, nil)
	req.Header.Set("Range", "bytes=6-10")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("GET range status = %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != "world" {
		t.Fatalf("GET range body = %q, want world", got)
	}

	for _, path := range []string{
		"/media/object/private/" + uid + "/file.txt",
		"/media/object/uploads/" + uid + "/../secret.txt",
		"/media/object/uploads/not-a-uuid/file.txt",
	} {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, rec.Code)
		}
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
