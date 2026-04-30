package httpserver

import (
	"testing"
	"time"
)

func intPtr(v int) *int {
	return &v
}

func TestNormalizeCustomFeedRankingDefaultsAndClamps(t *testing.T) {
	cfg := normalizeCustomFeedRanking(customFeedRankingRequest{
		Mode:     "rules",
		PresetID: "",
		Weights: customFeedRankingWeightsRequest{
			Recency:    intPtr(120),
			Popularity: intPtr(-10),
		},
	})

	if cfg.mode != "default" {
		t.Fatalf("mode = %q, want default", cfg.mode)
	}
	if cfg.presetID != "balanced" {
		t.Fatalf("presetID = %q, want balanced", cfg.presetID)
	}
	if cfg.weights.recency != 2 {
		t.Fatalf("recency weight = %v, want 2", cfg.weights.recency)
	}
	if cfg.weights.popularity != 0 {
		t.Fatalf("popularity weight = %v, want 0", cfg.weights.popularity)
	}
	if cfg.weights.affinity != 1 {
		t.Fatalf("affinity weight = %v, want 1", cfg.weights.affinity)
	}
	if cfg.constraints.diversity != 50 {
		t.Fatalf("diversity = %d, want 50", cfg.constraints.diversity)
	}
}

func TestCustomFeedScoreRespondsToWeights(t *testing.T) {
	recent := customFeedSignals{recency: 1, popularity: 0}
	popular := customFeedSignals{recency: 0.1, popularity: 10}

	recencyFirst := normalizeCustomFeedRanking(customFeedRankingRequest{
		Weights: customFeedRankingWeightsRequest{
			Recency:    intPtr(100),
			Popularity: intPtr(0),
			Affinity:   intPtr(0),
			Federated:  intPtr(50),
		},
		Constraints: customFeedRankingConstraintsRequest{Diversity: intPtr(0)},
	})
	if scoreCustomFeedSignals(recent, recencyFirst) <= scoreCustomFeedSignals(popular, recencyFirst) {
		t.Fatal("expected recency-heavy ranking to prefer the recent item")
	}

	popularityFirst := normalizeCustomFeedRanking(customFeedRankingRequest{
		Weights: customFeedRankingWeightsRequest{
			Recency:    intPtr(0),
			Popularity: intPtr(100),
			Affinity:   intPtr(0),
			Federated:  intPtr(50),
		},
		Constraints: customFeedRankingConstraintsRequest{Diversity: intPtr(0)},
	})
	if scoreCustomFeedSignals(popular, popularityFirst) <= scoreCustomFeedSignals(recent, popularityFirst) {
		t.Fatal("expected popularity-heavy ranking to prefer the popular item")
	}
}

func TestApplyCustomFeedDiversityAvoidsConsecutiveAuthors(t *testing.T) {
	now := time.Now().UTC()
	ranked := []customFeedRankedItem{
		{item: feedItem{ID: "a1", UserID: "author-a"}, score: 3, visibleAt: now, authorKey: "author-a"},
		{item: feedItem{ID: "a2", UserID: "author-a"}, score: 2, visibleAt: now.Add(-time.Minute), authorKey: "author-a"},
		{item: feedItem{ID: "b1", UserID: "author-b"}, score: 1, visibleAt: now.Add(-2 * time.Minute), authorKey: "author-b"},
	}

	out := applyCustomFeedDiversity(ranked, 100)
	if got := []string{out[0].ID, out[1].ID, out[2].ID}; got[0] != "a1" || got[1] != "b1" || got[2] != "a2" {
		t.Fatalf("order = %v, want [a1 b1 a2]", got)
	}
}
