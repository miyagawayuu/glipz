package repo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIdentityTransferStatsRoundTrip(t *testing.T) {
	in := IdentityTransferStats{
		Profile:   IdentityTransferCategoryStats{Total: 1, Imported: 1},
		Posts:     IdentityTransferCategoryStats{Total: 3, Imported: 2, Failed: 1},
		Following: IdentityTransferCategoryStats{Total: 2, Imported: 1, Skipped: 1},
	}
	raw, err := marshalIdentityTransferStats(in)
	if err != nil {
		t.Fatalf("marshalIdentityTransferStats: %v", err)
	}
	var out IdentityTransferStats
	if err := statsScanner(&out).Scan(raw); err != nil {
		t.Fatalf("stats scan: %v", err)
	}
	if out.Profile.Imported != 1 || out.Posts.Failed != 1 || out.Following.Skipped != 1 {
		t.Fatalf("stats round trip = %+v", out)
	}
}

func TestTransferProfilePayloadDoesNotExposeSensitiveFields(t *testing.T) {
	raw, err := json.Marshal(TransferProfilePayload{
		Handle:      "alice",
		DisplayName: "Alice",
		Bio:         "hello",
		AlsoKnownAs: []string{"alice@example.social"},
	})
	if err != nil {
		t.Fatalf("marshal profile payload: %v", err)
	}
	body := strings.ToLower(string(raw))
	for _, forbidden := range []string{"email", "password", "oauth", "token", "totp", "dm", "notification", "ip"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("profile transfer payload exposed %q in %s", forbidden, body)
		}
	}
}
