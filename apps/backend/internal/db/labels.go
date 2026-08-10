package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CuratedLabel struct {
	ID, Network, Address, Category, Label, EvidenceURL, Source, SourceVersion, Visibility, CreatedBy string
	Confidence                                                                                       float64
	TrustTier                                                                                        uint32
	CreatedAt                                                                                        time.Time
}

const upsertCuratedLabelSQL = `INSERT INTO curated_labels (id, network, address, category, label, confidence, evidence_url, source, source_version, visibility, trust_tier, created_by, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (id) DO UPDATE SET
  network = EXCLUDED.network,
  address = EXCLUDED.address,
  category = EXCLUDED.category,
  label = EXCLUDED.label,
  confidence = EXCLUDED.confidence,
  evidence_url = EXCLUDED.evidence_url,
  source = EXCLUDED.source,
  source_version = EXCLUDED.source_version,
  visibility = EXCLUDED.visibility,
  trust_tier = EXCLUDED.trust_tier,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at,
  imported_at = now()`

func (d *DB) UpsertCuratedLabels(ctx context.Context, labels []CuratedLabel) error {
	if len(labels) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin label import: %w", err)
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, upsertCuratedLabelSQL)
	if err != nil {
		return fmt.Errorf("prepare label import: %w", err)
	}
	defer statement.Close()
	for _, label := range labels {
		if _, err := statement.ExecContext(ctx, label.ID, label.Network, strings.ToLower(label.Address), label.Category, label.Label, label.Confidence, label.EvidenceURL, label.Source, label.SourceVersion, label.Visibility, label.TrustTier, label.CreatedBy, label.CreatedAt.UTC()); err != nil {
			return fmt.Errorf("upsert curated label %q: %w", label.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit label import: %w", err)
	}
	return nil
}

func (d *DB) GetCuratedLabels(ctx context.Context, network, address string) ([]CuratedLabel, error) {
	return d.queryCuratedLabels(ctx, `SELECT id, network, address, category, label, confidence, evidence_url, source, source_version, visibility, trust_tier, created_by, created_at
FROM curated_labels WHERE network = $1 AND address = $2 ORDER BY trust_tier, label`, network, strings.ToLower(address))
}

func (d *DB) SearchCuratedLabels(ctx context.Context, network, query, category string, limit int) ([]CuratedLabel, error) {
	return d.queryCuratedLabels(ctx, `SELECT id, network, address, category, label, confidence, evidence_url, source, source_version, visibility, trust_tier, created_by, created_at
FROM curated_labels
WHERE network = $1 AND ($2 = '' OR label ILIKE '%' || $2 || '%' OR address ILIKE '%' || $2 || '%') AND ($3 = '' OR category = $3)
ORDER BY trust_tier, label LIMIT $4`, network, query, category, limit)
}

func (d *DB) queryCuratedLabels(ctx context.Context, query string, arguments ...any) ([]CuratedLabel, error) {
	rows, err := d.SQL.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query curated labels: %w", err)
	}
	defer rows.Close()
	result := make([]CuratedLabel, 0)
	for rows.Next() {
		var label CuratedLabel
		if err := rows.Scan(&label.ID, &label.Network, &label.Address, &label.Category, &label.Label, &label.Confidence, &label.EvidenceURL, &label.Source, &label.SourceVersion, &label.Visibility, &label.TrustTier, &label.CreatedBy, &label.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan curated label: %w", err)
		}
		result = append(result, label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate curated labels: %w", err)
	}
	return result, nil
}
