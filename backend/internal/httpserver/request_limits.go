package httpserver

import "net/http"

const (
	smallJSONRequestBodyMaxBytes  = 16 << 10
	defaultAPIRequestBodyMaxBytes = 2 << 20
	oauthFormRequestBodyMaxBytes  = 16 << 10
)

func limitRequestBody(w http.ResponseWriter, r *http.Request, maxBytes int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
}

func limitAPIRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/api/v1/media/upload", "/api/v1/dm/upload":
			next.ServeHTTP(w, r)
			return
		}
		limitRequestBody(w, r, defaultAPIRequestBodyMaxBytes)
		next.ServeHTTP(w, r)
	})
}
