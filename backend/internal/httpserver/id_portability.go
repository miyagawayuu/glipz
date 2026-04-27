package httpserver

import (
	"context"
	"strings"

	"glipz.io/backend/internal/repo"
)

func (s *Server) rememberResolvedRemoteAccount(ctx context.Context, resolved ResolvedRemoteActor) (repo.RemoteAccount, error) {
	return s.db.UpsertRemoteAccount(ctx, repo.RemoteAccountUpsert{
		PortableID:  resolved.PortableID,
		CurrentAcct: resolved.Acct,
		ProfileURL:  resolved.ProfileURL,
		PostsURL:    resolved.PostsURL,
		InboxURL:    resolved.Inbox,
		PublicKey:   resolved.PublicKey,
		MovedTo:     resolved.MovedTo,
		AlsoKnownAs: resolved.AlsoKnownAs,
	})
}

func federationAuthorPortableID(author federationEventAuthor) string {
	return repo.PortableIDForRemote(author.Acct, author.ID)
}

func federationAuthorCurrentAcct(author federationEventAuthor) string {
	return repo.NormalizeFederationTargetAcct(strings.TrimSpace(author.Acct))
}

func (s *Server) rememberEventAuthorRemoteAccount(ctx context.Context, verified verifiedFederationRequest, author federationEventAuthor) (repo.RemoteAccount, error) {
	acct := federationAuthorCurrentAcct(author)
	profileURL := strings.TrimSpace(author.ProfileURL)
	return s.db.UpsertRemoteAccount(ctx, repo.RemoteAccountUpsert{
		PortableID:  federationAuthorPortableID(author),
		CurrentAcct: acct,
		ProfileURL:  profileURL,
		InboxURL:    strings.TrimSpace(verified.Discovery.Server.EventsURL),
		PublicKey:   strings.TrimSpace(author.PublicKey),
	})
}
