package httpserver

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"glipz.io/backend/internal/repo"
)

type federationServerDiscovery struct {
	ProtocolVersion           string   `json:"protocol_version"`
	SupportedProtocolVersions []string `json:"supported_protocol_versions,omitempty"`
	ServerSoftware            string   `json:"server_software,omitempty"`
	ServerVersion             string   `json:"server_version,omitempty"`
	EventSchemaVersion        int      `json:"event_schema_version,omitempty"`
	Host                      string   `json:"host"`
	Origin                    string   `json:"origin"`
	KeyID                     string   `json:"key_id"`
	PublicKey                 string   `json:"public_key"`
	EventsURL                 string   `json:"events_url"`
	FollowURL                 string   `json:"follow_url"`
	UnfollowURL               string   `json:"unfollow_url"`
	DMKeysURL                 string   `json:"dm_keys_url,omitempty"`
	KnownInstances            []string `json:"known_instances,omitempty"`
}

type federationAccountDiscovery struct {
	Resource string                    `json:"resource"`
	Server   federationServerDiscovery `json:"server"`
	Account  *federationPublicProfile  `json:"account,omitempty"`
}

type verifiedFederationRequest struct {
	InstanceHost    string
	Discovery       federationAccountDiscovery
	KeyID           string
	NormalizedKeyID string
	ProtocolVersion string
	ProtocolMajor   int
	Nonce           string
}

type federationPublicProfile struct {
	Acct        string `json:"acct"`
	Handle      string `json:"handle"`
	Domain      string `json:"domain"`
	DisplayName string `json:"display_name"`
	Summary     string `json:"summary,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	HeaderURL   string `json:"header_url,omitempty"`
	ProfileURL  string `json:"profile_url"`
	PostsURL    string `json:"posts_url"`
}

type federationPublicPost struct {
	ID                     string                      `json:"id"`
	URL                    string                      `json:"url"`
	Caption                string                      `json:"caption"`
	MediaType              string                      `json:"media_type"`
	MediaURLs              []string                    `json:"media_urls"`
	IsNSFW                 bool                        `json:"is_nsfw"`
	PublishedAt            string                      `json:"published_at"`
	LikeCount              int64                       `json:"like_count"`
	Poll                   *federationEventPoll        `json:"poll,omitempty"`
	ReplyToObjectURL       string                      `json:"reply_to_object_url,omitempty"`
	RepostOfObjectURL      string                      `json:"repost_of_object_url,omitempty"`
	RepostComment          string                      `json:"repost_comment,omitempty"`
	HasViewPassword        bool                        `json:"has_view_password,omitempty"`
	ViewPasswordScope      int                         `json:"view_password_scope,omitempty"`
	ViewPasswordTextRanges []viewPasswordTextRangeJSON `json:"view_password_text_ranges,omitempty"`
	UnlockURL              string                      `json:"unlock_url,omitempty"`
}

type federationFollowRequest struct {
	EventID      string `json:"event_id,omitempty"`
	FollowerAcct string `json:"follower_acct"`
	TargetAcct   string `json:"target_acct"`
	InboxURL     string `json:"inbox_url"`
}

type federationEventAuthor struct {
	Acct        string `json:"acct"`
	Handle      string `json:"handle"`
	Domain      string `json:"domain"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	ProfileURL  string `json:"profile_url,omitempty"`
}

type federationEventPost struct {
	ID                     string                      `json:"id"`
	URL                    string                      `json:"url"`
	Caption                string                      `json:"caption"`
	MediaType              string                      `json:"media_type"`
	MediaURLs              []string                    `json:"media_urls"`
	IsNSFW                 bool                        `json:"is_nsfw"`
	PublishedAt            string                      `json:"published_at"`
	LikeCount              int64                       `json:"like_count,omitempty"`
	Poll                   *federationEventPoll        `json:"poll,omitempty"`
	ReplyToObjectURL       string                      `json:"reply_to_object_url,omitempty"`
	RepostOfObjectURL      string                      `json:"repost_of_object_url,omitempty"`
	RepostComment          string                      `json:"repost_comment,omitempty"`
	HasViewPassword        bool                        `json:"has_view_password,omitempty"`
	ViewPasswordScope      int                         `json:"view_password_scope,omitempty"`
	ViewPasswordTextRanges []viewPasswordTextRangeJSON `json:"view_password_text_ranges,omitempty"`
	UnlockURL              string                      `json:"unlock_url,omitempty"`
}

type federationEventPollOption struct {
	Position int    `json:"position"`
	Label    string `json:"label"`
	Votes    int64  `json:"votes,omitempty"`
}

type federationEventPoll struct {
	EndsAt           string                      `json:"ends_at"`
	Options          []federationEventPollOption `json:"options"`
	SelectedPosition int                         `json:"selected_position,omitempty"`
}

type federationEventEnvelope struct {
	EventID string                `json:"event_id,omitempty"`
	V       int                   `json:"v"`
	Kind    string                `json:"kind"`
	Author  federationEventAuthor `json:"author"`
	Post    *federationEventPost  `json:"post,omitempty"`
	Reaction *federationEventReaction `json:"reaction,omitempty"`
	DM      *federationEventDM    `json:"dm,omitempty"`
}

type federationEventReaction struct {
	Emoji string `json:"emoji"`
}

type federationSealedBox struct {
	IV   string `json:"iv"`
	Data string `json:"data"`
	KID  string `json:"kid,omitempty"`
}

type federationEventDMAttachment struct {
	PublicURL       string             `json:"public_url"`
	FileName        string             `json:"file_name,omitempty"`
	ContentType     string             `json:"content_type,omitempty"`
	SizeBytes       int64              `json:"size_bytes,omitempty"`
	EncryptedBytes  int64              `json:"encrypted_bytes,omitempty"`
	FileIV          string             `json:"file_iv"`
	SenderKeyBox    federationSealedBox `json:"sender_key_box,omitempty"`
	RecipientKeyBox federationSealedBox `json:"recipient_key_box"`
}

type federationEventDM struct {
	ThreadID         string                    `json:"thread_id"`
	MessageID        string                    `json:"message_id,omitempty"`
	ToAcct           string                    `json:"to_acct"`
	FromAcct         string                    `json:"from_acct"`
	FromKID          string                    `json:"from_kid,omitempty"`
	Capabilities     map[string]any            `json:"capabilities,omitempty"`
	ExpiresAt        string                    `json:"expires_at,omitempty"`
	KeyBoxForInviter *federationSealedBox      `json:"key_box_for_inviter,omitempty"`
	RecipientPayload *federationSealedBox      `json:"recipient_payload,omitempty"`
	SentAt           string                    `json:"sent_at,omitempty"`
	Attachments      []federationEventDMAttachment `json:"attachments,omitempty"`
}

type federationUnlockRequest struct {
	EventID    string `json:"event_id,omitempty"`
	ViewerAcct string `json:"viewer_acct"`
	Password   string `json:"password"`
}

func (s *Server) federationPublicOrigin() string {
	if x := strings.TrimSpace(s.cfg.GlipzProtocolPublicOrigin); x != "" {
		return strings.TrimSuffix(x, "/")
	}
	if x := strings.TrimSpace(s.cfg.FrontendOrigin); x != "" {
		return strings.TrimSuffix(x, "/")
	}
	return ""
}

func (s *Server) federationDisplayHost() string {
	if h := strings.TrimSpace(strings.ToLower(s.cfg.GlipzProtocolHost)); h != "" {
		return strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	}
	origin := s.federationPublicOrigin()
	if origin == "" {
		return ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Host))
}

func (s *Server) federationConfigured(w http.ResponseWriter) bool {
	if s.federationPublicOrigin() == "" {
		if w != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "federation_disabled"})
		}
		return false
	}
	return true
}

func (s *Server) federationServerKeys() (ed25519.PublicKey, ed25519.PrivateKey) {
	sum := sha256.Sum256([]byte(s.cfg.JWTSecret + "|glipz-federation"))
	priv := ed25519.NewKeyFromSeed(sum[:])
	pub := priv.Public().(ed25519.PublicKey)
	return pub, priv
}

func (s *Server) federationServerKeyID() string {
	return strings.TrimSuffix(s.federationPublicOrigin(), "/") + "/.well-known/glipz-federation#default"
}

func (s *Server) federationServerDiscovery(ctx context.Context) federationServerDiscovery {
	pub, _ := s.federationServerKeys()
	base := strings.TrimSuffix(s.federationPublicOrigin(), "/")
	var known []string
	if ctx != nil {
		if rows, err := s.db.ListFederationKnownInstances(ctx, 300); err == nil {
			known = make([]string, 0, len(rows))
			for _, r := range rows {
				if h := strings.TrimSpace(r.Host); h != "" {
					known = append(known, h)
				}
			}
		}
	}
	return federationServerDiscovery{
		ProtocolVersion:           federationProtocolVersion,
		SupportedProtocolVersions: append([]string(nil), federationSupportedProtocolVersions...),
		ServerSoftware:            "glipz",
		ServerVersion:             glipzAppVersion,
		EventSchemaVersion:        federationEventSchemaVersion,
		Host:                      s.federationDisplayHost(),
		Origin:                    base,
		KeyID:                     s.federationServerKeyID(),
		PublicKey:                 base64.StdEncoding.EncodeToString(pub),
		EventsURL:                 base + "/federation/events",
		FollowURL:                 base + "/federation/follow",
		UnfollowURL:               base + "/federation/unfollow",
		DMKeysURL:                 base + "/federation/dm-keys",
		KnownInstances:            known,
	}
}

func (s *Server) mountGlipzFederation(r chi.Router) {
	if strings.TrimSpace(s.federationPublicOrigin()) == "" {
		return
	}
	r.Get("/.well-known/glipz-federation", s.handleGlipzFederationDiscovery)
	r.Get("/federation/profile/{handle}", s.handleFederationProfileByHandle)
	r.Get("/federation/dm-keys/{handle}", s.handleFederationDMKeysByHandle)
	r.Get("/federation/posts/{handle}", s.handleFederationPostsByHandle)
	r.Post("/federation/posts/{postID}/unlock", s.handleFederationPostUnlockInbound)
	r.Post("/federation/follow", s.handleFederationFollowInbound)
	r.Post("/federation/unfollow", s.handleFederationUnfollowInbound)
	r.Post("/federation/events", s.handleFederationEventInbound)
}

func (s *Server) localFullAcct(handle string) string {
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	host := s.federationDisplayHost()
	if handle == "" || host == "" {
		return handle
	}
	return handle + "@" + host
}

func (s *Server) localProfileURL(handle string) string {
	base := strings.TrimSuffix(s.cfg.FrontendOrigin, "/")
	if base == "" {
		base = strings.TrimSuffix(s.federationPublicOrigin(), "/")
	}
	return base + "/@" + url.PathEscape(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
}

func (s *Server) localPostURL(postID uuid.UUID) string {
	base := strings.TrimSuffix(s.cfg.FrontendOrigin, "/")
	if base == "" {
		base = strings.TrimSuffix(s.federationPublicOrigin(), "/")
	}
	return base + "/posts/" + url.PathEscape(postID.String())
}

func (s *Server) localFederationPostUnlockURL(postID uuid.UUID) string {
	return strings.TrimSuffix(s.federationPublicOrigin(), "/") + "/federation/posts/" + url.PathEscape(postID.String()) + "/unlock"
}

func federationPollPayload(p *repo.PostPoll, selectedPosition int) *federationEventPoll {
	if p == nil {
		return nil
	}
	opts := make([]federationEventPollOption, 0, len(p.Options))
	for i, opt := range p.Options {
		opts = append(opts, federationEventPollOption{
			Position: i + 1,
			Label:    opt.Label,
			Votes:    opt.Votes,
		})
	}
	return &federationEventPoll{
		EndsAt:           p.EndsAt.UTC().Format(time.RFC3339),
		Options:          opts,
		SelectedPosition: selectedPosition,
	}
}

func federationPollSnapshotFromEvent(in *federationEventPoll) *repo.FederatedIncomingPollSnapshot {
	if in == nil {
		return nil
	}
	endsAt, err := time.Parse(time.RFC3339, strings.TrimSpace(in.EndsAt))
	if err != nil {
		endsAt = time.Now().UTC()
	}
	opts := make([]repo.FederatedIncomingPollOptionSnapshot, 0, len(in.Options))
	for i, opt := range in.Options {
		pos := opt.Position
		if pos <= 0 {
			pos = i + 1
		}
		opts = append(opts, repo.FederatedIncomingPollOptionSnapshot{
			Position: pos,
			Label:    opt.Label,
			Votes:    opt.Votes,
		})
	}
	return &repo.FederatedIncomingPollSnapshot{
		EndsAt:  endsAt,
		Options: opts,
	}
}

func (s *Server) federationPublicProfileDoc(ctx context.Context, handle string) (federationPublicProfile, error) {
	pfl, err := s.db.PublicProfileByHandle(ctx, handle)
	if err != nil {
		return federationPublicProfile{}, err
	}
	doc := federationPublicProfile{
		Acct:        s.localFullAcct(pfl.Handle),
		Handle:      pfl.Handle,
		Domain:      s.federationDisplayHost(),
		DisplayName: resolvedDisplayName(pfl.DisplayName, pfl.Email),
		Summary:     strings.TrimSpace(pfl.Bio),
		ProfileURL:  s.localProfileURL(pfl.Handle),
		PostsURL:    strings.TrimSuffix(s.federationPublicOrigin(), "/") + "/federation/posts/" + url.PathEscape(pfl.Handle),
	}
	if pfl.AvatarObjectKey != nil && strings.TrimSpace(*pfl.AvatarObjectKey) != "" {
		doc.AvatarURL = s.glipzProtocolPublicMediaURL(*pfl.AvatarObjectKey)
	}
	if pfl.HeaderObjectKey != nil && strings.TrimSpace(*pfl.HeaderObjectKey) != "" {
		doc.HeaderURL = s.glipzProtocolPublicMediaURL(*pfl.HeaderObjectKey)
	}
	return doc, nil
}

func federationSignedRequestTarget(u *url.URL) string {
	if u == nil {
		return "/"
	}
	if p := strings.TrimSpace(u.EscapedPath()); p != "" {
		return p
	}
	return "/"
}

func federationNormalizeIRI(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Hostname()) == "" {
		return "", fmt.Errorf("bad federation iri")
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	port := strings.TrimSpace(u.Port())
	switch {
	case port == "":
	case (scheme == "https" && port == "443") || (scheme == "http" && port == "80"):
		port = ""
	default:
		host = host + ":" + port
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	var b strings.Builder
	b.WriteString(scheme)
	b.WriteString("://")
	b.WriteString(host)
	b.WriteString(path)
	if u.RawQuery != "" {
		b.WriteString("?")
		b.WriteString(u.RawQuery)
	}
	if u.Fragment != "" {
		b.WriteString("#")
		b.WriteString(u.Fragment)
	}
	return b.String(), nil
}

func federationHTTPSURLHost(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || strings.TrimSpace(u.Hostname()) == "" {
		return "", fmt.Errorf("bad federation https url")
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname())), nil
}

func federationValidateDiscoveryForInstance(disc federationAccountDiscovery, instanceHost, keyIDHeader string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(disc.Server.Host), instanceHost) {
		return "", fmt.Errorf("discovery host mismatch")
	}
	originHost, err := federationHTTPSURLHost(disc.Server.Origin)
	if err != nil {
		return "", fmt.Errorf("bad federation origin")
	}
	keyID := strings.TrimSpace(disc.Server.KeyID)
	if keyID == "" {
		return "", fmt.Errorf("missing federation key id")
	}
	normalizedDiscoveryKeyID, err := federationNormalizeIRI(keyID)
	if err != nil {
		return "", fmt.Errorf("bad federation key id")
	}
	if strings.TrimSpace(keyIDHeader) == "" {
		return "", fmt.Errorf("missing federation key id header")
	}
	normalizedHeaderKeyID, err := federationNormalizeIRI(keyIDHeader)
	if err != nil {
		return "", fmt.Errorf("bad federation key id header")
	}
	if normalizedHeaderKeyID != normalizedDiscoveryKeyID {
		return "", fmt.Errorf("federation key id mismatch")
	}
	for _, raw := range []string{keyID, disc.Server.EventsURL, disc.Server.FollowURL, disc.Server.UnfollowURL} {
		host, err := federationHTTPSURLHost(raw)
		if err != nil {
			return "", fmt.Errorf("bad federation discovery url")
		}
		if !strings.EqualFold(host, originHost) {
			return "", fmt.Errorf("federation origin mismatch")
		}
	}
	return normalizedDiscoveryKeyID, nil
}

func (s *Server) verifyFederationRequest(r *http.Request, body []byte) (verifiedFederationRequest, error) {
	instanceHost := strings.TrimSpace(strings.ToLower(r.Header.Get("X-Glipz-Instance")))
	keyIDHeader := strings.TrimSpace(r.Header.Get("X-Glipz-Key-Id"))
	protoHeader := strings.TrimSpace(r.Header.Get("X-Glipz-Protocol-Version"))
	ts := strings.TrimSpace(r.Header.Get("X-Glipz-Timestamp"))
	nonce := strings.TrimSpace(r.Header.Get("X-Glipz-Nonce"))
	sigB64 := strings.TrimSpace(r.Header.Get("X-Glipz-Signature"))
	if instanceHost == "" || keyIDHeader == "" || protoHeader == "" || ts == "" || sigB64 == "" {
		return verifiedFederationRequest{}, fmt.Errorf("missing federation signature headers")
	}
	name, major, ok := parseFederationProtocolVersion(protoHeader)
	if !ok || !strings.EqualFold(name, federationProtocolName) || major < 1 || major > 2 {
		return verifiedFederationRequest{}, fmt.Errorf("unsupported federation protocol")
	}
	if major >= 2 && nonce == "" {
		return verifiedFederationRequest{}, fmt.Errorf("missing federation nonce")
	}
	unixSec, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return verifiedFederationRequest{}, fmt.Errorf("invalid federation timestamp")
	}
	if d := time.Since(unixSec); d > 10*time.Minute || d < -10*time.Minute {
		return verifiedFederationRequest{}, fmt.Errorf("federation timestamp skew")
	}
	disc, err := fetchRemoteFederationDiscovery(r.Context(), instanceHost)
	if err != nil {
		return verifiedFederationRequest{}, err
	}
	if !federationDiscoverySupportsCurrentProtocol(disc.Server) {
		return verifiedFederationRequest{}, fmt.Errorf("unsupported federation protocol")
	}
	normalizedKeyID, err := federationValidateDiscoveryForInstance(disc, instanceHost, keyIDHeader)
	if err != nil {
		return verifiedFederationRequest{}, err
	}
	pubRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(disc.Server.PublicKey))
	if err != nil {
		return verifiedFederationRequest{}, fmt.Errorf("bad federation public key")
	}
	msg := federationSignatureMessage(r.Method, federationSignedRequestTarget(r.URL), ts, nonce, body, major)
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return verifiedFederationRequest{}, fmt.Errorf("bad federation signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), msg, sig) {
		return verifiedFederationRequest{}, fmt.Errorf("invalid federation signature")
	}
	if major >= 2 {
		ok, err := s.acceptFederationNonce(r.Context(), normalizedKeyID, nonce)
		if err != nil {
			return verifiedFederationRequest{}, err
		}
		if !ok {
			return verifiedFederationRequest{}, fmt.Errorf("replayed federation nonce")
		}
	}
	return verifiedFederationRequest{
		InstanceHost:    instanceHost,
		Discovery:       disc,
		KeyID:           keyIDHeader,
		NormalizedKeyID: normalizedKeyID,
		ProtocolVersion: protoHeader,
		ProtocolMajor:   major,
		Nonce:           nonce,
	}, nil
}

func federationSignatureMessage(method, path, ts, nonce string, body []byte, major int) []byte {
	sum := sha256.Sum256(body)
	var b strings.Builder
	b.WriteString(strings.ToUpper(strings.TrimSpace(method)))
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(path))
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(ts))
	b.WriteString("\n")
	if major >= 2 {
		b.WriteString(strings.TrimSpace(nonce))
		b.WriteString("\n")
	}
	b.WriteString(base64.StdEncoding.EncodeToString(sum[:]))
	return []byte(b.String())
}

func federationNewEventID() string {
	return uuid.NewString()
}

const (
	federationReplayNonceTTL = 15 * time.Minute
	federationReplayEventTTL = 7 * 24 * time.Hour
)

func federationNonceRedisKey(normalizedKeyID, nonce string) string {
	return "glipz:federation:nonce:" + normalizedKeyID + ":" + nonce
}

func federationEventRedisKey(normalizedKeyID, eventID string) string {
	return "glipz:federation:event:" + normalizedKeyID + ":" + eventID
}

func (s *Server) acceptFederationNonce(ctx context.Context, normalizedKeyID, nonce string) (bool, error) {
	return s.rdb.SetNX(ctx, federationNonceRedisKey(normalizedKeyID, strings.TrimSpace(nonce)), "1", federationReplayNonceTTL).Result()
}

func (s *Server) validateFederationEventID(verified verifiedFederationRequest, eventID string) (string, error) {
	eventID = strings.TrimSpace(eventID)
	if verified.ProtocolMajor >= 2 && eventID == "" {
		return "", fmt.Errorf("missing federation event id")
	}
	return eventID, nil
}

func (s *Server) federationEventAlreadyProcessed(ctx context.Context, verified verifiedFederationRequest, eventID string) (bool, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false, nil
	}
	n, err := s.rdb.Exists(ctx, federationEventRedisKey(verified.NormalizedKeyID, eventID)).Result()
	return n > 0, err
}

func (s *Server) rememberFederationEventID(ctx context.Context, verified verifiedFederationRequest, eventID string) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil
	}
	return s.rdb.Set(ctx, federationEventRedisKey(verified.NormalizedKeyID, eventID), "1", federationReplayEventTTL).Err()
}

func (s *Server) signedFederationPOSTJSON(ctx context.Context, endpoint string, body any) ([]byte, error) {
	_, priv := s.federationServerKeys()
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	nonce := uuid.NewString()
	msg := federationSignatureMessage(http.MethodPost, federationSignedRequestTarget(req.URL), ts, nonce, buf.Bytes(), 2)
	sig := ed25519.Sign(priv, msg)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Glipz-Instance", s.federationDisplayHost())
	req.Header.Set("X-Glipz-Key-Id", s.federationServerKeyID())
	req.Header.Set("X-Glipz-Protocol-Version", federationProtocolVersion)
	req.Header.Set("X-Glipz-App-Version", glipzAppVersion)
	req.Header.Set("X-Glipz-Timestamp", ts)
	req.Header.Set("X-Glipz-Nonce", nonce)
	req.Header.Set("X-Glipz-Signature", base64.StdEncoding.EncodeToString(sig))
	res, err := federationHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func (s *Server) signedFederationPOST(ctx context.Context, endpoint string, body any) error {
	_, err := s.signedFederationPOSTJSON(ctx, endpoint, body)
	return err
}

func (s *Server) handleGlipzFederationDiscovery(w http.ResponseWriter, r *http.Request) {
	if !s.federationConfigured(w) {
		return
	}
	resource := strings.TrimSpace(strings.TrimPrefix(r.URL.Query().Get("resource"), "@"))
	doc := federationAccountDiscovery{
		Resource: resource,
		Server:   s.federationServerDiscovery(r.Context()),
	}
	if resource == "" || strings.EqualFold(resource, "instance@"+s.federationDisplayHost()) {
		writeJSON(w, http.StatusOK, doc)
		return
	}
	user, host, err := splitAcct(resource)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_resource"})
		return
	}
	if !strings.EqualFold(host, s.federationDisplayHost()) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "wrong_host"})
		return
	}
	pfl, err := s.federationPublicProfileDoc(r.Context(), user)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "federationPublicProfileDoc", err)
		return
	}
	doc.Account = &pfl
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleFederationProfileByHandle(w http.ResponseWriter, r *http.Request) {
	if !s.federationConfigured(w) {
		return
	}
	handle := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if handle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	doc, err := s.federationPublicProfileDoc(r.Context(), handle)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "federationPublicProfileDoc", err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleFederationPostsByHandle(w http.ResponseWriter, r *http.Request) {
	if !s.federationConfigured(w) {
		return
	}
	handle := strings.TrimPrefix(strings.TrimSpace(chi.URLParam(r, "handle")), "@")
	if handle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_handle"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), handle)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle federation posts", err)
		return
	}
	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = n
	}
	var beforePub *time.Time
	var beforeID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		cur, err := decodeFederatedCursor(raw)
		if err != nil || cur.ID == uuid.Nil || cur.PublishedAt.IsZero() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		beforePub = &cur.PublishedAt
		beforeID = &cur.ID
	}
	rows, err := s.db.ListFederationPublicPosts(r.Context(), pfl.ID, limit, beforePub, beforeID)
	if err != nil {
		writeServerError(w, "ListFederationPublicPosts", err)
		return
	}
	items := make([]federationPublicPost, 0, len(rows))
	for _, row := range rows {
		post := s.federationEventPostPayload(row)
		items = append(items, federationPublicPost{
			ID:                     post.ID,
			URL:                    post.URL,
			Caption:                post.Caption,
			MediaType:              post.MediaType,
			MediaURLs:              post.MediaURLs,
			IsNSFW:                 post.IsNSFW,
			PublishedAt:            post.PublishedAt,
			LikeCount:              post.LikeCount,
			Poll:                   post.Poll,
			ReplyToObjectURL:       post.ReplyToObjectURL,
			RepostOfObjectURL:      post.RepostOfObjectURL,
			RepostComment:          post.RepostComment,
			HasViewPassword:        post.HasViewPassword,
			ViewPasswordScope:      post.ViewPasswordScope,
			ViewPasswordTextRanges: post.ViewPasswordTextRanges,
			UnlockURL:              post.UnlockURL,
		})
	}
	out := map[string]any{"items": items}
	if len(rows) == limit {
		last := rows[len(rows)-1]
		out["next_cursor"] = encodeFederatedCursor(last.VisibleAt, last.ID)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFederationPostUnlockInbound(w http.ResponseWriter, r *http.Request) {
	if !s.federationConfigured(w) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	verified, err := s.verifyFederationRequest(r, body)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported federation protocol") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_protocol"})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "postID")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	var req federationUnlockRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if _, err := s.validateFederationEventID(verified, req.EventID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_password"})
		return
	}
	row, err := s.db.PostSensitiveByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostSensitiveByID federation unlock", err)
		return
	}
	if row.ViewPasswordHash == nil || strings.TrimSpace(*row.ViewPasswordHash) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_password"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*row.ViewPasswordHash), []byte(req.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong_password"})
		return
	}
	_ = s.rememberFederationEventID(r.Context(), verified, req.EventID)
	s.writeUnlockedPostJSON(w, row)
}

func (s *Server) handleFederationFollowInbound(w http.ResponseWriter, r *http.Request) {
	if !s.federationConfigured(w) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	verified, err := s.verifyFederationRequest(r, body)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported federation protocol") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_protocol"})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		return
	}
	var req federationFollowRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	eventID, err := s.validateFederationEventID(verified, req.EventID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	if seen, err := s.federationEventAlreadyProcessed(r.Context(), verified, eventID); err == nil && seen {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": "accepted"})
		return
	}
	_, followerHost, err := splitAcct(req.FollowerAcct)
	if err != nil || !strings.EqualFold(followerHost, verified.InstanceHost) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_follower"})
		return
	}
	user, host, err := splitAcct(req.TargetAcct)
	if err != nil || !strings.EqualFold(host, s.federationDisplayHost()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target"})
		return
	}
	canonicalInbox := strings.TrimSpace(verified.Discovery.Server.EventsURL)
	if canonicalInbox == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_follower"})
		return
	}
	if strings.TrimSpace(req.InboxURL) != "" && !sameGlipzProtocolIRI(req.InboxURL, canonicalInbox) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_inbox"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), user)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle inbound follow", err)
		return
	}
	blocked, errB := s.db.HasFederationUserBlock(r.Context(), pfl.ID, strings.TrimSpace(req.FollowerAcct))
	if errB != nil {
		writeServerError(w, "HasFederationUserBlock follow", errB)
		return
	}
	if !blocked {
		if err := s.db.UpsertFederationSubscriber(r.Context(), pfl.ID, strings.TrimSpace(req.FollowerAcct), canonicalInbox); err != nil {
			writeServerError(w, "UpsertFederationSubscriber", err)
			return
		}
	}
	_ = s.rememberFederationEventID(r.Context(), verified, eventID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": "accepted"})
}

func (s *Server) handleFederationUnfollowInbound(w http.ResponseWriter, r *http.Request) {
	if !s.federationConfigured(w) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	verified, err := s.verifyFederationRequest(r, body)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported federation protocol") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_protocol"})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		return
	}
	var req federationFollowRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	eventID, err := s.validateFederationEventID(verified, req.EventID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	if seen, err := s.federationEventAlreadyProcessed(r.Context(), verified, eventID); err == nil && seen {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	_, followerHost, err := splitAcct(req.FollowerAcct)
	if err != nil || !strings.EqualFold(followerHost, verified.InstanceHost) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_follower"})
		return
	}
	user, host, err := splitAcct(req.TargetAcct)
	if err != nil || !strings.EqualFold(host, s.federationDisplayHost()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_target"})
		return
	}
	pfl, err := s.db.PublicProfileByHandle(r.Context(), user)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PublicProfileByHandle inbound unfollow", err)
		return
	}
	if err := s.db.DeleteFederationSubscriber(r.Context(), pfl.ID, strings.TrimSpace(req.FollowerAcct)); err != nil {
		writeServerError(w, "DeleteFederationSubscriber", err)
		return
	}
	_ = s.rememberFederationEventID(r.Context(), verified, eventID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleFederationEventInbound(w http.ResponseWriter, r *http.Request) {
	if !s.federationConfigured(w) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	verified, err := s.verifyFederationRequest(r, body)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported federation protocol") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_protocol"})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		return
	}
	var ev federationEventEnvelope
	if err := json.Unmarshal(body, &ev); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	kind := strings.TrimSpace(ev.Kind)
	if strings.HasPrefix(kind, "dm_") && s.federationInboxPostRateExceeded(r) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
		return
	}
	eventID, err := s.validateFederationEventID(verified, ev.EventID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	if seen, err := s.federationEventAlreadyProcessed(r.Context(), verified, eventID); err == nil && seen {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	ev.V = normalizeFederationEventVersion(ev.V)
	if strings.TrimSpace(ev.Author.Acct) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	if strings.HasPrefix(kind, "dm_") {
		if ev.DM == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
			return
		}
		if err := s.handleFederationDMEventInbound(r.Context(), verified, eventID, ev); err != nil {
			switch {
			case errors.Is(err, repo.ErrForbidden):
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			default:
				writeServerError(w, "handleFederationDMEventInbound", err)
			}
			return
		}
		_ = s.rememberFederationEventID(r.Context(), verified, eventID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if ev.Post == nil || (strings.TrimSpace(ev.Post.ID) == "" && strings.TrimSpace(ev.Post.URL) == "") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	pubAt, err := time.Parse(time.RFC3339, strings.TrimSpace(ev.Post.PublishedAt))
	if err != nil {
		pubAt = time.Now().UTC()
	}
	objectID := strings.TrimSpace(ev.Post.URL)
	if objectID == "" {
		objectID = fmt.Sprintf("glipz://%s/posts/%s", strings.TrimSpace(ev.Author.Acct), strings.TrimSpace(ev.Post.ID))
	}
	var localPostID uuid.UUID
	hasLocalPost := false
	if postID := strings.TrimSpace(ev.Post.ID); postID != "" {
		if parsed, err := uuid.Parse(postID); err == nil {
			if ok, err := s.db.PostExists(r.Context(), parsed); err == nil && ok {
				localPostID = parsed
				hasLocalPost = true
			}
		}
	}
	if !hasLocalPost {
		if u, err := url.Parse(objectID); err == nil {
			base, _ := url.Parse(s.federationPublicOrigin())
			if base != nil && strings.EqualFold(base.Host, u.Host) {
				parts := strings.Split(strings.Trim(strings.TrimSpace(u.Path), "/"), "/")
				if len(parts) >= 2 && strings.EqualFold(parts[len(parts)-2], "posts") {
					if parsed, err := uuid.Parse(parts[len(parts)-1]); err == nil {
						if ok, err := s.db.PostExists(r.Context(), parsed); err == nil && ok {
							localPostID = parsed
							hasLocalPost = true
						}
					}
				}
			}
		}
	}
	pollSnapshot := federationPollSnapshotFromEvent(ev.Post.Poll)
	switch kind {
	case "post_created", "repost_created":
		in := repo.InsertFederatedIncomingInput{
			ObjectIRI:              objectID,
			CreateActivityIRI:      kind,
			ActorIRI:               ev.Author.Acct,
			ActorAcct:              ev.Author.Acct,
			ActorName:              ev.Author.DisplayName,
			ActorIconURL:           ev.Author.AvatarURL,
			ActorProfileURL:        ev.Author.ProfileURL,
			CaptionText:            ev.Post.Caption,
			MediaType:              ev.Post.MediaType,
			MediaURLs:              ev.Post.MediaURLs,
			IsNSFW:                 ev.Post.IsNSFW,
			PublishedAt:            pubAt,
			LikeCount:              ev.Post.LikeCount,
			ReplyToObjectIRI:       ev.Post.ReplyToObjectURL,
			RepostOfObjectIRI:      ev.Post.RepostOfObjectURL,
			RepostComment:          ev.Post.RepostComment,
			HasViewPassword:        ev.Post.HasViewPassword,
			ViewPasswordScope:      ev.Post.ViewPasswordScope,
			ViewPasswordTextRanges: jsonRangesToRepo(ev.Post.ViewPasswordTextRanges),
			UnlockURL:              ev.Post.UnlockURL,
		}
		if err := s.db.UpdateFederationIncomingPost(r.Context(), in); err != nil {
			writeServerError(w, "UpdateFederationIncomingPost", err)
			return
		}
		if err := s.db.SyncFederatedIncomingPollByObjectIRI(r.Context(), objectID, pollSnapshot); err != nil {
			writeServerError(w, "SyncFederatedIncomingPollByObjectIRI", err)
			return
		}
		s.publishFederatedIncomingUpsertByObjectIRI(r.Context(), objectID)
	case "post_updated":
		if err := s.db.UpdateFederatedIncomingFromNote(
			r.Context(),
			objectID,
			ev.Post.Caption,
			ev.Post.MediaType,
			ev.Post.MediaURLs,
			ev.Post.IsNSFW,
			pubAt,
			ev.Post.LikeCount,
			ev.Post.ReplyToObjectURL,
			ev.Post.RepostOfObjectURL,
			ev.Post.RepostComment,
			ev.Post.HasViewPassword,
			ev.Post.ViewPasswordScope,
			jsonRangesToRepo(ev.Post.ViewPasswordTextRanges),
			ev.Post.UnlockURL,
		); err != nil {
			writeServerError(w, "UpdateFederatedIncomingFromNote", err)
			return
		}
		if err := s.db.SyncFederatedIncomingPollByObjectIRI(r.Context(), objectID, pollSnapshot); err != nil {
			writeServerError(w, "SyncFederatedIncomingPollByObjectIRI", err)
			return
		}
		if err := s.db.UpdateFederatedIncomingActorDisplay(r.Context(), ev.Author.Acct, ev.Author.Acct, ev.Author.DisplayName, ev.Author.AvatarURL, ev.Author.ProfileURL); err != nil {
			writeServerError(w, "UpdateFederatedIncomingActorDisplay", err)
			return
		}
		s.publishFederatedIncomingUpsertByObjectIRI(r.Context(), objectID)
	case "post_deleted":
		row, _ := s.db.GetFederatedIncomingByObjectIRI(r.Context(), objectID)
		if err := s.db.SoftDeleteFederatedIncomingByObjectIRI(r.Context(), objectID); err != nil {
			writeServerError(w, "SoftDeleteFederatedIncomingByObjectIRI", err)
			return
		}
		if row.ID != uuid.Nil {
			s.publishFederatedIncomingDelete(r.Context(), row)
		}
	case "post_liked", "post_unliked":
		liked := kind == "post_liked"
		if hasLocalPost {
			blocked, errBl := s.db.PostAuthorHasFederationBlock(r.Context(), localPostID, ev.Author.Acct)
			if errBl != nil && !errors.Is(errBl, repo.ErrNotFound) {
				writeServerError(w, "PostAuthorHasFederationBlock like", errBl)
				return
			}
			if blocked {
				_ = s.rememberFederationEventID(r.Context(), verified, eventID)
				writeJSON(w, http.StatusOK, map[string]any{"ok": true})
				return
			}
			changed, count, err := s.db.ApplyRemoteLikeToLocalPost(r.Context(), localPostID, ev.Author.Acct, ev.Author.Acct, liked)
			if err != nil && !errors.Is(err, repo.ErrNotFound) {
				writeServerError(w, "ApplyRemoteLikeToLocalPost", err)
				return
			}
			if changed {
				if ownerID, isRoot, err := s.db.PostFeedMeta(r.Context(), localPostID); err == nil {
					b, _ := json.Marshal(map[string]any{
						"v":         1,
						"kind":      "post_updated",
						"post_id":   localPostID.String(),
						"author_id": ownerID.String(),
					})
					if isRoot {
						s.publishFeedEventJSON(r.Context(), b, ownerID, repo.PostVisibilityPublic)
					}
					s.deliverFederationLikeEventToSubscribers(r.Context(), ownerID, ev.Author, localPostID, count, liked)
				}
			}
			break
		}
		if err := s.db.SetFederatedIncomingLikeCountByObjectIRI(r.Context(), objectID, ev.Post.LikeCount); err != nil {
			writeServerError(w, "SetFederatedIncomingLikeCountByObjectIRI", err)
			return
		}
		s.publishFederatedIncomingUpsertByObjectIRI(r.Context(), objectID)
	case "post_reaction_added", "post_reaction_removed":
		if ev.Reaction == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
			return
		}
		emoji, valid := repo.NormalizePostReactionEmoji(ev.Reaction.Emoji)
		if !valid {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
			return
		}
		added := kind == "post_reaction_added"
		if hasLocalPost {
			blocked, errBl := s.db.PostAuthorHasFederationBlock(r.Context(), localPostID, ev.Author.Acct)
			if errBl != nil && !errors.Is(errBl, repo.ErrNotFound) {
				writeServerError(w, "PostAuthorHasFederationBlock reaction", errBl)
				return
			}
			if blocked {
				_ = s.rememberFederationEventID(r.Context(), verified, eventID)
				writeJSON(w, http.StatusOK, map[string]any{"ok": true})
				return
			}
			changed, err := s.db.ApplyRemoteReactionToLocalPost(r.Context(), localPostID, ev.Author.Acct, ev.Author.Acct, emoji, added)
			if err != nil && !errors.Is(err, repo.ErrNotFound) {
				writeServerError(w, "ApplyRemoteReactionToLocalPost", err)
				return
			}
			if changed {
				if ownerID, isRoot, err := s.db.PostFeedMeta(r.Context(), localPostID); err == nil {
					b, _ := json.Marshal(map[string]any{
						"v":         1,
						"kind":      "post_updated",
						"post_id":   localPostID.String(),
						"author_id": ownerID.String(),
					})
					if isRoot {
						s.publishFeedEventJSON(r.Context(), b, ownerID, repo.PostVisibilityPublic)
					}
					s.deliverFederationReactionEventToSubscribers(r.Context(), ownerID, ev.Author, localPostID, emoji, added)
				}
			}
			break
		}
		// For now, federated incoming posts only sync like_count.
		// Reaction aggregation for federated incoming posts can be added later.
	case "poll_voted":
		selectedPosition := 0
		if ev.Post.Poll != nil {
			selectedPosition = ev.Post.Poll.SelectedPosition
		}
		if !hasLocalPost || selectedPosition <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
			return
		}
		blocked, errBl := s.db.PostAuthorHasFederationBlock(r.Context(), localPostID, ev.Author.Acct)
		if errBl != nil && !errors.Is(errBl, repo.ErrNotFound) {
			writeServerError(w, "PostAuthorHasFederationBlock poll", errBl)
			return
		}
		if blocked {
			_ = s.rememberFederationEventID(r.Context(), verified, eventID)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		changed, err := s.db.ApplyRemotePollVoteToLocalPost(r.Context(), localPostID, ev.Author.Acct, ev.Author.Acct, selectedPosition)
		if err != nil {
			switch {
			case errors.Is(err, repo.ErrPollNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "poll_not_found"})
			case errors.Is(err, repo.ErrPollClosed):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "poll_closed"})
			case errors.Is(err, repo.ErrPollInvalidOption):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_option"})
			default:
				writeServerError(w, "ApplyRemotePollVoteToLocalPost", err)
			}
			return
		}
		if changed {
			if ownerID, isRoot, err := s.db.PostFeedMeta(r.Context(), localPostID); err == nil {
				b, _ := json.Marshal(map[string]any{
					"v":         1,
					"kind":      "post_updated",
					"post_id":   localPostID.String(),
					"author_id": ownerID.String(),
				})
				if isRoot {
					s.publishFeedEventJSON(r.Context(), b, ownerID, repo.PostVisibilityPublic)
				}
				s.deliverFederationPollTallyUpdated(r.Context(), ownerID, localPostID)
			}
		}
	case "poll_tally_updated":
		if err := s.db.SetFederatedIncomingLikeCountByObjectIRI(r.Context(), objectID, ev.Post.LikeCount); err != nil {
			writeServerError(w, "SetFederatedIncomingLikeCountByObjectIRI", err)
			return
		}
		if err := s.db.SyncFederatedIncomingPollByObjectIRI(r.Context(), objectID, pollSnapshot); err != nil {
			writeServerError(w, "SyncFederatedIncomingPollByObjectIRI", err)
			return
		}
		s.publishFederatedIncomingUpsertByObjectIRI(r.Context(), objectID)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_event"})
		return
	}
	_ = s.rememberFederationEventID(r.Context(), verified, eventID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

