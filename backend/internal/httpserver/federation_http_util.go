package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func federationRemoteHostFromSignerKeyID(keyID string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(keyID))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("bad key id")
	}
	return strings.ToLower(strings.TrimPrefix(u.Hostname(), "www.")), nil
}

func federationHostFromInboxURL(inboxURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(inboxURL))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("bad inbox url")
	}
	return strings.ToLower(strings.TrimPrefix(u.Hostname(), "www.")), nil
}

// clientIPForFederationRL matches clientIPForAuthRateLimit so federation inbox rate limits
// cannot be bypassed by spoofing X-Forwarded-For when GLIPZ_TRUSTED_PROXY_CIDRS is set.
func (s *Server) clientIPForFederationRL(r *http.Request) string {
	return s.clientIPForAuthRateLimit(r)
}

const federationInboxRatePerMinute = 180

// federationInboxPostRateExceeded applies a lightweight Redis-backed rate limit to inbox POST requests.
// Requests are allowed through if Redis is unavailable.
func (s *Server) federationInboxPostRateExceeded(r *http.Request) bool {
	ip := s.clientIPForFederationRL(r)
	if ip == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	slot := time.Now().Unix() / 60
	key := fmt.Sprintf("rl:federation:inbox:%s:%d", ip, slot)
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return s.cfg.FederationInboxRateLimitFailClosed
	}
	if n == 1 {
		_ = s.rdb.Expire(ctx, key, 3*time.Minute).Err()
	}
	return n > federationInboxRatePerMinute
}
