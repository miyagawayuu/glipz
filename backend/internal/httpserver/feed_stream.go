package httpserver

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

const (
	redisFeedGlobal   = "glipz:feed:global"
	redisFeedLoggedIn = "glipz:feed:logged-in"
)

func redisFederatedIncomingActorChannel(actorIRI string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(actorIRI)))
	return "glipz:federation:incoming:actor:" + fmt.Sprintf("%x", sum[:12])
}

func redisFeedUserChannel(uid uuid.UUID) string {
	return "glipz:feed:user:" + uid.String()
}

// publishFeedEventGlobalOnly publishes top-level post updates to the global feed channel.
func (s *Server) publishFeedEventGlobalOnly(ctx context.Context, payload []byte) {
	if err := s.rdb.Publish(ctx, redisFeedGlobal, string(payload)).Err(); err != nil {
		log.Printf("redis Publish %s: %v", redisFeedGlobal, err)
	}
}

func (s *Server) publishFeedEventLoggedInOnly(ctx context.Context, payload []byte) {
	if err := s.rdb.Publish(ctx, redisFeedLoggedIn, string(payload)).Err(); err != nil {
		log.Printf("redis Publish %s: %v", redisFeedLoggedIn, err)
	}
}

func (s *Server) publishFeedEventToUsers(ctx context.Context, payload []byte, userIDs []uuid.UUID) {
	sent := map[uuid.UUID]bool{}
	for _, uid := range userIDs {
		if uid == uuid.Nil || sent[uid] {
			continue
		}
		sent[uid] = true
		ch := redisFeedUserChannel(uid)
		if err := s.rdb.Publish(ctx, ch, string(payload)).Err(); err != nil {
			log.Printf("redis Publish %s: %v", ch, err)
		}
	}
}

// publishFeedEventJSON routes top-level post updates over Redis Pub/Sub based on visibility.
func (s *Server) publishFeedEventJSON(ctx context.Context, payload []byte, authorID uuid.UUID, visibility string) {
	visibility = strings.TrimSpace(strings.ToLower(visibility))
	if visibility == "" {
		visibility = repo.PostVisibilityPublic
	}
	if visibility == repo.PostVisibilityPublic {
		s.publishFeedEventGlobalOnly(ctx, payload)
	}
	if visibility == repo.PostVisibilityLoggedIn {
		s.publishFeedEventLoggedInOnly(ctx, payload)
	}
	if visibility == repo.PostVisibilityPrivate {
		s.publishFeedEventToUsers(ctx, payload, []uuid.UUID{authorID})
		return
	}
	ids, err := s.db.ListFollowerIDs(ctx, authorID)
	if err != nil {
		log.Printf("ListFollowerIDs: %v", err)
		s.publishFeedEventToUsers(ctx, payload, []uuid.UUID{authorID})
		return
	}
	ids = append(ids, authorID)
	s.publishFeedEventToUsers(ctx, payload, ids)
}

// publishFederatedIncomingFeedEventJSON publishes updates for inbound federated posts.
// Public posts go to the global channel and to local users who follow the remote actor.
// When recipientUserID is present, publish only to that user's following channel.
func (s *Server) publishFederatedIncomingFeedEventJSON(ctx context.Context, payload []byte, actorIRI string, recipientUserID *uuid.UUID) {
	if recipientUserID != nil {
		ch := redisFeedUserChannel(*recipientUserID)
		if err := s.rdb.Publish(ctx, ch, string(payload)).Err(); err != nil {
			log.Printf("redis Publish %s: %v", ch, err)
		}
		return
	}
	if err := s.rdb.Publish(ctx, redisFeedGlobal, string(payload)).Err(); err != nil {
		log.Printf("redis Publish %s: %v", redisFeedGlobal, err)
	}
	if strings.TrimSpace(actorIRI) != "" {
		ch := redisFederatedIncomingActorChannel(actorIRI)
		if err := s.rdb.Publish(ctx, ch, string(payload)).Err(); err != nil {
			log.Printf("redis Publish %s: %v", ch, err)
		}
	}
	ids, err := s.db.ListAcceptedRemoteFollowLocalUserIDs(ctx, actorIRI)
	if err != nil {
		log.Printf("ListAcceptedRemoteFollowLocalUserIDs: %v", err)
		return
	}
	for _, uid := range ids {
		ch := redisFeedUserChannel(uid)
		if err := s.rdb.Publish(ctx, ch, string(payload)).Err(); err != nil {
			log.Printf("redis Publish %s: %v", ch, err)
		}
	}
}

func (s *Server) handleFeedStream(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	scope := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scope")))
	var channels []string
	if scope == "following" {
		channels = []string{redisFeedUserChannel(uid)}
	} else {
		channels = []string{redisFeedGlobal, redisFeedLoggedIn}
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
	pubsub := s.rdb.Subscribe(ctx, channels...)
	defer func() { _ = pubsub.Close() }()
	streamName := "feed_global"
	if scope == "following" {
		streamName = "feed_following"
	}
	trackSSEOpen(streamName)
	defer trackSSEClose(streamName)

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
			if _, err := fmt.Fprintf(w, "event: feed\ndata: %s\n\n", msg.Payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handlePublicFeedStream(w http.ResponseWriter, r *http.Request) {
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
	pubsub := s.rdb.Subscribe(ctx, redisFeedGlobal)
	defer func() { _ = pubsub.Close() }()
	trackSSEOpen("feed_public")
	defer trackSSEClose("feed_public")

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
			if _, err := fmt.Fprintf(w, "event: feed\ndata: %s\n\n", msg.Payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handlePostFeedItemGET(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	row, err := s.db.PostRowForViewer(r.Context(), uid, postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostRowForViewer feed-item", err)
		return
	}
	rows := []repo.PostRow{row}
	if err := s.attachPostTimelineMetadata(r.Context(), uid, rows); err != nil {
		writeServerError(w, "attachPostTimelineMetadata feed-item", err)
		return
	}
	badgeMap, err := s.userBadgeMap(r.Context(), []uuid.UUID{rows[0].UserID})
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs feed-item", err)
		return
	}
	item := s.postRowToFeedItem(r.Context(), rows[0], uid, badgeMap)
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) handlePostThreadGET(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		uid = uuid.Nil
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	_, isRoot, err := s.db.PostFeedMeta(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostFeedMeta thread", err)
		return
	}
	if !isRoot {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	readable, err := s.db.CanViewerReadPost(r.Context(), uid, postID)
	if err != nil {
		writeServerError(w, "CanViewerReadPost thread", err)
		return
	}
	if !readable {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	desc, err := s.db.ListThreadDescendants(r.Context(), uid, postID)
	if err != nil {
		writeServerError(w, "ListThreadDescendants", err)
		return
	}
	out := make([]feedItem, 0, len(desc))
	if len(desc) > 0 {
		rows := make([]repo.PostRow, len(desc))
		for i := range desc {
			rows[i] = desc[i].PostRow
		}
		if err := s.attachPostTimelineMetadata(r.Context(), uid, rows); err != nil {
			writeServerError(w, "attachPostTimelineMetadata thread", err)
			return
		}
		badgeIDs := make([]uuid.UUID, 0, len(rows))
		for i := range rows {
			badgeIDs = append(badgeIDs, rows[i].UserID)
		}
		badgeMap, err := s.userBadgeMap(r.Context(), badgeIDs)
		if err != nil {
			writeServerError(w, "ListUserBadgesByIDs thread", err)
			return
		}
		for i := range rows {
			it := s.postRowToFeedItem(r.Context(), rows[i], uid, badgeMap)
			if desc[i].ReplyToID != nil {
				it.ReplyToPostID = desc[i].ReplyToID.String()
			}
			out = append(out, it)
		}
	}
	fedReplies, err := s.db.ListFederatedIncomingRepliesByObjectIRI(r.Context(), uid, s.localPostURL(postID), 100)
	if err != nil {
		writeServerError(w, "ListFederatedIncomingRepliesByObjectIRI", err)
		return
	}
	if len(fedReplies) == 0 {
		fedReplies, err = s.db.ListFederatedIncomingRepliesByLocalPostIDSuffix(r.Context(), uid, postID, 100)
		if err != nil {
			writeServerError(w, "ListFederatedIncomingRepliesByLocalPostIDSuffix", err)
			return
		}
	}
	for _, row := range fedReplies {
		it := s.federatedIncomingToFeedItem(row)
		it.ReplyToPostID = postID.String()
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
