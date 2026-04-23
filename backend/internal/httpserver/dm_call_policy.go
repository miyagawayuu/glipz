package httpserver

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
)

func normalizeDMCallScope(scope string) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "all", "followers", "specific_users":
		return strings.TrimSpace(strings.ToLower(scope))
	default:
		return "none"
	}
}

func (s *Server) canReceiveDMCall(ctx context.Context, callerID, calleeID uuid.UUID) (bool, error) {
	u, err := s.db.UserByID(ctx, calleeID)
	if err != nil {
		return false, err
	}
	if !u.DMCallEnabled {
		return false, nil
	}
	switch normalizeDMCallScope(u.DMCallScope) {
	case "all":
		return true, nil
	case "followers":
		return s.db.IsFollowing(ctx, callerID, calleeID)
	case "specific_users":
		return slices.Contains(u.DMCallAllowedUserIDs, callerID), nil
	default:
		return false, nil
	}
}
