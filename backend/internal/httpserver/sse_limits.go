package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	sseMaxConnectionAge = 30 * time.Minute
	ssePublicIPMax      = 6
	sseAuthIPMax        = 24
	// One signed-in tab can hold feed, notification, and DM streams.
	// Allow a few tabs plus short reconnect overlap before rate limiting.
	sseAuthUserMax = 12
)

var sseConnectionCounts = struct {
	sync.Mutex
	byIP   map[string]int
	byUser map[string]int
}{
	byIP:   map[string]int{},
	byUser: map[string]int{},
}

func (s *Server) acquireSSEConnection(w http.ResponseWriter, r *http.Request, userID *uuid.UUID) (context.Context, func(), bool) {
	ip := s.clientIPForAuthRateLimit(r)
	userKey := ""
	ipMax := ssePublicIPMax
	if userID != nil && *userID != uuid.Nil {
		userKey = userID.String()
		ipMax = sseAuthIPMax
	}

	sseConnectionCounts.Lock()
	if ip != "" && sseConnectionCounts.byIP[ip] >= ipMax {
		sseConnectionCounts.Unlock()
		writeSSERateLimited(w)
		return nil, nil, false
	}
	if userKey != "" && sseConnectionCounts.byUser[userKey] >= sseAuthUserMax {
		sseConnectionCounts.Unlock()
		writeSSERateLimited(w)
		return nil, nil, false
	}
	if ip != "" {
		sseConnectionCounts.byIP[ip]++
	}
	if userKey != "" {
		sseConnectionCounts.byUser[userKey]++
	}
	sseConnectionCounts.Unlock()

	sharedKeys := make([]string, 0, 2)
	if s.rdb != nil {
		if ip != "" {
			key := sseSharedKey("ip", ip)
			n, err := s.acquireSharedSSESlot(r.Context(), key)
			if err != nil {
				log.Printf("sse shared ip acquire: %v", err)
				if s.cfg.AuthRateLimitFailClosed {
					releaseLocalSSE(ip, userKey)
					writeSSERateLimited(w)
					return nil, nil, false
				}
			} else {
				sharedKeys = append(sharedKeys, key)
				if n > int64(ipMax) {
					s.releaseSharedSSESlots(r.Context(), sharedKeys)
					releaseLocalSSE(ip, userKey)
					writeSSERateLimited(w)
					return nil, nil, false
				}
			}
		}
		if userKey != "" {
			key := sseSharedKey("user", userKey)
			n, err := s.acquireSharedSSESlot(r.Context(), key)
			if err != nil {
				log.Printf("sse shared user acquire: %v", err)
				if s.cfg.AuthRateLimitFailClosed {
					s.releaseSharedSSESlots(r.Context(), sharedKeys)
					releaseLocalSSE(ip, userKey)
					writeSSERateLimited(w)
					return nil, nil, false
				}
			} else {
				sharedKeys = append(sharedKeys, key)
				if n > int64(sseAuthUserMax) {
					s.releaseSharedSSESlots(r.Context(), sharedKeys)
					releaseLocalSSE(ip, userKey)
					writeSSERateLimited(w)
					return nil, nil, false
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), sseMaxConnectionAge)
	release := func() {
		cancel()
		s.releaseSharedSSESlots(context.Background(), sharedKeys)
		releaseLocalSSE(ip, userKey)
	}
	return ctx, release, true
}

func releaseLocalSSE(ip, userKey string) {
	sseConnectionCounts.Lock()
	defer sseConnectionCounts.Unlock()
	if ip != "" {
		sseConnectionCounts.byIP[ip]--
		if sseConnectionCounts.byIP[ip] <= 0 {
			delete(sseConnectionCounts.byIP, ip)
		}
	}
	if userKey != "" {
		sseConnectionCounts.byUser[userKey]--
		if sseConnectionCounts.byUser[userKey] <= 0 {
			delete(sseConnectionCounts.byUser, userKey)
		}
	}
}

func sseSharedKey(kind, value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sse:conn:" + kind + ":" + hex.EncodeToString(sum[:])
}

func (s *Server) acquireSharedSSESlot(ctx context.Context, key string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		_ = s.rdb.Expire(ctx, key, sseMaxConnectionAge+time.Minute).Err()
	}
	return n, nil
}

func (s *Server) releaseSharedSSESlots(ctx context.Context, keys []string) {
	if s.rdb == nil || len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, key := range keys {
		_ = s.rdb.Decr(ctx, key).Err()
	}
}

func writeSSERateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", fmt.Sprintf("%.0f", time.Minute.Seconds()))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
}
