package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const redisNotifyUserPrefix = "glipz:notify:user:"

func redisNotifyUserChannel(uid uuid.UUID) string {
	return redisNotifyUserPrefix + uid.String()
}

func (s *Server) publishNotifyUserEvent(ctx context.Context, recipientID, notificationID uuid.UUID) {
	m, err := s.db.NotificationJSONForRecipient(ctx, notificationID, recipientID)
	if err != nil {
		log.Printf("NotificationJSONForRecipient: %v", err)
		return
	}
	s.enrichNotificationListItems([]map[string]any{m})
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	ch := redisNotifyUserChannel(recipientID)
	if err := s.rdb.Publish(ctx, ch, string(b)).Err(); err != nil {
		log.Printf("redis notify Publish %s: %v", ch, err)
	}
	if payload, ok := s.webPushNotificationFromSocial(m); ok {
		s.queueWebPush(recipientID, payload)
	}
}

func (s *Server) handleNotifyStream(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	flusher, okFlush := w.(http.Flusher)
	if !okFlush {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming_unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	chName := redisNotifyUserChannel(uid)
	pubsub := s.rdb.Subscribe(ctx, chName)
	defer func() { _ = pubsub.Close() }()

	if _, err := fmt.Fprintf(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	msgCh := pubsub.Channel()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			if msg == nil || msg.Payload == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: notify\ndata: %s\n\n", msg.Payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func truncateNotificationText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
}

func mediaKeyToPublicURL(s *Server, mediaType, objectKey string) string {
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(mt, "image/") || strings.HasPrefix(mt, "video/") {
		return s.glipzProtocolPublicMediaURL(key)
	}
	return ""
}

// enrichNotificationListItems trims captions for list views, adds media URLs, and removes internal-only keys.
func (s *Server) enrichNotificationListItems(items []map[string]any) {
	const capMax = 280
	for _, m := range items {
		if k, ok := m["actor_avatar_key"].(string); ok && k != "" {
			m["actor_avatar_url"] = s.glipzProtocolPublicMediaURL(k)
		} else {
			m["actor_avatar_url"] = nil
		}
		delete(m, "actor_avatar_key")

		if rawID, _ := m["actor_user_id"].(string); rawID != "" {
			if actorID, err := uuid.Parse(rawID); err == nil {
				m["actor_badges"] = userBadgesJSON(s.visibleUserBadges(actorID, toStringSlice(m["actor_badges"])))
			}
		}
		delete(m, "actor_user_id")

		if k, ok := m["subject_author_avatar_key"].(string); ok && k != "" {
			m["subject_author_avatar_url"] = s.glipzProtocolPublicMediaURL(k)
		} else {
			m["subject_author_avatar_url"] = nil
		}
		delete(m, "subject_author_avatar_key")

		if rawID, _ := m["subject_author_user_id"].(string); rawID != "" {
			if authorID, err := uuid.Parse(rawID); err == nil {
				m["subject_author_badges"] = userBadgesJSON(s.visibleUserBadges(authorID, toStringSlice(m["subject_author_badges"])))
			}
		}
		delete(m, "subject_author_user_id")

		if c, ok := m["subject_caption"].(string); ok && c != "" {
			m["subject_caption_preview"] = truncateNotificationText(c, capMax)
		}
		delete(m, "subject_caption")

		subMT, _ := m["subject_media_type"].(string)
		subKey, _ := m["subject_object_key"].(string)
		if u := mediaKeyToPublicURL(s, subMT, subKey); u != "" {
			m["subject_media_url"] = u
		} else {
			m["subject_media_url"] = nil
		}
		delete(m, "subject_media_type")
		delete(m, "subject_object_key")

		if c, ok := m["actor_post_caption"].(string); ok && c != "" {
			m["actor_post_caption_preview"] = truncateNotificationText(c, capMax)
		}
		delete(m, "actor_post_caption")

		apMT, _ := m["actor_post_media_type"].(string)
		apKey, _ := m["actor_post_object_key"].(string)
		if u := mediaKeyToPublicURL(s, apMT, apKey); u != "" {
			m["actor_post_media_url"] = u
		} else {
			m["actor_post_media_url"] = nil
		}
		delete(m, "actor_post_media_type")
		delete(m, "actor_post_object_key")
	}
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	kind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("kind")))
	if kind != "" && kind != "reply" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_kind"})
		return
	}
	items, err := s.db.ListNotifications(r.Context(), uid, 50, kind)
	if err != nil {
		writeServerError(w, "ListNotifications", err)
		return
	}
	s.enrichNotificationListItems(items)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleNotificationUnreadCount(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	n, err := s.db.NotificationUnreadCount(r.Context(), uid)
	if err != nil {
		writeServerError(w, "NotificationUnreadCount", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := s.db.MarkAllNotificationsRead(r.Context(), uid); err != nil {
		writeServerError(w, "MarkAllNotificationsRead", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
