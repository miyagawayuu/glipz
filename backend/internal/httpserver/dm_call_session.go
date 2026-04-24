package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	redisDMCallPrefix       = "glipz:dm:call:"
	redisDMCallSessPrefix   = redisDMCallPrefix + "sess:"
	redisDMCallActiveThread = redisDMCallPrefix + "active_thread:"
)

type dmCallSession struct {
	CallID     string    `json:"call_id"`
	ThreadID   string    `json:"thread_id"`
	UserAID    string    `json:"user_a_id"`
	UserBID    string    `json:"user_b_id"`
	CallerID   string    `json:"caller_id"`
	Mode       string    `json:"mode"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func dmCallActiveThreadKey(threadID uuid.UUID) string {
	return redisDMCallActiveThread + threadID.String()
}

func dmCallSessKey(callID string) string {
	return redisDMCallSessPrefix + strings.TrimSpace(callID)
}

func dmCallSignalInboxKey(callID string, userID uuid.UUID) string {
	return fmt.Sprintf("%ssig:%s:%s", redisDMCallPrefix, strings.TrimSpace(callID), userID.String())
}

func (s *Server) getCallSession(ctx context.Context, callID string) (dmCallSession, bool, error) {
	raw, err := s.rdb.Get(ctx, dmCallSessKey(callID)).Result()
	if err != nil {
		if strings.Contains(err.Error(), "redis: nil") {
			return dmCallSession{}, false, nil
		}
		return dmCallSession{}, false, err
	}
	var sess dmCallSession
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return dmCallSession{}, false, err
	}
	return sess, true, nil
}

func (s *Server) putCallSession(ctx context.Context, sess dmCallSession) error {
	b, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	ttl := time.Until(sess.ExpiresAt)
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	return s.rdb.Set(ctx, dmCallSessKey(sess.CallID), string(b), ttl).Err()
}

// getOrCreateCallSession returns a stable call_id while the thread is active (SETNX lock).
// The first caller to acquire the thread lock becomes the session CallerID.
func (s *Server) getOrCreateCallSession(ctx context.Context, threadID uuid.UUID, uid, peerID uuid.UUID, mode string) (dmCallSession, string, error) {
	now := time.Now().UTC()
	ttl := time.Duration(s.cfg.TurnTTLSeconds) * time.Second
	if ttl < 60*time.Second {
		ttl = 10 * time.Minute
	}
	expiresAt := now.Add(ttl)

	lockKey := dmCallActiveThreadKey(threadID)
	callID := uuid.NewString()

	ok, err := s.rdb.SetNX(ctx, lockKey, callID, ttl).Result()
	if err != nil {
		return dmCallSession{}, "", err
	}
	if !ok {
		// Existing active call: reuse call_id.
		existing, err := s.rdb.Get(ctx, lockKey).Result()
		if err != nil {
			return dmCallSession{}, "", err
		}
		callID = strings.TrimSpace(existing)
		sess, found, err := s.getCallSession(ctx, callID)
		if err != nil {
			return dmCallSession{}, "", err
		}
		if found {
			role := "callee"
			if strings.EqualFold(sess.CallerID, uid.String()) {
				role = "caller"
			}
			return sess, role, nil
		}
		// Fallthrough: missing session data; recreate.
	}

	sess := dmCallSession{
		CallID:    callID,
		ThreadID:  threadID.String(),
		UserAID:   uid.String(),
		UserBID:   peerID.String(),
		CallerID:  uid.String(),
		Mode:      mode,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}
	if err := s.putCallSession(ctx, sess); err != nil {
		return dmCallSession{}, "", err
	}
	return sess, "caller", nil
}

