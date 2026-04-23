package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func shortRandomSuffix() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return uuid.NewString()[:8]
	}
	return hex.EncodeToString(buf)
}

func (s *Server) skywayConfigured() bool {
	return strings.TrimSpace(s.cfg.SkyWayAppID) != "" && strings.TrimSpace(s.cfg.SkyWaySecretKey) != ""
}

func (s *Server) issueSkyWayRoomToken(roomName, memberName string, allowCreate, allowPublish, allowSubscribe bool) (string, error) {
	now := time.Now().UTC()
	memberMethods := make([]string, 0, 3)
	if allowPublish {
		memberMethods = append(memberMethods, "publish")
	}
	if allowSubscribe {
		memberMethods = append(memberMethods, "subscribe")
	}
	memberMethods = append(memberMethods, "updateMetadata")
	roomMethods := []string{}
	if allowCreate {
		roomMethods = append(roomMethods, "create", "close", "updateMetadata")
	}
	claims := jwt.MapClaims{
		"iat":     now.Unix(),
		"jti":     uuid.NewString(),
		"exp":     now.Add(24 * time.Hour).Unix(),
		"version": 3,
		"scope": map[string]any{
			"appId": s.cfg.SkyWayAppID,
			"turn": map[string]any{
				"enabled": true,
			},
			"rooms": []map[string]any{
				{
					"id":      "*",
					"name":    roomName,
					"methods": roomMethods,
					"sfu": map[string]any{
						"enabled":             true,
						"maxSubscribersLimit": 99,
					},
					"member": map[string]any{
						"id":      "*",
						"name":    memberName,
						"methods": memberMethods,
					},
				},
			},
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(s.cfg.SkyWaySecretKey))
}
