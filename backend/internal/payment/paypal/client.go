package paypal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	cfg Config
	hc  *http.Client

	mu        sync.Mutex
	token     string
	tokenExp  time.Time
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: 15 * time.Second},
	}
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.TrimSpace(c.token) != "" && time.Until(c.tokenExp) > 30*time.Second {
		return c.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.apiBase()+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(strings.TrimSpace(c.cfg.ClientID), strings.TrimSpace(c.cfg.ClientSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("paypal oauth: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var tr oauthTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return "", errors.New("paypal oauth: missing access_token")
	}
	c.token = strings.TrimSpace(tr.AccessToken)
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	c.tokenExp = time.Now().Add(ttl)
	return c.token, nil
}

type createSubscriptionReq struct {
	PlanID             string `json:"plan_id"`
	ApplicationContext struct {
		BrandName          string `json:"brand_name,omitempty"`
		Locale             string `json:"locale,omitempty"`
		UserAction         string `json:"user_action,omitempty"`
		ShippingPreference string `json:"shipping_preference,omitempty"`
		ReturnURL          string `json:"return_url,omitempty"`
		CancelURL          string `json:"cancel_url,omitempty"`
	} `json:"application_context"`
}

type Link struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

type CreateSubscriptionResult struct {
	ID    string `json:"id"`
	Status string `json:"status,omitempty"`
	Links []Link `json:"links"`
}

func (r CreateSubscriptionResult) ApprovalURL() string {
	for _, l := range r.Links {
		if strings.EqualFold(strings.TrimSpace(l.Rel), "approve") && strings.TrimSpace(l.Href) != "" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

func (c *Client) CreateSubscription(ctx context.Context, planID, returnURL, cancelURL string) (CreateSubscriptionResult, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return CreateSubscriptionResult{}, err
	}
	var reqBody createSubscriptionReq
	reqBody.PlanID = strings.TrimSpace(planID)
	reqBody.ApplicationContext.BrandName = "Glipz"
	reqBody.ApplicationContext.UserAction = "SUBSCRIBE_NOW"
	reqBody.ApplicationContext.ShippingPreference = "NO_SHIPPING"
	reqBody.ApplicationContext.ReturnURL = strings.TrimSpace(returnURL)
	reqBody.ApplicationContext.CancelURL = strings.TrimSpace(cancelURL)

	raw, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.apiBase()+"/v1/billing/subscriptions", bytes.NewReader(raw))
	if err != nil {
		return CreateSubscriptionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.hc.Do(req)
	if err != nil {
		return CreateSubscriptionResult{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return CreateSubscriptionResult{}, fmt.Errorf("paypal create subscription: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var out CreateSubscriptionResult
	if err := json.Unmarshal(body, &out); err != nil {
		return CreateSubscriptionResult{}, err
	}
	return out, nil
}

type VerifyWebhookSignatureRequest struct {
	AuthAlgo         string          `json:"auth_algo"`
	CertURL          string          `json:"cert_url"`
	TransmissionID   string          `json:"transmission_id"`
	TransmissionSig  string          `json:"transmission_sig"`
	TransmissionTime string          `json:"transmission_time"`
	WebhookID        string          `json:"webhook_id"`
	WebhookEvent     json.RawMessage `json:"webhook_event"`
}

type VerifyWebhookSignatureResponse struct {
	VerificationStatus string `json:"verification_status"`
}

func (c *Client) VerifyWebhookSignature(ctx context.Context, headers http.Header, rawBody []byte) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	get := func(k string) string { return strings.TrimSpace(headers.Get(k)) }
	vreq := VerifyWebhookSignatureRequest{
		AuthAlgo:         get("PayPal-Auth-Algo"),
		CertURL:          get("PayPal-Cert-Url"),
		TransmissionID:   get("PayPal-Transmission-Id"),
		TransmissionSig:  get("PayPal-Transmission-Sig"),
		TransmissionTime: get("PayPal-Transmission-Time"),
		WebhookID:        strings.TrimSpace(c.cfg.WebhookID),
		WebhookEvent:     json.RawMessage(rawBody),
	}
	if vreq.AuthAlgo == "" || vreq.CertURL == "" || vreq.TransmissionID == "" || vreq.TransmissionSig == "" || vreq.TransmissionTime == "" {
		return errors.New("paypal webhook: missing required headers")
	}
	raw, _ := json.Marshal(vreq)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.apiBase()+"/v1/notifications/verify-webhook-signature", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("paypal verify webhook: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var vr VerifyWebhookSignatureResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(vr.VerificationStatus), "SUCCESS") {
		return fmt.Errorf("paypal webhook: verification_status=%s", strings.TrimSpace(vr.VerificationStatus))
	}
	return nil
}

