package httpserver

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const maxLegalDocBytes = 1 << 20

var legalDocNames = map[string]struct{}{
	"terms":           {},
	"privacy":         {},
	"nsfw-guidelines": {},
}

var legalDocLocalePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,16}$`)

type legalDocResponse struct {
	Name      string `json:"name"`
	Locale    string `json:"locale,omitempty"`
	Markdown  string `json:"markdown"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (s *Server) handleLegalDoc(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.cfg.LegalDocsDir)
	if root == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "legal_docs_not_configured"})
		return
	}
	doc := strings.TrimSpace(chi.URLParam(r, "doc"))
	if _, ok := legalDocNames[doc]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "legal_doc_not_found"})
		return
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		writeServerError(w, "LEGAL_DOCS_DIR abs", err)
		return
	}
	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "legal_docs_not_configured"})
		return
	}

	locale := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("locale")))
	if !legalDocLocalePattern.MatchString(locale) {
		locale = ""
	}

	candidates := []struct {
		filename string
		locale   string
	}{{filename: doc + ".md"}}
	if locale != "" {
		candidates = append([]struct {
			filename string
			locale   string
		}{{filename: fmt.Sprintf("%s.%s.md", doc, locale), locale: locale}}, candidates...)
	}

	for _, candidate := range candidates {
		full := filepath.Join(absRoot, candidate.filename)
		if !strings.HasPrefix(full, absRoot+string(os.PathSeparator)) && full != absRoot {
			continue
		}
		st, err := os.Stat(full)
		if err != nil || st.IsDir() {
			continue
		}
		if st.Size() > maxLegalDocBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "legal_doc_too_large"})
			return
		}
		body, err := os.ReadFile(full)
		if err != nil {
			writeServerError(w, "legal doc read", err)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		writeJSON(w, http.StatusOK, legalDocResponse{
			Name:      doc,
			Locale:    candidate.locale,
			Markdown:  string(body),
			UpdatedAt: st.ModTime().UTC().Format(time.RFC3339),
		})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "legal_doc_not_found"})
}
