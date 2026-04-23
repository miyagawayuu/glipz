package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var federationHTTP = &http.Client{Timeout: 20 * time.Second}

func jsonStringField(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		return s, s != ""
	}
	var o map[string]any
	if err := json.Unmarshal(raw, &o); err != nil {
		return "", false
	}
	if id, ok := o["id"].(string); ok {
		id = strings.TrimSpace(id)
		return id, id != ""
	}
	return "", false
}

func verifyHTTPSignature(_ *http.Request, _ []byte) (string, string, error) {
	return "", "", errors.New("legacy shared inbox support removed")
}

func (s *Server) apSharedInboxAcceptOutbound(_ context.Context, _ map[string]json.RawMessage, _ string) error {
	return errors.New("legacy shared inbox support removed")
}
