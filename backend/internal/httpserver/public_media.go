package httpserver

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"glipz.io/backend/internal/s3client"
)

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
		_, _ = io.Copy(w, obj.Body)
		return
	default:
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
