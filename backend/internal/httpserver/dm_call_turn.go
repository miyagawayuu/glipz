package httpserver

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type rtcIceServer struct {
	URLs           any    `json:"urls"`
	Username       string `json:"username,omitempty"`
	Credential     string `json:"credential,omitempty"`
	CredentialType string `json:"credentialType,omitempty"`
}

func (s *Server) turnConfigured() bool {
	return strings.TrimSpace(s.cfg.TurnHost) != "" && strings.TrimSpace(s.cfg.TurnSharedSecret) != ""
}

func (s *Server) issueTurnCredential(callID string, now time.Time) (username, credential string) {
	exp := now.Add(time.Duration(s.cfg.TurnTTLSeconds) * time.Second).Unix()
	username = fmt.Sprintf("%d:%s", exp, strings.TrimSpace(callID))
	mac := hmac.New(sha1.New, []byte(s.cfg.TurnSharedSecret))
	_, _ = mac.Write([]byte(username))
	credential = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return username, credential
}

func (s *Server) dmCallIceServers(callID string, now time.Time) []rtcIceServer {
	if !s.turnConfigured() {
		return nil
	}
	host := strings.TrimSpace(s.cfg.TurnHost)
	username, cred := s.issueTurnCredential(callID, now)
	// Provide 3 URLs as required:
	// - UDP 3478
	// - TCP 3478
	// - TURNS (TLS) 443 (TCP)
	return []rtcIceServer{
		{
			URLs:           fmt.Sprintf("turn:%s:%d?transport=udp", host, s.cfg.TurnPort),
			Username:       username,
			Credential:     cred,
			CredentialType: "password",
		},
		{
			URLs:           fmt.Sprintf("turn:%s:%d?transport=tcp", host, s.cfg.TurnPort),
			Username:       username,
			Credential:     cred,
			CredentialType: "password",
		},
		{
			URLs:           fmt.Sprintf("turns:%s:%d?transport=tcp", host, s.cfg.TurnsPort),
			Username:       username,
			Credential:     cred,
			CredentialType: "password",
		},
	}
}

