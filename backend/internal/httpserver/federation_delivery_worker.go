package httpserver

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

const (
	federationOutboxMaxAttempts = 10
	federationOutboxWorkerTick  = 8 * time.Second
)

func federationOutboxBackoff(attempt int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempt && i < 18; i++ {
		next := d * 2
		if next > time.Hour {
			return time.Hour
		}
		d = next
	}
	return d
}

func glipzProtocolOutboxBackoff(attempt int) time.Duration {
	return federationOutboxBackoff(attempt)
}

func (s *Server) startFederationDeliveryWorker() {
	if strings.TrimSpace(s.federationPublicOrigin()) == "" {
		return
	}
	go s.federationDeliveryWorkerLoop()
}

func (s *Server) federationDeliveryWorkerLoop() {
	t := time.NewTicker(federationOutboxWorkerTick)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		s.processFederationOutboxBatch(ctx)
		cancel()
	}
}

func (s *Server) processFederationOutboxBatch(ctx context.Context) {
	rows, err := s.db.ClaimFederationDeliveries(ctx, 25)
	if err != nil {
		log.Printf("federation outbox claim: %v", err)
		return
	}
	for _, row := range rows {
		s.deliverOneFederationOutboxRow(ctx, row)
	}
}

func (s *Server) deliverOneFederationOutboxRow(ctx context.Context, row repo.FederationDeliveryRow) {
	if h, err := federationHostFromInboxURL(row.InboxURL); err == nil {
		blocked, errB := s.db.IsFederationDomainBlocked(ctx, h)
		if errB == nil && blocked {
			_ = s.db.FailFederationDelivery(ctx, row.ID, row.AttemptCount+1, "domain blocked", time.Now().UTC(), true)
			return
		}
	}
	var payload any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		s.failFederationOutboxRow(ctx, row, err)
		return
	}
	if err := s.signedFederationPOST(ctx, row.InboxURL, payload); err != nil {
		s.failFederationOutboxRow(ctx, row, err)
		return
	}
	if err := s.db.CompleteFederationDelivery(ctx, row.ID); err != nil {
		log.Printf("federation outbox complete %s: %v", row.ID, err)
	}
}

func (s *Server) failFederationOutboxRow(ctx context.Context, row repo.FederationDeliveryRow, err error) {
	nextN := row.AttemptCount + 1
	dead := nextN >= federationOutboxMaxAttempts
	nextAt := time.Now().UTC().Add(glipzProtocolOutboxBackoff(nextN))
	if dead {
		nextAt = time.Now().UTC()
	}
	if e := s.db.FailFederationDelivery(ctx, row.ID, nextN, err.Error(), nextAt, dead); e != nil {
		log.Printf("federation outbox fail record %s: %v", row.ID, e)
		return
	}
	if dead {
		log.Printf("federation outbox dead letter id=%s kind=%s inbox=%s err=%v", row.ID, row.Kind, row.InboxURL, err)
	}
}

func (s *Server) federationAuthorPayload(ctx context.Context, authorID uuid.UUID) (federationEventAuthor, error) {
	u, err := s.db.UserByID(ctx, authorID)
	if err != nil {
		return federationEventAuthor{}, err
	}
	out := federationEventAuthor{
		Acct:        s.localFullAcct(u.Handle),
		Handle:      u.Handle,
		Domain:      s.federationDisplayHost(),
		DisplayName: resolvedDisplayName(u.DisplayName, u.Email),
		ProfileURL:  s.localProfileURL(u.Handle),
	}
	if u.AvatarObjectKey != nil && strings.TrimSpace(*u.AvatarObjectKey) != "" {
		out.AvatarURL = s.glipzProtocolPublicMediaURL(*u.AvatarObjectKey)
	}
	return out, nil
}

func (s *Server) federationEventPostPayload(row repo.FederationPublicPostRow) federationEventPost {
	caption := row.Caption
	keys := append([]string(nil), row.ObjectKeys...)
	if row.HasViewPassword {
		if scopeProtectsText(row.ViewPasswordScope) {
			if row.ViewPasswordScope == repo.ViewPasswordScopeAll {
				caption = ""
			} else {
				caption = maskCaptionText(row.Caption, row.ViewPasswordTextRanges)
			}
		}
		if scopeProtectsMedia(row.ViewPasswordScope) {
			keys = []string{}
		}
	}
	urls := make([]string, 0, len(keys))
	for _, key := range keys {
		urls = append(urls, s.glipzProtocolPublicMediaURL(key))
	}
	out := federationEventPost{
		ID:          row.ID.String(),
		URL:         s.localPostURL(row.ID),
		Caption:     caption,
		MediaType:   row.MediaType,
		MediaURLs:   urls,
		IsNSFW:      row.IsNSFW,
		PublishedAt: row.VisibleAt.UTC().Format(time.RFC3339),
		LikeCount:   row.LikeCount,
		Poll:        federationPollPayload(row.Poll, 0),
	}
	if row.ReplyToID != nil {
		out.ReplyToObjectURL = s.localPostURL(*row.ReplyToID)
	}
	if row.HasViewPassword {
		out.HasViewPassword = true
		out.ViewPasswordScope = row.ViewPasswordScope
		out.ViewPasswordTextRanges = repoRangesToJSON(row.ViewPasswordTextRanges)
		out.UnlockURL = s.localFederationPostUnlockURL(row.ID)
	}
	return out
}

func (s *Server) enqueueFederationEvent(ctx context.Context, authorID uuid.UUID, kind string, row repo.FederationPublicPostRow) {
	inboxes, err := s.db.ListFederationSubscriberInboxes(ctx, authorID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	author, err := s.federationAuthorPayload(ctx, authorID)
	if err != nil {
		return
	}
	post := s.federationEventPostPayload(row)
	s.enqueueFederationPayload(ctx, authorID, row.ID, inboxes, federationEventEnvelope{
		V:      federationEventSchemaVersion,
		Kind:   kind,
		Author: author,
		Post:   &post,
	})
}

func (s *Server) enqueueFederationPayload(ctx context.Context, authorID, refID uuid.UUID, inboxes []string, payload federationEventEnvelope) {
	payload.V = federationEventSchemaVersion
	if strings.TrimSpace(payload.EventID) == "" {
		payload.EventID = federationNewEventID()
	}
	items := make([]repo.FederationDeliveryInsert, 0, len(inboxes))
	for _, inbox := range inboxes {
		items = append(items, repo.FederationDeliveryInsert{
			AuthorUserID: authorID,
			PostID:       refID,
			Kind:         payload.Kind,
			InboxURL:     inbox,
			Payload:      repo.MustMarshalJSON(payload),
		})
	}
	_ = s.db.InsertFederationDeliveries(ctx, items)
}

func (s *Server) deliverFederationCreate(ctx context.Context, authorID, postID uuid.UUID) {
	row, err := s.db.GetFederationPublicPostForDelivery(ctx, authorID, postID)
	if err != nil {
		return
	}
	s.enqueueFederationEvent(ctx, authorID, "post_created", row)
}

func (s *Server) deliverFederationUpdate(ctx context.Context, authorID uuid.UUID, row repo.FederationPublicPostRow) {
	s.enqueueFederationEvent(ctx, authorID, "post_updated", row)
}

func (s *Server) deliverFederationDelete(ctx context.Context, authorID, postID uuid.UUID) {
	inboxes, err := s.db.ListFederationSubscriberInboxes(ctx, authorID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	author, err := s.federationAuthorPayload(ctx, authorID)
	if err != nil {
		return
	}
	payload := federationEventEnvelope{
		V:      federationEventSchemaVersion,
		Kind:   "post_deleted",
		Author: author,
		Post: &federationEventPost{
			ID:  postID.String(),
			URL: s.localPostURL(postID),
		},
	}
	s.enqueueFederationPayload(ctx, authorID, postID, inboxes, payload)
}

func (s *Server) deliverFederationLikeEventToSubscribers(ctx context.Context, postAuthorID uuid.UUID, actor federationEventAuthor, postID uuid.UUID, likeCount int64, liked bool) {
	row, err := s.db.GetFederationPublicPostForDelivery(ctx, postAuthorID, postID)
	if err != nil {
		return
	}
	inboxes, err := s.db.ListFederationSubscriberInboxes(ctx, postAuthorID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	post := s.federationEventPostPayload(row)
	post.LikeCount = likeCount
	kind := "post_liked"
	if !liked {
		kind = "post_unliked"
	}
	s.enqueueFederationPayload(ctx, postAuthorID, postID, inboxes, federationEventEnvelope{
		V:      federationEventSchemaVersion,
		Kind:   kind,
		Author: actor,
		Post:   &post,
	})
}

func (s *Server) deliverFederationReactionEventToSubscribers(ctx context.Context, postAuthorID uuid.UUID, actor federationEventAuthor, postID uuid.UUID, emoji string, added bool) {
	row, err := s.db.GetFederationPublicPostForDelivery(ctx, postAuthorID, postID)
	if err != nil {
		return
	}
	inboxes, err := s.db.ListFederationSubscriberInboxes(ctx, postAuthorID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	post := s.federationEventPostPayload(row)
	kind := "post_reaction_added"
	if !added {
		kind = "post_reaction_removed"
	}
	s.enqueueFederationPayload(ctx, postAuthorID, postID, inboxes, federationEventEnvelope{
		V:        federationEventSchemaVersion,
		Kind:     kind,
		Author:   actor,
		Post:     &post,
		Reaction: &federationEventReaction{Emoji: emoji},
	})
}

func (s *Server) deliverFederationPollTallyUpdated(ctx context.Context, postAuthorID, postID uuid.UUID) {
	row, err := s.db.GetFederationPublicPostForDelivery(ctx, postAuthorID, postID)
	if err != nil {
		return
	}
	inboxes, err := s.db.ListFederationSubscriberInboxes(ctx, postAuthorID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	author, err := s.federationAuthorPayload(ctx, postAuthorID)
	if err != nil {
		return
	}
	post := s.federationEventPostPayload(row)
	s.enqueueFederationPayload(ctx, postAuthorID, postID, inboxes, federationEventEnvelope{
		V:      federationEventSchemaVersion,
		Kind:   "poll_tally_updated",
		Author: author,
		Post:   &post,
	})
}

func (s *Server) enqueueFederationDirectedEvent(ctx context.Context, actorUserID, refID uuid.UUID, inboxURL string, payload federationEventEnvelope) {
	s.enqueueFederationPayload(ctx, actorUserID, refID, []string{inboxURL}, payload)
}

func federationSyntheticRepostID(boosterUserID, originalPostID uuid.UUID) string {
	return "repost:" + boosterUserID.String() + ":" + originalPostID.String()
}

func (s *Server) federationSyntheticRepostURL(boosterUserID, originalPostID uuid.UUID) string {
	return s.localPostURL(originalPostID) + "#repost-" + boosterUserID.String()
}

func (s *Server) deliverFederationRepost(ctx context.Context, boosterUserID, originalPostID uuid.UUID, comment *string) {
	ownerID, err := s.db.PostAuthorID(ctx, originalPostID)
	if err != nil {
		return
	}
	row, err := s.db.GetFederationPublicPostForDelivery(ctx, ownerID, originalPostID)
	if err != nil {
		return
	}
	inboxes, err := s.db.ListFederationSubscriberInboxes(ctx, boosterUserID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	author, err := s.federationAuthorPayload(ctx, boosterUserID)
	if err != nil {
		return
	}
	post := s.federationEventPostPayload(row)
	post.ID = federationSyntheticRepostID(boosterUserID, originalPostID)
	post.URL = s.federationSyntheticRepostURL(boosterUserID, originalPostID)
	post.RepostOfObjectURL = s.localPostURL(originalPostID)
	if comment != nil {
		post.RepostComment = strings.TrimSpace(*comment)
	}
	s.enqueueFederationPayload(ctx, boosterUserID, originalPostID, inboxes, federationEventEnvelope{
		V:      federationEventSchemaVersion,
		Kind:   "repost_created",
		Author: author,
		Post:   &post,
	})
}

func (s *Server) deliverFederationRepostDelete(ctx context.Context, boosterUserID, originalPostID uuid.UUID) {
	inboxes, err := s.db.ListFederationSubscriberInboxes(ctx, boosterUserID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	author, err := s.federationAuthorPayload(ctx, boosterUserID)
	if err != nil {
		return
	}
	postID := federationSyntheticRepostID(boosterUserID, originalPostID)
	postURL := s.federationSyntheticRepostURL(boosterUserID, originalPostID)
	s.enqueueFederationPayload(ctx, boosterUserID, originalPostID, inboxes, federationEventEnvelope{
		V:      federationEventSchemaVersion,
		Kind:   "post_deleted",
		Author: author,
		Post: &federationEventPost{
			ID:  postID,
			URL: postURL,
		},
	})
}
