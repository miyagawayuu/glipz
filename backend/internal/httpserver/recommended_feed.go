package httpserver

import (
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type recCandidate struct {
	kind      string // "local" | "federated"
	score     float64
	t         time.Time
	authorKey string

	localID uuid.UUID
	fedID   uuid.UUID
}

func (s *Server) handleRecommendedFeed(w http.ResponseWriter, r *http.Request, viewerID uuid.UUID) {
	ctx := r.Context()

	aff, err := s.db.AuthorAffinityScores(ctx, viewerID, time.Now().Add(-90*24*time.Hour))
	if err != nil {
		writeServerError(w, "AuthorAffinityScores", err)
		return
	}

	ids, err := s.db.RecommendedCandidatePostIDs(ctx, viewerID, 450)
	if err != nil {
		writeServerError(w, "RecommendedCandidatePostIDs", err)
		return
	}

	localRows, err := s.db.PostRowsByIDsForViewer(ctx, viewerID, ids)
	if err != nil {
		writeServerError(w, "PostRowsByIDsForViewer", err)
		return
	}
	localByID := make(map[uuid.UUID]repo.PostRow, len(localRows))
	for _, row := range localRows {
		localByID[row.ID] = row
	}

	remoteRows, err := s.db.ListFederatedIncomingForViewer(ctx, viewerID, 180, nil, nil)
	if err != nil {
		writeServerError(w, "ListFederatedIncomingForViewer recommended", err)
		return
	}
	fedByID := make(map[uuid.UUID]repo.FederatedIncomingPost, len(remoteRows))
	for _, row := range remoteRows {
		fedByID[row.ID] = row
	}

	cands := make([]recCandidate, 0, len(localRows)+len(remoteRows))
	now := time.Now().UTC()

	// local posts
	for _, row := range localRows {
		ageH := now.Sub(row.VisibleAt.UTC()).Hours()
		if ageH < 0 {
			ageH = 0
		}
		recency := math.Exp(-ageH / 36.0) // tau=36h
		quality := math.Log1p(float64(maxI64(row.LikeCount, 0) + 2*maxI64(row.RepostCount, 0) + 2*maxI64(row.ReplyCount, 0)))
		affScore := aff[row.UserID]
		score := 1.6*affScore + 1.0*quality + 1.4*recency
		cands = append(cands, recCandidate{
			kind:      "local",
			score:     score,
			t:         row.VisibleAt.UTC(),
			authorKey: row.UserID.String(),
			localID:   row.ID,
		})
	}

	// Federated incoming posts do not have affinity scores, so rank them mainly by quality and recency.
	for _, row := range remoteRows {
		ageH := now.Sub(row.PublishedAt.UTC()).Hours()
		if ageH < 0 {
			ageH = 0
		}
		recency := math.Exp(-ageH / 36.0)
		quality := math.Log1p(float64(maxI64(row.LikeCount, 0) + 2*maxI64(row.RepostCount, 0)))
		score := 0.9*quality + 1.4*recency
		ak := row.ActorIRI
		if ak == "" {
			ak = "federated:" + row.ID.String()
		}
		cands = append(cands, recCandidate{
			kind:      "federated",
			score:     score,
			t:         row.PublishedAt.UTC(),
			authorKey: ak,
			fedID:     row.ID,
		})
	}

	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		if !cands[i].t.Equal(cands[j].t) {
			return cands[i].t.After(cands[j].t)
		}
		// id tie-breaker
		return cands[i].authorKey > cands[j].authorKey
	})

	// diversity constraints
	const (
		limit        = 50
		maxPerAuthor = 3
	)
	authorCount := map[string]int{}
	var out []recCandidate
	lastAuthor := ""
	for _, c := range cands {
		if len(out) >= limit {
			break
		}
		if c.authorKey == "" {
			continue
		}
		if authorCount[c.authorKey] >= maxPerAuthor {
			continue
		}
		if lastAuthor != "" && c.authorKey == lastAuthor {
			continue
		}
		authorCount[c.authorKey]++
		lastAuthor = c.authorKey
		out = append(out, c)
	}
	// Backfill remaining slots while relaxing only the consecutive-author constraint.
	if len(out) < limit {
		for _, c := range cands {
			if len(out) >= limit {
				break
			}
			if c.authorKey == "" {
				continue
			}
			if authorCount[c.authorKey] >= maxPerAuthor {
				continue
			}
			// Avoid duplicating candidates that were already selected.
			dup := false
			for i := range out {
				if out[i].kind == c.kind && out[i].localID == c.localID && out[i].fedID == c.fedID {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			authorCount[c.authorKey]++
			out = append(out, c)
		}
	}

	// local rows: attach polls once
	localSel := make([]repo.PostRow, 0, len(out))
	for _, c := range out {
		if c.kind != "local" {
			continue
		}
		if row, ok := localByID[c.localID]; ok {
			localSel = append(localSel, row)
		}
	}
	if err := s.attachPostTimelineMetadata(ctx, viewerID, localSel); err != nil {
		writeServerError(w, "attachPostTimelineMetadata recommended", err)
		return
	}
	localSelByID := make(map[uuid.UUID]repo.PostRow, len(localSel))
	for _, row := range localSel {
		localSelByID[row.ID] = row
	}

	items := make([]feedItem, 0, len(out))
	badgeMap, err := s.userBadgeMap(ctx, func() []uuid.UUID {
		ids := make([]uuid.UUID, 0, len(localSelByID))
		for _, row := range localSelByID {
			ids = append(ids, row.UserID)
		}
		return ids
	}())
	if err != nil {
		writeServerError(w, "ListUserBadgesByIDs recommended", err)
		return
	}
	for _, c := range out {
		if c.kind == "local" {
			row, ok := localSelByID[c.localID]
			if !ok {
				continue
			}
			items = append(items, s.postRowToFeedItem(ctx, row, viewerID, badgeMap))
			continue
		}
		if c.kind == "federated" {
			row, ok := fedByID[c.fedID]
			if !ok {
				continue
			}
			items = append(items, s.federatedIncomingToFeedItem(row))
			continue
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func maxI64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
