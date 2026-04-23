package repo

import (
	"context"
	"fmt"
	"strings"
)

// FederationKnownInstanceRow represents one known/trusted federation instance.
type FederationKnownInstanceRow struct {
	Host      string `json:"host"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

// ListFederationKnownInstances returns known instances in reverse chronological order.
func (p *Pool) ListFederationKnownInstances(ctx context.Context, limit int) ([]FederationKnownInstanceRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := p.db.Query(ctx, `
		SELECT host, COALESCE(note, ''), to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM federation_known_instances
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederationKnownInstanceRow
	for rows.Next() {
		var r FederationKnownInstanceRow
		if err := rows.Scan(&r.Host, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddFederationKnownInstance adds a known instance idempotently, updating note when the row already exists.
func (p *Pool) AddFederationKnownInstance(ctx context.Context, host, note string) error {
	host = normalizeFederationHost(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	note = strings.TrimSpace(note)
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_known_instances (host, note) VALUES ($1, $2)
		ON CONFLICT ((lower(host))) DO UPDATE SET note = EXCLUDED.note
	`, host, note)
	return err
}

// RemoveFederationKnownInstance removes a known instance entry.
func (p *Pool) RemoveFederationKnownInstance(ctx context.Context, host string) error {
	host = normalizeFederationHost(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	_, err := p.db.Exec(ctx, `DELETE FROM federation_known_instances WHERE lower(host) = lower($1)`, host)
	return err
}

