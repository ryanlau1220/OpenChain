package labels

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

const ethereumMainnet = "ethereum-mainnet"

//go:embed seed_labels.json
var embeddedSeedLabels []byte

type LabelItem struct {
	ID            string    `json:"id"`
	Address       string    `json:"address"`
	Network       string    `json:"network"`
	Category      string    `json:"category"`
	Label         string    `json:"label"`
	Confidence    float64   `json:"confidence"`
	EvidenceURL   string    `json:"evidence_url"`
	Source        string    `json:"source"`
	SourceVersion string    `json:"source_version"`
	Visibility    string    `json:"visibility"`
	TrustTier     uint32    `json:"trust_tier"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

type Service struct{ database *db.DB }

func NewService(database *db.DB) *Service { return &Service{database: database} }

func SeedLabels() ([]LabelItem, error) {
	var items []LabelItem
	if err := json.Unmarshal(embeddedSeedLabels, &items); err != nil {
		return nil, fmt.Errorf("parse curated label seed: %w", err)
	}
	for index := range items {
		item := &items[index]
		item.Address = strings.ToLower(item.Address)
		if item.Network != ethereumMainnet || item.ID == "" || item.Address == "" || item.Label == "" || item.EvidenceURL == "" || item.Source == "" || item.SourceVersion == "" || item.Visibility != "public" || item.TrustTier < 1 || item.TrustTier > 3 || item.Confidence < 0 || item.Confidence > 1 {
			return nil, fmt.Errorf("invalid curated label %q", item.ID)
		}
		if item.CreatedAt.IsZero() {
			return nil, fmt.Errorf("curated label %q has no created_at", item.ID)
		}
	}
	return items, nil
}

func (s *Service) ImportSeed(ctx context.Context) error {
	if s == nil || s.database == nil {
		return fmt.Errorf("curated label database is unavailable")
	}
	items, err := SeedLabels()
	if err != nil {
		return err
	}
	values := make([]db.CuratedLabel, 0, len(items))
	for _, item := range items {
		values = append(values, db.CuratedLabel{ID: item.ID, Network: item.Network, Address: item.Address, Category: item.Category, Label: item.Label, Confidence: item.Confidence, EvidenceURL: item.EvidenceURL, Source: item.Source, SourceVersion: item.SourceVersion, Visibility: item.Visibility, TrustTier: item.TrustTier, CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt})
	}
	return s.database.UpsertCuratedLabels(ctx, values)
}

func (s *Service) GetLabels(ctx context.Context, network, address string) ([]LabelItem, error) {
	if s == nil || s.database == nil {
		return nil, fmt.Errorf("curated label database is unavailable")
	}
	items, err := s.database.GetCuratedLabels(ctx, network, address)
	return fromDB(items), err
}

func (s *Service) SearchLabels(ctx context.Context, network, query, category string, limit int) ([]LabelItem, error) {
	if s == nil || s.database == nil {
		return nil, fmt.Errorf("curated label database is unavailable")
	}
	items, err := s.database.SearchCuratedLabels(ctx, network, query, category, limit)
	return fromDB(items), err
}

func fromDB(items []db.CuratedLabel) []LabelItem {
	result := make([]LabelItem, 0, len(items))
	for _, item := range items {
		result = append(result, LabelItem{ID: item.ID, Network: item.Network, Address: item.Address, Category: item.Category, Label: item.Label, Confidence: item.Confidence, EvidenceURL: item.EvidenceURL, Source: item.Source, SourceVersion: item.SourceVersion, Visibility: item.Visibility, TrustTier: item.TrustTier, CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt})
	}
	return result
}
