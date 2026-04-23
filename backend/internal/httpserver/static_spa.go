package httpserver

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// mountStaticSPAFallback routes unmatched GET and HEAD requests to static files
// or to index.html for Vue Router when STATIC_WEB_ROOT is enabled.
func (s *Server) mountStaticSPAFallback(r *chi.Mux) {
	root := strings.TrimSpace(s.cfg.StaticWebRoot)
	if root == "" {
		return
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		log.Printf("STATIC_WEB_ROOT %q: not a directory (%v), SPA static disabled", root, err)
		return
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Printf("STATIC_WEB_ROOT abs: %v", err)
		return
	}
	indexPath := filepath.Join(absRoot, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		log.Printf("STATIC_WEB_ROOT: missing index.html under %q", absRoot)
		return
	}
	log.Printf("serving static web from %s", absRoot)

	r.NotFound(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			http.NotFound(w, req)
			return
		}
		p := req.URL.Path
		if strings.HasPrefix(p, "/api/") {
			http.NotFound(w, req)
			return
		}
		if p == "/ap" || strings.HasPrefix(p, "/ap/") {
			http.NotFound(w, req)
			return
		}
		if p == "/federation/guidelines" || p == "/federation/guidelines/" {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, req, indexPath)
			return
		}
		if p == "/federation" || strings.HasPrefix(p, "/federation/") {
			http.NotFound(w, req)
			return
		}
		if p == "/.well-known" || strings.HasPrefix(p, "/.well-known/") {
			http.NotFound(w, req)
			return
		}
		if p == "/live" || strings.HasPrefix(p, "/live/") {
			http.NotFound(w, req)
			return
		}
		if p == "/spaces" || strings.HasPrefix(p, "/spaces/") {
			http.NotFound(w, req)
			return
		}
		if p == "/health" {
			http.NotFound(w, req)
			return
		}

		rel := strings.TrimPrefix(path.Clean("/"+p), "/")
		full := filepath.Join(absRoot, filepath.FromSlash(rel))
		if !strings.HasPrefix(full, absRoot) {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			if strings.HasSuffix(strings.ToLower(full), ".html") {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFile(w, req, full)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, req, indexPath)
	}))
}
