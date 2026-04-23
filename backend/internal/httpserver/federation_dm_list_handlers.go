package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"glipz.io/backend/internal/repo"
)

func (s *Server) handleFederationDMThreadsList(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	rows, err := s.db.ListFederationDMThreadsForUser(r.Context(), uid, 50)
	if err != nil {
		writeServerError(w, "ListFederationDMThreadsForUser", err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"thread_id":   row.ThreadID.String(),
			"remote_acct": row.RemoteAcct,
			"state":       row.State,
			"updated_at":  row.UpdatedAt.UTC().Format(time.RFC3339),
			"created_at":  row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFederationDMMessagesList(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	threadID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "threadID")))
	if err != nil || threadID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_thread_id"})
		return
	}
	rows, err := s.db.ListFederationDMMessagesForUser(r.Context(), uid, threadID, 80)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeServerError(w, "ListFederationDMMessagesForUser", err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var senderPayload any
		if len(row.SenderPayload) > 0 {
			_ = json.Unmarshal(row.SenderPayload, &senderPayload)
		}
		var payload any
		_ = json.Unmarshal(row.RecipientPayload, &payload)
		var attachments any
		_ = json.Unmarshal(row.Attachments, &attachments)
		out := map[string]any{
			"message_id":       row.MessageID.String(),
			"thread_id":        row.ThreadID.String(),
			"sender_acct":      row.SenderAcct,
			"sender_payload":   senderPayload,
			"recipient_payload": payload,
			"attachments":      attachments,
			"created_at":       row.CreatedAt.UTC().Format(time.RFC3339),
		}
		if row.SentAt != nil {
			out["sent_at"] = row.SentAt.UTC().Format(time.RFC3339)
		} else {
			out["sent_at"] = nil
		}
		items = append(items, out)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

