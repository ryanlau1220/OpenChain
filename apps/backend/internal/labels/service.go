package labels

import (
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

//go:embed seed_labels.json
var embeddedSeedLabels []byte

type LabelItem struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	Network     string    `json:"network"`
	Category    string    `json:"category"` // Exchange, Mixer, DeFi, Sanction, Hack, Whale
	Label       string    `json:"label"`
	Confidence  float64   `json:"confidence"`
	EvidenceURL string    `json:"evidence_url"`
	Source      string    `json:"source"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type Registry struct {
	mu     sync.RWMutex
	labels map[string][]LabelItem // address -> labels
}

func NewRegistry() *Registry {
	r := &Registry{
		labels: make(map[string][]LabelItem),
	}
	r.seedWellKnownLabels()
	return r
}

func (r *Registry) seedWellKnownLabels() {
	var wellKnown []LabelItem
	if err := json.Unmarshal(embeddedSeedLabels, &wellKnown); err != nil {
		slog.Warn("failed to parse embedded seed_labels.json", "error", err)
		return
	}

	now := time.Now()
	for _, l := range wellKnown {
		if l.CreatedAt.IsZero() {
			l.CreatedAt = now
		}
		addrKey := strings.ToLower(l.Address)
		r.labels[addrKey] = append(r.labels[addrKey], l)
	}
}

func (r *Registry) AddLabel(ctx context.Context, l LabelItem) (LabelItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	l.CreatedAt = time.Now()
	addrKey := strings.ToLower(l.Address)

	r.labels[addrKey] = append(r.labels[addrKey], l)
	return l, nil
}

func (r *Registry) GetLabels(ctx context.Context, address string) []LabelItem {
	r.mu.RLock()
	defer r.mu.RUnlock()

	addrKey := strings.ToLower(address)
	return r.labels[addrKey]
}

func (r *Registry) SearchLabels(ctx context.Context, query string, category string, limit int) []LabelItem {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []LabelItem
	queryLower := strings.ToLower(query)
	catLower := strings.ToLower(category)

	for _, list := range r.labels {
		for _, l := range list {
			matchQuery := queryLower == "" || strings.Contains(strings.ToLower(l.Address), queryLower) || strings.Contains(strings.ToLower(l.Label), queryLower)
			matchCat := catLower == "" || strings.ToLower(l.Category) == catLower

			if matchQuery && matchCat {
				result = append(result, l)
				if limit > 0 && len(result) >= limit {
					return result
				}
			}
		}
	}
	return result
}
