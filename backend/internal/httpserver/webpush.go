package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

type webPushNotification struct {
	Version            int    `json:"v"`
	Kind               string `json:"kind"`
	Title              string `json:"title"`
	Body               string `json:"body"`
	URL                string `json:"url"`
	Tag                string `json:"tag"`
	Icon               string `json:"icon,omitempty"`
	Badge              string `json:"badge,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	ThreadID           string `json:"thread_id,omitempty"`
	NotificationID     string `json:"notification_id,omitempty"`
	MessageID          string `json:"message_id,omitempty"`
	RequireInteraction bool   `json:"require_interaction,omitempty"`
}

func (s *Server) queueWebPush(recipientID uuid.UUID, payload webPushNotification) {
	if !s.cfg.WebPushEnabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s.sendWebPushToUser(ctx, recipientID, payload)
	}()
}

func (s *Server) sendWebPushToUser(ctx context.Context, recipientID uuid.UUID, payload webPushNotification) {
	items, err := s.db.ListPushSubscriptionsByUser(ctx, recipientID)
	if err != nil {
		log.Printf("ListPushSubscriptionsByUser: %v", err)
		return
	}
	if len(items) == 0 {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal web push payload: %v", err)
		return
	}
	for _, item := range items {
		resp, err := webpush.SendNotification(body, &webpush.Subscription{
			Endpoint: item.Endpoint,
			Keys: webpush.Keys{
				P256dh: item.P256DH,
				Auth:   item.Auth,
			},
		}, &webpush.Options{
			Subscriber:      s.cfg.WebPushVAPIDSubject,
			VAPIDPublicKey:  s.cfg.WebPushVAPIDPublicKey,
			VAPIDPrivateKey: s.cfg.WebPushVAPIDPrivateKey,
			TTL:             60,
			Urgency:         webpush.UrgencyHigh,
		})
		if err != nil {
			_ = s.db.MarkPushSubscriptionFailure(ctx, item.Endpoint, err.Error())
			log.Printf("web push send %s: %v", item.Endpoint, err)
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
				_ = s.db.DeletePushSubscriptionByEndpoint(ctx, item.Endpoint)
				return
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				_ = s.db.MarkPushSubscriptionFailure(ctx, item.Endpoint, fmt.Sprintf("push_http_%d", resp.StatusCode))
				return
			}
			_ = s.db.MarkPushSubscriptionSuccess(ctx, item.Endpoint)
		}()
	}
}

func (s *Server) webPushAssetURL(path string) string {
	path = "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
	if strings.TrimSpace(s.cfg.FrontendOrigin) == "" {
		return path
	}
	return s.cfg.FrontendOrigin + path
}

func (s *Server) webPushNotificationFromSocial(m map[string]any) (webPushNotification, bool) {
	kind := strings.TrimSpace(anyString(m["kind"]))
	id := strings.TrimSpace(anyString(m["id"]))
	if kind == "" || id == "" {
		return webPushNotification{}, false
	}
	handle := strings.TrimSpace(anyString(m["actor_handle"]))
	name := strings.TrimSpace(anyString(m["actor_display_name"]))
	if name == "" {
		if handle != "" {
			name = "@" + handle
		} else {
			name = "誰か"
		}
	}
	payload := webPushNotification{
		Version:        1,
		Kind:           kind,
		Title:          "新しい通知",
		URL:            s.cfg.FrontendOrigin + socialNotificationURL(m),
		Tag:            "glipz-social-" + id,
		Icon:           s.webPushAssetURL("/icon.svg"),
		Badge:          s.webPushAssetURL("/badge.svg"),
		CreatedAt:      anyString(m["created_at"]),
		NotificationID: id,
	}
	switch kind {
	case "reply":
		payload.Title = "返信がありました"
		payload.Body = name + " があなたの投稿に返信しました"
	case "like":
		payload.Title = "いいねがありました"
		payload.Body = name + " があなたの投稿にいいねしました"
	case "repost":
		payload.Title = "リポストされました"
		payload.Body = name + " があなたの投稿をリポストしました"
	case "follow":
		payload.Title = "フォローされました"
		payload.Body = name + " があなたをフォローしました"
	case "dm_invite":
		payload.Title = "DMの招待"
		payload.Body = name + " からダイレクトメッセージの招待があります"
	default:
		payload.Body = "新しい通知があります"
	}
	return payload, true
}

func (s *Server) webPushNotificationFromDM(m map[string]any) (webPushNotification, bool) {
	kind := strings.TrimSpace(anyString(m["kind"]))
	threadID := strings.TrimSpace(anyString(m["thread_id"]))
	if kind == "" || threadID == "" {
		return webPushNotification{}, false
	}
	handle := strings.TrimSpace(anyString(m["sender_handle"]))
	name := strings.TrimSpace(anyString(m["sender_display_name"]))
	if name == "" {
		if handle != "" {
			name = "@" + handle
		} else {
			name = "相手"
		}
	}
	url := s.cfg.FrontendOrigin + "/messages/" + threadID
	payload := webPushNotification{
		Version:   1,
		Kind:      kind,
		Title:     "ダイレクトメッセージ",
		URL:       url,
		Tag:       "glipz-dm-" + threadID,
		Icon:      s.webPushAssetURL("/icon.svg"),
		Badge:     s.webPushAssetURL("/badge.svg"),
		CreatedAt: anyString(m["created_at"]),
		ThreadID:  threadID,
		MessageID: strings.TrimSpace(anyString(m["message_id"])),
	}
	switch kind {
	case "message":
		payload.Title = "新しいメッセージ"
		payload.Body = name + " から新しいダイレクトメッセージ"
		if payload.MessageID != "" {
			payload.Tag = "glipz-dm-message-" + payload.MessageID
		}
	case "call_invite":
		mode := dmCallModeLabel(anyString(m["call_mode"]))
		payload.Title = mode + "の着信"
		payload.Body = name + " から" + mode + "の着信があります"
		payload.URL = s.cfg.FrontendOrigin + "/messages/" + threadID + "?call=" + normalizeWebPushDMCallMode(anyString(m["call_mode"])) + "&incoming=1"
		payload.Tag = "glipz-dm-call-" + threadID
		payload.RequireInteraction = true
	case "call_cancel":
		payload.Title = "通話がキャンセルされました"
		payload.Body = name + " が通話をキャンセルしました"
		payload.Tag = "glipz-dm-call-" + threadID
	case "call_end":
		payload.Title = "通話が終了しました"
		payload.Body = name + " が通話を終了しました"
		payload.Tag = "glipz-dm-call-" + threadID
	case "call_missed":
		payload.Title = "不在着信"
		payload.Body = name + " からの通話は不在着信になりました"
		payload.Tag = "glipz-dm-call-" + threadID
	default:
		payload.Body = "新しいダイレクトメッセージ通知があります"
	}
	return payload, true
}

func socialNotificationURL(m map[string]any) string {
	kind := strings.TrimSpace(anyString(m["kind"]))
	actorHandle := strings.TrimSpace(anyString(m["actor_handle"]))
	if kind == "dm_invite" {
		return "/messages"
	}
	if kind == "follow" && actorHandle != "" {
		return "/@" + actorHandle
	}
	if kind == "reply" {
		if actorPostID := strings.TrimSpace(anyString(m["actor_post_id"])); actorPostID != "" && actorHandle != "" {
			return "/@" + actorHandle + "#post-" + actorPostID
		}
	}
	subjectPostID := strings.TrimSpace(anyString(m["subject_post_id"]))
	subjectAuthorHandle := strings.TrimSpace(anyString(m["subject_author_handle"]))
	if subjectPostID != "" && subjectAuthorHandle != "" {
		return "/@" + subjectAuthorHandle + "#post-" + subjectPostID
	}
	return "/feed"
}

func dmCallModeLabel(mode string) string {
	if normalizeWebPushDMCallMode(mode) == "video" {
		return "ビデオ通話"
	}
	return "音声通話"
}

func normalizeWebPushDMCallMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "video") {
		return "video"
	}
	return "audio"
}

func anyString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

type webPushConfigResponse struct {
	Available         bool   `json:"available"`
	VAPIDPublicKey    string `json:"vapid_public_key,omitempty"`
	SubscriptionCount int64  `json:"subscription_count"`
}

type putWebPushSubscriptionReq struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type deleteWebPushSubscriptionReq struct {
	Endpoint string `json:"endpoint"`
}

func (s *Server) handleGetMeWebPush(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	count, err := s.db.CountPushSubscriptionsByUser(r.Context(), uid)
	if err != nil {
		writeServerError(w, "CountPushSubscriptionsByUser", err)
		return
	}
	out := webPushConfigResponse{
		Available:         s.cfg.WebPushEnabled(),
		SubscriptionCount: count,
	}
	if s.cfg.WebPushEnabled() {
		out.VAPIDPublicKey = s.cfg.WebPushVAPIDPublicKey
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePutMeWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !s.cfg.WebPushEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "web_push_unavailable"})
		return
	}
	var req putWebPushSubscriptionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if strings.TrimSpace(req.Endpoint) == "" || strings.TrimSpace(req.Keys.P256DH) == "" || strings.TrimSpace(req.Keys.Auth) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_subscription"})
		return
	}
	if err := s.db.UpsertPushSubscription(r.Context(), uid, repo.UpsertPushSubscriptionInput{
		Endpoint:  req.Endpoint,
		P256DH:    req.Keys.P256DH,
		Auth:      req.Keys.Auth,
		UserAgent: r.UserAgent(),
	}); err != nil {
		writeServerError(w, "UpsertPushSubscription", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteMeWebPushSubscription(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req deleteWebPushSubscriptionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if strings.TrimSpace(req.Endpoint) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_subscription"})
		return
	}
	if err := s.db.DeletePushSubscription(r.Context(), uid, req.Endpoint); err != nil {
		writeServerError(w, "DeletePushSubscription", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
