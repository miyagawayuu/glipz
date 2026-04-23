package repo

import (
	"context"
	"fmt"
	"strings"
)

// FederationDomainBlockRow represents one blocked federation domain.
type FederationDomainBlockRow struct {
	Host      string `json:"host"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

func normalizeFederationHost(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimPrefix(h, "www.")
	return h
}

// IsFederationDomainBlocked reports whether a host such as evil.example is in the block list.
func (p *Pool) IsFederationDomainBlocked(ctx context.Context, host string) (bool, error) {
	host = normalizeFederationHost(host)
	if host == "" {
		return false, nil
	}
	var n int64
	err := p.db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM federation_domain_blocks WHERE lower(host) = lower($1)
	`, host).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListFederationDomainBlocks returns blocked domains in reverse chronological order.
func (p *Pool) ListFederationDomainBlocks(ctx context.Context, limit int) ([]FederationDomainBlockRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := p.db.Query(ctx, `
		SELECT host, COALESCE(note, ''), to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM federation_domain_blocks
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FederationDomainBlockRow
	for rows.Next() {
		var r FederationDomainBlockRow
		if err := rows.Scan(&r.Host, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddFederationDomainBlock adds a blocked domain idempotently, updating note when the row already exists.
func (p *Pool) AddFederationDomainBlock(ctx context.Context, host, note string) error {
	host = normalizeFederationHost(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	note = strings.TrimSpace(note)
	_, err := p.db.Exec(ctx, `
		INSERT INTO federation_domain_blocks (host, note) VALUES ($1, $2)
		ON CONFLICT ((lower(host))) DO UPDATE SET note = EXCLUDED.note
	`, host, note)
	return err
}

// RemoveFederationDomainBlock removes a blocked domain entry.
func (p *Pool) RemoveFederationDomainBlock(ctx context.Context, host string) error {
	host = normalizeFederationHost(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	_, err := p.db.Exec(ctx, `DELETE FROM federation_domain_blocks WHERE lower(host) = lower($1)`, host)
	return err
}
