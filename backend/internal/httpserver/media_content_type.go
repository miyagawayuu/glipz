package httpserver

import (
	"mime"
	"strings"
)

const fallbackDownloadContentType = "application/octet-stream"

var allowedInlineMediaContentTypes = map[string]struct{}{
	"image/avif":      {},
	"image/gif":       {},
	"image/jpeg":      {},
	"image/png":       {},
	"image/webp":      {},
	"video/mp4":       {},
	"video/quicktime": {},
	"video/webm":      {},
	"audio/aac":       {},
	"audio/flac":      {},
	"audio/mp4":       {},
	"audio/mpeg":      {},
	"audio/ogg":       {},
	"audio/wav":       {},
	"audio/webm":      {},
	"audio/x-wav":     {},
}

func normalizeMediaContentType(raw string) string {
	ct := strings.TrimSpace(strings.ToLower(raw))
	if ct == "" {
		return ""
	}
	if mediaType, _, err := mime.ParseMediaType(ct); err == nil {
		ct = mediaType
	}
	return ct
}

func isActiveMediaContentType(raw string) bool {
	ct := normalizeMediaContentType(raw)
	if ct == "" {
		return false
	}
	switch ct {
	case "application/ecmascript",
		"application/javascript",
		"application/xhtml+xml",
		"application/xml",
		"image/svg+xml",
		"text/css",
		"text/ecmascript",
		"text/html",
		"text/javascript",
		"text/xml":
		return true
	default:
		return strings.HasSuffix(ct, "+xml") || strings.HasSuffix(ct, "script")
	}
}

func isInlineSafeMediaContentType(raw string) bool {
	_, ok := allowedInlineMediaContentTypes[normalizeMediaContentType(raw)]
	return ok
}

func isAllowedUploadMediaContentType(raw string) bool {
	return isInlineSafeMediaContentType(raw)
}

func isAllowedPresignedMediaContentType(raw string) bool {
	return isAllowedUploadMediaContentType(raw)
}

func shouldDownloadMediaContentType(raw string) bool {
	return !isInlineSafeMediaContentType(raw)
}
