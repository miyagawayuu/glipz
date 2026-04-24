// Package patreon implements Patreon OAuth and API calls for the fanclub integration.
package patreon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const tokenURL = "https://www.patreon.com/api/oauth2/token"

// Config holds Patreon application settings.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Enabled reports whether the required OAuth settings are present.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.ClientSecret) != "" &&
		strings.TrimSpace(c.RedirectURI) != ""
}

// AuthorizeURL builds the browser-facing authorization URL.
func AuthorizeURL(cfg Config, scopes []string, state string) string {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", strings.TrimSpace(cfg.ClientID))
	v.Set("redirect_uri", strings.TrimSpace(cfg.RedirectURI))
	v.Set("scope", strings.Join(scopes, " "))
	v.Set("state", state)
	return "https://www.patreon.com/oauth2/authorize?" + v.Encode()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Exchange swaps an authorization code for access and refresh tokens.
func Exchange(cfg Config, code string) (access, refresh string, expiresAt time.Time, err error) {
	form := url.Values{}
	form.Set("code", strings.TrimSpace(code))
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", strings.TrimSpace(cfg.ClientID))
	form.Set("client_secret", strings.TrimSpace(cfg.ClientSecret))
	form.Set("redirect_uri", strings.TrimSpace(cfg.RedirectURI))
	return postToken(form)
}

// Refresh issues a new access token from a refresh token.
func Refresh(cfg Config, refreshToken string) (access, refresh string, expiresAt time.Time, err error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))
	form.Set("client_id", strings.TrimSpace(cfg.ClientID))
	form.Set("client_secret", strings.TrimSpace(cfg.ClientSecret))
	return postToken(form)
}

func postToken(form url.Values) (access, refresh string, expiresAt time.Time, err error) {
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", time.Time{}, fmt.Errorf("patreon token: status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", "", time.Time{}, fmt.Errorf("patreon token json: %w", err)
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return "", "", time.Time{}, fmt.Errorf("patreon token: empty access_token")
	}
	exp := time.Now().UTC().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return tr.AccessToken, tr.RefreshToken, exp, nil
}

// FetchFirstCampaignID returns the first campaign ID for the creator.
func FetchFirstCampaignID(accessToken string) (string, error) {
	u := "https://www.patreon.com/api/oauth2/v2/campaigns?fields%5Bcampaign%5D=created_at"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("patreon campaigns: status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	var root struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return "", err
	}
	if len(root.Data) == 0 || strings.TrimSpace(root.Data[0].ID) == "" {
		return "", fmt.Errorf("patreon campaigns: no campaign")
	}
	return root.Data[0].ID, nil
}

// CreatorCampaign represents a campaign managed by the creator.
type CreatorCampaign struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FetchCreatorCampaigns lists campaigns using a creator access token.
func FetchCreatorCampaigns(accessToken string) ([]CreatorCampaign, error) {
	u := "https://www.patreon.com/api/oauth2/v2/campaigns?fields%5Bcampaign%5D=creation_name,created_at"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("patreon campaigns list: status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	var root struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				CreationName string `json:"creation_name"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	var out []CreatorCampaign
	for _, row := range root.Data {
		id := strings.TrimSpace(row.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(row.Attributes.CreationName)
		if name == "" {
			name = id
		}
		out = append(out, CreatorCampaign{ID: id, Name: name})
	}
	return out, nil
}

// FetchIdentityUserID returns the Patreon user ID from the identity endpoint.
func FetchIdentityUserID(accessToken string) (string, error) {
	u := "https://www.patreon.com/api/oauth2/v2/identity"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("patreon identity: status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	var root struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return "", err
	}
	if strings.TrimSpace(root.Data.ID) == "" {
		return "", fmt.Errorf("patreon identity: empty id")
	}
	return root.Data.ID, nil
}

// MemberEntitledToReward reports whether the viewer has an active membership
// in the given campaign that includes requiredRewardTierID.
func MemberEntitledToReward(accessToken, campaignID, requiredRewardTierID string) (bool, error) {
	campaignID = strings.TrimSpace(campaignID)
	requiredRewardTierID = strings.TrimSpace(requiredRewardTierID)
	if campaignID == "" || requiredRewardTierID == "" {
		return false, nil
	}
	u := "https://www.patreon.com/api/oauth2/v2/identity?include=memberships,memberships.currently_entitled_tiers,memberships.campaign&fields%5Bmember%5D=patron_status"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("patreon identity memberships: status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return false, err
	}
	included, _ := root["included"].([]any)
	for _, raw := range included {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.ToLower(fmt.Sprint(m["type"])) != "member" {
			continue
		}
		attrs, _ := m["attributes"].(map[string]any)
		status := strings.ToLower(fmt.Sprint(attrs["patron_status"]))
		if status != "active_patron" {
			continue
		}
		rel, _ := m["relationships"].(map[string]any)
		campRel, _ := rel["campaign"].(map[string]any)
		campData, _ := campRel["data"].(map[string]any)
		if strings.TrimSpace(fmt.Sprint(campData["id"])) != campaignID {
			continue
		}
		tiersRel, _ := rel["currently_entitled_tiers"].(map[string]any)
		tierData, _ := tiersRel["data"].([]any)
		for _, td := range tierData {
			tm, ok := td.(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(fmt.Sprint(tm["id"])) == requiredRewardTierID {
				return true, nil
			}
		}
	}
	return false, nil
}

// CampaignTier represents a public tier attached to a campaign.
type CampaignTier struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	AmountCents int    `json:"amount_cents"`
}

// FetchCampaignTiers lists campaign tiers using a creator access token.
func FetchCampaignTiers(accessToken, campaignID string) ([]CampaignTier, error) {
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("patreon tiers: empty campaign id")
	}
	u := fmt.Sprintf(
		"https://www.patreon.com/api/oauth2/v2/campaigns/%s?include=tiers&fields%%5Btier%%5D=title,amount_cents",
		url.PathEscape(campaignID),
	)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("patreon tiers: status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	var root struct {
		Included []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Title       string `json:"title"`
				AmountCents int    `json:"amount_cents"`
			} `json:"attributes"`
		} `json:"included"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	var out []CampaignTier
	for _, it := range root.Included {
		if strings.ToLower(strings.TrimSpace(it.Type)) != "tier" {
			continue
		}
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		out = append(out, CampaignTier{
			ID:          id,
			Title:       strings.TrimSpace(it.Attributes.Title),
			AmountCents: it.Attributes.AmountCents,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AmountCents != out[j].AmountCents {
			return out[i].AmountCents < out[j].AmountCents
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
