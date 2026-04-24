package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"glipz.io/backend/internal/repo"
)

func (s *Server) handlePublicFederatedIncomingByActor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("actor"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("acct"))
	}
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_acct_or_actor"})
		return
	}
	resolved, err := ResolveRemoteActor(r.Context(), raw)
	if err != nil {
		st := resolveErrorHTTPStatus(err)
		writeJSON(w, st, map[string]string{"error": ResolveFailureAPIError(err)})
		return
	}
	var rows []repo.FederatedIncomingPost
	if uid, ok := userIDFrom(r.Context()); ok {
		rows, err = s.db.ListFederatedIncomingPublicByActorIRIForViewer(r.Context(), uid, resolved.ActorID, 50)
	} else {
		rows, err = s.db.ListFederatedIncomingPublicByActorIRI(r.Context(), resolved.ActorID, 50)
	}
	if err != nil {
		writeServerError(w, "ListFederatedIncomingPublicByActorIRI", err)
		return
	}
	items := make([]feedItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.federatedIncomingToFeedItem(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePublicFederatedIncomingStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("actor"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("acct"))
	}
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_acct_or_actor"})
		return
	}
	resolved, err := ResolveRemoteActor(r.Context(), raw)
	if err != nil {
		st := resolveErrorHTTPStatus(err)
		writeJSON(w, st, map[string]string{"error": ResolveFailureAPIError(err)})
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
	ch := redisFederatedIncomingActorChannel(resolved.ActorID)
	pubsub := s.rdb.Subscribe(ctx, ch)
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
			if _, err := fmt.Fprintf(w, "event: federated_incoming\ndata: %s\n\n", msg.Payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handlePublicRemoteProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	acct := strings.TrimSpace(r.URL.Query().Get("acct"))
	actor := strings.TrimSpace(r.URL.Query().Get("actor"))
	raw := actor
	if raw == "" {
		raw = acct
	}
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_acct_or_actor"})
		return
	}
	disp, err := FetchRemoteActorDisplay(r.Context(), raw)
	if err != nil {
		st := resolveErrorHTTPStatus(err)
		writeJSON(w, st, map[string]string{"error": ResolveFailureAPIError(err)})
		return
	}
	writeJSON(w, http.StatusOK, disp)
}

func (s *Server) handlePublicRemoteActorPosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	actorID := strings.TrimSpace(r.URL.Query().Get("actor"))
	if actorID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_actor"})
		return
	}
	resolved, err := ResolveRemoteActor(r.Context(), actorID)
	if err != nil {
		st := resolveErrorHTTPStatus(err)
		writeJSON(w, st, map[string]string{"error": ResolveFailureAPIError(err)})
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, resolved.PostsURL, nil)
	if err != nil {
		writeServerError(w, "remote posts request", err)
		return
	}
	res, err := federationHTTP.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "profile_unreachable"})
		return
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		writeServerError(w, "remote posts read", err)
		return
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("remote_posts_http_%d", res.StatusCode)})
		return
	}
	var payload struct {
		Items []federationPublicPost `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "profile_invalid_json"})
		return
	}
	items := make([]feedItem, 0, len(payload.Items))
	for _, row := range payload.Items {
		items = append(items, feedItem{
			ID:              "federated:" + row.ID,
			UserEmail:       "fed+" + resolved.Acct,
			UserHandle:      resolved.Acct,
			UserDisplayName: resolved.Name,
			UserAvatarURL:   resolved.IconURL,
			Caption:         row.Caption,
			MediaType:       row.MediaType,
			MediaURLs:       append([]string(nil), row.MediaURLs...),
			IsNSFW:          row.IsNSFW,
			CreatedAt:       row.PublishedAt,
			VisibleAt:       row.PublishedAt,
			Poll:            buildFederationFeedPoll(row.Poll),
			LikeCount:       row.LikeCount,
			FeedEntryID:     "federated:" + row.ID,
			IsFederated:     true,
			RemoteObjectURL: row.URL,
			RemoteActorURL:  resolved.ProfileURL,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePublicFederatedIncomingPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	idRaw := strings.TrimSpace(chi.URLParam(r, "id"))
	if idRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	parsed, err := uuid.Parse(idRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	row, err := s.db.GetFederatedIncomingByID(r.Context(), parsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "GetFederatedIncomingByID", err)
		return
	}
	pollViewer := uuid.Nil
	if uid, ok := userIDFrom(r.Context()); ok {
		pollViewer = uid
		hidden, errH := s.federatedIncomingHiddenFromViewer(r.Context(), uid, row)
		if errH != nil {
			writeServerError(w, "federatedIncomingHiddenFromViewer", errH)
			return
		}
		if hidden {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
	}
	rows := []repo.FederatedIncomingPost{row}
	if err := s.db.AttachPollsToFederatedIncoming(r.Context(), pollViewer, rows); err != nil {
		writeServerError(w, "AttachPollsToFederatedIncoming", err)
		return
	}
	item := s.federatedIncomingToFeedItem(rows[0])
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) handleFederatedIncomingFeedItemGET(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	idRaw := strings.TrimSpace(chi.URLParam(r, "incomingID"))
	if idRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	parsed, err := uuid.Parse(idRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	row, err := s.db.GetFederatedIncomingByID(r.Context(), parsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "GetFederatedIncomingByID", err)
		return
	}
	if row.RecipientUserID != nil && *row.RecipientUserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	hidden, errH := s.federatedIncomingHiddenFromViewer(r.Context(), uid, row)
	if errH != nil {
		writeServerError(w, "federatedIncomingHiddenFromViewer", errH)
		return
	}
	if hidden {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	rows := []repo.FederatedIncomingPost{row}
	if err := s.db.AttachPollsToFederatedIncoming(r.Context(), uid, rows); err != nil {
		writeServerError(w, "AttachPollsToFederatedIncoming", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.federatedIncomingToFeedItem(rows[0])})
}

func (s *Server) federatedIncomingThreadItems(ctx context.Context, viewerID uuid.UUID, row repo.FederatedIncomingPost) ([]feedItem, error) {
	rootID := "federated:" + row.ID.String()
	objectIRI := strings.TrimSpace(row.ObjectIRI)
	localRows, err := s.db.ListPostsByReplyToRemoteObjectIRI(ctx, viewerID, objectIRI, 100)
	if err != nil {
		return nil, err
	}
	if len(localRows) > 0 {
		if err := s.attachPostTimelineMetadata(ctx, viewerID, localRows); err != nil {
			return nil, err
		}
	}
	fedRows, err := s.db.ListFederatedIncomingRepliesByObjectIRI(ctx, viewerID, objectIRI, 100)
	if err != nil {
		return nil, err
	}
	items := make([]feedItem, 0, len(localRows)+len(fedRows))
	badgeMap, err := s.userBadgeMap(ctx, func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(localRows))
		for _, row := range localRows {
			ids = append(ids, row.UserID)
		}
		return ids
	}())
	if err != nil {
		return nil, err
	}
	for _, localRow := range localRows {
		it := s.postRowToFeedItem(ctx, localRow, viewerID, badgeMap)
		it.ReplyToPostID = rootID
		items = append(items, it)
	}
	for _, fedRow := range fedRows {
		it := s.federatedIncomingToFeedItem(fedRow)
		it.ReplyToPostID = rootID
		items = append(items, it)
	}
	return items, nil
}

func (s *Server) handlePublicFederatedIncomingThread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	idRaw := strings.TrimSpace(chi.URLParam(r, "id"))
	if idRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	parsed, err := uuid.Parse(idRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	row, err := s.db.GetFederatedIncomingByID(r.Context(), parsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "GetFederatedIncomingByID", err)
		return
	}
	if uid, ok := userIDFrom(r.Context()); ok {
		hidden, errH := s.federatedIncomingHiddenFromViewer(r.Context(), uid, row)
		if errH != nil {
			writeServerError(w, "federatedIncomingHiddenFromViewer", errH)
			return
		}
		if hidden {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
	}
	items, err := s.federatedIncomingThreadItems(r.Context(), uuid.Nil, row)
	if err != nil {
		writeServerError(w, "federatedIncomingThreadItems", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFederatedIncomingThreadGET(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	idRaw := strings.TrimSpace(chi.URLParam(r, "incomingID"))
	if idRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	parsed, err := uuid.Parse(idRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	row, err := s.db.GetFederatedIncomingByID(r.Context(), parsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "GetFederatedIncomingByID", err)
		return
	}
	if row.RecipientUserID != nil && *row.RecipientUserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	hidden, errH := s.federatedIncomingHiddenFromViewer(r.Context(), uid, row)
	if errH != nil {
		writeServerError(w, "federatedIncomingHiddenFromViewer", errH)
		return
	}
	if hidden {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	items, err := s.federatedIncomingThreadItems(r.Context(), uid, row)
	if err != nil {
		writeServerError(w, "federatedIncomingThreadItems", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
