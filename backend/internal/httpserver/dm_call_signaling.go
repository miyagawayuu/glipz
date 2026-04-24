package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"glipz.io/backend/internal/authjwt"
	"nhooyr.io/websocket"
)

const (
	dmCallWSMaxMessageBytes   = 64 * 1024
	dmCallMaxICECandidates    = 400
	dmCallICEPerSecondLimit   = 25
	dmCallWSPingInterval      = 25 * time.Second
	dmCallWSIdleTimeout       = 90 * time.Second
	dmCallWSWriteTimeout      = 10 * time.Second
)

type dmCallSignalMessage struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

func (m dmCallSignalMessage) validType() bool {
	switch m.Type {
	case "offer", "answer", "ice_candidate", "hangup", "ping":
		return true
	default:
		return false
	}
}

func originHostsFromConfig(origins []string) []string {
	out := make([]string, 0, len(origins)+4)
	for _, raw := range origins {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || strings.TrimSpace(u.Host) == "" {
			continue
		}
		out = append(out, u.Host)
	}
	// Common dev/capacitor origins:
	out = append(out, "localhost", "127.0.0.1")
	return out
}

func (s *Server) handleDMCallSignalingWS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Auth:
	// - Prefer a dedicated short-lived ws_token (query string), because browsers can't set Authorization headers for WebSockets.
	// - Allow Authorization: Bearer for non-browser clients.
	var uid uuid.UUID
	if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
		claims, err := authjwt.Parse(s.secret, tok)
		if err != nil || claims.Purpose != authjwt.PurposeDMCallWS {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		u, err := uuid.Parse(strings.TrimSpace(claims.Subject))
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		uid = u
	} else {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		u, ok := s.principalForAccess(ctx, raw)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		uid = u
	}
	suspended, err := s.db.IsUserSuspended(ctx, uid)
	if err != nil {
		writeServerError(w, "auth IsUserSuspended ws", err)
		return
	}
	if suspended {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_suspended"})
		return
	}

	callID := strings.TrimSpace(chi.URLParam(r, "callID"))
	if _, err := uuid.Parse(callID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_call_id"})
		return
	}

	sess, found, err := s.getCallSession(ctx, callID)
	if err != nil {
		writeServerError(w, "getCallSession", err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		writeJSON(w, http.StatusGone, map[string]string{"error": "expired"})
		return
	}

	// Participant check.
	if !strings.EqualFold(sess.UserAID, uid.String()) && !strings.EqualFold(sess.UserBID, uid.String()) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	peerID := uuid.Nil
	if strings.EqualFold(sess.UserAID, uid.String()) {
		peerID, _ = uuid.Parse(sess.UserBID)
	} else {
		peerID, _ = uuid.Parse(sess.UserAID)
	}

	// Policy check (caller -> callee).
	callerUUID, _ := uuid.Parse(sess.CallerID)
	if callerUUID != uuid.Nil {
		allowed, err := s.canReceiveDMCall(ctx, callerUUID, peerID)
		if err != nil {
			writeServerError(w, "canReceiveDMCall ws", err)
			return
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "dm_call_not_allowed"})
			return
		}
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: originHostsFromConfig(s.cfg.FrontendOrigins),
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusInternalError, "server_error")

	inboxKey := dmCallSignalInboxKey(callID, uid)
	peerInboxKey := dmCallSignalInboxKey(callID, peerID)

	pubsub := s.rdb.Subscribe(ctx, inboxKey)
	defer func() { _ = pubsub.Close() }()

	// Writer: forward redis messages to ws.
	writerCtx, writerCancel := context.WithCancel(ctx)
	defer writerCancel()
	go func() {
		ch := pubsub.Channel()
		for {
			select {
			case <-writerCtx.Done():
				return
			case msg, ok := <-ch:
				if !ok || msg == nil || msg.Payload == "" {
					continue
				}
				wctx, cancel := context.WithTimeout(writerCtx, dmCallWSWriteTimeout)
				_ = c.Write(wctx, websocket.MessageText, []byte(msg.Payload))
				cancel()
			}
		}
	}()

	// Reader loop with simple limits.
	var (
		offerSeen   bool
		answerSeen  bool
		iceTotal    int
		iceWinStart = time.Now()
		iceWinCount = 0
		lastRead    = time.Now()
	)

	pingTicker := time.NewTicker(dmCallWSPingInterval)
	defer pingTicker.Stop()

	for {
		// Idle timeout: if we haven't read for a while, close.
		if time.Since(lastRead) > dmCallWSIdleTimeout {
			_ = c.Close(websocket.StatusNormalClosure, "idle_timeout")
			return
		}

		// Non-blocking ping.
		select {
		case <-ctx.Done():
			_ = c.Close(websocket.StatusNormalClosure, "")
			return
		case <-pingTicker.C:
			wctx, cancel := context.WithTimeout(ctx, dmCallWSWriteTimeout)
			_ = c.Write(wctx, websocket.MessageText, []byte(`{"type":"ping"}`))
			cancel()
		default:
		}

		mtype, data, err := c.Read(ctx)
		if err != nil {
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				return
			}
			return
		}
		lastRead = time.Now()
		if mtype != websocket.MessageText || len(data) == 0 {
			continue
		}
		if len(data) > dmCallWSMaxMessageBytes {
			_ = c.Close(websocket.StatusMessageTooBig, "message_too_big")
			return
		}

		var msg dmCallSignalMessage
		if err := json.Unmarshal(data, &msg); err != nil || !msg.validType() {
			continue
		}
		switch msg.Type {
		case "ping":
			// Client ping; ignore (server sends pings too).
			continue
		case "offer":
			if offerSeen || answerSeen {
				continue
			}
			offerSeen = true
		case "answer":
			if !offerSeen || answerSeen {
				continue
			}
			answerSeen = true
		case "ice_candidate":
			iceTotal++
			if iceTotal > dmCallMaxICECandidates {
				_ = c.Close(websocket.StatusPolicyViolation, "too_many_ice_candidates")
				return
			}
			if time.Since(iceWinStart) > time.Second {
				iceWinStart = time.Now()
				iceWinCount = 0
			}
			iceWinCount++
			if iceWinCount > dmCallICEPerSecondLimit {
				continue
			}
		case "hangup":
			// Forward and end.
		}

		// Forward to peer via redis. Do not log SDP/ICE payloads.
		if err := s.rdb.Publish(ctx, peerInboxKey, string(data)).Err(); err != nil {
			// If redis publish fails, close to avoid desync.
			_ = c.Close(websocket.StatusInternalError, "signaling_failed")
			return
		}
		if msg.Type == "hangup" {
			_ = c.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

