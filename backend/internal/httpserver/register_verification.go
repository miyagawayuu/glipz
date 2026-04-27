package httpserver

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"glipz.io/backend/internal/authjwt"
	"glipz.io/backend/internal/mailer"
	"glipz.io/backend/internal/repo"
)

type registerVerifyReq struct {
	Token string `json:"token"`
}

type registerResendReq struct {
	Email string `json:"email"`
}

func newRegistrationToken() (raw string, sha256Hex string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	sha256Hex = hex.EncodeToString(sum[:])
	return raw, sha256Hex, nil
}

func registrationTokenSHA256(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Server) registrationVerificationLink(rawToken string) string {
	return strings.TrimSuffix(s.cfg.FrontendOrigin, "/") + "/register/verify?token=" + url.QueryEscape(rawToken)
}

func (s *Server) sendRegistrationVerificationEmail(email, verifyURL string, expiresAt time.Time) error {
	body := fmt.Sprintf(
		"Glipz のアカウント仮登録を受け付けました。\n\n以下のリンクにアクセスすると登録が完了します。\n%s\n\nこのリンクの有効期限は %s です。\n心当たりがない場合は、このメールを破棄してください。\n",
		verifyURL,
		expiresAt.Local().Format("2006-01-02 15:04:05 MST"),
	)
	cfg := mailer.Config{
		Domain:    s.cfg.MailgunDomain,
		APIKey:    s.cfg.MailgunAPIKey,
		APIBase:   s.cfg.MailgunAPIBase,
		FromEmail: s.cfg.MailFromEmail,
		FromName:  s.cfg.MailFromName,
	}
	if !cfg.Enabled() {
		log.Printf("registration verification email skipped for %s; verification token withheld from logs", email)
		return nil
	}
	return mailer.SendText(cfg, email, "Glipz メール認証", body)
}

func (s *Server) handleRegisterResend(w http.ResponseWriter, r *http.Request) {
	siteSettings, err := s.db.GetSiteSettings(r.Context())
	if err != nil {
		writeServerError(w, "register resend GetSiteSettings", err)
		return
	}
	if !siteSettings.RegistrationsEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "registrations_disabled"})
		return
	}
	limitRequestBody(w, r, smallJSONRequestBodyMaxBytes)
	var req registerResendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		return
	}
	if _, err := s.db.UserByEmail(r.Context(), email); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	} else if !errors.Is(err, repo.ErrNotFound) {
		writeServerError(w, "register resend UserByEmail", err)
		return
	}
	rawToken, tokenSHA256, err := newRegistrationToken()
	if err != nil {
		writeServerError(w, "register resend newRegistrationToken", err)
		return
	}
	expiresAt := time.Now().UTC().Add(s.cfg.RegistrationVerifyTTL)
	expiresAt, ok, err := s.db.RefreshPendingUserRegistrationToken(r.Context(), email, tokenSHA256, expiresAt)
	if err != nil {
		writeServerError(w, "register resend RefreshPendingUserRegistrationToken", err)
		return
	}
	if ok {
		if err := s.sendRegistrationVerificationEmail(email, s.registrationVerificationLink(rawToken), expiresAt); err != nil {
			writeServerError(w, "register resend sendRegistrationVerificationEmail", err)
			return
		}
	}
	out := map[string]any{"ok": true}
	if ok {
		out["expires_at"] = expiresAt.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRegisterVerify(w http.ResponseWriter, r *http.Request) {
	limitRequestBody(w, r, smallJSONRequestBodyMaxBytes)
	var req registerVerifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_token"})
		return
	}
	userID, err := s.db.CompletePendingUserRegistration(r.Context(), registrationTokenSHA256(token))
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_token"})
			return
		case errors.Is(err, repo.ErrPendingRegistrationExpired):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_token"})
			return
		case errors.Is(err, repo.ErrPendingRegistrationConsumed):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already_verified"})
			return
		case errors.Is(err, repo.ErrHandleTaken):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "handle_exhausted"})
			return
		case errors.Is(err, repo.ErrReservedHandle):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "reserved_handle"})
			return
		case strings.Contains(strings.ToLower(err.Error()), "email taken"):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email_taken"})
			return
		default:
			writeServerError(w, "register verify CompletePendingUserRegistration", err)
			return
		}
	}
	tok, err := authjwt.SignAccess(s.secret, userID, 24*time.Hour)
	if err != nil {
		writeServerError(w, "register verify SignAccess", err)
		return
	}
	csrfToken, err := randomHex(32)
	if err != nil {
		writeServerError(w, "register verify csrf token", err)
		return
	}
	accessTTL := 24 * time.Hour
	s.setAccessCookie(w, r, tok, accessTTL)
	s.setCSRFCookie(w, r, csrfToken, accessTTL)
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":      userID.String(),
		"access_token": tok,
		"token_type":   "Bearer",
	})
}
