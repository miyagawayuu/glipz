package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) handleFederationDMPeerKeysGet(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	acct := strings.TrimSpace(r.URL.Query().Get("acct"))
	if acct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_acct"})
		return
	}
	user, host, err := splitAcct(acct)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_acct"})
		return
	}
	disc, err := fetchRemoteFederationDiscovery(r.Context(), host)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "discovery_failed"})
		return
	}
	base := strings.TrimSpace(disc.Server.DMKeysURL)
	if base == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_peer"})
		return
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_peer"})
		return
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Hostname(), host) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_peer"})
		return
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + url.PathEscape(user)
	if _, err := validatePublicOutboundURL(r.Context(), u.String(), false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_peer"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	res, err := federationHTTP.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer_unreachable"})
		return
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("peer_http_%d", res.StatusCode)})
		return
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil || len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer_response"})
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer_response"})
		return
	}
	pub, ok := doc["public_jwk"]
	if !ok || pub == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer_response"})
		return
	}
	// Pass through algorithm/kid if present; clients primarily need public_jwk.
	writeJSON(w, http.StatusOK, doc)
}
