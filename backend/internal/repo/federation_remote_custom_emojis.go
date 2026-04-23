package repo

import (
	"context"
	"strings"
	"time"
)

// GetFederationRemoteCustomEmojiCache returns a cached image URL for a remote shortcode name.
func (p *Pool) GetFederationRemoteCustomEmojiCache(ctx context.Context, domain, name string) (string, bool) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	name = strings.TrimSpace(strings.ToLower(name))
	if domain == "" || name == "" {
		return "", false
	}
	var imageURL string
	var expiresAt time.Time
	err := p.db.QueryRow(ctx, `
		SELECT image_url, expires_at
		FROM federation_remote_custom_emojis
		WHERE lower(domain) = lower($1) AND lower(shortcode_name) = lower($2)
		LIMIT 1
	`, domain, name).Scan(&imageURL, &expiresAt)
	if err != nil {
		return "", false
	}
	if time.Now().UTC().After(expiresAt.UTC()) {
		return "", false
	}
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return "", false
	}
	return imageURL, true
}

func (p *Pool) UpsertFederationRemoteCustomEmojiCache(ctx context.Context, domain, name, imageURL string, expiresAt time.Time) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	name = strings.TrimSpace(strings.ToLower(name))
	imageURL = strings.TrimSpace(imageURL)
	if domain == "" || name == "" || imageURL == "" {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_remote_custom_emojis (domain, shortcode_name, image_url, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (domain, shortcode_name)
		DO UPDATE SET image_url = EXCLUDED.image_url, expires_at = EXCLUDED.expires_at, updated_at = NOW()
	`, domain, name, imageURL, expiresAt.UTC())
	return err
}

