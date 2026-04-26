package httpserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxLinkPreviewBodyBytes = 1 << 20

type linkPreviewJSON struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
}

func (s *Server) handleGetLinkPreview(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_url"})
		return
	}
	if s.linkPreviewRateLimitExceeded(r.Context(), r) {
		writeLinkPreviewRateLimited(w)
		return
	}
	u, err := validateLinkPreviewURL(r.Context(), raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url"})
		return
	}
	out, err := fetchLinkPreview(r.Context(), u)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "preview_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func validateLinkPreviewURL(ctx context.Context, raw string) (*url.URL, error) {
	return validatePublicOutboundURL(ctx, raw, true)
}

func ensurePublicPreviewHost(ctx context.Context, host string) error {
	return ensurePublicOutboundHost(ctx, host)
}

func fetchLinkPreview(ctx context.Context, target *url.URL) (linkPreviewJSON, error) {
	client := newPublicOutboundHTTPClient(6 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return linkPreviewJSON{}, err
	}
	req.Header.Set("User-Agent", "GlipzBot/1.0 (+https://glipz.io)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	res, err := client.Do(req)
	if err != nil {
		return linkPreviewJSON{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return linkPreviewJSON{}, fmt.Errorf("unexpected status")
	}
	ct := strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Type")))
	if ct != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml+xml") {
		return linkPreviewJSON{}, fmt.Errorf("unsupported content type")
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxLinkPreviewBodyBytes+1))
	if err != nil {
		return linkPreviewJSON{}, err
	}
	if len(body) > maxLinkPreviewBodyBytes {
		return linkPreviewJSON{}, fmt.Errorf("body too large")
	}
	out, err := extractLinkPreview(res.Request.URL, body)
	if err != nil {
		return linkPreviewJSON{}, err
	}
	if out.Title == "" && out.Description == "" && out.ImageURL == "" {
		return linkPreviewJSON{}, fmt.Errorf("empty preview")
	}
	out.URL = res.Request.URL.String()
	if out.Title == "" {
		out.Title = res.Request.URL.Hostname()
	}
	return out, nil
}

func extractLinkPreview(base *url.URL, body []byte) (linkPreviewJSON, error) {
	var out linkPreviewJSON
	var titleText string
	var inTitle bool
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			if err := z.Err(); err != nil && err != io.EOF {
				return linkPreviewJSON{}, err
			}
			if out.Title == "" {
				out.Title = cleanPreviewText(titleText)
			}
			out.Description = cleanPreviewText(out.Description)
			out.SiteName = cleanPreviewText(out.SiteName)
			if out.ImageURL != "" {
				if img, err := base.Parse(out.ImageURL); err == nil {
					out.ImageURL = img.String()
				}
			}
			return out, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			switch strings.ToLower(tok.Data) {
			case "title":
				inTitle = true
			case "meta":
				applyPreviewMeta(&out, tok)
			}
		case html.TextToken:
			if inTitle {
				titleText += string(z.Text())
			}
		case html.EndTagToken:
			tok := z.Token()
			if strings.EqualFold(tok.Data, "title") {
				inTitle = false
			}
		}
	}
}

func applyPreviewMeta(out *linkPreviewJSON, tok html.Token) {
	var prop, name, content string
	for _, attr := range tok.Attr {
		switch strings.ToLower(strings.TrimSpace(attr.Key)) {
		case "property":
			prop = strings.ToLower(strings.TrimSpace(attr.Val))
		case "name":
			name = strings.ToLower(strings.TrimSpace(attr.Val))
		case "content":
			content = strings.TrimSpace(attr.Val)
		}
	}
	if content == "" {
		return
	}
	switch {
	case prop == "og:title":
		out.Title = cleanPreviewText(content)
	case prop == "og:description":
		out.Description = cleanPreviewText(content)
	case prop == "og:image":
		out.ImageURL = content
	case prop == "og:site_name":
		out.SiteName = cleanPreviewText(content)
	case out.Title == "" && name == "twitter:title":
		out.Title = cleanPreviewText(content)
	case out.Description == "" && (name == "description" || name == "twitter:description"):
		out.Description = cleanPreviewText(content)
	case out.ImageURL == "" && name == "twitter:image":
		out.ImageURL = content
	}
}

func cleanPreviewText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
