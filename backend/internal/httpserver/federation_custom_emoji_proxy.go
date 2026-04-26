package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"glipz.io/backend/internal/repo"
)

const federationRemoteEmojiCacheTTL = 24 * time.Hour

// handlePublicFederationCustomEmojiResolve resolves a remote custom emoji shortcode (e.g. :party@remote.example:)
// into an image_url by fetching the remote instance's public custom emoji catalog, with caching.
//
// This endpoint is intentionally public so anonymous viewers can render federated content without CORS issues.
func (s *Server) handlePublicFederationCustomEmojiResolve(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("shortcode"))
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_shortcode"})
		return
	}
	ref, ok := repo.ParseEmojiReference(raw)
	if !ok || !ref.IsShortcode || ref.Name == "" || ref.Domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_shortcode"})
		return
	}
	host := strings.ToLower(strings.TrimSpace(ref.Domain))
	if !isSafeRemoteHost(host) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer"})
		return
	}

	// Cache hit?
	if cached, ok := s.db.GetFederationRemoteCustomEmojiCache(r.Context(), host, ref.Name); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"shortcode":   repo.MakeCustomEmojiShortcode(ref.Name, "", host),
			"image_url":   s.federationRemoteMediaURL(cached),
			"cache_hit":   true,
			"cache_ttl_s": int(federationRemoteEmojiCacheTTL.Seconds()),
		})
		return
	}

	disc, err := fetchRemoteFederationDiscovery(r.Context(), host)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "discovery_failed"})
		return
	}
	origin := strings.TrimSpace(disc.Server.Origin)
	if origin == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_peer"})
		return
	}
	base, err := url.Parse(origin)
	if err != nil || base.Scheme == "" || base.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_peer"})
		return
	}

	// Fetch remote custom emoji catalog (public endpoint on the remote instance).
	base.Path = strings.TrimSuffix(base.Path, "/") + "/api/v1/custom-emojis"
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	res, err := federationHTTP.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer_unreachable"})
		return
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer_http_error"})
		return
	}
	var doc struct {
		Items []struct {
			Shortcode     string `json:"shortcode"`
			ShortcodeName string `json:"shortcode_name"`
			ImageURL      string `json:"image_url"`
			IsEnabled     bool   `json:"is_enabled"`
		} `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, res.Body, 2<<20)).Decode(&doc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_peer_response"})
		return
	}

	// Resolve :name@domain: → remote's :name: entry.
	wantName := strings.ToLower(strings.TrimSpace(ref.Name))
	wantShort := repo.MakeCustomEmojiShortcode(wantName, "", "")
	imageURL := ""
	for _, it := range doc.Items {
		if !it.IsEnabled {
			continue
		}
		if strings.ToLower(strings.TrimSpace(it.ShortcodeName)) == wantName || strings.EqualFold(strings.TrimSpace(it.Shortcode), wantShort) {
			imageURL = strings.TrimSpace(it.ImageURL)
			break
		}
	}
	if imageURL == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	_ = s.db.UpsertFederationRemoteCustomEmojiCache(r.Context(), host, wantName, imageURL, time.Now().UTC().Add(federationRemoteEmojiCacheTTL))

	writeJSON(w, http.StatusOK, map[string]any{
		"shortcode":   repo.MakeCustomEmojiShortcode(wantName, "", host),
		"image_url":   s.federationRemoteMediaURL(imageURL),
		"cache_hit":   false,
		"cache_ttl_s": int(federationRemoteEmojiCacheTTL.Seconds()),
	})
}
