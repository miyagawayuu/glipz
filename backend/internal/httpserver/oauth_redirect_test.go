package httpserver

import "testing"

func TestNormalizeOAuthRedirectURIRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"https://user:pass@example.com/callback",
		"https://example.com/callback#fragment",
		"/callback",
		"javascript:alert(1)",
		"https://example.com/call back",
		"http://example.com/callback",
	} {
		if got, ok := normalizeOAuthRedirectURI(raw); ok {
			t.Fatalf("normalizeOAuthRedirectURI(%q) = %q, true; want false", raw, got)
		}
	}
}

func TestNormalizeOAuthRedirectURIAllowsHTTPSAndLocalhostHTTP(t *testing.T) {
	tests := map[string]string{
		"HTTPS://Example.COM/callback?x=1": "https://example.com/callback?x=1",
		"http://localhost:5173/callback":   "http://localhost:5173/callback",
		"http://127.0.0.1:5173/callback":   "http://127.0.0.1:5173/callback",
		"http://[::1]:5173/callback?x=1":   "http://[::1]:5173/callback?x=1",
	}
	for raw, want := range tests {
		got, ok := normalizeOAuthRedirectURI(raw)
		if !ok || got != want {
			t.Fatalf("normalizeOAuthRedirectURI(%q) = %q, %v; want %q, true", raw, got, ok, want)
		}
	}
}
