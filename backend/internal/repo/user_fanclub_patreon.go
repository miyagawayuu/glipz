package repo

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SetUserPatreonMemberTokens stores Patreon OAuth results for the member-facing integration.
func (p *Pool) SetUserPatreonMemberTokens(ctx context.Context, userID uuid.UUID, access, refresh string, expiresAt time.Time, patreonUserID string) error {
	_, err := p.db.Exec(ctx, `
		UPDATE users SET
			patreon_member_access_token = $2,
			patreon_member_refresh_token = $3,
			patreon_member_token_expires_at = $4,
			patreon_member_user_id = NULLIF(btrim($5), '')
		WHERE id = $1
	`, userID, access, refresh, expiresAt, patreonUserID)
	return err
}

// ClearUserPatreonMember disconnects the member-side Patreon integration.
func (p *Pool) ClearUserPatreonMember(ctx context.Context, userID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `
		UPDATE users SET
			patreon_member_access_token = NULL,
			patreon_member_refresh_token = NULL,
			patreon_member_token_expires_at = NULL,
			patreon_member_user_id = NULL
		WHERE id = $1
	`, userID)
	return err
}

// SetUserPatreonCreatorTokens stores Patreon OAuth results for the creator-side integration.
func (p *Pool) SetUserPatreonCreatorTokens(ctx context.Context, userID uuid.UUID, access, refresh string, expiresAt time.Time) error {
	_, err := p.db.Exec(ctx, `
		UPDATE users SET
			patreon_creator_access_token = $2,
			patreon_creator_refresh_token = $3,
			patreon_creator_token_expires_at = $4
		WHERE id = $1
	`, userID, access, refresh, expiresAt)
	return err
}

// SetUserPatreonCampaignID stores the campaign ID used as default for notes.
func (p *Pool) SetUserPatreonCampaignID(ctx context.Context, userID uuid.UUID, campaignID string) error {
	campaignID = strings.TrimSpace(campaignID)
	_, err := p.db.Exec(ctx, `
		UPDATE users SET patreon_campaign_id = NULLIF($2, '')
		WHERE id = $1
	`, userID, campaignID)
	return err
}

// SetUserPatreonRequiredRewardTierID stores the required tier ID used as default for notes.
func (p *Pool) SetUserPatreonRequiredRewardTierID(ctx context.Context, userID uuid.UUID, tierID string) error {
	tierID = strings.TrimSpace(tierID)
	_, err := p.db.Exec(ctx, `
		UPDATE users SET patreon_required_reward_tier_id = NULLIF($2, '')
		WHERE id = $1
	`, userID, tierID)
	return err
}

// ClearUserPatreonCreator disconnects the creator-side Patreon integration and clears defaults.
func (p *Pool) ClearUserPatreonCreator(ctx context.Context, userID uuid.UUID) error {
	_, err := p.db.Exec(ctx, `
		UPDATE users SET
			patreon_creator_access_token = NULL,
			patreon_creator_refresh_token = NULL,
			patreon_creator_token_expires_at = NULL,
			patreon_campaign_id = NULL,
			patreon_required_reward_tier_id = NULL
		WHERE id = $1
	`, userID)
	return err
}

