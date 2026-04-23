package httpserver

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"glipz.io/backend/internal/repo"
)

func (s *Server) handleFederationDMAttachmentProxy(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	acct := strings.TrimSpace(r.URL.Query().Get("acct"))
	if rawURL == "" || acct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	if err := s.ensureMutualFollowForFederationDM(r.Context(), uid, acct); err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeServerError(w, "ensureMutualFollowForFederationDM attachment", err)
		return
	}

	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url"})
		return
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url_scheme"})
		return
	}
	u.Fragment = ""

	_, host, err := splitAcct(acct)
	if err != nil || host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_acct"})
		return
	}
	if !strings.EqualFold(strings.TrimSpace(u.Host), strings.TrimSpace(host)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url_host_mismatch"})
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", u.String(), nil)
	if err != nil {
		writeServerError(w, "NewRequestWithContext attachment", err)
		return
	}
	req.Header.Set("Accept", "*/*")

	// Use the shared federation client (timeout, redirects, etc).
	res, err := federationHTTP.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "fetch_failed"})
		return
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "fetch_failed"})
		return
	}

	// Same-origin response. Avoid caching since this is user-scoped.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	if ct := strings.TrimSpace(res.Header.Get("Content-Type")); ct != "" {
		// Keep remote content-type if present.
		w.Header().Set("Content-Type", ct)
	}
	if cl := strings.TrimSpace(res.Header.Get("Content-Length")); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))

	_, _ = io.Copy(w, res.Body)
}

