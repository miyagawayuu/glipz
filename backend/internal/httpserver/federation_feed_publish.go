package httpserver

import (
	"context"
	"encoding/json"

	"glipz.io/backend/internal/repo"
)

func (s *Server) publishFederatedIncomingUpsertByObjectIRI(ctx context.Context, objectIRI string) {
	row, err := s.db.GetFederatedIncomingByObjectIRI(ctx, objectIRI)
	if err != nil {
		return
	}
	b, _ := json.Marshal(map[string]any{
		"v":          1,
		"kind":       "federated_post_upsert",
		"incoming_id": row.ID.String(),
	})
	s.publishFederatedIncomingFeedEventJSON(ctx, b, row.ActorIRI, row.RecipientUserID)
}

func (s *Server) publishFederatedIncomingDelete(ctx context.Context, row repo.FederatedIncomingPost) {
	b, _ := json.Marshal(map[string]any{
		"v":          1,
		"kind":       "federated_post_deleted",
		"incoming_id": row.ID.String(),
	})
	s.publishFederatedIncomingFeedEventJSON(ctx, b, row.ActorIRI, row.RecipientUserID)
}
