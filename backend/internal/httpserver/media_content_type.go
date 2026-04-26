package httpserver

import (
	"mime"
	"strings"
)

const fallbackDownloadContentType = "application/octet-stream"

func normalizeMediaContentType(raw string) string {
	ct := strings.TrimSpace(strings.ToLower(raw))
	if ct == "" {
		return ""
	}
	if parsed, _, err := mime.ParseMediaType(ct); err == nil {
		return strings.TrimSpace(strings.ToLower(parsed))
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

func isActiveMediaContentType(raw string) bool {
	ct := normalizeMediaContentType(raw)
	if ct == "" {
		return false
	}
	switch ct {
	case "image/svg+xml",
		"text/html",
		"application/xhtml+xml",
		"application/javascript",
		"application/ecmascript",
		"text/javascript",
		"text/ecmascript":
		return true
	}
	return strings.HasSuffix(ct, "+xml") ||
		ct == "application/xml" ||
		ct == "text/xml" ||
		strings.HasSuffix(ct, "script")
}

func isInlineSafeMediaContentType(raw string) bool {
	ct := normalizeMediaContentType(raw)
	if ct == "" || isActiveMediaContentType(ct) {
		return false
	}
	return strings.HasPrefix(ct, "image/") ||
		strings.HasPrefix(ct, "video/") ||
		strings.HasPrefix(ct, "audio/")
}

func isAllowedUploadMediaContentType(raw string) bool {
	return isInlineSafeMediaContentType(raw)
}

func isAllowedPresignedMediaContentType(raw string) bool {
	ct := normalizeMediaContentType(raw)
	return isInlineSafeMediaContentType(ct) && !strings.HasPrefix(ct, "audio/")
}

func shouldDownloadMediaContentType(raw string) bool {
	return !isInlineSafeMediaContentType(raw)
}
