package labels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

// OFACSanctionItem defines structure of static OFAC SDN address entries
type OFACSanctionItem struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Source   string `json:"source"`
}

// Tier1IngestionWorker runs periodic cron tasks fetching Tier 1 Authoritative datasets
type Tier1IngestionWorker struct {
	DB         *db.DB
	Registry   *Registry
	HTTPClient *http.Client
	FeedURL    string
}

// NewTier1Worker creates a new ingestion worker
func NewTier1Worker(database *db.DB, registry *Registry, feedURL string) *Tier1IngestionWorker {
	if feedURL == "" {
		// Default feed endpoint or open dataset placeholder
		feedURL = "https://raw.githubusercontent.com/ofac-sanctions/eth-addresses/main/sanctions.json"
	}
	return &Tier1IngestionWorker{
		DB:       database,
		Registry: registry,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		FeedURL: feedURL,
	}
}

// StartCron launches periodic ingestion every 1 hour (or context cancellation)
func (w *Tier1IngestionWorker) StartCron(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Perform initial fetch
	if err := w.IngestTier1Datasets(ctx); err != nil {
		slog.Warn("initial Tier 1 dataset ingestion warning", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("Tier 1 label worker stopped")
			return
		case <-ticker.C:
			if err := w.IngestTier1Datasets(ctx); err != nil {
				slog.Warn("scheduled Tier 1 dataset ingestion failed", "error", err)
			}
		}
	}
}

// IngestTier1Datasets fetches static feeds, normalizes addresses, and persists Tier 1 labels
func (w *Tier1IngestionWorker) IngestTier1Datasets(ctx context.Context) error {
	slog.Info("Fetching Tier 1 Authoritative sanction datasets...", "feed", w.FeedURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.FeedURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create feed request: %w", err)
	}

	resp, err := w.HTTPClient.Do(req)
	if err != nil {
		slog.Warn("Tier 1 feed endpoint offline, seeding static fallback dataset", "error", err)
		return w.seedFallbackTier1(ctx)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return w.seedFallbackTier1(ctx)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed reading feed body: %w", err)
	}

	var items []OFACSanctionItem
	if err := json.Unmarshal(body, &items); err != nil {
		return w.seedFallbackTier1(ctx)
	}

	for _, item := range items {
		addr := strings.ToLower(item.Address)
		if addr == "" {
			continue
		}

		labelID := fmt.Sprintf("ofac-%s", addr)
		labelNode := db.LabelNode{
			ID:         labelID,
			Category:   "Sanctions",
			Name:       item.Name,
			Confidence: 1.0,
			Source:     "OFAC_SDN",
			CreatedBy:  "AUTHORITATIVE_CRON",
			CreatedAt:  time.Now().Unix(),
		}

		attestation := db.AttestationData{
			Type:         "OFAC_SPECIFICALLY_DESIGNATED_NATIONAL",
			ReferenceURL: "https://sanctionssearch.ofac.treas.gov/",
			ProofHash:    fmt.Sprintf("sdn-%s", addr),
			Timestamp:    time.Now().Unix(),
		}

		if w.DB != nil {
			if err := w.DB.UpsertLabelVertex(ctx, labelNode); err == nil {
				_ = w.DB.AttachLabelEdge(ctx, addr, labelID, 1, attestation) // TrustTier 1
			}
		}

		if w.Registry != nil {
			_, _ = w.Registry.AddLabel(ctx, LabelItem{
				ID:          labelID,
				Address:     addr,
				Network:     "ETHEREUM_SEPOLIA",
				Category:    "Sanctions",
				Label:       item.Name,
				Confidence:  1.0,
				EvidenceURL: "https://sanctionssearch.ofac.treas.gov/",
				Source:      "OFAC_SDN",
				CreatedBy:   "AUTHORITATIVE_CRON",
				CreatedAt:   time.Now(),
			})
		}
	}

	slog.Info("Successfully ingested Tier 1 dataset items", "count", len(items))
	return nil
}

func (w *Tier1IngestionWorker) seedFallbackTier1(ctx context.Context) error {
	fallbackItems := []OFACSanctionItem{
		{
			Address:  "0x098B716B8Aaf21512996dC57EB0615e2383E2f96",
			Name:     "Tornado.Cash Router (OFAC Sanctioned)",
			Category: "Sanctions",
			Source:   "OFAC_SDN",
		},
		{
			Address:  "0xa160cd374d02b56f4767db39294f71884a796f38",
			Name:     "Tornado.Cash 100 ETH Vault (OFAC Sanctioned)",
			Category: "Sanctions",
			Source:   "OFAC_SDN",
		},
	}

	for _, item := range fallbackItems {
		addr := strings.ToLower(item.Address)
		labelID := fmt.Sprintf("ofac-%s", addr)
		labelNode := db.LabelNode{
			ID:         labelID,
			Category:   "Sanctions",
			Name:       item.Name,
			Confidence: 1.0,
			Source:     "OFAC_SDN",
			CreatedBy:  "AUTHORITATIVE_CRON",
			CreatedAt:  time.Now().Unix(),
		}

		attestation := db.AttestationData{
			Type:         "OFAC_SPECIFICALLY_DESIGNATED_NATIONAL",
			ReferenceURL: "https://sanctionssearch.ofac.treas.gov/",
			ProofHash:    fmt.Sprintf("sdn-%s", addr),
			Timestamp:    time.Now().Unix(),
		}

		if w.DB != nil {
			if err := w.DB.UpsertLabelVertex(ctx, labelNode); err == nil {
				_ = w.DB.AttachLabelEdge(ctx, addr, labelID, 1, attestation)
			}
		}

		if w.Registry != nil {
			_, _ = w.Registry.AddLabel(ctx, LabelItem{
				ID:          labelID,
				Address:     addr,
				Network:     "ETHEREUM_SEPOLIA",
				Category:    "Sanctions",
				Label:       item.Name,
				Confidence:  1.0,
				EvidenceURL: "https://sanctionssearch.ofac.treas.gov/",
				Source:      "OFAC_SDN",
				CreatedBy:   "AUTHORITATIVE_CRON",
				CreatedAt:   time.Now(),
			})
		}
	}
	return nil
}
