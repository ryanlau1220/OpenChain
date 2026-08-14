package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type CuratedLabel struct {
	ID, AssertionKey, Network, Address, Category, Label, EvidenceURL, EvidenceSnapshot, EvidenceHash, Source, SourceVersion, SupersedesID, ReviewState, Visibility, CreatedBy string
	Confidence                                                                                                                                                                float64
	TrustTier                                                                                                                                                                 uint32
	ValidFrom, CreatedAt                                                                                                                                                      time.Time
	ValidTo                                                                                                                                                                   *time.Time
}

const insertLabelEvidenceSQL = `INSERT INTO label_evidence (sha256, content, captured_at) VALUES ($1, $2, $3) ON CONFLICT (sha256) DO NOTHING`
const insertLabelAssertionSQL = `INSERT INTO label_assertions (id, assertion_key, network, address, category, label, confidence, evidence_sha256, evidence_url, source, source_version, supersedes_id, review_state, visibility, trust_tier, valid_from, valid_to, created_by, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
ON CONFLICT (assertion_key, source_version) DO NOTHING`

func (d *DB) InsertLabelAssertions(ctx context.Context, labels []CuratedLabel) error {
	if len(labels) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin label import: %w", err)
	}
	defer tx.Rollback()
	for _, label := range labels {
		if label.EvidenceSnapshot == "" {
			return fmt.Errorf("label assertion %q has no evidence snapshot", label.ID)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(label.EvidenceSnapshot)))
		if _, err := tx.ExecContext(ctx, insertLabelEvidenceSQL, hash, []byte(label.EvidenceSnapshot), label.CreatedAt.UTC()); err != nil {
			return fmt.Errorf("store label evidence %q: %w", label.ID, err)
		}
		result, err := tx.ExecContext(ctx, insertLabelAssertionSQL, label.ID, label.AssertionKey, label.Network, strings.ToLower(label.Address), label.Category, label.Label, label.Confidence, hash, label.EvidenceURL, label.Source, label.SourceVersion, nullableString(label.SupersedesID), label.ReviewState, label.Visibility, label.TrustTier, label.ValidFrom.UTC(), nullableTime(label.ValidTo), label.CreatedBy, label.CreatedAt.UTC())
		if err != nil {
			return fmt.Errorf("insert label assertion %q: %w", label.ID, err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			var existingHash string
			if err := tx.QueryRowContext(ctx, `SELECT evidence_sha256 FROM label_assertions WHERE assertion_key = $1 AND source_version = $2`, label.AssertionKey, label.SourceVersion).Scan(&existingHash); err != nil {
				return fmt.Errorf("read existing label assertion %q: %w", label.ID, err)
			}
			if existingHash != hash {
				return fmt.Errorf("label assertion %q reuses source version %q with different evidence", label.AssertionKey, label.SourceVersion)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit label import: %w", err)
	}
	return nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

const labelColumns = `assertion.id, assertion.assertion_key, assertion.network, assertion.address, assertion.category, assertion.label, assertion.confidence, assertion.evidence_sha256, assertion.evidence_url, assertion.source, assertion.source_version, assertion.supersedes_id, assertion.review_state, assertion.visibility, assertion.trust_tier, assertion.valid_from, assertion.valid_to, assertion.created_by, assertion.created_at, evidence.content`
const labelAssertionFrom = ` FROM label_assertions assertion JOIN label_evidence evidence ON evidence.sha256 = assertion.evidence_sha256 `
const currentLabelPredicate = `assertion.review_state = 'approved' AND assertion.valid_from <= now() AND (assertion.valid_to IS NULL OR assertion.valid_to > now()) AND NOT EXISTS (SELECT 1 FROM label_assertions replacement WHERE replacement.supersedes_id = assertion.id AND replacement.review_state = 'approved' AND replacement.valid_from <= now() AND (replacement.valid_to IS NULL OR replacement.valid_to > now()))`

func (d *DB) GetCuratedLabels(ctx context.Context, network, address string) ([]CuratedLabel, error) {
	return d.queryCuratedLabels(ctx, `SELECT `+labelColumns+labelAssertionFrom+`WHERE assertion.network = $1 AND assertion.address = $2 AND `+currentLabelPredicate+` ORDER BY assertion.trust_tier, assertion.label`, network, strings.ToLower(address))
}

func (d *DB) SearchCuratedLabels(ctx context.Context, network, query, category string, limit int) ([]CuratedLabel, error) {
	return d.queryCuratedLabels(ctx, `SELECT `+labelColumns+labelAssertionFrom+`
WHERE assertion.network = $1 AND `+currentLabelPredicate+` AND ($2 = '' OR assertion.label ILIKE '%' || $2 || '%' OR assertion.address ILIKE '%' || $2 || '%') AND ($3 = '' OR assertion.category = $3)
ORDER BY assertion.trust_tier, assertion.label LIMIT $4`, network, query, category, limit)
}

func (d *DB) queryCuratedLabels(ctx context.Context, query string, arguments ...any) ([]CuratedLabel, error) {
	rows, err := d.SQL.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query curated labels: %w", err)
	}
	defer rows.Close()
	result := make([]CuratedLabel, 0)
	for rows.Next() {
		label, err := scanCuratedLabel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate curated labels: %w", err)
	}
	return result, nil
}

type rowScanner interface{ Scan(...any) error }

func scanCuratedLabel(row rowScanner) (CuratedLabel, error) {
	var label CuratedLabel
	var validTo sql.NullTime
	var supersedesID sql.NullString
	var evidenceSnapshot []byte
	if err := row.Scan(&label.ID, &label.AssertionKey, &label.Network, &label.Address, &label.Category, &label.Label, &label.Confidence, &label.EvidenceHash, &label.EvidenceURL, &label.Source, &label.SourceVersion, &supersedesID, &label.ReviewState, &label.Visibility, &label.TrustTier, &label.ValidFrom, &validTo, &label.CreatedBy, &label.CreatedAt, &evidenceSnapshot); err != nil {
		return CuratedLabel{}, fmt.Errorf("scan curated label: %w", err)
	}
	label.EvidenceSnapshot = string(evidenceSnapshot)
	if supersedesID.Valid {
		label.SupersedesID = supersedesID.String
	}
	if validTo.Valid {
		label.ValidTo = &validTo.Time
	}
	return label, nil
}
