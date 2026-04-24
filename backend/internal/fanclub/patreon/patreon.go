package patreon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ProviderID is the stable fanclub provider id.
const ProviderID = "patreon"

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.ClientSecret) != "" && strings.TrimSpace(c.RedirectURI) != ""
}

func AuthorizeURL(cfg Config, scopes []string, state string) string {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("client_id", strings.TrimSpace(cfg.ClientID))
	v.Set("redirect_uri", strings.TrimSpace(cfg.RedirectURI))
	if len(scopes) > 0 {
		v.Set("scope", strings.Join(scopes, " "))
	}
	if strings.TrimSpace(state) != "" {
		v.Set("state", strings.TrimSpace(state))
	}
	return "https://www.patreon.com/oauth2/authorize?" + v.Encode()
}

func Exchange(cfg Config, code string) (accessToken string, refreshToken string, expiresAt time.Time, err error) {
	return tokenRequest(cfg, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {strings.TrimSpace(code)},
		"client_id":     {strings.TrimSpace(cfg.ClientID)},
		"client_secret": {strings.TrimSpace(cfg.ClientSecret)},
		"redirect_uri":  {strings.TrimSpace(cfg.RedirectURI)},
	})
}

func Refresh(cfg Config, refreshToken string) (accessToken string, newRefreshToken string, expiresAt time.Time, err error) {
	return tokenRequest(cfg, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {strings.TrimSpace(refreshToken)},
		"client_id":     {strings.TrimSpace(cfg.ClientID)},
		"client_secret": {strings.TrimSpace(cfg.ClientSecret)},
	})
}

func tokenRequest(cfg Config, form url.Values) (accessToken string, refreshToken string, expiresAt time.Time, err error) {
	req, err := http.NewRequest("POST", "https://www.patreon.com/api/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", "", time.Time{}, fmt.Errorf("patreon token http %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    any    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", time.Time{}, err
	}
	sec := 0
	switch v := out.ExpiresIn.(type) {
	case float64:
		sec = int(v)
	case int:
		sec = v
	case string:
		sec, _ = strconv.Atoi(v)
	}
	if sec <= 0 {
		sec = 3600
	}
	return strings.TrimSpace(out.AccessToken), strings.TrimSpace(out.RefreshToken), time.Now().UTC().Add(time.Duration(sec) * time.Second), nil
}

func apiGet(accessToken, urlStr string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	return body, res.StatusCode, nil
}

// FetchIdentityUserID returns the Patreon user id for the access token.
func FetchIdentityUserID(accessToken string) (string, error) {
	body, st, err := apiGet(accessToken, "https://www.patreon.com/api/oauth2/v2/identity")
	if err != nil {
		return "", err
	}
	if st < 200 || st >= 300 {
		return "", fmt.Errorf("patreon identity http %d", st)
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Data.ID), nil
}

type Campaign struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Tier struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	AmountCents int    `json:"amount_cents"`
}

// FetchCreatorCampaigns lists campaigns available for a creator token.
func FetchCreatorCampaigns(accessToken string) ([]Campaign, error) {
	// Patreon's API surface varies; use identity include=campaign for a pragmatic list.
	u := "https://www.patreon.com/api/oauth2/v2/identity?include=campaign&fields[campaign]=creation_name"
	body, st, err := apiGet(accessToken, u)
	if err != nil {
		return nil, err
	}
	if st < 200 || st >= 300 {
		return nil, fmt.Errorf("patreon campaigns http %d", st)
	}
	var out struct {
		Included []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				CreationName string `json:"creation_name"`
			} `json:"attributes"`
		} `json:"included"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	var camps []Campaign
	for _, inc := range out.Included {
		if inc.Type != "campaign" {
			continue
		}
		camps = append(camps, Campaign{ID: strings.TrimSpace(inc.ID), Name: strings.TrimSpace(inc.Attributes.CreationName)})
	}
	return camps, nil
}

// FetchCampaignTiers lists tiers for one campaign.
func FetchCampaignTiers(accessToken, campaignID string) ([]Tier, error) {
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, errors.New("missing campaign_id")
	}
	u := "https://www.patreon.com/api/oauth2/v2/campaigns/" + url.PathEscape(campaignID) + "?include=tiers&fields[tier]=title,amount_cents"
	body, st, err := apiGet(accessToken, u)
	if err != nil {
		return nil, err
	}
	if st < 200 || st >= 300 {
		return nil, fmt.Errorf("patreon tiers http %d", st)
	}
	var out struct {
		Included []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Title       string `json:"title"`
				AmountCents int    `json:"amount_cents"`
			} `json:"attributes"`
		} `json:"included"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	var tiers []Tier
	for _, inc := range out.Included {
		if inc.Type != "tier" {
			continue
		}
		tiers = append(tiers, Tier{
			ID:          strings.TrimSpace(inc.ID),
			Title:       strings.TrimSpace(inc.Attributes.Title),
			AmountCents: inc.Attributes.AmountCents,
		})
	}
	return tiers, nil
}

// MemberEntitledToReward checks whether the member token is currently entitled to the required tier.
func MemberEntitledToReward(accessToken, campaignID, requiredTierID string) (bool, error) {
	campaignID = strings.TrimSpace(campaignID)
	requiredTierID = strings.TrimSpace(requiredTierID)
	if campaignID == "" || requiredTierID == "" {
		return false, nil
	}
	u := "https://www.patreon.com/api/oauth2/v2/identity?include=memberships.currently_entitled_tiers&fields[tier]=title&fields[member]=patron_status&fields[membership]=patron_status"
	body, st, err := apiGet(accessToken, u)
	if err != nil {
		return false, err
	}
	if st == 401 || st == 403 {
		return false, nil
	}
	if st < 200 || st >= 300 {
		return false, fmt.Errorf("patreon entitled http %d", st)
	}
	var out struct {
		Included []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"included"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false, err
	}
	for _, inc := range out.Included {
		if inc.Type == "tier" && strings.EqualFold(strings.TrimSpace(inc.ID), requiredTierID) {
			return true, nil
		}
	}
	return false, nil
}

