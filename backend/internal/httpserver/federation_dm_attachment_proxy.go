package httpserver

import (
	"errors"
	"log"
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
	if !strings.EqualFold(strings.TrimSpace(u.Hostname()), strings.TrimSpace(host)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url_host_mismatch"})
		return
	}
	if _, err := validatePublicOutboundURL(r.Context(), u.String(), true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_remote_host"})
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
	maxBytes := s.cfg.FederationDMAttachmentMaxBytes
	if responseContentLengthExceeds(res.Header, maxBytes) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "attachment_too_large"})
		return
	}

	// Same-origin response. Avoid caching since this is user-scoped.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	if ct := strings.TrimSpace(res.Header.Get("Content-Type")); ct != "" {
		// Keep remote content-type if present.
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Date", time.Now().UTC().Format(time.RFC1123))

	n, exceeded, err := copyWithMaxBytes(w, res.Body, maxBytes)
	if exceeded {
		log.Printf("federation dm attachment proxy exceeded limit: url_host=%s bytes=%d max=%d", u.Hostname(), n, maxBytes)
		return
	}
	if err != nil {
		log.Printf("federation dm attachment proxy copy error: url_host=%s bytes=%d err=%v", u.Hostname(), n, err)
	}
}
