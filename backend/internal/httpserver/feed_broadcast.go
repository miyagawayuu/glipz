package httpserver

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

// runFeedBroadcastScheduler emits feed updates after scheduled posts become visible.
func (s *Server) runFeedBroadcastScheduler(ctx context.Context) {
	t := time.NewTicker(12 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			batch, err := s.db.ClaimPendingFeedBroadcasts(ctx, 30)
			if err != nil {
				log.Printf("ClaimPendingFeedBroadcasts: %v", err)
				continue
			}
			for _, b := range batch {
				pl, _ := json.Marshal(map[string]any{
					"v":         1,
					"kind":      "post_created",
					"post_id":   b.PostID.String(),
					"author_id": b.AuthorID.String(),
				})
				s.publishFeedEventJSON(context.Background(), pl, b.AuthorID, b.Visibility)
				if b.Visibility == repo.PostVisibilityPublic {
					aid, pid := b.AuthorID, b.PostID
					go func(authorID, postID uuid.UUID) {
						ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
						defer cancel()
						s.deliverFederationCreate(ctx, authorID, postID)
					}(aid, pid)
				}
			}
		}
	}
}
