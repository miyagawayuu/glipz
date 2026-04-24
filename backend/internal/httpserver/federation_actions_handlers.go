package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

func (s *Server) resolveIncomingPostTarget(ctx context.Context, row repo.FederatedIncomingPost) (ResolvedRemoteActor, error) {
	raw := strings.TrimSpace(row.ActorProfileURL)
	if raw == "" {
		raw = strings.TrimSpace(row.ActorAcct)
	}
	if raw == "" {
		raw = strings.TrimSpace(row.ActorIRI)
	}
	return ResolveRemoteActor(ctx, raw)
}

func (s *Server) resolveIncomingPostEventsInbox(ctx context.Context, row repo.FederatedIncomingPost) (string, error) {
	if resolved, err := s.resolveIncomingPostTarget(ctx, row); err == nil && strings.TrimSpace(resolved.Inbox) != "" {
		return strings.TrimSpace(resolved.Inbox), nil
	}
	candidates := []string{
		strings.TrimSpace(row.ObjectIRI),
		strings.TrimSpace(row.ActorProfileURL),
		strings.TrimSpace(row.ActorIRI),
	}
	for _, raw := range candidates {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || strings.TrimSpace(u.Host) == "" {
			continue
		}
		doc, err := fetchRemoteFederationDiscovery(ctx, u.Host)
		if err != nil {
			continue
		}
		inbox := strings.TrimSpace(doc.Server.EventsURL)
		if inbox != "" {
			return inbox, nil
		}
	}
	return "", errors.New("remote federation inbox not found")
}

func (s *Server) queueDirectedFederationEvent(ctx context.Context, actorUserID, refID uuid.UUID, inboxURL string, payload federationEventEnvelope) error {
	payload.V = federationEventSchemaVersion
	if strings.TrimSpace(payload.EventID) == "" {
		payload.EventID = federationNewEventID()
	}
	return s.db.InsertFederationDeliveries(ctx, []repo.FederationDeliveryInsert{{
		AuthorUserID: actorUserID,
		PostID:       refID,
		Kind:         payload.Kind,
		InboxURL:     inboxURL,
		Payload:      repo.MustMarshalJSON(payload),
	}})
}

func federationSyntheticIncomingRepostID(actorUserID, incomingID uuid.UUID) string {
	return "federated-repost:" + actorUserID.String() + ":" + incomingID.String()
}

func (s *Server) federationSyntheticIncomingRepostURL(actorUserID uuid.UUID, row repo.FederatedIncomingPost) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(row.ObjectIRI)))
	return strings.TrimSuffix(s.federationPublicOrigin(), "/") + "/federation/reposts/" + actorUserID.String() + "/" + hex.EncodeToString(sum[:12])
}

func (s *Server) loadFederatedIncomingForAction(ctx context.Context, incomingID uuid.UUID, objectURL string) (repo.FederatedIncomingPost, error) {
	row, err := s.db.GetFederatedIncomingByID(ctx, incomingID)
	if err == nil {
		return row, nil
	}
	if strings.TrimSpace(objectURL) == "" {
		return repo.FederatedIncomingPost{}, err
	}
	return s.db.GetFederatedIncomingByObjectIRI(ctx, objectURL)
}

func (s *Server) handleFederatedToggleLike(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	incomingID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "incomingID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	row, err := s.loadFederatedIncomingForAction(r.Context(), incomingID, r.URL.Query().Get("object_url"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "loadFederatedIncomingForAction", err)
		return
	}
	if s.rejectIfFederatedIncomingHidden(w, r, uid, row) {
		return
	}
	inboxURL, err := s.resolveIncomingPostEventsInbox(r.Context(), row)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_unavailable"})
		return
	}
	actor, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload", err)
		return
	}
	liked, count, err := s.db.ToggleFederatedIncomingLike(r.Context(), uid, row.ID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ToggleFederatedIncomingLike", err)
		return
	}
	kind := "post_liked"
	if !liked {
		kind = "post_unliked"
	}
	if err := s.queueDirectedFederationEvent(r.Context(), uid, row.ID, inboxURL, federationEventEnvelope{
		V:      1,
		Kind:   kind,
		Author: actor,
		Post: &federationEventPost{
			URL:         row.ObjectIRI,
			PublishedAt: row.PublishedAt.UTC().Format(time.RFC3339),
			LikeCount:   count,
		},
	}); err != nil {
		writeServerError(w, "queueDirectedFederationEvent", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"liked": liked, "like_count": count})
}

func (s *Server) handleFederatedToggleBookmark(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	incomingID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "incomingID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	row, err := s.loadFederatedIncomingForAction(r.Context(), incomingID, r.URL.Query().Get("object_url"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "loadFederatedIncomingForAction", err)
		return
	}
	if s.rejectIfFederatedIncomingHidden(w, r, uid, row) {
		return
	}
	bookmarked, err := s.db.ToggleFederatedIncomingBookmark(r.Context(), uid, row.ID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ToggleFederatedIncomingBookmark", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookmarked": bookmarked})
}

func (s *Server) handleFederatedPollVote(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	incomingID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "incomingID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var req pollVoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	row, err := s.loadFederatedIncomingForAction(r.Context(), incomingID, r.URL.Query().Get("object_url"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "loadFederatedIncomingForAction", err)
		return
	}
	if s.rejectIfFederatedIncomingHidden(w, r, uid, row) {
		return
	}
	inboxURL, err := s.resolveIncomingPostEventsInbox(r.Context(), row)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_unavailable"})
		return
	}
	optionID, position, err := s.resolveFederatedPollChoice(r.Context(), row.ID, req.OptionID)
	if err != nil {
		if errors.Is(err, repo.ErrPollInvalidOption) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_option"})
			return
		}
		writeServerError(w, "resolveFederatedPollChoice", err)
		return
	}
	if err := s.db.CastFederatedIncomingPollVote(r.Context(), uid, row.ID, optionID); err != nil {
		switch {
		case errors.Is(err, repo.ErrPollNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "poll_not_found"})
		case errors.Is(err, repo.ErrPollClosed):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "poll_closed"})
		case errors.Is(err, repo.ErrPollAlreadyVoted):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already_voted"})
		case errors.Is(err, repo.ErrPollInvalidOption):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_option"})
		default:
			writeServerError(w, "CastFederatedIncomingPollVote", err)
		}
		return
	}
	actor, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload", err)
		return
	}
	if err := s.queueDirectedFederationEvent(r.Context(), uid, row.ID, inboxURL, federationEventEnvelope{
		V:      1,
		Kind:   "poll_voted",
		Author: actor,
		Post: &federationEventPost{
			URL:         row.ObjectIRI,
			PublishedAt: row.PublishedAt.UTC().Format(time.RFC3339),
			LikeCount:   row.LikeCount,
			Poll: &federationEventPoll{
				SelectedPosition: position,
			},
		},
	}); err != nil {
		writeServerError(w, "queueDirectedFederationEvent", err)
		return
	}
	updated, err := s.db.GetFederatedIncomingByID(r.Context(), row.ID)
	if err != nil {
		writeServerError(w, "GetFederatedIncomingByID", err)
		return
	}
	rows := []repo.FederatedIncomingPost{updated}
	if err := s.db.AttachPollsToFederatedIncoming(r.Context(), uid, rows); err != nil {
		writeServerError(w, "AttachPollsToFederatedIncoming", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": s.federatedIncomingToFeedItem(rows[0])})
}

func (s *Server) handleFederatedPostUnlock(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	incomingID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "incomingID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var req unlockPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	if req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_password"})
		return
	}
	row, err := s.loadFederatedIncomingForAction(r.Context(), incomingID, r.URL.Query().Get("object_url"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "loadFederatedIncomingForAction", err)
		return
	}
	if s.rejectIfFederatedIncomingHidden(w, r, uid, row) {
		return
	}
	unlockURL := strings.TrimSpace(row.UnlockURL)
	if unlockURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_password"})
		return
	}
	actor, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload", err)
		return
	}
	body, err := s.signedFederationPOSTJSON(r.Context(), unlockURL, federationUnlockRequest{
		EventID:    federationNewEventID(),
		ViewerAcct: actor.Acct,
		Password:   req.Password,
	})
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "wrong_password") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong_password"})
			return
		}
		if strings.Contains(err.Error(), "400") || strings.Contains(err.Error(), "no_password") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_password"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_unavailable"})
		return
	}
	var unlocked struct {
		Caption                string                      `json:"caption"`
		MediaType              string                      `json:"media_type"`
		MediaURLs              []string                    `json:"media_urls"`
		IsNSFW                 bool                        `json:"is_nsfw"`
		HasViewPassword        bool                        `json:"has_view_password"`
		ViewPasswordScope      int                         `json:"view_password_scope"`
		ViewPasswordTextRanges []viewPasswordTextRangeJSON `json:"view_password_text_ranges"`
		ContentLocked          bool                        `json:"content_locked"`
		TextLocked             bool                        `json:"text_locked"`
		MediaLocked            bool                        `json:"media_locked"`
	}
	if err := json.Unmarshal(body, &unlocked); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_invalid_response"})
		return
	}
	if err := s.db.UpsertFederatedIncomingUnlock(r.Context(), row.ID, uid, unlocked.Caption, unlocked.MediaType, unlocked.MediaURLs, unlocked.IsNSFW, time.Now().UTC().Add(postUnlockRedisTTL)); err != nil {
		writeServerError(w, "UpsertFederatedIncomingUnlock", err)
		return
	}
	writeJSON(w, http.StatusOK, unlocked)
}

func (s *Server) handleFederatedToggleRepost(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	incomingID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "incomingID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var repostComment *string
	if r.Body != nil {
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if readErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		if len(strings.TrimSpace(string(raw))) > 0 {
			var body struct {
				Comment string `json:"comment"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
				return
			}
			c := strings.TrimSpace(body.Comment)
			if c != "" {
				if len([]rune(c)) > maxRepostCommentRunes {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repost_comment_too_long"})
					return
				}
				repostComment = &c
			}
		}
	}
	row, err := s.loadFederatedIncomingForAction(r.Context(), incomingID, r.URL.Query().Get("object_url"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "loadFederatedIncomingForAction", err)
		return
	}
	if s.rejectIfFederatedIncomingHidden(w, r, uid, row) {
		return
	}
	inboxURL, err := s.resolveIncomingPostEventsInbox(r.Context(), row)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote_unavailable"})
		return
	}
	reposted, count, err := s.db.ToggleFederatedIncomingRepost(r.Context(), uid, row.ID, repostComment)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ToggleFederatedIncomingRepost", err)
		return
	}
	author, err := s.federationAuthorPayload(r.Context(), uid)
	if err != nil {
		writeServerError(w, "federationAuthorPayload", err)
		return
	}
	if reposted {
		rows := []repo.FederatedIncomingPost{row}
		if err := s.db.AttachPollsToFederatedIncoming(r.Context(), uid, rows); err != nil {
			writeServerError(w, "AttachPollsToFederatedIncoming", err)
			return
		}
		row = rows[0]
		post := federationEventPost{
			ID:                federationSyntheticIncomingRepostID(uid, row.ID),
			URL:               s.federationSyntheticIncomingRepostURL(uid, row),
			Caption:           row.CaptionText,
			MediaType:         row.MediaType,
			MediaURLs:         append([]string(nil), row.MediaURLs...),
			IsNSFW:            row.IsNSFW,
			PublishedAt:       time.Now().UTC().Format(time.RFC3339),
			LikeCount:         row.LikeCount,
			Poll:              federationPollPayload(row.Poll, 0),
			RepostOfObjectURL: row.ObjectIRI,
		}
		if repostComment != nil {
			post.RepostComment = strings.TrimSpace(*repostComment)
		}
		if err := s.queueDirectedFederationEvent(r.Context(), uid, row.ID, inboxURL, federationEventEnvelope{
			V:      1,
			Kind:   "repost_created",
			Author: author,
			Post:   &post,
		}); err != nil {
			writeServerError(w, "queueDirectedFederationEvent", err)
			return
		}
	} else {
		if err := s.queueDirectedFederationEvent(r.Context(), uid, row.ID, inboxURL, federationEventEnvelope{
			V:      1,
			Kind:   "post_deleted",
			Author: author,
			Post: &federationEventPost{
				ID:  federationSyntheticIncomingRepostID(uid, row.ID),
				URL: s.federationSyntheticIncomingRepostURL(uid, row),
			},
		}); err != nil {
			writeServerError(w, "queueDirectedFederationEvent", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reposted": reposted, "repost_count": count})
}

func (s *Server) resolveFederatedPollChoice(ctx context.Context, incomingID uuid.UUID, raw string) (uuid.UUID, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, 0, repo.ErrPollInvalidOption
	}
	if optionID, err := uuid.Parse(raw); err == nil {
		position, err := s.db.FederatedIncomingPollOptionPosition(ctx, incomingID, optionID)
		return optionID, position, err
	}
	raw = strings.TrimPrefix(raw, "pos:")
	position, err := strconv.Atoi(raw)
	if err != nil || position <= 0 {
		return uuid.Nil, 0, repo.ErrPollInvalidOption
	}
	optionID, err := s.db.FederatedIncomingPollOptionIDByPosition(ctx, incomingID, position)
	return optionID, position, err
}
