package httpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

var htmlTagStripper = regexp.MustCompile(`(?i)<[^>]+>`)

func stripHTMLToCaption(s string) string {
	s = htmlTagStripper.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func actorURLFromKeyID(keyID string) string {
	if i := strings.IndexByte(keyID, '#'); i >= 0 {
		return strings.TrimSpace(keyID[:i])
	}
	return strings.TrimSpace(keyID)
}

func fetchActorMeta(ctx context.Context, actorURL string) (acct, name, iconURL, profileURL string, err error) {
	actorURL = strings.TrimSpace(actorURL)
	if actorURL == "" {
		return "", "", "", "", fmt.Errorf("empty actor")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil)
	if err != nil {
		return "", "", "", "", err
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	res, err := federationHTTP.Do(req)
	if err != nil {
		return "", "", "", "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return "", "", "", "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", "", "", "", fmt.Errorf("actor http %d", res.StatusCode)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", "", "", "", err
	}
	u, err := url.Parse(actorURL)
	if err != nil {
		return "", "", "", "", err
	}
	host := u.Hostname()
	pu, _ := doc["preferredUsername"].(string)
	pu = strings.TrimSpace(pu)
	if pu != "" && host != "" {
		acct = pu + "@" + host
	} else {
		acct = host
	}
	if n, ok := doc["name"].(string); ok {
		name = strings.TrimSpace(n)
	}
	if icon, ok := doc["icon"].(map[string]any); ok {
		if uu, ok := icon["url"].(string); ok {
			iconURL = strings.TrimSpace(uu)
		}
	}
	if id, ok := doc["id"].(string); ok && strings.TrimSpace(id) != "" {
		profileURL = strings.TrimSpace(id)
	} else {
		profileURL = actorURL
	}
	return acct, name, iconURL, profileURL, nil
}

func parseNoteObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty object")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
		return nil, fmt.Errorf("object by reference only not supported")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	t, _ := m["type"].(string)
	if !strings.EqualFold(strings.TrimSpace(t), "Note") {
		return nil, fmt.Errorf("object is not Note")
	}
	return m, nil
}

func isFederationAudioAttachmentURL(u string) bool {
	lu := strings.ToLower(u)
	for _, ext := range []string{".mp3", ".m4a", ".wav", ".ogg", ".opus", ".flac", ".aac"} {
		if strings.HasSuffix(lu, ext) {
			return true
		}
	}
	return false
}

func noteAttachmentsToMedia(note map[string]any) (mediaType string, urls []string) {
	raw, ok := note["attachment"].([]any)
	if !ok || len(raw) == 0 {
		return "none", nil
	}
	var imgURLs []string
	var vidURL string
	var audURL string
	for _, a := range raw {
		o, ok := a.(map[string]any)
		if !ok {
			continue
		}
		t, _ := o["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(t), "Document") && !strings.EqualFold(strings.TrimSpace(t), "Image") {
			continue
		}
		u, _ := o["url"].(string)
		u = strings.TrimSpace(u)
		if u == "" || !strings.HasPrefix(u, "https://") {
			continue
		}
		mt, _ := o["mediaType"].(string)
		mt = strings.ToLower(strings.TrimSpace(mt))
		if strings.HasPrefix(mt, "audio/") {
			audURL = u
		} else if strings.HasPrefix(mt, "video/") {
			vidURL = u
		} else if strings.HasPrefix(mt, "image/") {
			imgURLs = append(imgURLs, u)
		} else if strings.HasSuffix(strings.ToLower(u), ".mp4") || strings.HasSuffix(strings.ToLower(u), ".webm") {
			vidURL = u
		} else if isFederationAudioAttachmentURL(u) {
			audURL = u
		} else {
			imgURLs = append(imgURLs, u)
		}
	}
	if vidURL != "" {
		return "video", []string{vidURL}
	}
	if audURL != "" {
		return "audio", []string{audURL}
	}
	if len(imgURLs) == 0 {
		return "none", nil
	}
	if len(imgURLs) > 4 {
		imgURLs = imgURLs[:4]
	}
	return "image", imgURLs
}

func parseTimeRFC3339(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	return time.Parse(time.RFC3339, s)
}

func (s *Server) apInboxCreate(ctx context.Context, recipient *uuid.UUID, body []byte, envelope map[string]json.RawMessage, signerKeyID string) error {
	actorRaw, ok := envelope["actor"]
	if !ok {
		return fmt.Errorf("missing actor")
	}
	actorStr, ok := jsonStringField(actorRaw)
	if !ok || actorStr == "" {
		return fmt.Errorf("bad actor")
	}
	keyActor := actorURLFromKeyID(signerKeyID)
	if !strings.EqualFold(strings.TrimSuffix(keyActor, "/"), strings.TrimSuffix(actorStr, "/")) {
		return fmt.Errorf("create actor mismatch signature")
	}
	objRaw, ok := envelope["object"]
	if !ok {
		return fmt.Errorf("missing object")
	}
	note, err := parseNoteObject(objRaw)
	if err != nil {
		return err
	}
	var createID string
	if raw, ok := envelope["id"]; ok {
		var sid string
		if json.Unmarshal(raw, &sid) == nil {
			createID = strings.TrimSpace(sid)
		}
	}
	return s.persistFederatedIncomingNote(ctx, recipient, actorStr, note, createID)
}

// persistFederatedIncomingNote stores a federated inbound row from a Note map, such as Create or Announce.
func (s *Server) persistFederatedIncomingNote(ctx context.Context, recipient *uuid.UUID, actorStr string, note map[string]any, createActivityIRI string) error {
	noteID, _ := note["id"].(string)
	noteID = strings.TrimSpace(noteID)
	if noteID == "" {
		return fmt.Errorf("note without id")
	}
	content, _ := note["content"].(string)
	sensitive, _ := note["sensitive"].(bool)
	pubStr, _ := note["published"].(string)
	pubAt, err := parseTimeRFC3339(pubStr)
	if err != nil {
		pubAt = time.Now().UTC()
	}
	mt, urls := noteAttachmentsToMedia(note)
	acct, dispName, iconURL, profileURL, err := fetchActorMeta(ctx, actorStr)
	if err != nil {
		log.Printf("glipz protocol create fetch actor meta: %v", err)
		acct = strings.TrimPrefix(strings.TrimPrefix(actorStr, "https://"), "http://")
		profileURL = actorStr
	}
	in := repo.InsertFederatedIncomingInput{
		ObjectIRI:         noteID,
		CreateActivityIRI: strings.TrimSpace(createActivityIRI),
		ActorIRI:          actorStr,
		ActorAcct:         acct,
		ActorName:         dispName,
		ActorIconURL:      iconURL,
		ActorProfileURL:   profileURL,
		CaptionText:       stripHTMLToCaption(content),
		MediaType:         mt,
		MediaURLs:         urls,
		IsNSFW:            sensitive,
		PublishedAt:       pubAt,
		RecipientUserID:   recipient,
	}
	if utf8.RuneCountInString(in.CaptionText) == 0 && len(in.MediaURLs) == 0 {
		return fmt.Errorf("empty note")
	}
	if _, err = s.db.InsertFederatedIncomingPost(ctx, in); err != nil {
		return err
	}
	s.publishFederatedIncomingUpsertByObjectIRI(ctx, in.ObjectIRI)
	return nil
}

func parseDeleteObjectIRI(envelope map[string]json.RawMessage) (string, error) {
	raw, ok := envelope["object"]
	if !ok {
		return "", fmt.Errorf("missing object")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s), nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id), nil
	}
	return "", fmt.Errorf("delete object without id")
}

func (s *Server) apInboxDeleteActivity(ctx context.Context, envelope map[string]json.RawMessage) error {
	iri, err := parseDeleteObjectIRI(envelope)
	if err != nil {
		return err
	}
	row, err := s.db.GetFederatedIncomingByObjectIRI(ctx, iri)
	if err != nil {
		return s.db.SoftDeleteFederatedIncomingByObjectIRI(ctx, iri)
	}
	if err := s.db.SoftDeleteFederatedIncomingByObjectIRI(ctx, iri); err != nil {
		return err
	}
	s.publishFederatedIncomingDelete(ctx, row)
	return nil
}

func (s *Server) handleGlipzProtocolSharedInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.federationInboxPostRateExceeded(r) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	keyID, _, err := verifyHTTPSignature(r, body)
	if err != nil {
		log.Printf("glipz protocol shared inbox: bad signature: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if rh, errH := federationRemoteHostFromSignerKeyID(keyID); errH == nil {
		blocked, errB := s.db.IsFederationDomainBlocked(r.Context(), rh)
		if errB == nil && blocked {
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	typeField := activityTypeString(envelope)
	switch strings.ToLower(typeField) {
	case "create":
		if err := s.apInboxCreate(r.Context(), nil, body, envelope, keyID); err != nil {
			log.Printf("glipz protocol shared inbox create: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	case "delete":
		if err := s.apInboxDeleteActivity(r.Context(), envelope); err != nil {
			log.Printf("glipz protocol shared inbox delete: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	case "accept":
		if err := s.apSharedInboxAcceptOutbound(r.Context(), envelope, keyID); err != nil {
			log.Printf("glipz protocol shared inbox accept: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	case "update":
		if err := s.apInboxUpdateActivity(r.Context(), envelope, keyID); err != nil {
			log.Printf("glipz protocol shared inbox update: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	case "announce":
		if err := s.apInboxAnnounceActivity(r.Context(), nil, envelope, keyID); err != nil {
			log.Printf("glipz protocol shared inbox announce: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	case "move":
		if err := s.apInboxMoveActivity(r.Context(), envelope); err != nil {
			log.Printf("glipz protocol shared inbox move: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	default:
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// normalizeActivityTypeToken reduces JSON-LD type tokens to ActivityStreams short names.
func normalizeActivityTypeToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '#'); i >= 0 && i < len(s)-1 {
		return strings.TrimSpace(s[i+1:])
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		s = strings.TrimSuffix(s, "/")
		if slash := strings.LastIndex(s, "/"); slash >= 0 && slash < len(s)-1 {
			return strings.TrimSpace(s[slash+1:])
		}
	}
	return s
}

// activityPrimaryType returns the primary activity type from envelope["type"], whether it is a string or an array.
func activityPrimaryType(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
		return normalizeActivityTypeToken(s)
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return ""
	}
	for i := len(arr) - 1; i >= 0; i-- {
		s, ok := arr[i].(string)
		if !ok {
			continue
		}
		if t := normalizeActivityTypeToken(s); t != "" {
			return t
		}
	}
	return ""
}

func activityTypeString(envelope map[string]json.RawMessage) string {
	raw, ok := envelope["type"]
	if !ok {
		return ""
	}
	return activityPrimaryType(raw)
}

func (s *Server) handleFederatedFeed(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	const limit = 50
	var beforePub *time.Time
	var beforeID *uuid.UUID
	if cur := strings.TrimSpace(r.URL.Query().Get("cursor")); cur != "" {
		raw, err := decodeFederatedCursor(cur)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_cursor"})
			return
		}
		beforePub = &raw.PublishedAt
		beforeID = &raw.ID
	}
	rows, err := s.db.ListFederatedIncomingForViewer(r.Context(), uid, limit, beforePub, beforeID)
	if err != nil {
		writeServerError(w, "ListFederatedIncoming", err)
		return
	}
	items := make([]feedItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.federatedIncomingToFeedItem(row))
	}
	out := map[string]any{"items": items}
	if len(rows) == limit {
		last := rows[len(rows)-1]
		out["next_cursor"] = encodeFederatedCursor(last.PublishedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, out)
}

type federatedCursorPayload struct {
	PublishedAt time.Time `json:"p"`
	ID          uuid.UUID `json:"i"`
}

func encodeFederatedCursor(t time.Time, id uuid.UUID) string {
	b, _ := json.Marshal(federatedCursorPayload{PublishedAt: t.UTC(), ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeFederatedCursor(s string) (federatedCursorPayload, error) {
	var p federatedCursorPayload
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	return p, nil
}

func (s *Server) federatedIncomingToFeedItem(row repo.FederatedIncomingPost) feedItem {
	acct := strings.TrimSpace(row.ActorAcct)
	if acct == "" {
		acct = "remote"
	}
	synthEmail := "fed+" + row.ID.String()[:8] + "@federated.invalid"
	disp := strings.TrimSpace(row.ActorName)
	if disp == "" {
		disp = acct
	}
	prof := strings.TrimSpace(row.ActorProfileURL)
	if prof == "" {
		prof = strings.TrimSpace(row.ActorIRI)
	}
	ca := strings.ToLower(strings.TrimSpace(row.CreateActivityIRI))
	fedBoost := strings.Contains(ca, "/announce/") || ca == "repost_created" || strings.Contains(ca, "repost")
	caption := row.CaptionText
	mediaType := row.MediaType
	mediaURLs := append([]string(nil), row.MediaURLs...)
	isNSFW := row.IsNSFW
	contentLocked := false
	textLocked := false
	mediaLocked := false
	unlocked := strings.TrimSpace(row.UnlockedMediaType) != ""
	hasMembershipLock := strings.TrimSpace(row.MembershipProvider) != "" || (!row.HasViewPassword && strings.TrimSpace(row.UnlockURL) != "")
	if (row.HasViewPassword || hasMembershipLock) && !unlocked {
		if scopeProtectsText(row.ViewPasswordScope) {
			textLocked = true
		}
		if scopeProtectsMedia(row.ViewPasswordScope) {
			mediaLocked = true
		}
		if hasMembershipLock && !textLocked && !mediaLocked {
			// Membership locks always hide both text and media when not yet unlocked.
			textLocked = true
			mediaLocked = true
		}
		contentLocked = textLocked || mediaLocked
	} else if unlocked {
		caption = row.UnlockedCaptionText
		mediaType = row.UnlockedMediaType
		mediaURLs = append([]string(nil), row.UnlockedMediaURLs...)
		isNSFW = row.UnlockedIsNSFW
	}
	var repost *feedRepostMetaJSON
	if fedBoost {
		repost = &feedRepostMetaJSON{
			UserID:          row.ID.String(),
			UserEmail:       synthEmail,
			UserHandle:      acct,
			UserDisplayName: disp,
			UserAvatarURL:   strings.TrimSpace(row.ActorIconURL),
			RepostedAt:      row.PublishedAt.UTC().Format(time.RFC3339),
		}
		if c := strings.TrimSpace(row.RepostComment); c != "" {
			repost.Comment = c
		}
	}
	return feedItem{
		ID:                     "federated:" + row.ID.String(),
		UserEmail:              synthEmail,
		UserHandle:             acct,
		UserDisplayName:        disp,
		UserAvatarURL:          strings.TrimSpace(row.ActorIconURL),
		Caption:                caption,
		MediaType:              mediaType,
		MediaURLs:              mediaURLs,
		IsNSFW:                 isNSFW,
		HasViewPassword:        row.HasViewPassword,
		HasMembershipLock:      hasMembershipLock,
		MembershipProvider:     row.MembershipProvider,
		MembershipCreatorID:    row.MembershipCreatorID,
		MembershipTierID:       row.MembershipTierID,
		ViewPasswordScope:      row.ViewPasswordScope,
		ViewPasswordTextRanges: repoRangesToJSON(row.ViewPasswordTextRanges),
		ContentLocked:          contentLocked,
		TextLocked:             textLocked,
		MediaLocked:            mediaLocked,
		CreatedAt:              row.ReceivedAt.UTC().Format(time.RFC3339),
		VisibleAt:              row.PublishedAt.UTC().Format(time.RFC3339),
		Poll:                   buildFeedPoll(row.Poll),
		Reactions:              feedReactionsJSON(row.Reactions),
		ReplyCount:             0,
		LikeCount:              row.LikeCount,
		RepostCount:            row.RepostCount,
		LikedByMe:              row.LikedByMe,
		RepostedByMe:           row.RepostedByMe,
		BookmarkedByMe:         row.BookmarkedByMe,
		FeedEntryID:            "federated:" + row.ID.String(),
		IsFederated:            true,
		FederatedBoost:         fedBoost,
		RemoteObjectURL:        row.ObjectIRI,
		RemoteActorURL:         prof,
		Repost:                 repost,
		ReplyToObjectURL:       strings.TrimSpace(row.ReplyToObjectIRI),
	}
}

func trimIRIEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(a), "/"), strings.TrimSuffix(strings.TrimSpace(b), "/"))
}

// federationActivityURLAllowed validates whether a URL is safe to fetch during inbox processing.
// This is only a lightweight SSRF guard. Production hardening should also review
// loopback and metadata ranges such as 169.254.0.0/16, redirect limits,
// body size limits versus inbox rate limiting, and media proxy policy.
func federationActivityURLAllowed(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("invalid federation url")
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" {
		return nil, fmt.Errorf("blocked host")
	}
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return nil, fmt.Errorf("blocked ip")
		}
	}
	return u, nil
}

func federationFetchActivityDoc(ctx context.Context, documentURL string) (map[string]any, error) {
	u, err := federationActivityURLAllowed(documentURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	res, err := federationHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch status %d", res.StatusCode)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func activityDocToNoteMap(doc map[string]any) (map[string]any, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil doc")
	}
	t, _ := doc["type"].(string)
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "note":
		return doc, nil
	case "create":
		raw, ok := doc["object"]
		if !ok {
			return nil, fmt.Errorf("create without object")
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return parseNoteObject(json.RawMessage(b))
	default:
		return nil, fmt.Errorf("unsupported activity type %q", t)
	}
}

func resolveAnnounceObjectToNote(ctx context.Context, objRaw json.RawMessage) (map[string]any, error) {
	if s, ok := jsonStringField(objRaw); ok && s != "" {
		doc, err := federationFetchActivityDoc(ctx, s)
		if err != nil {
			return nil, err
		}
		return activityDocToNoteMap(doc)
	}
	var m map[string]any
	if err := json.Unmarshal(objRaw, &m); err != nil {
		return nil, err
	}
	return activityDocToNoteMap(m)
}

func noteAttributedToString(note map[string]any) string {
	raw := note["attributedTo"]
	if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if arr, ok := raw.([]any); ok {
		for _, v := range arr {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func (s *Server) apInboxUpdateActivity(ctx context.Context, envelope map[string]json.RawMessage, signerKeyID string) error {
	objRaw, ok := envelope["object"]
	if !ok {
		return fmt.Errorf("missing object")
	}
	var obj map[string]any
	if s, ok := jsonStringField(objRaw); ok && s != "" {
		doc, err := federationFetchActivityDoc(ctx, s)
		if err != nil {
			return err
		}
		obj = doc
	} else if err := json.Unmarshal(objRaw, &obj); err != nil {
		return err
	}
	typ, _ := obj["type"].(string)
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "note":
		b, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		note, err := parseNoteObject(json.RawMessage(b))
		if err != nil {
			return err
		}
		attr := noteAttributedToString(note)
		if attr == "" {
			return fmt.Errorf("note without attributedTo")
		}
		if !trimIRIEqual(actorURLFromKeyID(signerKeyID), attr) {
			return fmt.Errorf("update note: signer must match attributedTo")
		}
		noteID, _ := note["id"].(string)
		noteID = strings.TrimSpace(noteID)
		if noteID == "" {
			return fmt.Errorf("note without id")
		}
		content, _ := note["content"].(string)
		sensitive, _ := note["sensitive"].(bool)
		pubStr, _ := note["published"].(string)
		pubAt, err := parseTimeRFC3339(pubStr)
		if err != nil {
			pubAt = time.Now().UTC()
		}
		mt, urls := noteAttachmentsToMedia(note)
		if err := s.db.UpdateFederatedIncomingFromNote(ctx, noteID, stripHTMLToCaption(content), mt, urls, sensitive, pubAt, 0, "", "", "", false, 0, nil, "", "", "", ""); err != nil {
			return err
		}
		s.publishFederatedIncomingUpsertByObjectIRI(ctx, noteID)
		return nil
	case "person", "service":
		id, _ := obj["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("actor without id")
		}
		if !trimIRIEqual(actorURLFromKeyID(signerKeyID), id) {
			return fmt.Errorf("update actor: signer mismatch")
		}
		acct, name, icon, profile, err := fetchActorMeta(ctx, id)
		if err != nil {
			log.Printf("update actor fetch meta: %v", err)
			acct = strings.TrimPrefix(strings.TrimPrefix(id, "https://"), "http://")
			profile = id
		}
		return s.db.UpdateFederatedIncomingActorDisplay(ctx, id, acct, name, icon, profile)
	default:
		return nil
	}
}

func (s *Server) apInboxAnnounceActivity(ctx context.Context, recipient *uuid.UUID, envelope map[string]json.RawMessage, signerKeyID string) error {
	actorRaw, ok := envelope["actor"]
	if !ok {
		return fmt.Errorf("missing actor")
	}
	announcer, ok := jsonStringField(actorRaw)
	if !ok || announcer == "" {
		return fmt.Errorf("bad actor")
	}
	if !trimIRIEqual(actorURLFromKeyID(signerKeyID), announcer) {
		return fmt.Errorf("announce actor mismatch signature")
	}
	objRaw, ok := envelope["object"]
	if !ok {
		return fmt.Errorf("missing object")
	}
	note, err := resolveAnnounceObjectToNote(ctx, objRaw)
	if err != nil {
		return err
	}
	// The federated timeline is filtered by followed actor_iri values.
	// For Mastodon boosts, the followed actor is the booster, so stored and displayed actor data should use the announcer.
	var announceID string
	if raw, ok := envelope["id"]; ok {
		var sid string
		if json.Unmarshal(raw, &sid) == nil {
			announceID = strings.TrimSpace(sid)
		}
	}
	if err := s.persistFederatedIncomingNote(ctx, recipient, announcer, note, announceID); err != nil {
		if strings.Contains(err.Error(), "empty note") {
			return nil
		}
		return err
	}
	return nil
}

func (s *Server) apInboxMoveActivity(ctx context.Context, envelope map[string]json.RawMessage) error {
	actorStr, ok := jsonStringField(envelope["actor"])
	if !ok || actorStr == "" {
		return fmt.Errorf("missing actor")
	}
	objStr, ok := jsonStringField(envelope["object"])
	if !ok || objStr == "" {
		return fmt.Errorf("missing object")
	}
	targetStr, ok := jsonStringField(envelope["target"])
	if !ok || targetStr == "" {
		return fmt.Errorf("missing target")
	}
	if !trimIRIEqual(actorStr, objStr) {
		return fmt.Errorf("move actor object mismatch")
	}
	if err := s.db.RepointFederatedIncomingActor(ctx, actorStr, targetStr); err != nil {
		return err
	}
	if err := s.db.RepointRemoteFollowRemoteActor(ctx, actorStr, targetStr); err != nil {
		return err
	}
	return s.db.RepointGlipzProtocolRemoteFollowerActor(ctx, actorStr, targetStr)
}
