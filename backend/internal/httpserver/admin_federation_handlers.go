package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func (s *Server) isSiteAdmin(uid uuid.UUID) bool {
	for _, a := range s.cfg.AdminUserIDs {
		if a == uid {
			return true
		}
	}
	return false
}

func (s *Server) requireSiteAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := userIDFrom(r.Context())
		if !ok || !s.isSiteAdmin(uid) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAdminFederationDeliveries(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	rows, err := s.db.ListFederationDeliveriesAdmin(r.Context(), status, 120)
	if err != nil {
		writeServerError(w, "admin federation deliveries", err)
		return
	}
	type item struct {
		ID             string `json:"id"`
		AuthorUserID   string `json:"author_user_id"`
		PostID         string `json:"post_id"`
		Kind           string `json:"kind"`
		InboxURL       string `json:"inbox_url"`
		AttemptCount   int    `json:"attempt_count"`
		NextAttemptAt  string `json:"next_attempt_at"`
		Status         string `json:"status"`
		LastErrorShort string `json:"last_error_short"`
		CreatedAt      string `json:"created_at"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		out = append(out, item{
			ID:             row.ID.String(),
			AuthorUserID:   row.AuthorUserID.String(),
			PostID:         row.PostID.String(),
			Kind:           row.Kind,
			InboxURL:       row.InboxURL,
			AttemptCount:   row.AttemptCount,
			NextAttemptAt:  row.NextAttemptAt.UTC().Format("2006-01-02T15:04:05Z"),
			Status:         row.Status,
			LastErrorShort: row.LastErrorShort,
			CreatedAt:      row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleAdminFederationDeliveryCounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pending, errP := s.db.CountFederationDeliveriesByStatus(ctx, "pending")
	if errP != nil {
		writeServerError(w, "admin federation counts pending", errP)
		return
	}
	dead, errD := s.db.CountFederationDeliveriesByStatus(ctx, "dead")
	if errD != nil {
		writeServerError(w, "admin federation counts dead", errD)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pending": pending,
		"dead":    dead,
	})
}

func (s *Server) handleAdminFederationDomainBlocksList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListFederationDomainBlocks(r.Context(), 300)
	if err != nil {
		writeServerError(w, "admin federation domain blocks", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

type adminDomainBlockReq struct {
	Host string `json:"host"`
	Note string `json:"note"`
}

func (s *Server) handleAdminFederationDomainBlocksAdd(w http.ResponseWriter, r *http.Request) {
	var body adminDomainBlockReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := s.db.AddFederationDomainBlock(r.Context(), body.Host, body.Note); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "ok"})
}

func (s *Server) handleAdminFederationDomainBlocksRemove(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_host"})
		return
	}
	if err := s.db.RemoveFederationDomainBlock(r.Context(), host); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "ok"})
}

func (s *Server) handleAdminFederationKnownInstancesList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.ListFederationKnownInstances(r.Context(), 300)
	if err != nil {
		writeServerError(w, "admin federation known instances", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

type adminKnownInstanceReq struct {
	Host string `json:"host"`
	Note string `json:"note"`
}

func (s *Server) handleAdminFederationKnownInstancesAdd(w http.ResponseWriter, r *http.Request) {
	var body adminKnownInstanceReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := s.db.AddFederationKnownInstance(r.Context(), body.Host, body.Note); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "ok"})
}

func (s *Server) handleAdminFederationKnownInstancesRemove(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_host"})
		return
	}
	if err := s.db.RemoveFederationKnownInstance(r.Context(), host); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "ok"})
}
