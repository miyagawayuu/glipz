package httpserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/authjwt"
	"glipz.io/backend/internal/repo"
)

func TestFederatedIncomingRecipientVisible(t *testing.T) {
	recipient := uuid.New()
	other := uuid.New()

	publicRow := repo.FederatedIncomingPost{}
	if !federatedIncomingRecipientVisible(publicRow, uuid.Nil, false) {
		t.Fatal("public federated incoming row should be visible anonymously")
	}

	directedRow := repo.FederatedIncomingPost{RecipientUserID: &recipient}
	if federatedIncomingRecipientVisible(directedRow, uuid.Nil, false) {
		t.Fatal("directed federated incoming row should not be visible anonymously")
	}
	if federatedIncomingRecipientVisible(directedRow, other, true) {
		t.Fatal("directed federated incoming row should not be visible to another user")
	}
	if !federatedIncomingRecipientVisible(directedRow, recipient, true) {
		t.Fatal("directed federated incoming row should be visible to its recipient")
	}
}

func TestFederationAuthorMustMatchSigningInstance(t *testing.T) {
	verified := verifiedFederationRequest{InstanceHost: "remote.example"}
	if err := validateFederationAuthorForInstance(verified, federationEventAuthor{Acct: "alice@remote.example"}); err != nil {
		t.Fatalf("same-host author rejected: %v", err)
	}
	if err := validateFederationAuthorForInstance(verified, federationEventAuthor{Acct: "alice@evil.example"}); err == nil {
		t.Fatal("cross-host author accepted")
	}
}

func TestFederationMoveMustStayWithinSigningInstance(t *testing.T) {
	verified := verifiedFederationRequest{InstanceHost: "remote.example"}
	if err := validateFederationMoveForInstance(verified, &federationAccountMove{
		OldAcct: "alice@remote.example",
		NewAcct: "alice2@remote.example",
	}); err != nil {
		t.Fatalf("same-host move rejected: %v", err)
	}
	if err := validateFederationMoveForInstance(verified, &federationAccountMove{
		OldAcct: "alice@remote.example",
		NewAcct: "alice@evil.example",
	}); err == nil {
		t.Fatal("cross-host move accepted without additional proof")
	}
}

func TestOAuthScopeRejectsEmptyAndUnknown(t *testing.T) {
	if scope, ok := normalizeOAuthScope(""); ok || scope != "" {
		t.Fatalf("empty scope accepted: scope=%q ok=%v", scope, ok)
	}
	if scope, ok := normalizeOAuthScope("admin:all"); ok || scope != "" {
		t.Fatalf("unknown scope accepted: scope=%q ok=%v", scope, ok)
	}
	if scope, ok := normalizeOAuthScope("posts:read media:write"); !ok || scope != "posts:read media:write" {
		t.Fatalf("valid scope rejected or reordered unexpectedly: scope=%q ok=%v", scope, ok)
	}
}

func TestOAuthCommunityScopes(t *testing.T) {
	readClaims := &authjwt.Claims{TokenUse: authjwt.TokenUseOAuth, Scope: "posts:read"}
	writeClaims := &authjwt.Claims{TokenUse: authjwt.TokenUseOAuth, Scope: "posts:write"}

	communityID := uuid.New().String()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/communities/"+communityID+"/posts", nil)
	if !oauthClaimsAllowRequest(readClaims, req) {
		t.Fatal("posts:read OAuth scope should allow public community reads")
	}
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/communities/"+communityID+"/join-requests", nil)
	if oauthClaimsAllowRequest(readClaims, req) {
		t.Fatal("posts:read OAuth scope should not allow community mutations")
	}
	if !oauthClaimsAllowRequest(writeClaims, req) {
		t.Fatal("posts:write OAuth scope should allow community mutations")
	}
}

func TestFederationProtocolV1Unsupported(t *testing.T) {
	if federationDiscoverySupportsCurrentProtocol(federationServerDiscovery{ProtocolVersion: federationProtocolName + "/1"}) {
		t.Fatal("federation protocol v1 should not be supported")
	}
	if !federationDiscoverySupportsCurrentProtocol(federationServerDiscovery{ProtocolVersion: federationProtocolName + "/2"}) {
		t.Fatal("federation protocol v2 should be supported")
	}
}

func TestPostRowToFeedItemOmitsEmail(t *testing.T) {
	uid := uuid.New()
	postID := uuid.New()
	s := &Server{}
	item := s.postRowToFeedItem(context.Background(), repo.PostRow{
		ID:         postID,
		UserID:     uid,
		Email:      "alice@example.com",
		UserHandle: "alice",
		MediaType:  "none",
		Visibility: repo.PostVisibilityPublic,
		CreatedAt:  time.Unix(1, 0).UTC(),
		VisibleAt:  time.Unix(1, 0).UTC(),
	}, uid, nil)
	if item.UserEmail != "" {
		t.Fatalf("feed item leaked email: %q", item.UserEmail)
	}
	if item.UserDisplayName != "alice" {
		t.Fatalf("display name fallback = %q, want handle", item.UserDisplayName)
	}
}
