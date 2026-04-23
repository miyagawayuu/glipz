package repo

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const profileExternalURLMaxCount = 5
const profileExternalURLMaxLen = 2048

var (
	// ErrProfileURLsTooMany is returned when profile external URLs exceed the limit of five.
	ErrProfileURLsTooMany = errors.New("profile_urls: too many")
	// ErrProfileURLTooLong is returned when a single URL is too long.
	ErrProfileURLTooLong = errors.New("profile_urls: url too long")
	// ErrInvalidProfileURL is returned when the URL scheme or host is invalid.
	ErrInvalidProfileURL = errors.New("profile_urls: invalid")
)

func parsePublicHTTPURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrInvalidProfileURL
	}
	if len(s) > profileExternalURLMaxLen {
		return "", ErrProfileURLTooLong
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", ErrInvalidProfileURL
	}
	if u.Scheme == "" || u.Host == "" {
		u2, err2 := url.Parse("https://" + strings.TrimPrefix(s, "//"))
		if err2 != nil || u2.Host == "" {
			return "", ErrInvalidProfileURL
		}
		u = u2
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidProfileURL
	}
	if strings.TrimSpace(u.Hostname()) == "" {
		return "", ErrInvalidProfileURL
	}
	if u.User != nil {
		if strings.TrimSpace(u.User.Username()) != "" {
			return "", ErrInvalidProfileURL
		}
		if _, hasPw := u.User.Password(); hasPw {
			return "", ErrInvalidProfileURL
		}
	}
	u.Scheme = scheme
	canon := strings.TrimRight(u.String(), "/")
	if len(canon) > profileExternalURLMaxLen {
		return "", ErrProfileURLTooLong
	}
	return canon, nil
}

// NormalizeProfileExternalURLs validates and normalizes profile external URLs.
// At most five URLs are allowed, and only http/https schemes are accepted.
func NormalizeProfileExternalURLs(raw []string) ([]string, error) {
	var cand []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		cand = append(cand, s)
	}
	if len(cand) > profileExternalURLMaxCount {
		return nil, ErrProfileURLsTooMany
	}
	out := make([]string, 0, len(cand))
	seen := map[string]struct{}{}
	for _, s := range cand {
		canon, err := parsePublicHTTPURL(s)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(canon)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, canon)
	}
	return out, nil
}

func unmarshalProfileExternalURLsJSON(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}
	var s []string
	if err := json.Unmarshal(b, &s); err != nil || s == nil {
		return []string{}
	}
	return s
}

// UserProfileExternalURLs returns users.profile_external_urls, or ErrNotFound when the user row is missing.
func (p *Pool) UserProfileExternalURLs(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var raw []byte
	err := p.db.QueryRow(ctx, `
		SELECT COALESCE(profile_external_urls, '[]'::jsonb)::text
		FROM users WHERE id = $1
	`, userID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return unmarshalProfileExternalURLsJSON(raw), nil
}
