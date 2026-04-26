package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var errResolveBadInput = errors.New("bad federation resolve input")

func ResolveFailureAPIError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errResolveBadInput) {
		return "bad_acct_or_actor"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "discovery fetch:"):
		return "discovery_unreachable"
	case strings.Contains(s, "discovery: http"):
		return "discovery_http_error"
	case strings.Contains(s, "discovery: unsupported protocol"):
		return "discovery_unsupported_protocol"
	case strings.Contains(s, "discovery json:"):
		return "discovery_invalid_json"
	case strings.Contains(s, "profile fetch:"):
		return "profile_unreachable"
	case strings.Contains(s, "profile: http"):
		return "profile_http_error"
	case strings.Contains(s, "profile json:"):
		return "profile_invalid_json"
	default:
		return "remote_resolve_failed"
	}
}

type ResolvedRemoteActor struct {
	ActorID     string
	Inbox       string
	FollowURL   string
	UnfollowURL string
	ProfileURL  string
	PostsURL    string
	Acct        string
	Name        string
	IconURL     string
	HeaderURL   string
	Summary     string
}

func (r ResolvedRemoteActor) DeliveryInbox() string {
	return strings.TrimSpace(r.Inbox)
}

func sameGlipzProtocolIRI(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	ka, oka := normalizedGlipzIRIKey(a)
	kb, okb := normalizedGlipzIRIKey(b)
	if oka && okb {
		return ka == kb
	}
	return strings.EqualFold(strings.TrimSuffix(a, "/"), strings.TrimSuffix(b, "/"))
}

func normalizedGlipzIRIKey(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return "", false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return "", false
	}
	// Normalize common equivalents:
	// - ignore scheme differences (http vs https) for equality checks
	// - trim trailing slashes in the path
	p := strings.TrimSuffix(strings.TrimSpace(u.EscapedPath()), "/")
	if p == "" {
		p = "/"
	}
	// Keep port only when both sides specify a non-default port.
	port := strings.TrimSpace(u.Port())
	if port == "" || port == "80" || port == "443" {
		port = ""
	}
	if port != "" {
		return host + ":" + port + p, true
	}
	return host + p, true
}

func splitAcct(s string) (user, host string, err error) {
	s = strings.TrimSpace(s)
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return "", "", errResolveBadInput
	}
	user = strings.TrimSpace(s[:at])
	host = strings.ToLower(strings.TrimSpace(s[at+1:]))
	if user == "" || host == "" {
		return "", "", errResolveBadInput
	}
	return user, host, nil
}

type jrdLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href"`
}

type jrdDoc struct {
	Links []jrdLink `json:"links"`
}

func fetchRemoteFederationDiscovery(ctx context.Context, host string) (federationAccountDiscovery, error) {
	host = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(strings.ToLower(host)), "https://"), "http://")
	if host == "" {
		return federationAccountDiscovery{}, errResolveBadInput
	}
	if err := ensurePublicOutboundHost(ctx, host); err != nil {
		return federationAccountDiscovery{}, err
	}
	rawURL := fmt.Sprintf("https://%s/.well-known/glipz-federation", host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return federationAccountDiscovery{}, err
	}
	res, err := federationHTTP.Do(req)
	if err != nil {
		return federationAccountDiscovery{}, fmt.Errorf("discovery fetch: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return federationAccountDiscovery{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return federationAccountDiscovery{}, fmt.Errorf("discovery: http %d", res.StatusCode)
	}
	var doc federationAccountDiscovery
	if err := json.Unmarshal(body, &doc); err != nil {
		return federationAccountDiscovery{}, fmt.Errorf("discovery json: %w", err)
	}
	if !federationDiscoverySupportsCurrentProtocol(doc.Server) {
		return federationAccountDiscovery{}, fmt.Errorf("discovery: unsupported protocol")
	}
	return doc, nil
}

func fetchRemoteFederationAccount(ctx context.Context, acct string) (federationAccountDiscovery, error) {
	user, host, err := splitAcct(strings.TrimPrefix(strings.TrimSpace(acct), "@"))
	if err != nil {
		return federationAccountDiscovery{}, err
	}
	if err := ensurePublicOutboundHost(ctx, host); err != nil {
		return federationAccountDiscovery{}, err
	}
	rawURL := fmt.Sprintf("https://%s/.well-known/glipz-federation?resource=%s", host, url.QueryEscape(user+"@"+host))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return federationAccountDiscovery{}, err
	}
	res, err := federationHTTP.Do(req)
	if err != nil {
		return federationAccountDiscovery{}, fmt.Errorf("discovery fetch: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return federationAccountDiscovery{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return federationAccountDiscovery{}, fmt.Errorf("discovery: http %d", res.StatusCode)
	}
	var doc federationAccountDiscovery
	if err := json.Unmarshal(body, &doc); err != nil {
		return federationAccountDiscovery{}, fmt.Errorf("discovery json: %w", err)
	}
	if !federationDiscoverySupportsCurrentProtocol(doc.Server) {
		return federationAccountDiscovery{}, fmt.Errorf("discovery: unsupported protocol")
	}
	if doc.Account == nil {
		return federationAccountDiscovery{}, fmt.Errorf("discovery: missing account")
	}
	return doc, nil
}

// RemoteActorDisplay represents the public profile fields shown for a federated user.
type RemoteActorDisplay struct {
	ActorID     string `json:"actor_id"`
	Acct        string `json:"acct"`
	Name        string `json:"name"`
	Summary     string `json:"summary,omitempty"`
	IconURL     string `json:"icon_url,omitempty"`
	HeaderURL   string `json:"header_url,omitempty"`
	ProfileURL  string `json:"profile_url,omitempty"`
	Inbox       string `json:"inbox,omitempty"`
	SharedInbox string `json:"shared_inbox,omitempty"`
}

func fetchRemoteFederationProfile(ctx context.Context, profileURL string) (federationPublicProfile, error) {
	if _, err := validatePublicOutboundURL(ctx, profileURL, false); err != nil {
		return federationPublicProfile{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(profileURL), nil)
	if err != nil {
		return federationPublicProfile{}, err
	}
	res, err := federationHTTP.Do(req)
	if err != nil {
		return federationPublicProfile{}, fmt.Errorf("profile fetch: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return federationPublicProfile{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return federationPublicProfile{}, fmt.Errorf("profile: http %d", res.StatusCode)
	}
	var doc federationPublicProfile
	if err := json.Unmarshal(body, &doc); err != nil {
		return federationPublicProfile{}, fmt.Errorf("profile json: %w", err)
	}
	return doc, nil
}

func FetchRemoteActorDisplay(ctx context.Context, raw string) (RemoteActorDisplay, error) {
	resolved, err := ResolveRemoteActor(ctx, raw)
	if err != nil {
		return RemoteActorDisplay{}, err
	}
	return RemoteActorDisplay{
		ActorID:     resolved.ActorID,
		Acct:        resolved.Acct,
		Name:        resolved.Name,
		Summary:     resolved.Summary,
		IconURL:     resolved.IconURL,
		HeaderURL:   resolved.HeaderURL,
		ProfileURL:  resolved.ProfileURL,
		Inbox:       resolved.Inbox,
		SharedInbox: resolved.Inbox,
	}, nil
}

func ResolveRemoteActor(ctx context.Context, raw string) (ResolvedRemoteActor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ResolvedRemoteActor{}, errResolveBadInput
	}
	low := strings.ToLower(raw)
	if strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "http://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return ResolvedRemoteActor{}, errResolveBadInput
		}
		doc, err := fetchRemoteFederationDiscovery(ctx, u.Host)
		if err != nil {
			return ResolvedRemoteActor{}, err
		}
		if strings.TrimSpace(doc.Server.EventsURL) == "" {
			return ResolvedRemoteActor{}, fmt.Errorf("discovery: missing events url")
		}
		profURL := strings.TrimSpace(raw)
		prof, err := fetchRemoteFederationProfile(ctx, profURL)
		if err != nil {
			return ResolvedRemoteActor{}, err
		}
		return ResolvedRemoteActor{
			ActorID:     prof.Acct,
			Inbox:       doc.Server.EventsURL,
			FollowURL:   doc.Server.FollowURL,
			UnfollowURL: doc.Server.UnfollowURL,
			ProfileURL:  prof.ProfileURL,
			PostsURL:    prof.PostsURL,
			Acct:        prof.Acct,
			Name:        prof.DisplayName,
			IconURL:     prof.AvatarURL,
			HeaderURL:   prof.HeaderURL,
			Summary:     prof.Summary,
		}, nil
	}
	doc, err := fetchRemoteFederationAccount(ctx, strings.TrimPrefix(strings.TrimPrefix(raw, "acct:"), "@"))
	if err != nil {
		return ResolvedRemoteActor{}, err
	}
	prof := doc.Account
	return ResolvedRemoteActor{
		ActorID:     prof.Acct,
		Inbox:       doc.Server.EventsURL,
		FollowURL:   doc.Server.FollowURL,
		UnfollowURL: doc.Server.UnfollowURL,
		ProfileURL:  prof.ProfileURL,
		PostsURL:    prof.PostsURL,
		Acct:        prof.Acct,
		Name:        prof.DisplayName,
		IconURL:     prof.AvatarURL,
		HeaderURL:   prof.HeaderURL,
		Summary:     prof.Summary,
	}, nil
}
