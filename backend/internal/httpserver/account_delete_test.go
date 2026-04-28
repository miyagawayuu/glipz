package httpserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/config"
)

func TestAccountDeleteRequiresAuthentication(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/account", strings.NewReader(`{"confirm":"DELETE"}`))
	rec := httptest.NewRecorder()

	s.handleMeAccountDelete(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAccountDeleteRequiresConfirmationBeforeDBLookup(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/account", bytes.NewBufferString(`{"password":"secret","confirm":"delete"}`))
	req = req.WithContext(context.WithValue(req.Context(), ctxUserID{}, uuid.New()))
	rec := httptest.NewRecorder()

	s.handleMeAccountDelete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "confirmation_required") {
		t.Fatalf("body = %s, want confirmation_required", rec.Body.String())
	}
}

func TestClearAuthCookiesExpiresAccountSessionCookies(t *testing.T) {
	s := &Server{cfg: config.Config{TrustProxyHeaders: true}}
	req := httptest.NewRequest(http.MethodPost, "https://example.com/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()

	s.clearAuthCookies(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 3 {
		t.Fatalf("cleared cookies = %d, want 3", len(cookies))
	}
	for _, c := range cookies {
		if c.MaxAge != -1 {
			t.Fatalf("%s MaxAge = %d, want -1", c.Name, c.MaxAge)
		}
		if !c.Expires.Before(time.Now()) {
			t.Fatalf("%s Expires = %v, want past", c.Name, c.Expires)
		}
	}
}
