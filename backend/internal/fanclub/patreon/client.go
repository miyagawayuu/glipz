package patreon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tokenEndpoint = "https://www.patreon.com/api/oauth2/token"
	apiV2         = "https://www.patreon.com/api/oauth2/v2"
)

// Config holds Patreon app credentials. RedirectURI must match the app registration exactly.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Client is a thin Patreon API v2 client.
type Client struct {
	HTTP   *http.Client
	Config Config
}

// TokenResponse is returned by the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// ExchangeCode exchanges an authorization code for tokens.
func (c *Client) ExchangeCode(ctx context.Context, code string) (TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {strings.TrimSpace(code)},
		"client_id":     {c.Config.ClientID},
		"client_secret": {c.Config.ClientSecret},
		"redirect_uri":  {c.Config.RedirectURI},
	})
}

// Refresh uses a refresh token.
func (c *Client) Refresh(ctx context.Context, refresh string) (TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {strings.TrimSpace(refresh)},
		"client_id":     {c.Config.ClientID},
		"client_secret": {c.Config.ClientSecret},
	})
}

func (c *Client) postToken(ctx context.Context, v url.Values) (TokenResponse, error) {
	hc := c.httpc()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(v.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := hc.Do(req)
	if err != nil {
		return TokenResponse{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("patreon token: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var out TokenResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return TokenResponse{}, err
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return TokenResponse{}, fmt.Errorf("patreon token: empty access_token")
	}
	return out, nil
}

func (c *Client) httpc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// PatreonUserID fetches the v2 /identity resource id for the token holder.
func (c *Client) PatreonUserID(ctx context.Context, accessToken string) (string, error) {
	u, err := url.Parse(apiV2 + "/identity")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := c.httpc().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("patreon identity: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var top struct {
		Data struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return "", err
	}
	id := strings.TrimSpace(top.Data.ID)
	if id == "" {
		return "", fmt.Errorf("patreon identity: missing id")
	}
	return id, nil
}

// EntitlementArgs are the post’s Patreon lock fields.
type EntitlementArgs struct {
	CampaignID string
	TierID     string
}

// EntitlementMatch fetches /identity and checks whether a membership is entitled for campaign+tier.
// Returns the Patreon user id (v2 "user" data id) and whether there is a matching entitled tier.
func (c *Client) EntitlementMatch(ctx context.Context, accessToken string, want EntitlementArgs) (patreonUserID string, ok bool, err error) {
	campaignWant := strings.TrimSpace(want.CampaignID)
	tierWant := strings.TrimSpace(want.TierID)
	if campaignWant == "" || tierWant == "" {
		return "", false, fmt.Errorf("empty campaign or tier")
	}
	u, err := url.Parse(apiV2 + "/identity")
	if err != nil {
		return "", false, err
	}
	q := u.Query()
	q.Set("include", "memberships,memberships.currently_entitled_tiers,memberships.campaign")
	q.Set("fields[user]", "full_name,email,image_url")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := c.httpc().Do(req)
	if err != nil {
		return "", false, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", false, fmt.Errorf("patreon identity: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var top struct {
		Data struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
		Included []patreonResource `json:"included"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return "", false, err
	}
	patreonUserID = strings.TrimSpace(top.Data.ID)
	if patreonUserID == "" {
		return "", false, fmt.Errorf("patreon identity: missing user id")
	}
	for _, m := range top.Included {
		if !strings.EqualFold(strings.TrimSpace(m.Type), "member") {
			continue
		}
		camID, cok := relSingleID(m.Relationships, "campaign")
		if !cok || camID != campaignWant {
			continue
		}
		for _, tid := range relManyIDs(m.Relationships, "currently_entitled_tiers") {
			if tid == tierWant {
				return patreonUserID, true, nil
			}
		}
	}
	return patreonUserID, false, nil
}

type patreonResource struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Attributes    json.RawMessage `json:"attributes"`
	Relationships json.RawMessage `json:"relationships"`
}

func relSingleID(rel json.RawMessage, name string) (string, bool) {
	if len(rel) == 0 {
		return "", false
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(rel, &wrap); err != nil {
		return "", false
	}
	raw, ok := wrap[name]
	if !ok {
		return "", false
	}
	var r struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", false
	}
	ids := parseIDBlock(r.Data)
	if len(ids) == 0 {
		return "", false
	}
	return ids[0], true
}

func relManyIDs(rel json.RawMessage, name string) []string {
	if len(rel) == 0 {
		return nil
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(rel, &wrap); err != nil {
		return nil
	}
	raw, ok := wrap[name]
	if !ok {
		return nil
	}
	var r struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil
	}
	return parseIDBlock(r.Data)
}

// parseIDBlock decodes `data: {id}` or `data: [{id},…]`
func parseIDBlock(data json.RawMessage) []string {
	if len(data) == 0 {
		return nil
	}
	var one struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &one); err == nil && one.ID != "" {
		return []string{one.ID}
	}
	var many []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &many); err != nil {
		return nil
	}
	var out []string
	for _, x := range many {
		if x.ID != "" {
			out = append(out, x.ID)
		}
	}
	return out
}

// CreatorCampaign is returned for the compose UI when the token holder is a Patreon creator.
type CreatorCampaign struct {
	ID    string
	Title string
	Tiers []CreatorTier
}

// CreatorTier is a reward / tier the creator can require for a post.
type CreatorTier struct {
	ID   string
	Name string
}

// ListCreatorCampaigns fetches campaigns with tier relationships for the creator account.
func (c *Client) ListCreatorCampaigns(ctx context.Context, accessToken string) ([]CreatorCampaign, error) {
	u, err := url.Parse(apiV2 + "/campaigns")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("include", "tiers")
	q.Set("fields[campaign]", "created_at,creation_name,patron_count")
	q.Set("fields[tier]", "title,amount_cents,patron_count,published,description")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := c.httpc().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// not a creator / missing scope — return empty, not an error, for calmer UI
		if res.StatusCode == http.StatusForbidden {
			return nil, nil
		}
		return nil, fmt.Errorf("patreon campaigns: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var doc struct {
		Data     json.RawMessage   `json:"data"`
		Included []patreonResource `json:"included"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	tierName := map[string]string{}
	for _, o := range doc.Included {
		tt := strings.ToLower(strings.TrimSpace(o.Type))
		if tt != "tier" && tt != "reward" {
			continue
		}
		var attr struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(o.Attributes, &attr)
		if o.ID == "" {
			continue
		}
		tn := strings.TrimSpace(attr.Title)
		if tn == "" {
			tn = o.ID
		}
		tierName[o.ID] = tn
	}
	var out []CreatorCampaign
	for i := range doc.Included {
		o := &doc.Included[i]
		if !strings.EqualFold(strings.TrimSpace(o.Type), "campaign") {
			continue
		}
		var attr struct {
			CreationName string `json:"creation_name"`
		}
		_ = json.Unmarshal(o.Attributes, &attr)
		title := strings.TrimSpace(attr.CreationName)
		if title == "" {
			title = o.ID
		}
		tids := relManyIDs(o.Relationships, "tiers")
		var tiers []CreatorTier
		for _, tid := range tids {
			tiers = append(tiers, CreatorTier{ID: tid, Name: tierName[tid]})
		}
		out = append(out, CreatorCampaign{ID: o.ID, Title: title, Tiers: tiers})
	}
	if len(out) == 0 && len(doc.Data) > 0 {
		// fallback: data-only (no full objects in `included`)
		var rawList []json.RawMessage
		if err := json.Unmarshal(doc.Data, &rawList); err != nil {
			var one json.RawMessage
			if err2 := json.Unmarshal(doc.Data, &one); err2 == nil {
				rawList = []json.RawMessage{one}
			}
		}
		for _, ch := range rawList {
			var d patreonResource
			if err := json.Unmarshal(ch, &d); err != nil {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(d.Type), "campaign") {
				continue
			}
			var attr struct {
				CreationName string `json:"creation_name"`
			}
			_ = json.Unmarshal(d.Attributes, &attr)
			title := strings.TrimSpace(attr.CreationName)
			if title == "" {
				title = d.ID
			}
			tids := relManyIDs(d.Relationships, "tiers")
			var tiers []CreatorTier
			for _, tid := range tids {
				tiers = append(tiers, CreatorTier{ID: tid, Name: tierName[tid]})
			}
			out = append(out, CreatorCampaign{ID: d.ID, Title: title, Tiers: tiers})
		}
	}
	return out, nil
}

// ExpiresAt returns token expiry if ExpiresIn is set from a token response.
func ExpiresAt(tr TokenResponse) *time.Time {
	if tr.ExpiresIn <= 0 {
		return nil
	}
	t := time.Now().UTC().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return &t
}
