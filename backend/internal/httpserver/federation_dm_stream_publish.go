package httpserver

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
)

func (s *Server) publishFederationDMStreamEvent(ctx context.Context, recipientID uuid.UUID, kind string, threadID uuid.UUID, remoteAcct string) {
	if recipientID == uuid.Nil || threadID == uuid.Nil {
		return
	}
	m := map[string]any{
		"v":            1,
		"kind":         kind,
		"thread_id":    threadID.String(),
		"sender_handle": remoteAcct,
		"sender_display_name": remoteAcct,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	ch := redisDMUserChannel(recipientID)
	if err := s.rdb.Publish(ctx, ch, string(b)).Err(); err != nil {
		log.Printf("redis dm Publish %s: %v", ch, err)
	}
}

func (s *Server) publishFederationNotifyEvent(ctx context.Context, recipientID uuid.UUID, kind string, remoteAcct string) {
	if recipientID == uuid.Nil {
		return
	}
	m := map[string]any{
		"v":                  1,
		"id":                 uuid.NewString(),
		"kind":               kind,
		"actor_handle":       remoteAcct,
		"actor_display_name": remoteAcct,
		"created_at":         time.Now().UTC().Format(time.RFC3339),
		"read_at":            nil,
		"subject_post_id":    nil,
		"actor_post_id":      nil,
		"subject_author_handle": nil,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	ch := redisNotifyUserChannel(recipientID)
	if err := s.rdb.Publish(ctx, ch, string(b)).Err(); err != nil {
		log.Printf("redis notify Publish %s: %v", ch, err)
	}
}

