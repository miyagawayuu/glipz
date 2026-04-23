package httpserver

import (
	"context"
	"fmt"
	"net"
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

func clientIPForFederationRL(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = strings.TrimSpace(xff[:i])
		}
		if xff != "" {
			return xff
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

const federationInboxRatePerMinute = 180

// federationInboxPostRateExceeded applies a lightweight Redis-backed rate limit to inbox POST requests.
// Requests are allowed through if Redis is unavailable.
func (s *Server) federationInboxPostRateExceeded(r *http.Request) bool {
	ip := clientIPForFederationRL(r)
	if ip == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	slot := time.Now().Unix() / 60
	key := fmt.Sprintf("rl:federation:inbox:%s:%d", ip, slot)
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false
	}
	if n == 1 {
		_ = s.rdb.Expire(ctx, key, 3*time.Minute).Err()
	}
	return n > federationInboxRatePerMinute
}
