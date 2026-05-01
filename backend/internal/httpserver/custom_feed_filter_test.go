package httpserver

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeCustomFeedTerms(t *testing.T) {
	got := normalizeCustomFeedTerms([]string{" @Alice ", "alice", "", "@Bob", strings.Repeat("x", 90)}, 3)
	want := []string{"alice", "bob", strings.Repeat("x", 80)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCustomFeedTerms() = %#v, want %#v", got, want)
	}
}

func TestCustomFeedUserMatches(t *testing.T) {
	item := feedItem{
		UserHandle:     "@Alice",
		UserEmail:      "alice@example.social",
		RemoteActorURL: "https://remote.example/users/alice",
	}

	if !customFeedUserMatches(item, []string{"alice"}) {
		t.Fatal("expected handle match")
	}
	if !customFeedUserMatches(item, []string{"example.social"}) {
		t.Fatal("expected email match")
	}
	if !customFeedUserMatches(item, []string{"remote.example"}) {
		t.Fatal("expected remote actor match")
	}
	if customFeedUserMatches(item, []string{"bob"}) {
		t.Fatal("unexpected user match")
	}
}

func TestCustomFeedContainsKeywordIncludesRepostComment(t *testing.T) {
	item := feedItem{
		Caption: "plain caption",
		Repost:  &feedRepostMetaJSON{Comment: "Glipz release notes"},
	}

	if !customFeedContainsKeyword(item, []string{"release"}) {
		t.Fatal("expected keyword match in repost comment")
	}
	if customFeedContainsKeyword(item, []string{"missing"}) {
		t.Fatal("unexpected keyword match")
	}
}

func TestFilterCustomFeedItems(t *testing.T) {
	items := []feedItem{
		{ID: "keep", Caption: "Glipz release", UserHandle: "alice", UserEmail: "alice@example.social"},
		{ID: "keyword-miss", Caption: "unrelated", UserHandle: "alice"},
		{ID: "excluded-user", Caption: "Glipz release", UserHandle: "blocked"},
		{ID: "repost", Caption: "Glipz release", UserHandle: "alice", Repost: &feedRepostMetaJSON{Comment: "boost"}},
		{ID: "federated", Caption: "Glipz release", UserHandle: "alice", IsFederated: true},
		{ID: "nsfw", Caption: "Glipz release", UserHandle: "alice", IsNSFW: true},
	}

	got := filterCustomFeedItems(items, customFeedFilters{
		Keywords:         []string{"glipz"},
		IncludeUsers:     []string{"@alice"},
		ExcludeUsers:     []string{"blocked"},
		IncludeReposts:   false,
		IncludeFederated: false,
		IncludeNSFW:      false,
	})

	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("filtered IDs = %#v, want only keep", got)
	}
}
