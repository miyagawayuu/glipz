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
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"glipz.io/backend/internal/authjwt"
	"glipz.io/backend/internal/config"
	"glipz.io/backend/internal/repo"
	"glipz.io/backend/internal/s3client"
)

type Server struct {
	cfg    config.Config
	db     *repo.Pool
	rdb    *redis.Client
	s3     *s3client.Client
	secret []byte
}

func New(cfg config.Config, pool *pgxpool.Pool, rdb *redis.Client, s3c *s3client.Client) http.Handler {
	s := &Server{
		cfg:    cfg,
		db:     repo.New(pool),
		rdb:    rdb,
		s3:     s3c,
		secret: []byte(cfg.JWTSecret),
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	allowedOrigins := []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	for _, capOrigin := range []string{
		"capacitor://localhost",
		"http://localhost",
		"https://localhost",
	} {
		dup := false
		for _, o := range allowedOrigins {
			if o == capOrigin {
				dup = true
				break
			}
		}
		if !dup {
			allowedOrigins = append(allowedOrigins, capOrigin)
		}
	}
	for _, fo := range cfg.FrontendOrigins {
		fo = strings.TrimSpace(fo)
		if fo == "" {
			continue
		}
		dup := false
		for _, o := range allowedOrigins {
			if o == fo {
				dup = true
				break
			}
		}
		if !dup {
			allowedOrigins = append(allowedOrigins, fo)
		}
	}
	if strings.TrimSpace(cfg.StaticWebRoot) != "" {
		for _, o := range []string{
			"http://127.0.0.1:" + strings.TrimSpace(cfg.Port),
			"http://localhost:" + strings.TrimSpace(cfg.Port),
		} {
			dup := false
			for _, x := range allowedOrigins {
				if x == o {
					dup = true
					break
				}
			}
			if !dup {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	s.mountGlipzFederation(r)
	s.startFederationDeliveryWorker()

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", s.handleRegister)
		r.Get("/auth/handle-availability", s.handleRegisterHandleAvailability)
		r.Post("/auth/register/verify", s.handleRegisterVerify)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/mfa/verify", s.handleMFAVerify)
		r.Post("/oauth/token", s.handleOAuthToken)
		r.Get("/patreon/oauth/callback", s.handlePatreonOAuthCallback)
		r.Get("/custom-emojis", s.handleListEnabledCustomEmojis)
		r.Get("/public/posts/feed", s.handlePublicFeed)
		r.Get("/public/posts/feed/stream", s.handlePublicFeedStream)
		r.Get("/public/federation/profile", s.handlePublicRemoteProfile)
		r.Get("/public/federation/posts", s.handlePublicRemoteActorPosts)
		r.Get("/public/federation/incoming", s.handlePublicFederatedIncomingByActor)
		r.Get("/public/federation/incoming/stream", s.handlePublicFederatedIncomingStream)
		r.Get("/public/federation/incoming/{id}", s.handlePublicFederatedIncomingPost)
		r.Get("/public/federation/incoming/{id}/thread", s.handlePublicFederatedIncomingThread)
		r.Get("/public/federation/custom-emoji", s.handlePublicFederationCustomEmojiResolve)
		r.Get("/media/object/*", s.handlePublicMediaObject)
		r.Head("/media/object/*", s.handlePublicMediaObject)

		// Optional login: attach the viewer context when a valid token is present, otherwise keep the request anonymous.
		r.With(s.optionalAccessMiddleware).Group(func(r chi.Router) {
			r.Get("/link-preview", s.handleGetLinkPreview)
			r.Get("/users/by-handle/{handle}", s.handlePublicProfileByHandle)
			r.Get("/users/by-handle/{handle}/followers", s.handleUserFollowersByHandle)
			r.Get("/users/by-handle/{handle}/following", s.handleUserFollowingByHandle)
			r.Get("/users/by-handle/{handle}/posts", s.handleUserPostsByHandle)
			r.Get("/users/by-handle/{handle}/replies", s.handleUserRepliesByHandle)
			r.Get("/users/by-handle/{handle}/notes", s.handleUserNotesListByHandle)
			r.Get("/users/by-handle/{handle}/post-media-tiles", s.handleUserPostMediaTilesByHandle)
			r.Get("/posts/{postID}/feed-item", s.handlePostFeedItemGET)
			r.Get("/posts/{postID}/thread", s.handlePostThreadGET)
		})

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware(authjwt.PurposeAccess))
			r.Use(s.requireSiteAdmin)
			r.Get("/admin/federation/deliveries", s.handleAdminFederationDeliveries)
			r.Get("/admin/federation/delivery-counts", s.handleAdminFederationDeliveryCounts)
			r.Get("/admin/federation/domain-blocks", s.handleAdminFederationDomainBlocksList)
			r.Post("/admin/federation/domain-blocks", s.handleAdminFederationDomainBlocksAdd)
			r.Delete("/admin/federation/domain-blocks", s.handleAdminFederationDomainBlocksRemove)
			r.Get("/admin/federation/known-instances", s.handleAdminFederationKnownInstancesList)
			r.Post("/admin/federation/known-instances", s.handleAdminFederationKnownInstancesAdd)
			r.Delete("/admin/federation/known-instances", s.handleAdminFederationKnownInstancesRemove)
			r.Get("/admin/reports/posts", s.handleAdminPostReports)
			r.Get("/admin/reports/federated-posts", s.handleAdminFederatedPostReports)
			r.Patch("/admin/reports/posts/{reportID}", s.handleAdminUpdatePostReportStatus)
			r.Patch("/admin/reports/federated-posts/{reportID}", s.handleAdminUpdateFederatedPostReportStatus)
			r.Get("/admin/users/by-handle/{handle}/badges", s.handleAdminGetUserBadges)
			r.Put("/admin/users/by-handle/{handle}/badges", s.handleAdminPutUserBadges)
			r.Get("/admin/custom-emojis/site", s.handleAdminListSiteCustomEmojis)
			r.Post("/admin/custom-emojis/site", s.handleAdminCreateSiteCustomEmoji)
			r.Patch("/admin/custom-emojis/site/{emojiID}", s.handleAdminPatchSiteCustomEmoji)
			r.Delete("/admin/custom-emojis/site/{emojiID}", s.handleAdminDeleteSiteCustomEmoji)
			r.Post("/admin/posts/{postID}/suspend-author", s.handleAdminSuspendPostAuthor)
		})
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware(authjwt.PurposeAccess))
			r.Get("/me", s.handleMe)
			r.Get("/me/custom-emojis", s.handleMeListCustomEmojis)
			r.Post("/me/custom-emojis", s.handleMeCreateCustomEmoji)
			r.Patch("/me/custom-emojis/{emojiID}", s.handleMePatchCustomEmoji)
			r.Delete("/me/custom-emojis/{emojiID}", s.handleMeDeleteCustomEmoji)
			r.Get("/notifications", s.handleListNotifications)
			r.Get("/notifications/unread-count", s.handleNotificationUnreadCount)
			r.Post("/notifications/read-all", s.handleMarkAllNotificationsRead)
			r.Get("/notifications/stream", s.handleNotifyStream)
			r.Get("/dm/identity", s.handleGetDMIdentity)
			r.Put("/dm/identity", s.handlePutDMIdentity)
			r.Get("/dm/threads", s.handleListDMThreads)
			r.Post("/dm/threads", s.handleCreateDMThread)
			r.Post("/dm/invite-peer", s.handleInviteDMPeer)
			r.Get("/dm/unread-count", s.handleDMUnreadCount)
			r.Get("/dm/stream", s.handleDMStream)
			r.Post("/dm/upload", s.handleDMUpload)
			r.Get("/dm/threads/{threadID}", s.handleGetDMThread)
			r.Get("/dm/threads/{threadID}/messages", s.handleListDMMessages)
			r.Get("/dm/threads/{threadID}/call-history", s.handleListDMCallHistory)
			r.Post("/dm/threads/{threadID}/messages", s.handleCreateDMMessage)
			r.Post("/dm/threads/{threadID}/read", s.handleMarkDMThreadRead)
			r.Post("/dm/threads/{threadID}/call-token", s.handleIssueDMCallToken)
			r.Post("/dm/threads/{threadID}/call-invite", s.handleInviteDMCall)
			r.Post("/dm/threads/{threadID}/call-cancel", s.handleCancelDMCall)
			r.Post("/dm/threads/{threadID}/call-end", s.handleEndDMCall)
			r.Post("/dm/threads/{threadID}/call-missed", s.handleMissedDMCall)
			r.Post("/auth/mfa/setup", s.handleMFASetup)
			r.Post("/auth/mfa/enable", s.handleMFAEnable)
			r.Post("/media/presign", s.handlePresign)
			r.Post("/media/upload", s.handleMediaUpload)
			r.Post("/me/oauth-clients", s.handleOAuthClientCreate)
			r.Get("/me/oauth-clients", s.handleOAuthClientList)
			r.Delete("/me/oauth-clients/{clientID}", s.handleOAuthClientDelete)
			r.Post("/me/oauth-authorize", s.handleOAuthAuthorizeConsent)
			r.Post("/me/personal-access-tokens", s.handlePersonalAccessTokenCreate)
			r.Get("/me/personal-access-tokens", s.handlePersonalAccessTokenList)
			r.Delete("/me/personal-access-tokens/{tokenID}", s.handlePersonalAccessTokenDelete)
			r.Patch("/me/dm-settings", s.handlePatchMeDMSettings)
			r.Get("/me/web-push", s.handleGetMeWebPush)
			r.Put("/me/web-push/subscription", s.handlePutMeWebPushSubscription)
			r.Post("/me/web-push/unsubscribe", s.handleDeleteMeWebPushSubscription)
			r.Get("/me/scheduled-posts", s.handleListScheduledPosts)
			r.Get("/posts/feed", s.handleFeed)
			r.Get("/posts/bookmarks", s.handleBookmarks)
			r.Get("/search", s.handleSearch)
			r.Post("/federation/remote-follow", s.handleRemoteFollowPOST)
			r.Get("/federation/remote-follow", s.handleRemoteFollowGET)
			r.Delete("/federation/remote-follow", s.handleRemoteFollowDELETE)
			r.Post("/federation/dm/invite", s.handleFederationDMInviteOutbound)
			r.Post("/federation/dm/accept", s.handleFederationDMAcceptOutbound)
			r.Post("/federation/dm/reject", s.handleFederationDMRejectOutbound)
			r.Post("/federation/dm/message", s.handleFederationDMMessageOutbound)
			r.Get("/federation/dm/threads", s.handleFederationDMThreadsList)
			r.Get("/federation/dm/threads/{threadID}/messages", s.handleFederationDMMessagesList)
			r.Get("/federation/dm/keys", s.handleFederationDMPeerKeysGet)
			r.Get("/federation/dm/attachment", s.handleFederationDMAttachmentProxy)
			r.Get("/federation/posts/{incomingID}/feed-item", s.handleFederatedIncomingFeedItemGET)
			r.Get("/federation/posts/{incomingID}/thread", s.handleFederatedIncomingThreadGET)
			r.Get("/posts/feed/stream", s.handleFeedStream)
			r.Post("/posts/{postID}/unlock", s.handlePostUnlock)
			r.Post("/federation/posts/{incomingID}/unlock", s.handleFederatedPostUnlock)
			r.Post("/posts/{postID}/reactions", s.handleAddPostReaction)
			r.Delete("/posts/{postID}/reactions/{emoji}", s.handleDeletePostReaction)
			r.Post("/posts/{postID}/poll/vote", s.handlePollVote)
			r.Post("/posts/{postID}/like", s.handleToggleLike)
			r.Post("/posts/{postID}/bookmark", s.handleToggleBookmark)
			r.Post("/posts/{postID}/report", s.handleCreatePostReport)
			r.Post("/federation/posts/{incomingID}/poll/vote", s.handleFederatedPollVote)
			r.Post("/federation/posts/{incomingID}/like", s.handleFederatedToggleLike)
			r.Post("/federation/posts/{incomingID}/bookmark", s.handleFederatedToggleBookmark)
			r.Post("/federation/posts/{incomingID}/report", s.handleCreateFederatedIncomingPostReport)
			r.Post("/federation/posts/{incomingID}/repost", s.handleFederatedToggleRepost)
			r.Post("/posts/{postID}/repost", s.handleToggleRepost)
			r.Patch("/posts/{postID}", s.handlePatchPost)
			r.Delete("/posts/{postID}", s.handleDeletePost)
			r.Post("/posts", s.handleCreatePost)
			r.Post("/users/by-handle/{handle}/follow", s.handleToggleFollow)
			r.Patch("/me/profile", s.handlePatchMeProfile)
			r.Patch("/me/patreon-note-paywall", s.handlePatchMePatreonNotePaywall)
			r.Get("/patreon/member/authorize-url", s.handlePatreonMemberAuthorizeURL)
			r.Get("/patreon/creator/authorize-url", s.handlePatreonCreatorAuthorizeURL)
			r.Get("/patreon/creator/campaigns", s.handlePatreonCreatorCampaigns)
			r.Get("/patreon/creator/tiers", s.handlePatreonCreatorTiers)
			r.Post("/patreon/member/disconnect", s.handlePatreonMemberDisconnect)
			r.Post("/patreon/creator/disconnect", s.handlePatreonCreatorDisconnect)
			r.Post("/notes", s.handleNoteCreate)
			r.Get("/notes/{noteID}", s.handleNoteGet)
			r.Patch("/notes/{noteID}", s.handleNotePatch)
			r.Delete("/notes/{noteID}", s.handleNoteDelete)
		})
	})
	s.mountStaticSPAFallback(r)
	go s.runFeedBroadcastScheduler(context.Background())
	return r
}

// principalForAccess resolves a user ID from either an access JWT or a personal access token.
func (s *Server) principalForAccess(ctx context.Context, raw string) (uuid.UUID, bool) {
	claims, err := authjwt.Parse(s.secret, raw)
	if err == nil && claims.Purpose == authjwt.PurposeAccess {
		u, e := uuid.Parse(claims.Subject)
		return u, e == nil
	}
	u, err := s.db.UserIDFromPersonalAccessToken(ctx, raw)
	if err == nil {
		return u, true
	}
	return uuid.Nil, false
}

func (s *Server) authMiddleware(requiredPurpose string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			if requiredPurpose == authjwt.PurposeAccess {
				if uid, ok := s.principalForAccess(r.Context(), raw); ok {
					suspended, err := s.db.IsUserSuspended(r.Context(), uid)
					if err != nil {
						writeServerError(w, "auth IsUserSuspended", err)
						return
					}
					if suspended {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_suspended"})
						return
					}
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserID{}, uid)))
					return
				}
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			claims, err := authjwt.Parse(s.secret, raw)
			if err != nil || claims.Purpose != requiredPurpose {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			uid, err := uuid.Parse(claims.Subject)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID{}, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// optionalAccessMiddleware attaches the viewer user ID only when a valid access token is present.
func (s *Server) optionalAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if uid, ok := s.principalForAccess(r.Context(), raw); ok {
			suspended, err := s.db.IsUserSuspended(r.Context(), uid)
			if err != nil {
				log.Printf("optional access IsUserSuspended: %v", err)
				next.ServeHTTP(w, r)
				return
			}
			if suspended {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserID{}, uid)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ctxUserID struct{}

func userIDFrom(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(ctxUserID{}).(uuid.UUID)
	return v, ok
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func isPGUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "duplicate key")
}

func writeServerError(w http.ResponseWriter, where string, err error) {
	if err != nil {
		log.Printf("%s: %v", where, err)
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server"})
}

type registerReq struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"password_confirm"`
	Handle          string `json:"handle"`
	BirthDate       string `json:"birth_date"`
	TermsAgreed     bool   `json:"terms_agreed"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(req.Email, "@") || len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email_or_password"})
		return
	}
	if !req.TermsAgreed {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "terms_not_agreed"})
		return
	}
	if req.Password != req.PasswordConfirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_mismatch"})
		return
	}
	birthDate, err := time.Parse("2006-01-02", strings.TrimSpace(req.BirthDate))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_birth_date"})
		return
	}
	now := time.Now().UTC()
	if birthDate.After(now) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_birth_date"})
		return
	}
	if birthDate.AddDate(13, 0, 0).After(now) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "under_age"})
		return
	}
	handleNorm, err := repo.NormalizeHandle(req.Handle)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	if repo.IsReservedHandle(handleNorm) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reserved_handle"})
		return
	}
	available, err := s.db.IsHandleAvailable(r.Context(), handleNorm)
	if err != nil {
		writeServerError(w, "register IsHandleAvailable", err)
		return
	}
	if !available {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "handle_taken"})
		return
	}
	if _, err := s.db.UserByEmail(r.Context(), req.Email); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email_taken"})
		return
	} else if !errors.Is(err, repo.ErrNotFound) {
		writeServerError(w, "register UserByEmail", err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeServerError(w, "register bcrypt", err)
		return
	}
	rawToken, tokenSHA256, err := newRegistrationToken()
	if err != nil {
		writeServerError(w, "register newRegistrationToken", err)
		return
	}
	expiresAt := time.Now().UTC().Add(s.cfg.RegistrationVerifyTTL)
	if err := s.db.UpsertPendingUserRegistration(r.Context(), req.Email, string(hash), handleNorm, birthDate, tokenSHA256, expiresAt); err != nil {
		writeServerError(w, "register UpsertPendingUserRegistration", err)
		return
	}
	if err := s.sendRegistrationVerificationEmail(req.Email, s.registrationVerificationLink(rawToken), expiresAt); err != nil {
		writeServerError(w, "register sendRegistrationVerificationEmail", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":     "pending_verification",
		"expires_at": expiresAt.Format(time.RFC3339),
		"email":      req.Email,
		"handle":     handleNorm,
	})
}

func (s *Server) handleRegisterHandleAvailability(w http.ResponseWriter, r *http.Request) {
	handleRaw := strings.TrimSpace(r.URL.Query().Get("handle"))
	handleNorm, err := repo.NormalizeHandle(handleRaw)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"handle":    strings.TrimPrefix(strings.ToLower(strings.TrimSpace(handleRaw)), "@"),
			"available": false,
			"reason":    "invalid_handle",
		})
		return
	}
	if repo.IsReservedHandle(handleNorm) {
		writeJSON(w, http.StatusOK, map[string]any{
			"handle":    handleNorm,
			"available": false,
			"reason":    "reserved_handle",
		})
		return
	}
	available, err := s.db.IsHandleAvailable(r.Context(), handleNorm)
	if err != nil {
		writeServerError(w, "handle availability", err)
		return
	}
	reason := ""
	if !available {
		reason = "handle_taken"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"handle":    handleNorm,
		"available": available,
		"reason":    reason,
	})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	u, err := s.db.UserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
			return
		}
		writeServerError(w, "login UserByEmail", err)
		return
	}
	if u.SuspendedAt != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_suspended"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if u.TOTPEnabled {
		if u.TOTPSecret == nil || *u.TOTPSecret == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mfa_misconfigured"})
			return
		}
		tok, err := authjwt.SignMFA(s.secret, u.ID, 5*time.Minute)
		if err != nil {
			writeServerError(w, "login SignMFA", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": tok, "token_type": "Bearer"})
		return
	}
	tok, err := authjwt.SignAccess(s.secret, u.ID, 24*time.Hour)
	if err != nil {
		writeServerError(w, "login SignAccess", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mfa_required": false, "access_token": tok, "token_type": "Bearer"})
}

type mfaVerifyReq struct {
	Code string `json:"code"`
}

func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing_mfa_token"})
		return
	}
	raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	claims, err := authjwt.Parse(s.secret, raw)
	if err != nil || claims.Purpose != authjwt.PurposeMFA {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_mfa_token"})
		return
	}
	var req mfaVerifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_mfa_token"})
		return
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_mfa_token"})
		return
	}
	if u.SuspendedAt != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_suspended"})
		return
	}
	if u.TOTPSecret == nil || !totp.Validate(req.Code, *u.TOTPSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_code"})
		return
	}
	tok, err := authjwt.SignAccess(s.secret, uid, 24*time.Hour)
	if err != nil {
		writeServerError(w, "mfa verify SignAccess", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": tok, "token_type": "Bearer"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	out := map[string]any{
		"id":                       u.ID.String(),
		"email":                    u.Email,
		"handle":                   u.Handle,
		"display_name":             resolvedDisplayName(u.DisplayName, u.Email),
		"badges":                   userBadgesJSON(s.visibleUserBadges(u.ID, u.Badges)),
		"bio":                      u.Bio,
		"totp_enabled":             u.TOTPEnabled,
		"dm_call_timeout_seconds":  u.DMCallTimeoutSeconds,
		"dm_call_enabled":          u.DMCallEnabled,
		"dm_call_scope":            u.DMCallScope,
		"dm_call_allowed_user_ids": u.DMCallAllowedUserIDs,
		"dm_invite_auto_accept":    u.DMInviteAutoAccept,
	}
	if u.AvatarObjectKey != nil && *u.AvatarObjectKey != "" {
		out["avatar_url"] = s.glipzProtocolPublicMediaURL(*u.AvatarObjectKey)
	} else {
		out["avatar_url"] = nil
	}
	if u.HeaderObjectKey != nil && *u.HeaderObjectKey != "" {
		out["header_url"] = s.glipzProtocolPublicMediaURL(*u.HeaderObjectKey)
	} else {
		out["header_url"] = nil
	}
	out["patreon"] = mePatreonJSON(u)
	out["is_site_admin"] = s.isSiteAdmin(uid)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	u, err := s.db.UserByID(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if u.TOTPEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "already_enabled"})
		return
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Glipz",
		AccountName: u.Email,
	})
	if err != nil {
		writeServerError(w, "mfa setup totp.Generate", err)
		return
	}
	pipe := s.rdb.Set(r.Context(), "totp_pending:"+uid.String(), key.Secret(), 10*time.Minute)
	if err := pipe.Err(); err != nil {
		writeServerError(w, "mfa setup redis Set", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":  key.Secret(),
		"uri":     key.URL(),
		"issuer":  "Glipz",
		"account": u.Email,
	})
}

type mfaEnableReq struct {
	Code string `json:"code"`
}

func (s *Server) handleMFAEnable(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req mfaEnableReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	sec, err := s.rdb.Get(r.Context(), "totp_pending:"+uid.String()).Result()
	if err == redis.Nil || sec == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "setup_required"})
		return
	} else if err != nil {
		writeServerError(w, "mfa enable redis Get", err)
		return
	}
	if !totp.Validate(req.Code, sec) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_code"})
		return
	}
	if err := s.db.SetTOTPSecret(r.Context(), uid, sec, true); err != nil {
		writeServerError(w, "mfa enable SetTOTPSecret", err)
		return
	}
	_ = s.rdb.Del(r.Context(), "totp_pending:"+uid.String()).Err()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type presignReq struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

func (s *Server) handlePresign(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req presignReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Filename = strings.TrimSpace(req.Filename)
	req.ContentType = strings.TrimSpace(strings.ToLower(req.ContentType))
	if req.Filename == "" || req.ContentType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_params"})
		return
	}
	if !strings.HasPrefix(req.ContentType, "image/") && !strings.HasPrefix(req.ContentType, "video/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_type"})
		return
	}
	ext := ""
	if i := strings.LastIndex(req.Filename, "."); i >= 0 {
		ext = strings.ToLower(req.Filename[i:])
		if len(ext) > 8 {
			ext = ""
		}
	}
	objectKey := "uploads/" + uid.String() + "/" + uuid.NewString() + ext
	url, err := s.s3.PresignPut(r.Context(), objectKey, req.ContentType, 15*time.Minute)
	if err != nil {
		writeServerError(w, "presign", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"method":     "PUT",
		"upload_url": url,
		"object_key": objectKey,
		"public_url": s.glipzProtocolPublicMediaURL(objectKey),
		"headers": map[string]string{
			"Content-Type": req.ContentType,
		},
	})
}

const mediaUploadMaxBytes = 50 << 20 // 50 MiB leaves room for a single video or multiple images.

// handleMediaUpload stores multipart file uploads through the backend.
// It exists for clients where direct presigned PUT uploads fail because of localhost or mixed-content constraints.
func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := r.ParseMultipartForm(mediaUploadMaxBytes + (2 << 20)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_multipart"})
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_file"})
		return
	}
	defer file.Close()

	ct := strings.TrimSpace(strings.ToLower(hdr.Header.Get("Content-Type")))
	if ct == "" {
		ct = "application/octet-stream"
	}
	if !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "video/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_type"})
		return
	}
	filename := strings.TrimSpace(hdr.Filename)
	ext := ""
	if i := strings.LastIndex(filename, "."); i >= 0 {
		ext = strings.ToLower(filename[i:])
		if len(ext) > 8 {
			ext = ""
		}
	}
	objectKey := "uploads/" + uid.String() + "/" + uuid.NewString() + ext

	tmp, err := os.CreateTemp("", "glipz-media-*")
	if err != nil {
		writeServerError(w, "upload tempfile", err)
		return
	}
	tmpPath := tmp.Name()
	closed := false
	closeTmp := func() {
		if !closed {
			_ = tmp.Close()
			closed = true
		}
		_ = os.Remove(tmpPath)
	}
	defer closeTmp()

	n, err := io.Copy(tmp, io.LimitReader(file, mediaUploadMaxBytes+1))
	if err != nil {
		writeServerError(w, "upload read", err)
		return
	}
	if n > mediaUploadMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file_too_large"})
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_file"})
		return
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		writeServerError(w, "upload seek", err)
		return
	}

	if err := s.s3.PutObject(r.Context(), objectKey, ct, tmp, n); err != nil {
		writeServerError(w, "s3 put", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object_key": objectKey,
		"public_url": s.glipzProtocolPublicMediaURL(objectKey),
	})
}

type createPostPollReq struct {
	Options []string `json:"options"`
	EndsAt  string   `json:"ends_at"`
}

type createPostReq struct {
	Caption                string                      `json:"caption"`
	MediaType              string                      `json:"media_type"`
	ObjectKey              string                      `json:"object_key"`
	ObjectKeys             []string                    `json:"object_keys"`
	ReplyToPostID          string                      `json:"reply_to_post_id"`
	ReplyToIncomingID      string                      `json:"reply_to_incoming_id"`
	ReplyToObjectURL       string                      `json:"reply_to_object_url"`
	IsNSFW                 bool                        `json:"is_nsfw"`
	Visibility             string                      `json:"visibility"`
	ViewPassword           string                      `json:"view_password"`
	ViewPasswordScope      int                         `json:"view_password_scope"`
	ViewPasswordTextRanges []viewPasswordTextRangeJSON `json:"view_password_text_ranges"`
	VisibleAt              string                      `json:"visible_at"`
	Poll                   *createPostPollReq          `json:"poll"`
}

type patchPostReq struct {
	Caption                string                      `json:"caption"`
	IsNSFW                 bool                        `json:"is_nsfw"`
	Visibility             string                      `json:"visibility"`
	ClearViewPassword      bool                        `json:"clear_view_password"`
	ViewPassword           *string                     `json:"view_password"`
	ViewPasswordScope      int                         `json:"view_password_scope"`
	ViewPasswordTextRanges []viewPasswordTextRangeJSON `json:"view_password_text_ranges"`
}

func normalizeObjectKeys(req createPostReq) []string {
	var keys []string
	for _, k := range req.ObjectKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		if k := strings.TrimSpace(req.ObjectKey); k != "" {
			keys = []string{k}
		}
	}
	// pgx sends nil slices as SQL NULL, so keep empty arrays non-nil for TEXT[] NOT NULL columns.
	if keys == nil {
		return []string{}
	}
	return keys
}

func normalizePollLabels(raw []string) []string {
	var out []string
	for _, x := range raw {
		t := strings.TrimSpace(x)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req createPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	keys := normalizeObjectKeys(req)
	mt := strings.TrimSpace(strings.ToLower(req.MediaType))
	switch {
	case len(keys) == 0:
		mt = "none"
	case mt == "" || mt == "image" || mt == "video":
		if mt == "" {
			mt = "image"
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_media_type"})
		return
	}
	if mt != "none" && mt != "image" && mt != "video" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_media_type"})
		return
	}
	if mt == "video" && len(keys) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "video_single_media"})
		return
	}
	if len(keys) > 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_object_keys"})
		return
	}
	var pollLabels []string
	var pollEnds time.Time
	if req.Poll != nil {
		pollLabels = normalizePollLabels(req.Poll.Options)
		if len(pollLabels) < 2 || len(pollLabels) > 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_poll_options"})
			return
		}
		es := strings.TrimSpace(req.Poll.EndsAt)
		if es == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_poll_ends_at"})
			return
		}
		t, err := time.Parse(time.RFC3339, es)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_poll_ends_at"})
			return
		}
		pollEnds = t.UTC()
	}

	now := time.Now().UTC()
	visibleAt := now
	if strings.TrimSpace(req.VisibleAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.VisibleAt))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_visible_at"})
			return
		}
		visibleAt = t.UTC()
		if visibleAt.Before(now.Add(-2 * time.Minute)) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "visible_at_past"})
			return
		}
		if visibleAt.After(now.Add(7 * 24 * time.Hour)) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "visible_at_too_far"})
			return
		}
		if visibleAt.Before(now) {
			visibleAt = now
		}
	}

	if len(pollLabels) >= 2 && !pollEnds.After(visibleAt) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "poll_ends_before_visible"})
		return
	}

	cap := strings.TrimSpace(req.Caption)
	hasMedia := len(keys) > 0
	hasPoll := len(pollLabels) >= 2
	if cap == "" && !hasMedia && !hasPoll {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_post"})
		return
	}

	prefix := "uploads/" + uid.String() + "/"
	for _, k := range keys {
		if !strings.HasPrefix(k, prefix) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_object_key"})
			return
		}
	}
	var replyTo *uuid.UUID
	if replyToRaw := strings.TrimSpace(req.ReplyToPostID); replyToRaw != "" {
		rid, err := uuid.Parse(replyToRaw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_reply_to"})
			return
		}
		replyTo = &rid
		readable, err := s.db.CanViewerReadPost(r.Context(), uid, rid)
		if err != nil {
			writeServerError(w, "CanViewerReadPost", err)
			return
		}
		if !readable {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "reply_target_not_found"})
			return
		}
	}
	var federatedReplyTarget *repo.FederatedIncomingPost
	replyToRemoteObjectIRI := ""
	if strings.TrimSpace(req.ReplyToIncomingID) != "" {
		if replyTo != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_reply_to"})
			return
		}
		incomingID, err := uuid.Parse(strings.TrimSpace(req.ReplyToIncomingID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_reply_to"})
			return
		}
		row, err := s.loadFederatedIncomingForAction(r.Context(), incomingID, req.ReplyToObjectURL)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "reply_target_not_found"})
				return
			}
			writeServerError(w, "loadFederatedIncomingForAction", err)
			return
		}
		federatedReplyTarget = &row
		replyToRemoteObjectIRI = strings.TrimSpace(row.ObjectIRI)
		if visibleAt.After(now) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scheduled_federated_reply_unsupported"})
			return
		}
	}
	pw := strings.TrimSpace(req.ViewPassword)
	var viewHash *string
	viewScope := repo.ViewPasswordScopeNone
	var viewRanges []repo.ViewPasswordTextRange
	if pw != "" {
		if len(pw) < 4 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
			return
		}
		if len(pw) > 72 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
			return
		}
		h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			writeServerError(w, "bcrypt view password", err)
			return
		}
		hs := string(h)
		viewHash = &hs
		viewScope, viewRanges, err = repo.NormalizeViewPasswordProtection(req.Caption, req.ViewPasswordScope, jsonRangesToRepo(req.ViewPasswordTextRanges))
		if err != nil {
			switch {
			case errors.Is(err, repo.ErrInvalidViewPasswordScope):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_view_password_scope"})
			case errors.Is(err, repo.ErrInvalidViewPasswordTextRanges):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_view_password_text_ranges"})
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_password"})
			}
			return
		}
	}

	var pollIn *repo.PollCreateInput
	if hasPoll {
		pollIn = &repo.PollCreateInput{EndsAt: pollEnds, Labels: pollLabels}
	}
	visibility := strings.TrimSpace(strings.ToLower(req.Visibility))
	if visibility == "" {
		visibility = repo.PostVisibilityPublic
	}

	id, err := s.db.CreatePost(r.Context(), uid, req.Caption, mt, keys, replyTo, replyToRemoteObjectIRI, req.IsNSFW, visibility, viewHash, viewScope, viewRanges, visibleAt, pollIn)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "reply_target_not_found"})
			return
		}
		writeServerError(w, "CreatePost", err)
		return
	}
	if replyTo != nil || federatedReplyTarget != nil {
		if replyTo != nil {
			owner, errOwner := s.db.PostAuthorID(r.Context(), *replyTo)
			if errOwner == nil && owner != uid {
				nid, errN := s.db.InsertNotification(r.Context(), owner, uid, "reply", replyTo, &id)
				if errN == nil {
					s.publishNotifyUserEvent(r.Context(), owner, nid)
				}
			}
		}
		if visibility == repo.PostVisibilityPublic && !visibleAt.After(time.Now().UTC()) {
			aid, pid := uid, id
			if federatedReplyTarget != nil {
				target := *federatedReplyTarget
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
					defer cancel()
					inboxURL, err := s.resolveIncomingPostEventsInbox(ctx, target)
					if err != nil {
						log.Printf("federated reply resolve inbox: %v", err)
						return
					}
					row, err := s.db.GetFederationPublicPostForDelivery(ctx, aid, pid)
					if err != nil {
						log.Printf("federated reply get post: %v", err)
						return
					}
					author, err := s.federationAuthorPayload(ctx, aid)
					if err != nil {
						log.Printf("federated reply author payload: %v", err)
						return
					}
					post := s.federationEventPostPayload(row)
					post.ReplyToObjectURL = strings.TrimSpace(target.ObjectIRI)
					if err := s.queueDirectedFederationEvent(ctx, aid, pid, inboxURL, federationEventEnvelope{
						V:      1,
						Kind:   "post_created",
						Author: author,
						Post:   &post,
					}); err != nil {
						log.Printf("federated reply enqueue: %v", err)
					}
				}()
			} else {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
					defer cancel()
					s.deliverFederationCreate(ctx, aid, pid)
				}()
			}
		}
	} else {
		authorID, published, errPub := s.db.TryPublishRootPost(r.Context(), id)
		if errPub != nil {
			log.Printf("TryPublishRootPost: %v", errPub)
		} else if published {
			b, _ := json.Marshal(map[string]any{
				"v":         1,
				"kind":      "post_created",
				"post_id":   id.String(),
				"author_id": authorID.String(),
			})
			s.publishFeedEventJSON(r.Context(), b, authorID, visibility)
			if visibility == repo.PostVisibilityPublic {
				aid, pid := authorID, id
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
					defer cancel()
					s.deliverFederationCreate(ctx, aid, pid)
				}()
			}
		}
	}
	urls := make([]string, 0, len(keys))
	for _, k := range keys {
		urls = append(urls, s.glipzProtocolPublicMediaURL(k))
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":                        id.String(),
		"media_urls":                urls,
		"object_keys":               keys,
		"media_type":                mt,
		"visible_at":                visibleAt.Format(time.RFC3339),
		"is_nsfw":                   req.IsNSFW,
		"visibility":                visibility,
		"has_view_password":         viewHash != nil,
		"view_password_scope":       viewScope,
		"view_password_text_ranges": repoRangesToJSON(viewRanges),
		"content_locked":            false,
		"text_locked":               false,
		"media_locked":              false,
	})
}

func postUnlockRedisKey(viewerID, postID uuid.UUID) string {
	return "postunlock:v1:" + viewerID.String() + ":" + postID.String()
}

// postMediaPreviewURL returns the first media URL for profile grids.
// Protected media is shown only to the author or after an unlock.
func (s *Server) postMediaPreviewURL(ctx context.Context, viewer, author, postID uuid.UUID, hasPW bool, scope int, objectKey string) string {
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return ""
	}
	if !hasPW || author == viewer || !scopeProtectsMedia(scope) {
		return s.glipzProtocolPublicMediaURL(key)
	}
	rk := postUnlockRedisKey(viewer, postID)
	n, err := s.rdb.Exists(ctx, rk).Result()
	if err != nil {
		log.Printf("redis Exists media tile %s: %v", postID, err)
		return ""
	}
	if n == 0 {
		return ""
	}
	return s.glipzProtocolPublicMediaURL(key)
}

const postUnlockRedisTTL = 30 * 24 * time.Hour

type feedPollOptionJSON struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Votes int64  `json:"votes"`
}

type feedPollJSON struct {
	EndsAt     string               `json:"ends_at"`
	Closed     bool                 `json:"closed"`
	Options    []feedPollOptionJSON `json:"options"`
	MyOptionID string               `json:"my_option_id,omitempty"`
	TotalVotes int64                `json:"total_votes"`
}

type feedReactionJSON struct {
	Emoji       string `json:"emoji"`
	Count       int64  `json:"count"`
	ReactedByMe bool   `json:"reacted_by_me"`
}

type feedItem struct {
	ID                     string                      `json:"id"`
	UserEmail              string                      `json:"user_email"`
	UserHandle             string                      `json:"user_handle"`
	UserDisplayName        string                      `json:"user_display_name"`
	UserBadges             []string                    `json:"user_badges,omitempty"`
	UserAvatarURL          string                      `json:"user_avatar_url"`
	Caption                string                      `json:"caption"`
	MediaType              string                      `json:"media_type"`
	MediaURLs              []string                    `json:"media_urls"`
	IsNSFW                 bool                        `json:"is_nsfw"`
	Visibility             string                      `json:"visibility"`
	HasViewPassword        bool                        `json:"has_view_password"`
	ViewPasswordScope      int                         `json:"view_password_scope"`
	ViewPasswordTextRanges []viewPasswordTextRangeJSON `json:"view_password_text_ranges"`
	ContentLocked          bool                        `json:"content_locked"`
	TextLocked             bool                        `json:"text_locked"`
	MediaLocked            bool                        `json:"media_locked"`
	CreatedAt              string                      `json:"created_at"`
	VisibleAt              string                      `json:"visible_at"`
	Poll                   *feedPollJSON               `json:"poll,omitempty"`
	Reactions              []feedReactionJSON          `json:"reactions,omitempty"`
	ReplyCount             int64                       `json:"reply_count"`
	LikeCount              int64                       `json:"like_count"`
	RepostCount            int64                       `json:"repost_count"`
	LikedByMe              bool                        `json:"liked_by_me"`
	RepostedByMe           bool                        `json:"reposted_by_me"`
	BookmarkedByMe         bool                        `json:"bookmarked_by_me"`
	// ReplyToPostID is populated only for reply rows returned by the thread API.
	ReplyToPostID string `json:"reply_to_post_id,omitempty"`
	// ReplyToObjectURL is used when the parent reply target does not have a local post ID.
	ReplyToObjectURL string `json:"reply_to_object_url,omitempty"`
	// FeedEntryID is the unique key for a timeline row, even when repost rows share the original post ID.
	FeedEntryID string `json:"feed_entry_id"`
	// When Repost is set, the top-level identifiers above refer to the embedded original post.
	Repost *feedRepostMetaJSON `json:"repost,omitempty"`
	// Fields specific to rows sourced from the federated timeline.
	IsFederated     bool   `json:"is_federated,omitempty"`
	FederatedBoost  bool   `json:"federated_boost,omitempty"` // Equivalent to a received Announce or boost.
	RemoteObjectURL string `json:"remote_object_url,omitempty"`
	RemoteActorURL  string `json:"remote_actor_url,omitempty"`
}

type feedRepostMetaJSON struct {
	UserID          string   `json:"user_id"`
	UserEmail       string   `json:"user_email"`
	UserHandle      string   `json:"user_handle"`
	UserDisplayName string   `json:"user_display_name"`
	UserBadges      []string `json:"user_badges,omitempty"`
	UserAvatarURL   string   `json:"user_avatar_url"`
	RepostedAt      string   `json:"reposted_at"`
	Comment         string   `json:"comment,omitempty"`
}

type timelineCell struct {
	post        repo.PostRow
	repostEntry *repo.RepostFeedEntry
}

func buildFeedPoll(p *repo.PostPoll) *feedPollJSON {
	if p == nil {
		return nil
	}
	now := time.Now().UTC()
	closed := !p.EndsAt.After(now)
	var total int64
	opts := make([]feedPollOptionJSON, 0, len(p.Options))
	for _, o := range p.Options {
		total += o.Votes
		opts = append(opts, feedPollOptionJSON{ID: o.ID.String(), Label: o.Label, Votes: o.Votes})
	}
	out := &feedPollJSON{
		EndsAt:     p.EndsAt.UTC().Format(time.RFC3339),
		Closed:     closed,
		Options:    opts,
		TotalVotes: total,
	}
	if p.MyOptionID != nil {
		out.MyOptionID = p.MyOptionID.String()
	}
	return out
}

func buildFederationFeedPoll(p *federationEventPoll) *feedPollJSON {
	if p == nil {
		return nil
	}
	endsAt, err := time.Parse(time.RFC3339, strings.TrimSpace(p.EndsAt))
	if err != nil {
		endsAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	closed := !endsAt.After(now)
	var total int64
	opts := make([]feedPollOptionJSON, 0, len(p.Options))
	for i, o := range p.Options {
		total += o.Votes
		pos := o.Position
		if pos <= 0 {
			pos = i + 1
		}
		opts = append(opts, feedPollOptionJSON{
			ID:    fmt.Sprintf("pos:%d", pos),
			Label: o.Label,
			Votes: o.Votes,
		})
	}
	return &feedPollJSON{
		EndsAt:     endsAt.UTC().Format(time.RFC3339),
		Closed:     closed,
		Options:    opts,
		TotalVotes: total,
	}
}

func (s *Server) postRowToFeedItem(ctx context.Context, row repo.PostRow, viewer uuid.UUID, badgeMap map[uuid.UUID][]string) feedItem {
	caption := row.Caption
	keys := row.ObjectKeys
	scope := row.ViewPasswordScope
	contentLocked := false
	textLocked := false
	mediaLocked := false
	unlocked := row.UserID == viewer
	if row.HasViewPassword && row.UserID != viewer {
		rk := postUnlockRedisKey(viewer, row.ID)
		n, err := s.rdb.Exists(ctx, rk).Result()
		if err != nil {
			log.Printf("redis Exists %s: %v", rk, err)
			n = 0
		}
		if n == 0 {
			if scopeProtectsText(scope) {
				if scope == repo.ViewPasswordScopeAll {
					caption = ""
				} else {
					caption = maskCaptionText(row.Caption, row.ViewPasswordTextRanges)
				}
				textLocked = true
			}
			if scopeProtectsMedia(scope) {
				keys = []string{}
				mediaLocked = true
			}
			contentLocked = textLocked || mediaLocked
		} else {
			unlocked = true
		}
	}
	urls := make([]string, 0, len(keys))
	for _, k := range keys {
		urls = append(urls, s.glipzProtocolPublicMediaURL(k))
	}
	avatarURL := ""
	if row.AvatarObjectKey != nil {
		if k := strings.TrimSpace(*row.AvatarObjectKey); k != "" {
			avatarURL = s.glipzProtocolPublicMediaURL(k)
		}
	}
	pollJSON := buildFeedPoll(row.Poll)
	if row.HasViewPassword && !unlocked && scope == repo.ViewPasswordScopeAll {
		pollJSON = nil
	}
	return feedItem{
		ID:                     row.ID.String(),
		UserEmail:              row.Email,
		UserHandle:             row.UserHandle,
		UserDisplayName:        resolvedDisplayName(row.DisplayName, row.Email),
		UserBadges:             userBadgesJSON(badgeMap[row.UserID]),
		UserAvatarURL:          avatarURL,
		Caption:                caption,
		MediaType:              row.MediaType,
		MediaURLs:              urls,
		IsNSFW:                 row.IsNSFW,
		Visibility:             row.Visibility,
		HasViewPassword:        row.HasViewPassword,
		ViewPasswordScope:      scope,
		ViewPasswordTextRanges: repoRangesToJSON(row.ViewPasswordTextRanges),
		ContentLocked:          contentLocked,
		TextLocked:             textLocked,
		MediaLocked:            mediaLocked,
		CreatedAt:              row.CreatedAt.UTC().Format(time.RFC3339),
		VisibleAt:              row.VisibleAt.UTC().Format(time.RFC3339),
		Poll:                   pollJSON,
		Reactions:              feedReactionsJSON(row.Reactions),
		ReplyCount:             row.ReplyCount,
		LikeCount:              row.LikeCount,
		RepostCount:            row.RepostCount,
		LikedByMe:              row.LikedByMe,
		RepostedByMe:           row.RepostedByMe,
		BookmarkedByMe:         row.BookmarkedByMe,
		ReplyToPostID:          "",
		FeedEntryID:            row.ID.String(),
	}
}

func feedReactionsJSON(in []repo.PostReaction) []feedReactionJSON {
	if len(in) == 0 {
		return []feedReactionJSON{}
	}
	out := make([]feedReactionJSON, 0, len(in))
	for _, row := range in {
		out = append(out, feedReactionJSON{
			Emoji:       row.Emoji,
			Count:       row.Count,
			ReactedByMe: row.ReactedByMe,
		})
	}
	return out
}

func (s *Server) attachPostTimelineMetadata(ctx context.Context, viewer uuid.UUID, rows []repo.PostRow) error {
	if err := s.db.AttachPollsToPosts(ctx, viewer, rows); err != nil {
		return err
	}
	return s.db.AttachReactionsToPosts(ctx, viewer, rows)
}

func (s *Server) repostMetaJSON(rr *repo.RepostFeedEntry, badgeMap map[uuid.UUID][]string) *feedRepostMetaJSON {
	av := ""
	if rr.ReposterAvatarKey != nil {
		if k := strings.TrimSpace(*rr.ReposterAvatarKey); k != "" {
			av = s.glipzProtocolPublicMediaURL(k)
		}
	}
	out := &feedRepostMetaJSON{
		UserID:          rr.ReposterID.String(),
		UserEmail:       rr.ReposterEmail,
		UserHandle:      rr.ReposterHandle,
		UserDisplayName: resolvedDisplayName(rr.ReposterDisplayName, rr.ReposterEmail),
		UserBadges:      userBadgesJSON(badgeMap[rr.ReposterID]),
		UserAvatarURL:   av,
		RepostedAt:      rr.RepostedAt.UTC().Format(time.RFC3339),
	}
	if rr.RepostComment != nil {
		if c := strings.TrimSpace(*rr.RepostComment); c != "" {
			out.Comment = c
		}
	}
	return out
}

func mergePostsAndReposts(posts []repo.PostRow, reposts []repo.RepostFeedEntry, limit int) []timelineCell {
	type tagged struct {
		t    time.Time
		id   string
		cell timelineCell
	}
	buf := make([]tagged, 0, len(posts)+len(reposts))
	for _, p := range posts {
		buf = append(buf, tagged{t: p.VisibleAt, id: "p:" + p.ID.String(), cell: timelineCell{post: p, repostEntry: nil}})
	}
	for i := range reposts {
		rp := reposts[i]
		id := fmt.Sprintf("r:%s:%s:%d", rp.ReposterID, rp.Original.ID, rp.RepostedAt.UTC().UnixNano())
		buf = append(buf, tagged{t: rp.RepostedAt, id: id, cell: timelineCell{post: rp.Original, repostEntry: &rp}})
	}
	sort.Slice(buf, func(i, j int) bool {
		if !buf[i].t.Equal(buf[j].t) {
			return buf[i].t.After(buf[j].t)
		}
		return buf[i].id > buf[j].id
	})
	if len(buf) > limit {
		buf = buf[:limit]
	}
	out := make([]timelineCell, len(buf))
	for i := range buf {
		out[i] = buf[i].cell
	}
	return out
}

func (s *Server) encodeTimelineCells(ctx context.Context, cells []timelineCell, viewer uuid.UUID) ([]feedItem, error) {
	if len(cells) == 0 {
		return []feedItem{}, nil
	}
	uniq := make([]repo.PostRow, 0, len(cells))
	seen := make(map[uuid.UUID]struct{}, len(cells))
	for _, c := range cells {
		if _, ok := seen[c.post.ID]; ok {
			continue
		}
		seen[c.post.ID] = struct{}{}
		uniq = append(uniq, c.post)
	}
	if err := s.attachPostTimelineMetadata(ctx, viewer, uniq); err != nil {
		return nil, err
	}
	badgeIDs := make([]uuid.UUID, 0, len(uniq)+len(cells))
	for i := range uniq {
		badgeIDs = append(badgeIDs, uniq[i].UserID)
	}
	for _, c := range cells {
		if c.repostEntry != nil {
			badgeIDs = append(badgeIDs, c.repostEntry.ReposterID)
		}
	}
	badgeMap, err := s.userBadgeMap(ctx, badgeIDs)
	if err != nil {
		return nil, err
	}
	pollBy := make(map[uuid.UUID]*repo.PostPoll, len(uniq))
	reactionsBy := make(map[uuid.UUID][]repo.PostReaction, len(uniq))
	for i := range uniq {
		pollBy[uniq[i].ID] = uniq[i].Poll
		reactionsBy[uniq[i].ID] = uniq[i].Reactions
	}
	out := make([]feedItem, 0, len(cells))
	for _, c := range cells {
		pr := c.post
		if p, ok := pollBy[pr.ID]; ok {
			pr.Poll = p
		}
		if reactions, ok := reactionsBy[pr.ID]; ok {
			pr.Reactions = reactions
		}
		item := s.postRowToFeedItem(ctx, pr, viewer, badgeMap)
		if c.repostEntry != nil {
			item.FeedEntryID = fmt.Sprintf("repost:%s:%s:%d", c.repostEntry.ReposterID, pr.ID, c.repostEntry.RepostedAt.UTC().UnixNano())
			item.Repost = s.repostMetaJSON(c.repostEntry, badgeMap)
		}
		out = append(out, item)
	}
	return out, nil
}

func mergeFeedItemsByVisibleAt(a []feedItem, b []feedItem, limit int) []feedItem {
	type tagged struct {
		t  time.Time
		id string
		it feedItem
	}
	buf := make([]tagged, 0, len(a)+len(b))
	for _, it := range a {
		vt, _ := time.Parse(time.RFC3339, it.VisibleAt)
		buf = append(buf, tagged{t: vt, id: it.FeedEntryID, it: it})
	}
	for _, it := range b {
		vt, _ := time.Parse(time.RFC3339, it.VisibleAt)
		buf = append(buf, tagged{t: vt, id: it.FeedEntryID, it: it})
	}
	sort.Slice(buf, func(i, j int) bool {
		if !buf[i].t.Equal(buf[j].t) {
			return buf[i].t.After(buf[j].t)
		}
		return buf[i].id > buf[j].id
	})
	if len(buf) > limit {
		buf = buf[:limit]
	}
	out := make([]feedItem, len(buf))
	for i := range buf {
		out[i] = buf[i].it
	}
	return out
}

func mergeBookmarkedFeedItems(local []repo.BookmarkedPostRow, localItems []feedItem, fed []repo.BookmarkedFederatedIncomingPost, fedItems []feedItem, limit int) []feedItem {
	type tagged struct {
		t  time.Time
		id string
		it feedItem
	}
	buf := make([]tagged, 0, len(localItems)+len(fedItems))
	for i, it := range localItems {
		if i >= len(local) {
			break
		}
		buf = append(buf, tagged{t: local[i].BookmarkedAt, id: it.FeedEntryID, it: it})
	}
	for i, it := range fedItems {
		if i >= len(fed) {
			break
		}
		buf = append(buf, tagged{t: fed[i].BookmarkedAt, id: it.FeedEntryID, it: it})
	}
	sort.Slice(buf, func(i, j int) bool {
		if !buf[i].t.Equal(buf[j].t) {
			return buf[i].t.After(buf[j].t)
		}
		return buf[i].id > buf[j].id
	})
	if len(buf) > limit {
		buf = buf[:limit]
	}
	out := make([]feedItem, len(buf))
	for i := range buf {
		out[i] = buf[i].it
	}
	return out
}

func (s *Server) encodeFeedRows(ctx context.Context, rows []repo.PostRow, viewer uuid.UUID) ([]feedItem, error) {
	if len(rows) == 0 {
		return []feedItem{}, nil
	}
	if err := s.attachPostTimelineMetadata(ctx, viewer, rows); err != nil {
		return nil, err
	}
	badgeIDs := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		badgeIDs = append(badgeIDs, rows[i].UserID)
	}
	badgeMap, err := s.userBadgeMap(ctx, badgeIDs)
	if err != nil {
		return nil, err
	}
	out := make([]feedItem, 0, len(rows))
	for i := range rows {
		out = append(out, s.postRowToFeedItem(ctx, rows[i], viewer, badgeMap))
	}
	return out, nil
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	scope := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scope")))
	if scope == "recommended" {
		s.handleRecommendedFeed(w, r, uid)
		return
	}
	var posts []repo.PostRow
	var err error
	if scope == "following" {
		posts, err = s.db.ListFeedFollowing(r.Context(), uid, 50)
	} else {
		posts, err = s.db.ListFeed(r.Context(), uid, 50)
	}
	if err != nil {
		writeServerError(w, "ListFeed", err)
		return
	}
	var reposts []repo.RepostFeedEntry
	if scope == "following" {
		reposts, err = s.db.ListRecentRepostsFollowing(r.Context(), uid, 50)
	} else {
		reposts, err = s.db.ListRecentRepostsAll(r.Context(), uid, 50)
	}
	if err != nil {
		writeServerError(w, "ListRecentReposts", err)
		return
	}
	cells := mergePostsAndReposts(posts, reposts, 50)
	items, err := s.encodeTimelineCells(r.Context(), cells, uid)
	if err != nil {
		writeServerError(w, "encodeTimelineCells", err)
		return
	}
	var remoteRows []repo.FederatedIncomingPost
	var errR error
	if scope == "following" {
		remoteRows, errR = s.db.ListFederatedIncomingForRemoteFollows(r.Context(), uid, 50)
	} else {
		remoteRows, errR = s.db.ListFederatedIncomingForViewer(r.Context(), uid, 50, nil, nil)
	}
	if errR != nil {
		writeServerError(w, "ListFederatedIncoming", errR)
		return
	}
	fedItems := make([]feedItem, 0, len(remoteRows))
	for _, row := range remoteRows {
		fedItems = append(fedItems, s.federatedIncomingToFeedItem(row))
	}
	items = mergeFeedItemsByVisibleAt(items, fedItems, 50)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	localRows, err := s.db.ListBookmarkedPosts(r.Context(), uid, 100)
	if err != nil {
		writeServerError(w, "ListBookmarkedPosts", err)
		return
	}
	localBase := make([]repo.PostRow, len(localRows))
	for i := range localRows {
		localBase[i] = localRows[i].PostRow
	}
	localItems, err := s.encodeFeedRows(r.Context(), localBase, uid)
	if err != nil {
		writeServerError(w, "encodeFeedRows bookmarks", err)
		return
	}
	fedRows, err := s.db.ListBookmarkedFederatedIncoming(r.Context(), uid, 100)
	if err != nil {
		writeServerError(w, "ListBookmarkedFederatedIncoming", err)
		return
	}
	fedItems := make([]feedItem, 0, len(fedRows))
	for _, row := range fedRows {
		fedItems = append(fedItems, s.federatedIncomingToFeedItem(row.FederatedIncomingPost))
	}
	items := mergeBookmarkedFeedItems(localRows, localItems, fedRows, fedItems, 50)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePublicFeed(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListFeed(r.Context(), uuid.Nil, 30)
	if err != nil {
		writeServerError(w, "ListFeed public", err)
		return
	}
	items, err := s.encodeFeedRows(r.Context(), rows, uuid.Nil)
	if err != nil {
		writeServerError(w, "encodeFeedRows public", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rawQ := strings.TrimSpace(r.URL.Query().Get("q"))
	rawTag := strings.TrimSpace(r.URL.Query().Get("tag"))
	q := rawQ
	if q == "" && rawTag != "" {
		q = "#" + rawTag
	}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []feedItem{}, "accounts": []map[string]any{}})
		return
	}
	localRows, err := s.db.SearchPostsForViewer(r.Context(), uid, q, 50)
	if err != nil {
		writeServerError(w, "SearchPostsForViewer", err)
		return
	}
	localItems, err := s.encodeFeedRows(r.Context(), localRows, uid)
	if err != nil {
		writeServerError(w, "encodeFeedRows search", err)
		return
	}
	fedRows, err := s.db.SearchFederatedIncomingForViewer(r.Context(), uid, q, 50)
	if err != nil {
		writeServerError(w, "SearchFederatedIncomingForViewer", err)
		return
	}
	fedItems := make([]feedItem, 0, len(fedRows))
	for _, row := range fedRows {
		fedItems = append(fedItems, s.federatedIncomingToFeedItem(row))
	}
	accountRows, err := s.db.SearchAccounts(r.Context(), q, 20)
	if err != nil {
		writeServerError(w, "SearchAccounts", err)
		return
	}
	accounts := make([]map[string]any, 0, len(accountRows))
	accountBadgeMap, err := s.userBadgeMap(r.Context(), func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(accountRows))
		for _, row := range accountRows {
			ids = append(ids, row.ID)
		}
		return ids
	}())
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs search accounts", err)
		return
	}
	for _, row := range accountRows {
		account := map[string]any{
			"handle":       row.Handle,
			"display_name": resolvedDisplayName(row.DisplayName, row.Email),
			"badges":       userBadgesJSON(accountBadgeMap[row.ID]),
			"bio":          row.Bio,
			"avatar_url":   nil,
		}
		if row.AvatarObjectKey != nil && strings.TrimSpace(*row.AvatarObjectKey) != "" {
			account["avatar_url"] = s.glipzProtocolPublicMediaURL(*row.AvatarObjectKey)
		}
		accounts = append(accounts, account)
	}
	items := mergeFeedItemsByVisibleAt(localItems, fedItems, 50)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "accounts": accounts, "query": q})
}

type unlockPostReq struct {
	Password string `json:"password"`
}

func (s *Server) handlePostUnlock(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var req unlockPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	row, err := s.db.PostSensitiveByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostSensitiveByID", err)
		return
	}
	if row.UserID == uid {
		s.writeUnlockedPostJSON(w, row)
		return
	}
	if row.ViewPasswordHash == nil || strings.TrimSpace(*row.ViewPasswordHash) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_password"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*row.ViewPasswordHash), []byte(req.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong_password"})
		return
	}
	rk := postUnlockRedisKey(uid, postID)
	if err := s.rdb.Set(r.Context(), rk, "1", postUnlockRedisTTL).Err(); err != nil {
		writeServerError(w, "unlock redis Set", err)
		return
	}
	s.writeUnlockedPostJSON(w, row)
}

func (s *Server) writeUnlockedPostJSON(w http.ResponseWriter, row repo.PostSensitive) {
	urls := make([]string, 0, len(row.ObjectKeys))
	for _, k := range row.ObjectKeys {
		urls = append(urls, s.glipzProtocolPublicMediaURL(k))
	}
	hasPW := row.ViewPasswordHash != nil && strings.TrimSpace(*row.ViewPasswordHash) != ""
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                        row.ID.String(),
		"caption":                   row.Caption,
		"media_type":                row.MediaType,
		"media_urls":                urls,
		"is_nsfw":                   row.IsNSFW,
		"has_view_password":         hasPW,
		"view_password_scope":       row.ViewPasswordScope,
		"view_password_text_ranges": repoRangesToJSON(row.ViewPasswordTextRanges),
		"content_locked":            false,
		"text_locked":               false,
		"media_locked":              false,
	})
}

func (s *Server) handlePatchPost(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var req patchPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if req.ClearViewPassword && req.ViewPassword != nil && strings.TrimSpace(*req.ViewPassword) != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_conflict"})
		return
	}
	authorID, isRoot, metaErr := s.db.PostFeedMeta(r.Context(), postID)
	if metaErr != nil {
		if errors.Is(metaErr, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostFeedMeta patch", metaErr)
		return
	}
	oldVisibility, err := s.db.PostVisibilityByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostVisibilityByID patch", err)
		return
	}
	viewRanges := jsonRangesToRepo(req.ViewPasswordTextRanges)
	var newPW *string
	if req.ViewPassword != nil {
		t := strings.TrimSpace(*req.ViewPassword)
		if t != "" {
			if req.ClearViewPassword {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_conflict"})
				return
			}
			if len(t) < 4 || len(t) > 72 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_password"})
				return
			}
			newPW = &t
		}
	}
	err = s.db.UpdatePost(r.Context(), uid, postID, req.Caption, req.IsNSFW, req.Visibility, req.ClearViewPassword, newPW, req.ViewPasswordScope, viewRanges)
	if err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if errors.Is(err, repo.ErrInvalidViewPassword) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_password"})
			return
		}
		if errors.Is(err, repo.ErrInvalidViewPasswordScope) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_view_password_scope"})
			return
		}
		if errors.Is(err, repo.ErrInvalidViewPasswordTextRanges) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_view_password_text_ranges"})
			return
		}
		writeServerError(w, "UpdatePost", err)
		return
	}
	row, err := s.db.PostRowForViewer(r.Context(), uid, postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostRowForViewer", err)
		return
	}
	rows := []repo.PostRow{row}
	if err := s.attachPostTimelineMetadata(r.Context(), uid, rows); err != nil {
		writeServerError(w, "attachPostTimelineMetadata", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), []uuid.UUID{rows[0].UserID})
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs post update", err)
		return
	}
	item := s.postRowToFeedItem(r.Context(), rows[0], uid, badgeMap)
	if isRoot {
		updatePayload, _ := json.Marshal(map[string]any{
			"v":         1,
			"kind":      "post_updated",
			"post_id":   postID.String(),
			"author_id": authorID.String(),
		})
		createPayload, _ := json.Marshal(map[string]any{
			"v":         1,
			"kind":      "post_created",
			"post_id":   postID.String(),
			"author_id": authorID.String(),
		})
		deletePayload, _ := json.Marshal(map[string]any{
			"v":         1,
			"kind":      "post_deleted",
			"post_id":   postID.String(),
			"author_id": authorID.String(),
		})
		switch {
		case oldVisibility == repo.PostVisibilityPublic && row.Visibility != repo.PostVisibilityPublic:
			s.publishFeedEventGlobalOnly(r.Context(), deletePayload)
			s.publishFeedEventJSON(r.Context(), updatePayload, authorID, row.Visibility)
		case oldVisibility == repo.PostVisibilityLoggedIn && row.Visibility != repo.PostVisibilityLoggedIn && row.Visibility != repo.PostVisibilityPublic:
			s.publishFeedEventLoggedInOnly(r.Context(), deletePayload)
			s.publishFeedEventJSON(r.Context(), updatePayload, authorID, row.Visibility)
		case oldVisibility != repo.PostVisibilityPublic && row.Visibility == repo.PostVisibilityPublic:
			s.publishFeedEventJSON(r.Context(), createPayload, authorID, row.Visibility)
		case oldVisibility != repo.PostVisibilityLoggedIn && row.Visibility == repo.PostVisibilityLoggedIn:
			s.publishFeedEventJSON(r.Context(), createPayload, authorID, row.Visibility)
		default:
			s.publishFeedEventJSON(r.Context(), updatePayload, authorID, row.Visibility)
		}
	}
	uidCopy, postIDCopy := uid, postID
	newVisibility := row.Visibility
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if newVisibility != repo.PostVisibilityPublic && oldVisibility == repo.PostVisibilityPublic {
			s.deliverFederationDelete(ctx, uidCopy, postIDCopy)
			return
		}
		row, err := s.db.GetFederationPublicPostForDelivery(ctx, uidCopy, postIDCopy)
		if err == nil {
			if oldVisibility == repo.PostVisibilityPublic {
				s.deliverFederationUpdate(ctx, uidCopy, row)
			} else {
				s.deliverFederationCreate(ctx, uidCopy, postIDCopy)
			}
			return
		}
		if !errors.Is(err, repo.ErrNotFound) {
			log.Printf("federation patch get post: %v", err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	authorID, isRoot, metaErr := s.db.PostFeedMeta(r.Context(), postID)
	if metaErr != nil {
		if errors.Is(metaErr, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostFeedMeta", metaErr)
		return
	}
	visibility, visErr := s.db.PostVisibilityByID(r.Context(), postID)
	if visErr != nil {
		if errors.Is(visErr, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostVisibilityByID delete", visErr)
		return
	}
	_, fedErr := s.db.GetFederationPublicPostForDelivery(r.Context(), authorID, postID)
	shouldDeliverFederation := fedErr == nil
	err = s.db.DeletePostByActor(r.Context(), uid, postID, s.isSiteAdmin(uid))
	if err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "DeletePost", err)
		return
	}
	if isRoot {
		b, _ := json.Marshal(map[string]any{
			"v":         1,
			"kind":      "post_deleted",
			"post_id":   postID.String(),
			"author_id": authorID.String(),
		})
		s.publishFeedEventJSON(r.Context(), b, authorID, visibility)
		aid, pid := authorID, postID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s.deliverFederationDelete(ctx, aid, pid)
		}()
	} else if shouldDeliverFederation {
		aid, pid := authorID, postID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s.deliverFederationDelete(ctx, aid, pid)
		}()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleToggleLike(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	pid, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	liked, count, err := s.db.ToggleLike(r.Context(), uid, pid)
	if err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ToggleLike", err)
		return
	}
	postOwnerID, isRoot, metaErr := s.db.PostFeedMeta(r.Context(), pid)
	visibility, _ := s.db.PostVisibilityByID(r.Context(), pid)
	if liked {
		if metaErr == nil && postOwnerID != uid {
			nid, errN := s.db.InsertNotification(r.Context(), postOwnerID, uid, "like", &pid, nil)
			if errN == nil {
				s.publishNotifyUserEvent(r.Context(), postOwnerID, nid)
			}
		}
	}
	if metaErr == nil {
		if actor, err := s.federationAuthorPayload(r.Context(), uid); err == nil {
			s.deliverFederationLikeEventToSubscribers(r.Context(), postOwnerID, actor, pid, count, liked)
		}
		if isRoot {
			b, _ := json.Marshal(map[string]any{
				"v":         1,
				"kind":      "post_updated",
				"post_id":   pid.String(),
				"author_id": postOwnerID.String(),
			})
			s.publishFeedEventJSON(r.Context(), b, postOwnerID, visibility)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"liked": liked, "like_count": count})
}

type postReactionReq struct {
	Emoji string `json:"emoji"`
}

func (s *Server) handleAddPostReaction(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var req postReactionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	emoji, valid := repo.NormalizePostReactionEmoji(req.Emoji)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_emoji"})
		return
	}
	changed, err := s.db.AddPostReaction(r.Context(), uid, postID, emoji)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if errors.Is(err, repo.ErrInvalidReactionEmoji) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_emoji"})
			return
		}
		writeServerError(w, "AddPostReaction", err)
		return
	}
	s.writePostReactionItem(w, r, uid, postID, emoji, true, changed, changed)
}

func (s *Server) handleDeletePostReaction(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	rawEmoji, err := url.PathUnescape(strings.TrimSpace(chi.URLParam(r, "emoji")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_emoji"})
		return
	}
	emoji, valid := repo.NormalizePostReactionEmoji(rawEmoji)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_emoji"})
		return
	}
	changed, err := s.db.RemovePostReaction(r.Context(), uid, postID, emoji)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if errors.Is(err, repo.ErrInvalidReactionEmoji) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_emoji"})
			return
		}
		writeServerError(w, "RemovePostReaction", err)
		return
	}
	s.writePostReactionItem(w, r, uid, postID, emoji, false, changed, false)
}

func (s *Server) writePostReactionItem(w http.ResponseWriter, r *http.Request, uid, postID uuid.UUID, emoji string, added bool, changed bool, notify bool) {
	row, err := s.db.PostRowForViewer(r.Context(), uid, postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostRowForViewer reaction", err)
		return
	}
	rows := []repo.PostRow{row}
	if err := s.attachPostTimelineMetadata(r.Context(), uid, rows); err != nil {
		writeServerError(w, "attachPostTimelineMetadata reaction", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), []uuid.UUID{rows[0].UserID})
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs reaction", err)
		return
	}
	item := s.postRowToFeedItem(r.Context(), rows[0], uid, badgeMap)
	postOwnerID, isRoot, metaErr := s.db.PostFeedMeta(r.Context(), postID)
	visibility, _ := s.db.PostVisibilityByID(r.Context(), postID)
	if metaErr == nil {
		if changed && isRoot {
			b, _ := json.Marshal(map[string]any{
				"v":         1,
				"kind":      "post_updated",
				"post_id":   postID.String(),
				"author_id": postOwnerID.String(),
			})
			s.publishFeedEventJSON(r.Context(), b, postOwnerID, visibility)
		}
		if changed {
			if actor, err := s.federationAuthorPayload(r.Context(), uid); err == nil {
				s.deliverFederationReactionEventToSubscribers(r.Context(), postOwnerID, actor, postID, emoji, added)
			}
		}
		if notify && postOwnerID != uid {
			if nid, errN := s.db.InsertNotification(r.Context(), postOwnerID, uid, "like", &postID, nil); errN == nil {
				s.publishNotifyUserEvent(r.Context(), postOwnerID, nid)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"emoji": emoji,
		"item":  item,
	})
}

func (s *Server) handleToggleBookmark(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	pid, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	bookmarked, err := s.db.ToggleBookmark(r.Context(), uid, pid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ToggleBookmark", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookmarked": bookmarked})
}

const maxRepostCommentRunes = 2000

func (s *Server) handleToggleRepost(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	pid, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var repostComment *string
	if r.Body != nil {
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if readErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		if len(strings.TrimSpace(string(raw))) > 0 {
			var body struct {
				Comment string `json:"comment"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
				return
			}
			c := strings.TrimSpace(body.Comment)
			if c != "" {
				if utf8.RuneCountInString(c) > maxRepostCommentRunes {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repost_comment_too_long"})
					return
				}
				repostComment = &c
			}
		}
	}
	reposted, count, err := s.db.ToggleRepost(r.Context(), uid, pid, repostComment)
	if err != nil {
		if errors.Is(err, repo.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ToggleRepost", err)
		return
	}
	if reposted {
		owner, errOwner := s.db.PostAuthorID(r.Context(), pid)
		if errOwner == nil && owner != uid {
			nid, errN := s.db.InsertNotification(r.Context(), owner, uid, "repost", &pid, nil)
			if errN == nil {
				s.publishNotifyUserEvent(r.Context(), owner, nid)
			}
		}
		go func(booster, post uuid.UUID, comment *string) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			s.deliverFederationRepost(ctx, booster, post, comment)
		}(uid, pid, repostComment)
	} else {
		go func(booster, post uuid.UUID) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			s.deliverFederationRepostDelete(ctx, booster, post)
		}(uid, pid)
	}
	writeJSON(w, http.StatusOK, map[string]any{"reposted": reposted, "repost_count": count})
}

func displayNameFromEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return strings.TrimSpace(email)
	}
	return strings.TrimSpace(email[:at])
}

func resolvedDisplayName(stored, email string) string {
	if t := strings.TrimSpace(stored); t != "" {
		return t
	}
	return displayNameFromEmail(email)
}

func (s *Server) handlePublicProfileByHandle(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle", err)
		return
	}
	isMe := pfl.ID == uid
	followers, following, err := s.db.FollowCounts(r.Context(), pfl.ID)
	if err != nil {
		writeServerError(w, "FollowCounts", err)
		return
	}
	remoteFollowers, err := s.db.CountFederationRemoteFollowers(r.Context(), pfl.ID)
	if err != nil {
		writeServerError(w, "CountFederationRemoteFollowers", err)
		return
	}
	remoteFollowing, err := s.db.CountAcceptedRemoteFollowsForUser(r.Context(), pfl.ID)
	if err != nil {
		writeServerError(w, "CountAcceptedRemoteFollowsForUser", err)
		return
	}
	followedByMe, err := s.db.IsFollowing(r.Context(), uid, pfl.ID)
	if err != nil {
		writeServerError(w, "IsFollowing", err)
		return
	}
	followsYou, err := s.db.IsFollowing(r.Context(), pfl.ID, uid)
	if err != nil {
		writeServerError(w, "IsFollowing reverse", err)
		return
	}
	out := map[string]any{
		"handle":          pfl.Handle,
		"display_name":    resolvedDisplayName(pfl.DisplayName, pfl.Email),
		"badges":          userBadgesJSON(s.visibleUserBadges(pfl.ID, pfl.Badges)),
		"bio":             pfl.Bio,
		"profile_urls":    pfl.ProfileExternalURLs,
		"is_me":           isMe,
		"follower_count":  followers + remoteFollowers,
		"following_count": following + remoteFollowing,
		"followed_by_me":  followedByMe,
		"follows_you":     followsYou,
		"avatar_url":      nil,
		"header_url":      nil,
	}
	if pfl.AvatarObjectKey != nil && *pfl.AvatarObjectKey != "" {
		out["avatar_url"] = s.glipzProtocolPublicMediaURL(*pfl.AvatarObjectKey)
	}
	if pfl.HeaderObjectKey != nil && *pfl.HeaderObjectKey != "" {
		out["header_url"] = s.glipzProtocolPublicMediaURL(*pfl.HeaderObjectKey)
	}
	if isMe {
		out["email"] = pfl.Email
		out["display_name_raw"] = strings.TrimSpace(pfl.DisplayName)
		if pfl.AvatarObjectKey != nil {
			out["avatar_object_key"] = *pfl.AvatarObjectKey
		} else {
			out["avatar_object_key"] = ""
		}
		if pfl.HeaderObjectKey != nil {
			out["header_object_key"] = *pfl.HeaderObjectKey
		} else {
			out["header_object_key"] = ""
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleToggleFollow(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle follow", err)
		return
	}
	following, followerCount, err := s.db.ToggleFollow(r.Context(), uid, pfl.ID)
	if err != nil {
		if errors.Is(err, repo.ErrCannotFollowSelf) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_follow_self"})
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ToggleFollow", err)
		return
	}
	if following {
		nid, errN := s.db.InsertNotification(r.Context(), pfl.ID, uid, "follow", nil, nil)
		if errN == nil {
			s.publishNotifyUserEvent(r.Context(), pfl.ID, nid)
		}
	}
	remoteN, _ := s.db.CountFederationRemoteFollowers(r.Context(), pfl.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"following":      following,
		"follower_count": followerCount + remoteN,
	})
}

func (s *Server) handleUserFollowersByHandle(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle followers", err)
		return
	}
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			limit = n
		}
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			offset = n
		}
	}
	items, nextOffset, hasNext, err := s.db.ListUserFollowers(r.Context(), uid, pfl.ID, limit, offset)
	if err != nil {
		writeServerError(w, "ListUserFollowers", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.ID)
		}
		return ids
	}())
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs followers", err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		row := map[string]any{
			"handle":         it.Handle,
			"display_name":   resolvedDisplayName(it.DisplayName, it.Email),
			"badges":         userBadgesJSON(badgeMap[it.ID]),
			"bio":            strings.TrimSpace(it.Bio),
			"avatar_url":     nil,
			"followed_by_me": it.FollowedByMe,
			"follows_you":    it.FollowsYou,
		}
		if it.AvatarObjectKey != nil && strings.TrimSpace(*it.AvatarObjectKey) != "" {
			row["avatar_url"] = s.glipzProtocolPublicMediaURL(*it.AvatarObjectKey)
		}
		out = append(out, row)
	}

	// Append remote followers (Glipz Protocol inbound follows) on the first page.
	// These are not local users and are shown as links to the remote profile view.
	if offset == 0 {
		remoteRows, err := s.db.ListGlipzProtocolRemoteFollowers(r.Context(), pfl.ID, 50, 0)
		if err != nil {
			writeServerError(w, "ListGlipzProtocolRemoteFollowers followers", err)
			return
		}
		for _, rr := range remoteRows {
			raw := strings.TrimSpace(rr.RemoteActorID)
			if raw == "" {
				continue
			}
			disp, err := FetchRemoteActorDisplay(r.Context(), raw)
			if err != nil {
				// Best-effort: still show something clickable even if resolution fails.
				disp = RemoteActorDisplay{ActorID: raw, Acct: raw, Name: raw}
			}
			h := strings.TrimPrefix(strings.TrimSpace(disp.Acct), "@")
			out = append(out, map[string]any{
				"is_remote":      true,
				"remote_actor_id": strings.TrimSpace(disp.ActorID),
				"handle":         h,
				"display_name":   strings.TrimSpace(disp.Name),
				"badges":         []any{},
				"bio":            strings.TrimSpace(disp.Summary),
				"avatar_url":     strings.TrimSpace(disp.IconURL),
				"followed_by_me": false,
				"follows_you":    false,
			})
		}
	}
	resp := map[string]any{"items": out}
	if hasNext {
		resp["next_offset"] = nextOffset
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUserFollowingByHandle(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle following", err)
		return
	}
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			limit = n
		}
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			offset = n
		}
	}
	items, nextOffset, hasNext, err := s.db.ListUserFollowing(r.Context(), uid, pfl.ID, limit, offset)
	if err != nil {
		writeServerError(w, "ListUserFollowing", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.ID)
		}
		return ids
	}())
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs following", err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		row := map[string]any{
			"handle":         it.Handle,
			"display_name":   resolvedDisplayName(it.DisplayName, it.Email),
			"badges":         userBadgesJSON(badgeMap[it.ID]),
			"bio":            strings.TrimSpace(it.Bio),
			"avatar_url":     nil,
			"followed_by_me": it.FollowedByMe,
			"follows_you":    it.FollowsYou,
		}
		if it.AvatarObjectKey != nil && strings.TrimSpace(*it.AvatarObjectKey) != "" {
			row["avatar_url"] = s.glipzProtocolPublicMediaURL(*it.AvatarObjectKey)
		}
		out = append(out, row)
	}

	// Append accepted remote follows on the first page.
	if offset == 0 {
		remoteRows, err := s.db.ListAcceptedRemoteFollowsForUser(r.Context(), pfl.ID, 50, 0)
		if err != nil {
			writeServerError(w, "ListAcceptedRemoteFollowsForUser following", err)
			return
		}
		for _, rr := range remoteRows {
			raw := strings.TrimSpace(rr.RemoteActorID)
			if raw == "" {
				continue
			}
			disp, err := FetchRemoteActorDisplay(r.Context(), raw)
			if err != nil {
				disp = RemoteActorDisplay{ActorID: raw, Acct: raw, Name: raw}
			}
			h := strings.TrimPrefix(strings.TrimSpace(disp.Acct), "@")
			out = append(out, map[string]any{
				"is_remote":      true,
				"remote_actor_id": strings.TrimSpace(disp.ActorID),
				"handle":         h,
				"display_name":   strings.TrimSpace(disp.Name),
				"badges":         []any{},
				"bio":            strings.TrimSpace(disp.Summary),
				"avatar_url":     strings.TrimSpace(disp.IconURL),
				"followed_by_me": true,
				"follows_you":    false,
			})
		}
	}
	resp := map[string]any{"items": out}
	if hasNext {
		resp["next_offset"] = nextOffset
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUserPostsByHandle(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle posts", err)
		return
	}
	rows, err := s.db.ListUserPosts(r.Context(), uid, pfl.ID, 50)
	if err != nil {
		writeServerError(w, "ListUserPosts", err)
		return
	}
	items, err := s.encodeFeedRows(r.Context(), rows, uid)
	if err != nil {
		writeServerError(w, "encodeFeedRows", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleUserRepliesByHandle(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle replies", err)
		return
	}
	rows, err := s.db.ListUserReplyPosts(r.Context(), uid, pfl.ID, 50)
	if err != nil {
		writeServerError(w, "ListUserReplyPosts", err)
		return
	}
	if len(rows) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []feedItem{}})
		return
	}
	postRows := make([]repo.PostRow, len(rows))
	for i := range rows {
		postRows[i] = rows[i].PostRow
	}
	if err := s.attachPostTimelineMetadata(r.Context(), uid, postRows); err != nil {
		writeServerError(w, "attachPostTimelineMetadata user replies", err)
		return
	}
	badgeIDs := make([]uuid.UUID, 0, len(postRows))
	for i := range postRows {
		badgeIDs = append(badgeIDs, postRows[i].UserID)
	}
	badgeMap, err := s.userBadgeMap(r.Context(), badgeIDs)
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs user replies", err)
		return
	}
	out := make([]feedItem, 0, len(postRows))
	for i := range postRows {
		it := s.postRowToFeedItem(r.Context(), postRows[i], uid, badgeMap)
		if rows[i].ReplyToID != nil {
			it.ReplyToPostID = rows[i].ReplyToID.String()
		} else if remoteObject := strings.TrimSpace(rows[i].ReplyToRemoteObjectIRI); remoteObject != "" {
			if incoming, err := s.db.GetFederatedIncomingByObjectIRI(r.Context(), remoteObject); err == nil && incoming.ID != uuid.Nil {
				it.ReplyToPostID = "federated:" + incoming.ID.String()
			} else {
				it.ReplyToObjectURL = remoteObject
			}
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleUserNotesListByHandle(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle notes list", err)
		return
	}
	rows, err := s.db.ListUserNotesForProfile(r.Context(), pfl.ID, uid, 50)
	if err != nil {
		writeServerError(w, "ListUserNotesForProfile", err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id":         row.ID.String(),
			"title":      row.Title,
			"status":     row.Status,
			"updated_at": row.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleUserPostMediaTilesByHandle(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	h := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), h)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle media tiles", err)
		return
	}
	rows, err := s.db.ListUserPostMediaTiles(r.Context(), pfl.ID, uid, 90)
	if err != nil {
		writeServerError(w, "ListUserPostMediaTiles", err)
		return
	}
	tiles := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		url := s.postMediaPreviewURL(r.Context(), uid, row.AuthorUserID, row.PostID, row.HasViewPassword, row.ViewPasswordScope, row.FirstObjectKey)
		locked := url == "" && row.HasViewPassword && row.AuthorUserID != uid && scopeProtectsMedia(repo.EffectiveViewPasswordScope(row.HasViewPassword, row.ViewPasswordScope))
		tiles = append(tiles, map[string]any{
			"post_id":     row.PostID.String(),
			"media_type":  row.MediaType,
			"preview_url": url,
			"locked":      locked,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tiles": tiles})
}

type pollVoteReq struct {
	OptionID string `json:"option_id"`
}

func (s *Server) handlePollVote(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var req pollVoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	optionID, err := uuid.Parse(strings.TrimSpace(req.OptionID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_option_id"})
		return
	}
	if err := s.db.CastPollVote(r.Context(), uid, postID, optionID); err != nil {
		switch {
		case errors.Is(err, repo.ErrPollNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "poll_not_found"})
		case errors.Is(err, repo.ErrPollClosed):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "poll_closed"})
		case errors.Is(err, repo.ErrPollAlreadyVoted):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already_voted"})
		case errors.Is(err, repo.ErrPollInvalidOption):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_option"})
		case errors.Is(err, repo.ErrPollNotVisible):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "post_not_visible"})
		default:
			writeServerError(w, "CastPollVote", err)
		}
		return
	}
	if ownerID, _, err := s.db.PostFeedMeta(r.Context(), postID); err == nil {
		s.deliverFederationPollTallyUpdated(r.Context(), ownerID, postID)
	}
	row, err := s.db.PostRowForViewer(r.Context(), uid, postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostRowForViewer", err)
		return
	}
	rows := []repo.PostRow{row}
	if err := s.attachPostTimelineMetadata(r.Context(), uid, rows); err != nil {
		writeServerError(w, "attachPostTimelineMetadata", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), []uuid.UUID{rows[0].UserID})
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs poll vote", err)
		return
	}
	item := s.postRowToFeedItem(r.Context(), rows[0], uid, badgeMap)
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) handleListScheduledPosts(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.ListScheduledRootPosts(r.Context(), uid, 30)
	if err != nil {
		writeServerError(w, "ListScheduledRootPosts", err)
		return
	}
	items, err := s.encodeFeedRows(r.Context(), rows, uid)
	if err != nil {
		writeServerError(w, "encodeFeedRows", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type patchProfileReq struct {
	Bio             string   `json:"bio"`
	DisplayName     string   `json:"display_name"`
	Handle          string   `json:"handle"`
	AvatarObjectKey string   `json:"avatar_object_key"`
	HeaderObjectKey string   `json:"header_object_key"`
	ProfileURLs     []string `json:"profile_urls"`
	IsBot           bool     `json:"is_bot"`
	IsAI            bool     `json:"is_ai"`
}

type patchMeDMSettingsReq struct {
	CallTimeoutSeconds int         `json:"call_timeout_seconds"`
	CallEnabled        bool        `json:"call_enabled"`
	CallScope          string      `json:"call_scope"`
	AllowedUserIDs     []uuid.UUID `json:"allowed_user_ids"`
	DmInviteAutoAccept *bool       `json:"dm_invite_auto_accept"`
}

func (s *Server) handlePatchMeProfile(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req patchProfileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if len([]rune(req.Bio)) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bio_too_long"})
		return
	}
	if len([]rune(strings.TrimSpace(req.DisplayName))) > 50 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name_too_long"})
		return
	}
	handleNorm, err := repo.NormalizeHandle(req.Handle)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	prefix := "uploads/" + uid.String() + "/"
	if a := strings.TrimSpace(req.AvatarObjectKey); a != "" && !strings.HasPrefix(a, prefix) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_avatar_key"})
		return
	}
	if h := strings.TrimSpace(req.HeaderObjectKey); h != "" && !strings.HasPrefix(h, prefix) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_header_key"})
		return
	}
	displayStored := strings.TrimSpace(req.DisplayName)
	var profileURLs []string
	if req.ProfileURLs != nil {
		var errN error
		profileURLs, errN = repo.NormalizeProfileExternalURLs(req.ProfileURLs)
		if errN != nil {
			switch {
			case errors.Is(errN, repo.ErrProfileURLsTooMany):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "profile_urls_too_many"})
			case errors.Is(errN, repo.ErrProfileURLTooLong):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "profile_url_too_long"})
			case errors.Is(errN, repo.ErrInvalidProfileURL):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_profile_url"})
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_profile_url"})
			}
			return
		}
	} else {
		var errN error
		profileURLs, errN = s.db.UserProfileExternalURLs(r.Context(), uid)
		if errN != nil {
			writeServerError(w, "UserProfileExternalURLs", errN)
			return
		}
	}
	if err := s.db.UpdateUserProfile(r.Context(), uid, req.Bio, displayStored, handleNorm, req.AvatarObjectKey, req.HeaderObjectKey, profileURLs, req.IsBot, req.IsAI); err != nil {
		if errors.Is(err, repo.ErrHandleTaken) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "handle_taken"})
			return
		}
		if strings.Contains(err.Error(), "bio too long") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bio_too_long"})
			return
		}
		if strings.Contains(err.Error(), "display name too long") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name_too_long"})
			return
		}
		writeServerError(w, "UpdateUserProfile", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePatchMeDMSettings(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req patchMeDMSettingsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if req.CallTimeoutSeconds < 5 || req.CallTimeoutSeconds > 300 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_call_timeout_seconds"})
		return
	}
	if err := s.db.SetDMCallTimeoutSeconds(r.Context(), uid, req.CallTimeoutSeconds); err != nil {
		writeServerError(w, "SetDMCallTimeoutSeconds", err)
		return
	}
	if err := s.db.SetDMCallPolicy(r.Context(), uid, req.CallEnabled, req.CallScope, req.AllowedUserIDs); err != nil {
		writeServerError(w, "SetDMCallPolicy", err)
		return
	}
	dmInviteAuto := false
	if req.DmInviteAutoAccept != nil {
		dmInviteAuto = *req.DmInviteAutoAccept
		if err := s.db.SetDMInviteAutoAccept(r.Context(), uid, dmInviteAuto); err != nil {
			writeServerError(w, "SetDMInviteAutoAccept", err)
			return
		}
	} else {
		u, err := s.db.UserByID(r.Context(), uid)
		if err != nil {
			writeServerError(w, "UserByID dm invite flag", err)
			return
		}
		dmInviteAuto = u.DMInviteAutoAccept
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                "ok",
		"call_timeout_seconds":  req.CallTimeoutSeconds,
		"call_enabled":          req.CallEnabled,
		"call_scope":            req.CallScope,
		"allowed_user_ids":      req.AllowedUserIDs,
		"dm_invite_auto_accept": dmInviteAuto,
	})
}
