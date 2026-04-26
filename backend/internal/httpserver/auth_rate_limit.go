package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	loginRateLimitWindow       = 15 * time.Minute
	loginRateLimitIPMax        = 30
	loginRateLimitAccountMax   = 10
	mfaRateLimitWindow         = 10 * time.Minute
	mfaRateLimitIPMax          = 60
	mfaRateLimitUserMax        = 10
	remoteMediaRateLimitWindow = 15 * time.Minute
	linkPreviewRateLimitWindow = 15 * time.Minute
	loginRateLimitRedisTimeout = 2 * time.Second
)

func (s *Server) clientIPForAuthRateLimit(r *http.Request) string {
	if s.cfg.TrustProxyHeaders {
		if ip := trustedProxyClientIP(r); ip != "" {
			return ip
		}
	}
	return directClientIP(r)
}

func loginAccountRateKey(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "rl:auth:login:acct:" + hex.EncodeToString(sum[:])
}

func loginIPRateKey(ip string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip)))
	return "rl:auth:login:ip:" + hex.EncodeToString(sum[:])
}

func remoteMediaIPRateKey(ip string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip)))
	return "rl:media:remote:ip:" + hex.EncodeToString(sum[:])
}

func linkPreviewIPRateKey(ip string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip)))
	return "rl:link_preview:ip:" + hex.EncodeToString(sum[:])
}

func linkPreviewUserRateKey(subject string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(subject)))
	return "rl:link_preview:user:" + hex.EncodeToString(sum[:])
}

func mfaUserRateKey(subject string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(subject)))
	return "rl:auth:mfa:user:" + hex.EncodeToString(sum[:])
}

func mfaIPRateKey(ip string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip)))
	return "rl:auth:mfa:ip:" + hex.EncodeToString(sum[:])
}

func (s *Server) loginRateLimitExceeded(ctx context.Context, r *http.Request, email string) bool {
	if s.rdb == nil {
		return false
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	keys := []string{}
	if ip := s.clientIPForAuthRateLimit(r); ip != "" {
		keys = append(keys, loginIPRateKey(ip))
	}
	if strings.TrimSpace(email) != "" {
		keys = append(keys, loginAccountRateKey(email))
	}
	for _, key := range keys {
		n, err := s.rdb.Get(limitCtx, key).Int()
		if err != nil {
			if err != redis.Nil {
				addRateLimitError("login.get")
				log.Printf("login rate limit get %s: %v", key, err)
				if s.cfg.AuthRateLimitFailClosed {
					return true
				}
			}
			continue
		}
		switch {
		case strings.Contains(key, ":ip:") && n >= loginRateLimitIPMax:
			return true
		case strings.Contains(key, ":acct:") && n >= loginRateLimitAccountMax:
			return true
		}
	}
	return false
}

func (s *Server) recordLoginFailure(ctx context.Context, r *http.Request, email string) {
	if s.rdb == nil {
		return
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	keys := []string{}
	if ip := s.clientIPForAuthRateLimit(r); ip != "" {
		keys = append(keys, loginIPRateKey(ip))
	}
	if strings.TrimSpace(email) != "" {
		keys = append(keys, loginAccountRateKey(email))
	}
	for _, key := range keys {
		n, err := s.rdb.Incr(limitCtx, key).Result()
		if err != nil {
			addRateLimitError("login.incr")
			log.Printf("login rate limit incr %s: %v", key, err)
			continue
		}
		if n == 1 {
			_ = s.rdb.Expire(limitCtx, key, loginRateLimitWindow).Err()
		}
	}
}

func (s *Server) clearLoginFailures(ctx context.Context, r *http.Request, email string) {
	if s.rdb == nil {
		return
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	keys := []string{}
	if ip := s.clientIPForAuthRateLimit(r); ip != "" {
		keys = append(keys, loginIPRateKey(ip))
	}
	if strings.TrimSpace(email) != "" {
		keys = append(keys, loginAccountRateKey(email))
	}
	if len(keys) > 0 {
		_ = s.rdb.Del(limitCtx, keys...).Err()
	}
}

func (s *Server) mfaRateLimitExceeded(ctx context.Context, r *http.Request, subject string) bool {
	if s.rdb == nil {
		return false
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	keys := []string{}
	if ip := s.clientIPForAuthRateLimit(r); ip != "" {
		keys = append(keys, mfaIPRateKey(ip))
	}
	if strings.TrimSpace(subject) != "" {
		keys = append(keys, mfaUserRateKey(subject))
	}
	for _, key := range keys {
		n, err := s.rdb.Get(limitCtx, key).Int()
		if err != nil {
			if err != redis.Nil {
				addRateLimitError("mfa.get")
				log.Printf("mfa rate limit get %s: %v", key, err)
				if s.cfg.AuthRateLimitFailClosed {
					return true
				}
			}
			continue
		}
		switch {
		case strings.Contains(key, ":ip:") && n >= mfaRateLimitIPMax:
			return true
		case strings.Contains(key, ":user:") && n >= mfaRateLimitUserMax:
			return true
		}
	}
	return false
}

func (s *Server) recordMFAFailure(ctx context.Context, r *http.Request, subject string) {
	if s.rdb == nil {
		return
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	keys := []string{}
	if ip := s.clientIPForAuthRateLimit(r); ip != "" {
		keys = append(keys, mfaIPRateKey(ip))
	}
	if strings.TrimSpace(subject) != "" {
		keys = append(keys, mfaUserRateKey(subject))
	}
	for _, key := range keys {
		n, err := s.rdb.Incr(limitCtx, key).Result()
		if err != nil {
			addRateLimitError("mfa.incr")
			log.Printf("mfa rate limit incr %s: %v", key, err)
			continue
		}
		if n == 1 {
			_ = s.rdb.Expire(limitCtx, key, mfaRateLimitWindow).Err()
		}
	}
}

func (s *Server) clearMFAFailures(ctx context.Context, r *http.Request, subject string) {
	if s.rdb == nil {
		return
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	keys := []string{}
	if ip := s.clientIPForAuthRateLimit(r); ip != "" {
		keys = append(keys, mfaIPRateKey(ip))
	}
	if strings.TrimSpace(subject) != "" {
		keys = append(keys, mfaUserRateKey(subject))
	}
	if len(keys) > 0 {
		_ = s.rdb.Del(limitCtx, keys...).Err()
	}
}

func writeLoginRateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", fmt.Sprintf("%.0f", loginRateLimitWindow.Seconds()))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
}

func writeMFARateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", fmt.Sprintf("%.0f", mfaRateLimitWindow.Seconds()))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
}

func (s *Server) remoteMediaRateLimitExceeded(ctx context.Context, r *http.Request) bool {
	if s.rdb == nil {
		return false
	}
	ip := s.clientIPForAuthRateLimit(r)
	if strings.TrimSpace(ip) == "" {
		return false
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	key := remoteMediaIPRateKey(ip)
	n, err := s.rdb.Incr(limitCtx, key).Result()
	if err != nil {
		addRateLimitError("remote_media.incr")
		log.Printf("remote media rate limit incr %s: %v", key, err)
		return s.cfg.RemoteMediaProxyRateLimitFailClosed
	}
	if n == 1 {
		_ = s.rdb.Expire(limitCtx, key, remoteMediaRateLimitWindow).Err()
	}
	return int(n) > s.cfg.RemoteMediaProxyRateLimitMax
}

func writeRemoteMediaRateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", fmt.Sprintf("%.0f", remoteMediaRateLimitWindow.Seconds()))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
}

func (s *Server) linkPreviewRateLimitExceeded(ctx context.Context, r *http.Request) bool {
	if s.rdb == nil {
		return false
	}
	keys := []string{}
	if ip := s.clientIPForAuthRateLimit(r); strings.TrimSpace(ip) != "" {
		keys = append(keys, linkPreviewIPRateKey(ip))
	}
	if uid, ok := userIDFrom(r.Context()); ok {
		keys = append(keys, linkPreviewUserRateKey(uid.String()))
	}
	if len(keys) == 0 {
		return false
	}
	limitCtx, cancel := context.WithTimeout(ctx, loginRateLimitRedisTimeout)
	defer cancel()
	exceeded := false
	for _, key := range keys {
		n, err := s.rdb.Incr(limitCtx, key).Result()
		if err != nil {
			addRateLimitError("link_preview.incr")
			log.Printf("link preview rate limit incr %s: %v", key, err)
			return s.cfg.LinkPreviewRateLimitFailClosed
		}
		if n == 1 {
			_ = s.rdb.Expire(limitCtx, key, linkPreviewRateLimitWindow).Err()
		}
		if int(n) > s.cfg.LinkPreviewRateLimitMax {
			exceeded = true
		}
	}
	return exceeded
}

func writeLinkPreviewRateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", fmt.Sprintf("%.0f", linkPreviewRateLimitWindow.Seconds()))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
}
