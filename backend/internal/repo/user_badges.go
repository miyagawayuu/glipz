package repo

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const (
	UserBadgeOperator = "operator"
	UserBadgeVerified = "verified"
	UserBadgeBot      = "bot"
	UserBadgeAI       = "ai"
)

var storedUserBadgeOrder = []string{
	UserBadgeVerified,
	UserBadgeBot,
	UserBadgeAI,
}

var validStoredUserBadges = map[string]struct{}{
	UserBadgeVerified: {},
	UserBadgeBot:      {},
	UserBadgeAI:       {},
}

func AvailableUserBadges() []string {
	return []string{UserBadgeVerified}
}

func AdminSelectableUserBadges(stored []string) []string {
	if slices.Contains(NormalizeUserBadges(stored), UserBadgeVerified) {
		return []string{UserBadgeVerified}
	}
	return []string{}
}

func NormalizeUserBadges(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		badge := strings.TrimSpace(strings.ToLower(raw))
		if _, ok := validStoredUserBadges[badge]; !ok {
			continue
		}
		seen[badge] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for _, badge := range storedUserBadgeOrder {
		if _, ok := seen[badge]; ok {
			out = append(out, badge)
		}
	}
	return out
}

func VisibleUserBadges(stored []string, isOperator bool) []string {
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	if isOperator {
		seen[UserBadgeOperator] = struct{}{}
		out = append(out, UserBadgeOperator)
	}
	for _, badge := range NormalizeUserBadges(stored) {
		if _, ok := seen[badge]; ok {
			continue
		}
		seen[badge] = struct{}{}
		out = append(out, badge)
	}
	return out
}

func ProfileManagedUserBadges(existing []string, isBot, isAI bool) []string {
	normalized := NormalizeUserBadges(existing)
	out := make([]string, 0, 3)
	if slices.Contains(normalized, UserBadgeVerified) {
		out = append(out, UserBadgeVerified)
	}
	if isBot {
		out = append(out, UserBadgeBot)
	}
	if isAI {
		out = append(out, UserBadgeAI)
	}
	return NormalizeUserBadges(out)
}

func AdminManagedUserBadges(existing, requested []string) []string {
	normalized := NormalizeUserBadges(existing)
	req := NormalizeUserBadges(requested)
	out := make([]string, 0, 3)
	if slices.Contains(req, UserBadgeVerified) {
		out = append(out, UserBadgeVerified)
	}
	if slices.Contains(normalized, UserBadgeBot) {
		out = append(out, UserBadgeBot)
	}
	if slices.Contains(normalized, UserBadgeAI) {
		out = append(out, UserBadgeAI)
	}
	return NormalizeUserBadges(out)
}

func (p *Pool) ListUserBadgesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string)
	if len(ids) == 0 {
		return out, nil
	}
	uniq := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out, nil
	}
	rows, err := p.db.Query(ctx, `
		SELECT id, COALESCE(badges, '{}'::text[])
		FROM users
		WHERE id = ANY($1::uuid[])
	`, uniq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var badges []string
		if err := rows.Scan(&id, &badges); err != nil {
			return nil, err
		}
		out[id] = NormalizeUserBadges(badges)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Pool) UpdateUserBadges(ctx context.Context, userID uuid.UUID, badges []string) error {
	_, err := p.db.Exec(ctx, `
		UPDATE users
		SET badges = $2
		WHERE id = $1
	`, userID, NormalizeUserBadges(badges))
	return err
}
