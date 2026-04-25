package gumroad

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const licenseVerifyEndpoint = "https://api.gumroad.com/v2/licenses/verify"

// Client is a small Gumroad license verification client.
type Client struct {
	HTTP *http.Client
}

// VerifyResult is the subset of Gumroad's license response needed for access checks.
type VerifyResult struct {
	Success  bool
	Purchase Purchase
}

// Purchase contains the lifecycle fields that can make a license ineligible.
type Purchase struct {
	ProductID               string `json:"product_id"`
	LicenseKey              string `json:"license_key"`
	Refunded                bool   `json:"refunded"`
	Chargebacked            bool   `json:"chargebacked"`
	Disputed                bool   `json:"disputed"`
	SubscriptionEndedAt     string `json:"subscription_ended_at"`
	SubscriptionCancelledAt string `json:"subscription_cancelled_at"`
	SubscriptionFailedAt    string `json:"subscription_failed_at"`
}

// Entitled reports whether Gumroad says the license is valid and not ended/refunded.
func (r VerifyResult) Entitled(productID string) bool {
	if !r.Success {
		return false
	}
	p := r.Purchase
	if strings.TrimSpace(p.ProductID) != "" && strings.TrimSpace(p.ProductID) != strings.TrimSpace(productID) {
		return false
	}
	return !p.Refunded &&
		!p.Chargebacked &&
		!p.Disputed &&
		strings.TrimSpace(p.SubscriptionEndedAt) == "" &&
		strings.TrimSpace(p.SubscriptionCancelledAt) == "" &&
		strings.TrimSpace(p.SubscriptionFailedAt) == ""
}

// VerifyLicense checks a Gumroad Membership product license key.
func (c *Client) VerifyLicense(ctx context.Context, productID, licenseKey string) (VerifyResult, error) {
	productID = strings.TrimSpace(productID)
	licenseKey = strings.TrimSpace(licenseKey)
	if productID == "" || licenseKey == "" {
		return VerifyResult{}, fmt.Errorf("gumroad verify: empty product or license")
	}
	v := url.Values{
		"product_id":           {productID},
		"license_key":          {licenseKey},
		"increment_uses_count": {"false"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, licenseVerifyEndpoint, strings.NewReader(v.Encode()))
	if err != nil {
		return VerifyResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.httpc().Do(req)
	if err != nil {
		return VerifyResult{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusNotFound {
		return VerifyResult{Success: false}, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return VerifyResult{}, fmt.Errorf("gumroad verify: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var top struct {
		Success  bool     `json:"success"`
		Purchase Purchase `json:"purchase"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{Success: top.Success, Purchase: top.Purchase}, nil
}

func (c *Client) httpc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
