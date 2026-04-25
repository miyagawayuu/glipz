package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"glipz.io/backend/internal/payment/kernel"
	"glipz.io/backend/internal/payment/paypal"
	"glipz.io/backend/internal/repo"
)

type paypalPlanUpsertReq struct {
	PlanID string `json:"plan_id"`
	Label  string `json:"label,omitempty"`
	Active *bool  `json:"active,omitempty"`
}

// POST /api/v1/payment/paypal/plans
func (s *Server) handlePayPalUpsertPlan(w http.ResponseWriter, r *http.Request) {
	if !s.paypalIntegrationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "paypal_unavailable"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req paypalPlanUpsertReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	planID := strings.TrimSpace(req.PlanID)
	if planID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "paypal_plan_required"})
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	id, err := s.db.UpsertCreatorPaymentPlan(r.Context(), uid, paypal.ProviderID, planID, strings.TrimSpace(req.Label), active)
	if err != nil {
		writeServerError(w, "UpsertCreatorPaymentPlan", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id.String()})
}

// GET /api/v1/payment/paypal/plans
func (s *Server) handlePayPalListPlans(w http.ResponseWriter, r *http.Request) {
	if !s.paypalIntegrationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "paypal_unavailable"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.ListCreatorPaymentPlans(r.Context(), uid, paypal.ProviderID)
	if err != nil {
		writeServerError(w, "ListCreatorPaymentPlans", err)
		return
	}
	var out []map[string]any
	for _, rr := range rows {
		out = append(out, map[string]any{
			"id":      rr.ID.String(),
			"plan_id": rr.PlanID,
			"label":   rr.Label,
			"active":  rr.Active,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": out})
}

type paypalSubCreateReq struct {
	PostID string `json:"post_id"`
}

type paypalPendingSubscription struct {
	ViewerUserID  string `json:"viewer_user_id"`
	CreatorUserID string `json:"creator_user_id"`
	PlanID        string `json:"plan_id"`
	PostID        string `json:"post_id"`
}

func (s *Server) paypalPublicAPIOrigin(r *http.Request) string {
	if base := strings.TrimSuffix(strings.TrimSpace(s.cfg.GlipzProtocolPublicOrigin), "/"); base != "" {
		return base
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host != "" {
		return proto + "://" + host
	}
	return strings.TrimSuffix(strings.TrimSpace(s.cfg.FrontendOrigin), "/")
}

// POST /api/v1/payment/paypal/subscription/create
func (s *Server) handlePayPalSubscriptionCreate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !s.paypalIntegrationAvailable() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "paypal_unavailable"})
		return
	}
	var req paypalSubCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(req.PostID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	row, err := s.db.PostSensitiveByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostSensitiveByID paypal subscription", err)
		return
	}
	if !row.HasPaymentLock || !strings.EqualFold(strings.TrimSpace(row.PaymentProvider), paypal.ProviderID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_payment_locked"})
		return
	}
	creatorID, err := uuid.Parse(strings.TrimSpace(row.PaymentCreatorID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payment_creator"})
		return
	}
	planID := strings.TrimSpace(row.PaymentPlanID)
	if planID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "paypal_plan_required"})
		return
	}

	ppcfg := paypal.Config{
		ClientID:     s.cfg.PayPalClientID,
		ClientSecret: s.cfg.PayPalClientSecret,
		WebhookID:    s.cfg.PayPalWebhookID,
		Env:          paypal.Env(strings.TrimSpace(s.cfg.PayPalEnv)),
	}
	client := paypal.NewClient(ppcfg)

	// Authoritative linking happens on the backend return endpoint using the pending Redis state.
	apiBase := strings.TrimSuffix(s.paypalPublicAPIOrigin(r), "/")
	returnURL := apiBase + "/api/v1/payment/paypal/subscription/return"
	fe := strings.TrimSuffix(strings.TrimSpace(s.cfg.FrontendOrigin), "/")
	if fe == "" {
		fe = "http://localhost:5173"
	}
	cancelURL := fe + "/posts/" + postID.String() + "?paypal_subscription=cancelled"
	out, err := client.CreateSubscription(r.Context(), planID, returnURL, cancelURL)
	if err != nil {
		writeServerError(w, "paypal CreateSubscription", err)
		return
	}
	subID := strings.TrimSpace(out.ID)
	if subID == "" {
		writeServerError(w, "paypal CreateSubscription missing id", errors.New("missing subscription id"))
		return
	}
	approval := out.ApprovalURL()
	if approval == "" {
		writeServerError(w, "paypal CreateSubscription missing approval_url", errors.New("missing approval url"))
		return
	}

	// Store pending link in Redis so return endpoint can finalize without auth.
	pendingKey := "payment:paypal:pending_sub:" + subID
	payload := paypalPendingSubscription{
		ViewerUserID:  uid.String(),
		CreatorUserID: creatorID.String(),
		PlanID:        planID,
		PostID:        postID.String(),
	}
	raw, _ := json.Marshal(payload)
	if err := s.rdb.Set(r.Context(), pendingKey, string(raw), 30*time.Minute).Err(); err != nil {
		writeServerError(w, "paypal pending redis set", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"subscription_id": subID,
		"approval_url":    approval,
	})
}

// GET /api/v1/payment/paypal/subscription/return?subscription_id=...
func (s *Server) handlePayPalSubscriptionReturn(w http.ResponseWriter, r *http.Request) {
	subID := strings.TrimSpace(r.URL.Query().Get("subscription_id"))
	if subID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_subscription_id"})
		return
	}
	pendingKey := "payment:paypal:pending_sub:" + subID
	raw, err := s.rdb.GetDel(r.Context(), pendingKey).Result()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	var payload paypalPendingSubscription
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	viewerID, err := uuid.Parse(strings.TrimSpace(payload.ViewerUserID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	creatorID, err := uuid.Parse(strings.TrimSpace(payload.CreatorUserID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	planID := strings.TrimSpace(payload.PlanID)
	if planID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
		return
	}
	if err := s.db.CreatePaymentSubscriptionLink(r.Context(), paypal.ProviderID, viewerID, creatorID, planID, subID, "APPROVAL_PENDING", time.Now().UTC()); err != nil {
		writeServerError(w, "CreatePaymentSubscriptionLink", err)
		return
	}
	// Redirect back to the locked post when possible so the viewer can verify and unlock.
	fe := strings.TrimSuffix(strings.TrimSpace(s.cfg.FrontendOrigin), "/")
	if fe == "" {
		fe = "http://localhost:5173"
	}
	postID := strings.TrimSpace(payload.PostID)
	if _, err := uuid.Parse(postID); err != nil {
		postID = ""
	}
	redirectPath := "/settings"
	if postID != "" {
		redirectPath = "/posts/" + postID
	}
	u, _ := url.Parse(fe + redirectPath)
	q := u.Query()
	q.Set("paypal_subscription", "ok")
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// POST /api/v1/payment/paypal/webhook
func (s *Server) handlePayPalWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.paypalIntegrationAvailable() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "paypal_unavailable"})
		return
	}
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	ppcfg := paypal.Config{
		ClientID:     s.cfg.PayPalClientID,
		ClientSecret: s.cfg.PayPalClientSecret,
		WebhookID:    s.cfg.PayPalWebhookID,
		Env:          paypal.Env(strings.TrimSpace(s.cfg.PayPalEnv)),
	}
	client := paypal.NewClient(ppcfg)
	if err := client.VerifyWebhookSignature(r.Context(), r.Header, rawBody); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		return
	}
	var ev paypal.WebhookEvent
	if err := json.Unmarshal(rawBody, &ev); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	eventID := strings.TrimSpace(ev.ID)
	if eventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
		return
	}
	dedupKey := kernel.WebhookEventDedupKey(paypal.ProviderID, eventID)
	ok, err := s.rdb.SetNX(r.Context(), dedupKey, "1", 72*time.Hour).Result()
	if err != nil {
		writeServerError(w, "paypal webhook dedup", err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(ev.EventType)), "BILLING.SUBSCRIPTION.") {
		subID, _, status, ok := paypal.ExtractSubscription(ev)
		if ok {
			_, _ = s.db.UpdatePaymentSubscriptionStatusFromWebhook(r.Context(), paypal.ProviderID, subID, strings.TrimSpace(status), ev.ParsedCreateTime())
		}
	}
	// BILLING.SUBSCRIPTION.PAYMENT.FAILED can be handled with a dedicated schema if needed; for Phase 1 we rely on UPDATED status.

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type paypalEntitlementReq struct {
	PostID string `json:"post_id"`
}

// POST /api/v1/payment/paypal/entitlement
func (s *Server) handlePayPalEntitlement(w http.ResponseWriter, r *http.Request) {
	if !s.paypalIntegrationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "paypal_unavailable"})
		return
	}
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req paypalEntitlementReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	postID, err := uuid.Parse(strings.TrimSpace(req.PostID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_post_id"})
		return
	}
	row, err := s.db.PostSensitiveByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "PostSensitiveByID paypal entitlement", err)
		return
	}
	if !row.HasPaymentLock || !strings.EqualFold(strings.TrimSpace(row.PaymentProvider), paypal.ProviderID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_payment_locked"})
		return
	}
	creatorID, err := uuid.Parse(strings.TrimSpace(row.PaymentCreatorID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payment_creator"})
		return
	}
	planID := strings.TrimSpace(row.PaymentPlanID)
	if planID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "paypal_plan_required"})
		return
	}
	active, err := s.db.ViewerHasActivePaymentSubscription(r.Context(), paypal.ProviderID, uid, creatorID, planID)
	if err != nil {
		writeServerError(w, "ViewerHasActivePaymentSubscription", err)
		return
	}
	if !active {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "paypal_subscription_required"})
		return
	}
	scope := "post:" + postID.String() + ":unlock"
	jws, err := kernel.SignEntitlement(s.secret, paypal.ProviderID, uid, scope, 10*time.Minute)
	if err != nil {
		writeServerError(w, "SignEntitlement", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entitlement_jwt": jws})
}
