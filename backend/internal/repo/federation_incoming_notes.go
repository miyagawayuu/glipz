package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type FederatedIncomingNote struct {
	ID                          uuid.UUID
	ObjectIRI                   string
	ActorIRI                    string
	ActorAcct                   string
	ActorName                   string
	ActorIconURL                *string
	ActorProfileURL             *string
	Title                       string
	BodyMd                      string
	Visibility                  string
	PublishedAt                 time.Time
	UpdatedAt                   time.Time
	ReceivedAt                  time.Time
	HasPremium                  bool
	PaywallProvider             string
	PatreonCampaignID           string
	PatreonRequiredRewardTierID string
	UnlockURL                   string
	UnlockedBodyPremiumMd       string
	DeletedAt                   *time.Time
}

type UpsertFederatedIncomingNoteInput struct {
	ObjectIRI                   string
	ActorIRI                    string
	ActorAcct                   string
	ActorName                   string
	ActorIconURL                string
	ActorProfileURL             string
	Title                       string
	BodyMd                      string
	Visibility                  string
	PublishedAt                 time.Time
	UpdatedAt                   time.Time
	HasPremium                  bool
	PaywallProvider             string
	PatreonCampaignID           string
	PatreonRequiredRewardTierID string
	UnlockURL                   string
}

func (p *Pool) UpsertFederatedIncomingNote(ctx context.Context, in UpsertFederatedIncomingNoteInput) error {
	in.ObjectIRI = strings.TrimSpace(in.ObjectIRI)
	in.ActorIRI = strings.TrimSpace(in.ActorIRI)
	if in.ObjectIRI == "" || in.ActorIRI == "" {
		return errors.New("missing object_iri or actor_iri")
	}
	if in.PublishedAt.IsZero() {
		in.PublishedAt = time.Now().UTC()
	}
	if in.UpdatedAt.IsZero() {
		in.UpdatedAt = in.PublishedAt
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_incoming_notes (
			object_iri, actor_iri, actor_acct, actor_name, actor_icon_url, actor_profile_url,
			title, body_md, visibility, published_at, updated_at,
			has_premium, paywall_provider, patreon_campaign_id, patreon_required_reward_tier_id, unlock_url,
			received_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, NULLIF(trim($5), ''), NULLIF(trim($6), ''),
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16,
			NOW(), NULL
		)
		ON CONFLICT (object_iri) DO UPDATE
		SET actor_iri = EXCLUDED.actor_iri,
			actor_acct = EXCLUDED.actor_acct,
			actor_name = EXCLUDED.actor_name,
			actor_icon_url = EXCLUDED.actor_icon_url,
			actor_profile_url = EXCLUDED.actor_profile_url,
			title = EXCLUDED.title,
			body_md = EXCLUDED.body_md,
			visibility = EXCLUDED.visibility,
			published_at = EXCLUDED.published_at,
			updated_at = EXCLUDED.updated_at,
			has_premium = EXCLUDED.has_premium,
			paywall_provider = EXCLUDED.paywall_provider,
			patreon_campaign_id = EXCLUDED.patreon_campaign_id,
			patreon_required_reward_tier_id = EXCLUDED.patreon_required_reward_tier_id,
			unlock_url = EXCLUDED.unlock_url,
			deleted_at = NULL
	`, in.ObjectIRI, in.ActorIRI, strings.TrimSpace(in.ActorAcct), strings.TrimSpace(in.ActorName), in.ActorIconURL, in.ActorProfileURL,
		strings.TrimSpace(in.Title), strings.TrimSpace(in.BodyMd), strings.TrimSpace(in.Visibility), in.PublishedAt.UTC(), in.UpdatedAt.UTC(),
		in.HasPremium, strings.TrimSpace(in.PaywallProvider), strings.TrimSpace(in.PatreonCampaignID), strings.TrimSpace(in.PatreonRequiredRewardTierID), strings.TrimSpace(in.UnlockURL))
	return err
}

func (p *Pool) SoftDeleteFederatedIncomingNoteByObjectIRI(ctx context.Context, objectIRI string) error {
	objectIRI = strings.TrimSpace(objectIRI)
	if objectIRI == "" {
		return nil
	}
	_, err := p.db.Exec(ctx, `
		UPDATE federation_incoming_notes
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE object_iri = $1 AND deleted_at IS NULL
	`, objectIRI)
	return err
}

func (p *Pool) GetFederatedIncomingNoteByID(ctx context.Context, id uuid.UUID) (FederatedIncomingNote, error) {
	var out FederatedIncomingNote
	var icon, profile pgtype.Text
	var deleted pgtype.Timestamptz
	err := p.db.QueryRow(ctx, `
		SELECT id, object_iri, actor_iri, actor_acct, actor_name, actor_icon_url, actor_profile_url,
			title, body_md, visibility, published_at, updated_at, received_at,
			has_premium, paywall_provider, patreon_campaign_id, patreon_required_reward_tier_id, unlock_url,
			unlocked_body_premium_md,
			deleted_at
		FROM federation_incoming_notes
		WHERE id = $1
	`, id).Scan(
		&out.ID, &out.ObjectIRI, &out.ActorIRI, &out.ActorAcct, &out.ActorName, &icon, &profile,
		&out.Title, &out.BodyMd, &out.Visibility, &out.PublishedAt, &out.UpdatedAt, &out.ReceivedAt,
		&out.HasPremium, &out.PaywallProvider, &out.PatreonCampaignID, &out.PatreonRequiredRewardTierID, &out.UnlockURL,
		&out.UnlockedBodyPremiumMd,
		&deleted,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return FederatedIncomingNote{}, ErrNotFound
	}
	if err != nil {
		return FederatedIncomingNote{}, err
	}
	out.ActorIconURL = ptrText(icon)
	out.ActorProfileURL = ptrText(profile)
	out.DeletedAt = ptrTimestamptz(deleted)
	return out, nil
}

func (p *Pool) ListFederatedIncomingNotesPublicByActorIRI(ctx context.Context, actorIRI string, limit int) ([]FederatedIncomingNote, error) {
	actorIRI = strings.TrimSpace(actorIRI)
	if actorIRI == "" {
		return []FederatedIncomingNote{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.db.Query(ctx, `
		SELECT id, object_iri, actor_iri, actor_acct, actor_name, actor_icon_url, actor_profile_url,
			title, body_md, visibility, published_at, updated_at, received_at,
			has_premium, paywall_provider, patreon_campaign_id, patreon_required_reward_tier_id, unlock_url,
			unlocked_body_premium_md,
			deleted_at
		FROM federation_incoming_notes
		WHERE actor_iri = $1
		  AND deleted_at IS NULL
		ORDER BY published_at DESC, id DESC
		LIMIT $2
	`, actorIRI, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederatedIncomingNote
	for rows.Next() {
		var r FederatedIncomingNote
		var icon, profile pgtype.Text
		var deleted pgtype.Timestamptz
		if err := rows.Scan(
			&r.ID, &r.ObjectIRI, &r.ActorIRI, &r.ActorAcct, &r.ActorName, &icon, &profile,
			&r.Title, &r.BodyMd, &r.Visibility, &r.PublishedAt, &r.UpdatedAt, &r.ReceivedAt,
			&r.HasPremium, &r.PaywallProvider, &r.PatreonCampaignID, &r.PatreonRequiredRewardTierID, &r.UnlockURL,
			&r.UnlockedBodyPremiumMd,
			&deleted,
		); err != nil {
			return nil, err
		}
		r.ActorIconURL = ptrText(icon)
		r.ActorProfileURL = ptrText(profile)
		r.DeletedAt = ptrTimestamptz(deleted)
		out = append(out, r)
	}
	return out, rows.Err()
}

type FederatedIncomingNoteUnlockUpsert struct {
	ObjectIRI             string
	UnlockedBodyPremiumMd string
}

func (p *Pool) UpsertFederatedIncomingNoteUnlock(ctx context.Context, objectIRI, unlockedBodyPremiumMd string) error {
	objectIRI = strings.TrimSpace(objectIRI)
	if objectIRI == "" {
		return errors.New("missing object_iri")
	}
	_, err := p.db.Exec(ctx, `
		UPDATE federation_incoming_notes
		SET unlocked_body_premium_md = $2,
		    updated_at = NOW()
		WHERE object_iri = $1 AND deleted_at IS NULL
	`, objectIRI, strings.TrimSpace(unlockedBodyPremiumMd))
	return err
}

// UpsertFederatedIncomingNoteUnlockForUser stores unlocked premium body for one viewer (per-user unlock).
func (p *Pool) UpsertFederatedIncomingNoteUnlockForUser(ctx context.Context, incomingNoteID, userID uuid.UUID, bodyPremiumMd string, expiresAt *time.Time) error {
	bodyPremiumMd = strings.TrimSpace(bodyPremiumMd)
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_incoming_note_unlocks (federation_incoming_note_id, user_id, body_premium_md, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (federation_incoming_note_id, user_id) DO UPDATE
		SET body_premium_md = EXCLUDED.body_premium_md,
		    expires_at = EXCLUDED.expires_at,
		    created_at = NOW()
	`, incomingNoteID, userID, bodyPremiumMd, expiresAt)
	return err
}

// ListFederatedIncomingNoteUnlockBodiesForUser returns unlocked premium bodies keyed by note id.
func (p *Pool) ListFederatedIncomingNoteUnlockBodiesForUser(ctx context.Context, userID uuid.UUID, noteIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if userID == uuid.Nil || len(noteIDs) == 0 {
		return out, nil
	}
	rows, err := p.db.Query(ctx, `
		SELECT federation_incoming_note_id, body_premium_md
		FROM federation_incoming_note_unlocks
		WHERE user_id = $1
		  AND federation_incoming_note_id = ANY($2::uuid[])
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, userID, noteIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var body string
		if err := rows.Scan(&id, &body); err != nil {
			return nil, err
		}
		if strings.TrimSpace(body) != "" {
			out[id] = body
		}
	}
	return out, rows.Err()
}

// Marshal helper to keep parity with other federation payload storage patterns.
func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

