package httpserver

import (
	"expvar"
	"net"
	"net/http"
)

func (s *Server) handleDebugVars(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	ip := net.ParseIP(directClientIP(r))
	if ip == nil || !ip.IsLoopback() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	expvar.Handler().ServeHTTP(w, r)
}
