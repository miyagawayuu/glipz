package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

const (
	identityTransferSessionTTL       = 24 * time.Hour
	identityTransferMaxTokenAttempts = 20
	identityTransferPostBatchLimit   = 20
	identityTransferMediaMaxBytes    = mediaUploadMaxBytes
)

type secureIdentityExportRequest struct {
	Passphrase   string `json:"passphrase"`
	TargetOrigin string `json:"target_origin"`
}

type secureIdentityImportRequest struct {
	Bundle     identityBundle `json:"bundle"`
	Passphrase string         `json:"passphrase"`
}

type identityTransferSessionCreateRequest struct {
	TargetOrigin   string `json:"target_origin"`
	IncludePrivate bool   `json:"include_private"`
	IncludeGated   bool   `json:"include_gated"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

type identityTransferSessionCreateResponse struct {
	Session repo.IdentityTransferSession `json:"session"`
	Token   string                       `json:"token"`
}

type identityTransferPostsResponse struct {
	Posts      []repo.TransferPostPayload `json:"posts"`
	NextCursor string                     `json:"next_cursor"`
	Done       bool                       `json:"done"`
}

type identityTransferImportJobCreateRequest struct {
	SourceOrigin    string `json:"source_origin"`
	TargetOrigin    string `json:"target_origin"`
	SourceSessionID string `json:"source_session_id"`
	Token           string `json:"token"`
	IncludePrivate  bool   `json:"include_private"`
	IncludeGated    bool   `json:"include_gated"`
}

func (s *Server) identityTransferTargetOrigin(fallback string) string {
	if origin := strings.TrimSpace(fallback); origin != "" {
		return strings.TrimRight(origin, "/")
	}
	if origin := strings.TrimSpace(s.cfg.FrontendOrigin); origin != "" {
		return strings.TrimRight(origin, "/")
	}
	return strings.TrimRight(s.federationPublicOrigin(), "/")
}

func (s *Server) handleMeIdentityExportSecure(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req secureIdentityExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if strings.TrimSpace(req.Passphrase) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_passphrase"})
		return
	}
	targetOrigin := ""
	if strings.TrimSpace(req.TargetOrigin) != "" {
		origin, _, err := normalizeTransferOrigin(req.TargetOrigin)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_origin"})
			return
		}
		targetOrigin = origin
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeServerError(w, "UserByID identity secure export", err)
		return
	}
	identity, err := s.db.EnsureUserPortableIdentity(r.Context(), uid)
	if err != nil {
		writeServerError(w, "EnsureUserPortableIdentity secure export", err)
		return
	}
	enc, err := encryptIdentityPrivateKey(req.Passphrase, identity.AccountPrivateKeyEncrypted)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "weak_passphrase"})
		return
	}
	writeJSON(w, http.StatusOK, identityBundle{
		V:                identityBundleV2,
		PortableID:       identity.PortableID,
		AccountPublicKey: identity.AccountPublicKey,
		PrivateKey:       &enc,
		Handle:           u.Handle,
		DisplayName:      u.DisplayName,
		Bio:              u.Bio,
		AlsoKnownAs:      append([]string(nil), u.AlsoKnownAs...),
		CreatedForOrigin: targetOrigin,
		ExportedAt:       time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleMeIdentityImportSecure(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req secureIdentityImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if req.Bundle.V != identityBundleV2 || req.Bundle.PrivateKey == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_identity_bundle"})
		return
	}
	privateKey, err := decryptIdentityPrivateKey(req.Passphrase, *req.Bundle.PrivateKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_identity_bundle"})
		return
	}
	if err := s.db.SetUserPortableIdentity(r.Context(), uid, repo.PortableIdentity{
		PortableID:                 req.Bundle.PortableID,
		AccountPublicKey:           req.Bundle.AccountPublicKey,
		AccountPrivateKeyEncrypted: privateKey,
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_identity_bundle"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeIdentityTransferSessionCreate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req identityTransferSessionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	targetOrigin, _, err := normalizeTransferOrigin(req.TargetOrigin)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_origin"})
		return
	}
	identity, err := s.db.EnsureUserPortableIdentity(r.Context(), uid)
	if err != nil {
		writeServerError(w, "EnsureUserPortableIdentity transfer session", err)
		return
	}
	token, err := randomBase64URL(identityTransferTokenBytes)
	if err != nil {
		writeServerError(w, "identity transfer token", err)
		return
	}
	nonce, err := randomBase64URL(16)
	if err != nil {
		writeServerError(w, "identity transfer nonce", err)
		return
	}
	ttl := time.Duration(req.ExpiresInHours) * time.Hour
	if ttl <= 0 || ttl > identityTransferSessionTTL {
		ttl = identityTransferSessionTTL
	}
	session, err := s.db.CreateIdentityTransferSession(r.Context(), repo.IdentityTransferSessionInsert{
		UserID:              uid,
		PortableID:          identity.PortableID,
		TokenHash:           hashTransferToken(token),
		TokenNonce:          nonce,
		AllowedTargetOrigin: targetOrigin,
		IncludePrivate:      req.IncludePrivate,
		IncludeGated:        req.IncludeGated,
		ExpiresAt:           time.Now().UTC().Add(ttl),
		CreatedIPHash:       hmacIP(s.secret, directClientIP(r)),
	})
	if err != nil {
		writeServerError(w, "CreateIdentityTransferSession", err)
		return
	}
	writeJSON(w, http.StatusOK, identityTransferSessionCreateResponse{Session: session, Token: token})
}

func (s *Server) handleMeIdentityTransferSessionGet(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}
	session, err := s.db.IdentityTransferSessionByID(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) || (err == nil && session.UserID != uid) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeServerError(w, "IdentityTransferSessionByID", err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleMeIdentityTransferSessionRevoke(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}
	if err := s.db.RevokeIdentityTransferSession(r.Context(), uid, id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, repo.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) authenticatedTransferSession(r *http.Request) (repo.IdentityTransferSession, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		return repo.IdentityTransferSession{}, false
	}
	session, err := s.db.IdentityTransferSessionByID(r.Context(), id)
	if err != nil {
		return repo.IdentityTransferSession{}, false
	}
	if session.RevokedAt != nil || time.Now().UTC().After(session.ExpiresAt) || session.AttemptCount >= identityTransferMaxTokenAttempts {
		return repo.IdentityTransferSession{}, false
	}
	token := strings.TrimSpace(r.Header.Get("X-Glipz-Transfer-Token"))
	if token == "" {
		if h := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(h), "bearer ") {
			token = strings.TrimSpace(h[len("Bearer "):])
		}
	}
	targetOrigin, _, err := normalizeTransferOrigin(r.Header.Get("X-Glipz-Target-Origin"))
	if err != nil || !strings.EqualFold(targetOrigin, session.AllowedTargetOrigin) {
		_ = s.db.RecordIdentityTransferTokenFailure(r.Context(), session.ID, hmacIP(s.secret, directClientIP(r)))
		return repo.IdentityTransferSession{}, false
	}
	if !constantEqualBase64Hash(session.TokenHash, hashTransferToken(token)) {
		_ = s.db.RecordIdentityTransferTokenFailure(r.Context(), session.ID, hmacIP(s.secret, directClientIP(r)))
		return repo.IdentityTransferSession{}, false
	}
	return session, true
}

func (s *Server) handleIdentityTransferManifest(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authenticatedTransferSession(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	manifest, err := s.db.IdentityTransferManifest(r.Context(), session.UserID, session.IncludePrivate, session.IncludeGated)
	if err != nil {
		writeServerError(w, "IdentityTransferManifest", err)
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleIdentityTransferPosts(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authenticatedTransferSession(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("cursor")))
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if limit <= 0 || limit > identityTransferPostBatchLimit {
		limit = identityTransferPostBatchLimit
	}
	posts, next, err := s.db.ListIdentityTransferPosts(r.Context(), session.UserID, session.IncludePrivate, session.IncludeGated, offset, limit)
	if err != nil {
		writeServerError(w, "ListIdentityTransferPosts", err)
		return
	}
	done := len(posts) < limit
	nextCursor := ""
	if !done {
		nextCursor = strconv.Itoa(next)
	}
	writeJSON(w, http.StatusOK, identityTransferPostsResponse{Posts: posts, NextCursor: nextCursor, Done: done})
}

func (s *Server) handleIdentityTransferMedia(w http.ResponseWriter, r *http.Request) {
	session, ok := s.authenticatedTransferSession(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	objectKey := strings.TrimSpace(r.URL.Query().Get("object_key"))
	allowed, err := s.db.TransferObjectKeyAllowed(r.Context(), session.UserID, objectKey, session.IncludePrivate, session.IncludeGated)
	if err != nil {
		writeServerError(w, "TransferObjectKeyAllowed", err)
		return
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	obj, err := s.s3.GetObject(r.Context(), objectKey, "")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	defer obj.Body.Close()
	if obj.ContentLength > identityTransferMediaMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file_too_large"})
		return
	}
	ct := normalizeMediaContentType(obj.ContentType)
	if !isAllowedUploadMediaContentType(ct) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_type"})
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.ContentLength, 10))
	_, _ = io.Copy(w, io.LimitReader(obj.Body, identityTransferMediaMaxBytes+1))
}

func (s *Server) handleMeIdentityImportJobCreate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req identityTransferImportJobCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	sourceOrigin, _, err := normalizeTransferOrigin(req.SourceOrigin)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_source_origin"})
		return
	}
	targetOrigin, _, err := normalizeTransferOrigin(s.identityTransferTargetOrigin(req.TargetOrigin))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target_origin"})
		return
	}
	sessionID, err := uuid.Parse(req.SourceSessionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}
	encToken, err := s.encryptServerSecret(req.Token)
	if err != nil {
		writeServerError(w, "encrypt transfer token", err)
		return
	}
	job, err := s.db.CreateIdentityTransferImportJob(r.Context(), repo.IdentityTransferImportJobInsert{
		UserID:               uid,
		SourceOrigin:         sourceOrigin,
		TargetOrigin:         targetOrigin,
		SourceSessionID:      sessionID,
		SourceTokenEncrypted: encToken,
		IncludePrivate:       req.IncludePrivate,
		IncludeGated:         req.IncludeGated,
	})
	if err != nil {
		writeServerError(w, "CreateIdentityTransferImportJob", err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleMeIdentityImportJobGet(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_job_id"})
		return
	}
	job, err := s.db.IdentityTransferImportJobByID(r.Context(), uid, id)
	if errors.Is(err, repo.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeServerError(w, "IdentityTransferImportJobByID", err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleMeIdentityImportJobRetry(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_job_id"})
		return
	}
	if err := s.db.RetryIdentityTransferImportJob(r.Context(), uid, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeIdentityImportJobCancel(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_job_id"})
		return
	}
	if err := s.db.CancelIdentityTransferImportJob(r.Context(), uid, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) startIdentityTransferImportWorker() {
	go s.identityTransferImportWorkerLoop()
}

func (s *Server) identityTransferImportWorkerLoop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		s.processIdentityTransferImportJobs(ctx)
		cancel()
	}
}

func (s *Server) processIdentityTransferImportJobs(ctx context.Context) {
	rows, err := s.db.ClaimIdentityTransferImportJobs(ctx, 3)
	if err != nil {
		log.Printf("identity transfer import claim: %v", err)
		return
	}
	for _, row := range rows {
		s.processIdentityTransferImportJob(ctx, row)
	}
}

func (s *Server) processIdentityTransferImportJob(ctx context.Context, job repo.IdentityTransferImportJob) {
	token, err := s.decryptServerSecret(job.SourceTokenEncrypted)
	if err != nil {
		s.failIdentityTransferImportJob(ctx, job, err)
		return
	}
	manifest, err := s.fetchTransferManifest(ctx, job, token)
	if err != nil {
		s.failIdentityTransferImportJob(ctx, job, err)
		return
	}
	cursor := strings.TrimSpace(job.NextCursor)
	imported := job.ImportedPosts
	for {
		resp, err := s.fetchTransferPosts(ctx, job, token, cursor)
		if err != nil {
			s.failIdentityTransferImportJob(ctx, job, err)
			return
		}
		for _, post := range resp.Posts {
			if err := s.importOneTransferPost(ctx, job, token, post); err != nil {
				s.failIdentityTransferImportJob(ctx, job, err)
				return
			}
			imported++
		}
		cursor = strings.TrimSpace(resp.NextCursor)
		if err := s.db.ProgressIdentityTransferImportJob(ctx, job.ID, int(manifest.TotalPosts), imported, cursor); err != nil {
			log.Printf("identity transfer progress %s: %v", job.ID, err)
		}
		if resp.Done || cursor == "" {
			break
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	if err := s.db.CompleteIdentityTransferImportJob(ctx, job.ID, int(manifest.TotalPosts), imported); err != nil {
		log.Printf("identity transfer complete %s: %v", job.ID, err)
	}
}

func (s *Server) failIdentityTransferImportJob(ctx context.Context, job repo.IdentityTransferImportJob, err error) {
	nextAttempt := job.AttemptCount + 1
	dead := nextAttempt >= repo.IdentityTransferJobMaxAttempts
	nextAt := time.Now().UTC().Add(federationOutboxBackoff(nextAttempt))
	if dead {
		nextAt = time.Now().UTC()
	}
	if e := s.db.FailIdentityTransferImportJob(ctx, job.ID, nextAttempt, err.Error(), nextAt, dead); e != nil {
		log.Printf("identity transfer import fail record %s: %v", job.ID, e)
	}
}

func (s *Server) importOneTransferPost(ctx context.Context, job repo.IdentityTransferImportJob, token string, post repo.TransferPostPayload) error {
	objectKeys := make([]string, 0, len(post.ObjectKeys))
	for _, key := range post.ObjectKeys {
		nextKey, err := s.copyTransferMedia(ctx, job, token, key)
		if err != nil {
			return err
		}
		objectKeys = append(objectKeys, nextKey)
	}
	var replyTo *uuid.UUID
	if strings.TrimSpace(post.ReplyToObjectID) != "" {
		mapped, err := s.db.MigratedPostIDByOriginal(ctx, job.ID, post.ReplyToObjectID)
		if err != nil {
			return err
		}
		replyTo = mapped
	}
	_, _, err := s.db.InsertMigratedPost(ctx, job.UserID, job.ID, post, objectKeys, replyTo)
	return err
}

func (s *Server) copyTransferMedia(ctx context.Context, job repo.IdentityTransferImportJob, token, objectKey string) (string, error) {
	mediaURL, err := transferURL(job.SourceOrigin, "/api/v1/identity/transfers/"+job.SourceSessionID.String()+"/media")
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(mediaURL)
	q := u.Query()
	q.Set("object_key", objectKey)
	u.RawQuery = q.Encode()
	if err := validateTransferRequestURL(u.String()); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Glipz-Transfer-Token", token)
	req.Header.Set("X-Glipz-Target-Origin", s.identityTransferTargetOrigin(job.TargetOrigin))
	res, err := newIdentityTransferHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("source media status %d", res.StatusCode)
	}
	if res.ContentLength > identityTransferMediaMaxBytes {
		return "", fmt.Errorf("source media too large")
	}
	ct := normalizeMediaContentType(res.Header.Get("Content-Type"))
	if !isAllowedUploadMediaContentType(ct) {
		return "", fmt.Errorf("unsupported media content type")
	}
	ext := path.Ext(strings.TrimSpace(objectKey))
	if len(ext) > 8 {
		ext = ""
	}
	nextKey := "imports/" + job.UserID.String() + "/" + uuid.NewString() + strings.ToLower(ext)
	limited := io.LimitReader(res.Body, identityTransferMediaMaxBytes+1)
	if err := s.s3.PutObject(ctx, nextKey, ct, limited, res.ContentLength); err != nil {
		return "", err
	}
	return nextKey, nil
}

func (s *Server) fetchTransferManifest(ctx context.Context, job repo.IdentityTransferImportJob, token string) (repo.IdentityTransferManifest, error) {
	var out repo.IdentityTransferManifest
	err := s.fetchTransferJSON(ctx, job, token, "/api/v1/identity/transfers/"+job.SourceSessionID.String()+"/manifest", &out)
	return out, err
}

func (s *Server) fetchTransferPosts(ctx context.Context, job repo.IdentityTransferImportJob, token, cursor string) (identityTransferPostsResponse, error) {
	p := "/api/v1/identity/transfers/" + job.SourceSessionID.String() + "/posts?limit=" + strconv.Itoa(identityTransferPostBatchLimit)
	if strings.TrimSpace(cursor) != "" {
		p += "&cursor=" + url.QueryEscape(cursor)
	}
	var out identityTransferPostsResponse
	err := s.fetchTransferJSON(ctx, job, token, p, &out)
	return out, err
}

func (s *Server) fetchTransferJSON(ctx context.Context, job repo.IdentityTransferImportJob, token, pathAndQuery string, out any) error {
	u, err := url.Parse(strings.TrimRight(job.SourceOrigin, "/") + pathAndQuery)
	if err != nil {
		return err
	}
	if err := validateTransferRequestURL(u.String()); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Glipz-Transfer-Token", token)
	req.Header.Set("X-Glipz-Target-Origin", s.identityTransferTargetOrigin(job.TargetOrigin))
	res, err := newIdentityTransferHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("source status %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out)
}
