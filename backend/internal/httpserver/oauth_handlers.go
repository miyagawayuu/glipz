package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"glipz.io/backend/internal/authjwt"
	"glipz.io/backend/internal/repo"
)

const oauthCodeTTL = 10 * time.Minute
const oauthAccessTokenTTL = 1 * time.Hour
const oauthClientCredentialsScope = "client_credentials"

type oauthClientCreateReq struct {
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
}

func (s *Server) handleOAuthClientCreate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	limitRequestBody(w, r, smallJSONRequestBodyMaxBytes)
	var req oauthClientCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 120 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_name"})
		return
	}
	var lines []string
	for _, u := range req.RedirectURIs {
		if strings.TrimSpace(u) == "" {
			continue
		}
		normalized, ok := normalizeOAuthRedirectURI(u)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect_uri"})
			return
		}
		if normalized != "" {
			lines = append(lines, normalized)
		}
	}
	if len(lines) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect_uri"})
		return
	}
	redirectBlock := strings.Join(lines, "\n")
	plainSecret, err := randomHex(32)
	if err != nil {
		writeServerError(w, "oauth client secret", err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plainSecret), bcrypt.DefaultCost)
	if err != nil {
		writeServerError(w, "oauth client bcrypt", err)
		return
	}
	id, err := s.db.OAuthClientCreate(r.Context(), uid, name, redirectBlock, string(hash))
	if err != nil {
		writeServerError(w, "OAuthClientCreate", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":     id.String(),
		"client_secret": plainSecret,
		"name":          name,
		"redirect_uris": lines,
	})
}

func (s *Server) handleOAuthClientList(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.OAuthClientList(r.Context(), uid)
	if err != nil {
		writeServerError(w, "OAuthClientList", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		uris := []string{}
		for _, line := range strings.Split(row.RedirectURIs, "\n") {
			if t := strings.TrimSpace(line); t != "" {
				uris = append(uris, t)
			}
		}
		out = append(out, map[string]any{
			"client_id":     row.ClientIDStr,
			"name":          row.Name,
			"redirect_uris": uris,
			"created_at":    row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleOAuthClientDelete(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	cid, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "clientID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client_id"})
		return
	}
	if err := s.db.OAuthClientDelete(r.Context(), uid, cid); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "OAuthClientDelete", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type oauthAuthorizeReq struct {
	ClientID    string `json:"client_id"`
	RedirectURI string `json:"redirect_uri"`
	State       string `json:"state"`
	Scope       string `json:"scope"`
}

func normalizeOAuthScope(raw string) (string, bool) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(parts) == 0 {
		return "", false
	}
	allowed := map[string]bool{}
	rejected := false
	for _, part := range parts {
		switch part {
		case "posts":
			allowed["posts:read"] = true
			allowed["posts:write"] = true
			allowed["media:write"] = true
		case "posts:read", "posts:write", "media:write":
			allowed[part] = true
		default:
			rejected = true
		}
	}
	if rejected || len(allowed) == 0 {
		return "", false
	}
	out := make([]string, 0, len(allowed))
	for _, scope := range []string{"posts:read", "posts:write", "media:write"} {
		if allowed[scope] {
			out = append(out, scope)
		}
	}
	return strings.Join(out, " "), true
}

func (s *Server) handleOAuthAuthorizeConsent(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	limitRequestBody(w, r, smallJSONRequestBodyMaxBytes)
	var req oauthAuthorizeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	cid, err := uuid.Parse(strings.TrimSpace(req.ClientID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client_id"})
		return
	}
	row, err := s.db.OAuthClientByIDPublic(r.Context(), cid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown_client"})
			return
		}
		writeServerError(w, "OAuthClientByIDPublic", err)
		return
	}
	red, ok := normalizeOAuthRedirectURI(req.RedirectURI)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect_uri"})
		return
	}
	if !repo.OAuthRedirectURIAllowed(row.RedirectURIs, red) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect_uri"})
		return
	}
	scope, ok := normalizeOAuthScope(req.Scope)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_scope"})
		return
	}
	codePlain, err := randomHex(24)
	if err != nil {
		writeServerError(w, "oauth code", err)
		return
	}
	if err := s.db.OAuthAuthorizationCodeInsert(r.Context(), cid, uid, red, scope, codePlain, oauthCodeTTL); err != nil {
		writeServerError(w, "OAuthAuthorizationCodeInsert", err)
		return
	}
	u, err := url.Parse(red)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect_uri"})
		return
	}
	q := u.Query()
	q.Set("code", codePlain)
	if strings.TrimSpace(req.State) != "" {
		q.Set("state", req.State)
	}
	u.RawQuery = q.Encode()
	writeJSON(w, http.StatusOK, map[string]any{"redirect_to": u.String()})
}

// handleOAuthToken implements an application/x-www-form-urlencoded token endpoint close to RFC 6749.
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	limitRequestBody(w, r, oauthFormRequestBodyMaxBytes)
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	grant := strings.TrimSpace(r.Form.Get("grant_type"))
	switch grant {
	case "client_credentials":
		s.oauthTokenClientCredentials(w, r)
	case "authorization_code":
		s.oauthTokenAuthorizationCode(w, r)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
	}
}

func (s *Server) oauthTokenClientCredentials(w http.ResponseWriter, r *http.Request) {
	cid, err := uuid.Parse(strings.TrimSpace(r.Form.Get("client_id")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client"})
		return
	}
	secret := strings.TrimSpace(r.Form.Get("client_secret"))
	if secret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client"})
		return
	}
	ownerID, err := s.db.OAuthClientCredentialsValid(r.Context(), cid, secret)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	tok, err := authjwt.SignOAuthAccess(s.secret, ownerID, cid, oauthClientCredentialsScope, oauthAccessTokenTTL)
	if err != nil {
		writeServerError(w, "oauth SignAccess", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": tok,
		"token_type":   "Bearer",
		"expires_in":   int(oauthAccessTokenTTL.Seconds()),
	})
}

func (s *Server) oauthTokenAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	cid, err := uuid.Parse(strings.TrimSpace(r.Form.Get("client_id")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client"})
		return
	}
	secret := strings.TrimSpace(r.Form.Get("client_secret"))
	code := strings.TrimSpace(r.Form.Get("code"))
	red, redOK := normalizeOAuthRedirectURI(r.Form.Get("redirect_uri"))
	if secret == "" || code == "" || red == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if !redOK {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if _, err := s.db.OAuthClientCredentialsValid(r.Context(), cid, secret); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	userID, scope, err := s.db.OAuthAuthorizationCodeExchange(r.Context(), cid, code, red)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	normalizedScope, ok := normalizeOAuthScope(scope)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	tok, err := authjwt.SignOAuthAccess(s.secret, userID, cid, normalizedScope, oauthAccessTokenTTL)
	if err != nil {
		writeServerError(w, "oauth code SignAccess", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": tok,
		"token_type":   "Bearer",
		"expires_in":   int(oauthAccessTokenTTL.Seconds()),
	})
}

type patCreateReq struct {
	Label string `json:"label"`
}

func (s *Server) handlePersonalAccessTokenCreate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req patCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	secretPart, err := randomHex(24)
	if err != nil {
		writeServerError(w, "pat secret", err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secretPart), bcrypt.DefaultCost)
	if err != nil {
		writeServerError(w, "pat bcrypt", err)
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = "token"
	}
	id, err := s.db.PersonalAccessTokenCreate(r.Context(), uid, label, string(hash), "glpat")
	if err != nil {
		writeServerError(w, "PersonalAccessTokenCreate", err)
		return
	}
	full := "glpat_" + id.String() + "_" + secretPart
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      full,
		"token_id":   id.String(),
		"label":      label,
		"token_type": "Bearer",
	})
}

func (s *Server) handlePersonalAccessTokenList(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.PersonalAccessTokenList(r.Context(), uid)
	if err != nil {
		writeServerError(w, "PersonalAccessTokenList", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id := row.ID.String()
		short := id
		if len(short) > 8 {
			short = short[:8]
		}
		out = append(out, map[string]any{
			"id":           id,
			"label":        row.Label,
			"token_prefix": "glpat_" + short + "…",
			"created_at":   row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handlePersonalAccessTokenDelete(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	tid, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "tokenID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_token_id"})
		return
	}
	if err := s.db.PersonalAccessTokenDelete(r.Context(), uid, tid); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PersonalAccessTokenDelete", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func randomHex(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
