package httpserver

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"glipz.io/backend/internal/s3client"
)

const remoteMediaProxyCacheControl = "public, max-age=3600"

func (s *Server) mediaProxyBaseURL() string {
	if base := strings.TrimSpace(s.cfg.GlipzProtocolMediaPublicBase); base != "" {
		return strings.TrimSuffix(base, "/")
	}
	if origin := strings.TrimSpace(s.federationPublicOrigin()); origin != "" {
		return strings.TrimSuffix(origin, "/") + "/api/v1/media/object"
	}
	return ""
}

func (s *Server) glipzProtocolPublicMediaURL(objectKey string) string {
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	var u string
	if base := s.mediaProxyBaseURL(); base != "" && key != "" {
		u = strings.TrimSuffix(base, "/") + "/" + key
	} else {
		u = s.s3.PublicURL(objectKey)
	}
	origin := strings.TrimSpace(s.federationPublicOrigin())
	if strings.HasPrefix(strings.ToLower(origin), "https://") && len(u) >= 7 && strings.EqualFold(u[:7], "http://") {
		return "https://" + u[7:]
	}
	return u
}

func (s *Server) localMediaDirectURL(objectKey string) string {
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if base := strings.TrimSpace(s.cfg.GlipzProtocolMediaPublicBase); base != "" && key != "" {
		return strings.TrimSuffix(base, "/") + "/" + key
	}
	return s.s3.PublicURL(objectKey)
}

func (s *Server) remoteMediaProxyBaseURL() string {
	if origin := strings.TrimSpace(s.federationPublicOrigin()); origin != "" {
		return strings.TrimSuffix(origin, "/") + "/api/v1/media/remote"
	}
	return "/api/v1/media/remote"
}

func (s *Server) isOwnMediaURL(u *url.URL) bool {
	if u == nil || strings.TrimSpace(u.Host) == "" {
		return false
	}
	origin, err := url.Parse(strings.TrimSpace(s.federationPublicOrigin()))
	if err != nil || origin == nil || strings.TrimSpace(origin.Host) == "" {
		return false
	}
	return strings.EqualFold(u.Host, origin.Host) && strings.HasPrefix(strings.TrimSpace(u.EscapedPath()), "/api/v1/media/")
}

func (s *Server) federationRemoteMediaURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return raw
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return raw
	}
	if !isSafeRemoteHost(u.Hostname()) || s.isOwnMediaURL(u) {
		return raw
	}
	u.Fragment = ""
	return s.remoteMediaProxyBaseURL() + "?url=" + url.QueryEscape(u.String())
}

func (s *Server) federationRemoteMediaURLs(urls []string) []string {
	if len(urls) == 0 {
		return urls
	}
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		if u := s.federationRemoteMediaURL(raw); strings.TrimSpace(u) != "" {
			out = append(out, u)
		}
	}
	return out
}

func writeMediaProxyHeaders(w http.ResponseWriter, meta s3client.ObjectMeta) {
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	if meta.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.ContentLength, 10))
	}
	if meta.ETag != "" {
		w.Header().Set("ETag", meta.ETag)
	}
	if !meta.LastModified.IsZero() {
		w.Header().Set("Last-Modified", meta.LastModified.UTC().Format(http.TimeFormat))
	}
	if meta.CacheControl != "" {
		w.Header().Set("Cache-Control", meta.CacheControl)
	}
	if meta.AcceptRanges != "" {
		w.Header().Set("Accept-Ranges", meta.AcceptRanges)
	} else {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	if meta.ContentRange != "" {
		w.Header().Set("Content-Range", meta.ContentRange)
	}
}

func isAllowedRemoteMediaContentType(raw string) bool {
	ct := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "" || ct == "application/octet-stream" {
		return true
	}
	return strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/")
}

func copyRemoteMediaProxyHeaders(w http.ResponseWriter, h http.Header) {
	for _, name := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"ETag",
		"Last-Modified",
		"Accept-Ranges",
	} {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			w.Header().Set(name, v)
		}
	}
	if cc := strings.TrimSpace(h.Get("Cache-Control")); cc != "" {
		w.Header().Set("Cache-Control", cc)
	} else {
		w.Header().Set("Cache-Control", remoteMediaProxyCacheControl)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
}

func (s *Server) handlePublicRemoteMediaProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	u, err := url.Parse(rawURL)
	if rawURL == "" || err != nil || u == nil || strings.TrimSpace(u.Host) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url"})
		return
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url_scheme"})
		return
	}
	if !isSafeRemoteHost(u.Hostname()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_remote_host"})
		return
	}
	u.Fragment = ""

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url"})
		return
	}
	if accept := strings.TrimSpace(r.Header.Get("Accept")); accept != "" {
		req.Header.Set("Accept", accept)
	} else {
		req.Header.Set("Accept", "image/*,video/*,audio/*,*/*;q=0.1")
	}
	if rg := strings.TrimSpace(r.Header.Get("Range")); rg != "" {
		req.Header.Set("Range", rg)
	}

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
	if !isAllowedRemoteMediaContentType(res.Header.Get("Content-Type")) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "unsupported_media_type"})
		return
	}

	copyRemoteMediaProxyHeaders(w, res.Header)
	w.WriteHeader(res.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	n, _ := io.Copy(w, res.Body)
	addMediaProxyBytes("remote", n)
}

func (s *Server) handlePublicMediaObject(w http.ResponseWriter, r *http.Request) {
	rawKey := strings.TrimSpace(chi.URLParam(r, "*"))
	if rawKey == "" {
		http.NotFound(w, r)
		return
	}
	objectKey, err := url.PathUnescape(rawKey)
	if err != nil || strings.TrimSpace(objectKey) == "" {
		http.NotFound(w, r)
		return
	}
	if s.cfg.MediaProxyMode == "direct" {
		http.Redirect(w, r, s.localMediaDirectURL(objectKey), http.StatusTemporaryRedirect)
		return
	}

	switch r.Method {
	case http.MethodHead:
		meta, err := s.s3.HeadObject(r.Context(), objectKey)
		if err != nil {
			switch {
			case s3client.IsNotFound(err):
				http.NotFound(w, r)
			default:
				writeServerError(w, "media head", err)
			}
			return
		}
		writeMediaProxyHeaders(w, meta)
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodGet:
		obj, err := s.s3.GetObject(r.Context(), objectKey, strings.TrimSpace(r.Header.Get("Range")))
		if err != nil {
			switch {
			case s3client.IsNotFound(err):
				http.NotFound(w, r)
			case s3client.IsInvalidRange(err):
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			default:
				writeServerError(w, "media get", err)
			}
			return
		}
		defer obj.Body.Close()
		writeMediaProxyHeaders(w, obj.ObjectMeta)
		status := http.StatusOK
		if obj.ContentRange != "" {
			status = http.StatusPartialContent
		}
		w.WriteHeader(status)
		n, _ := io.Copy(w, obj.Body)
		addMediaProxyBytes("local", n)
		return
	default:
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
