package cases

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type CaseNodeItem struct {
	Address string `json:"address"`
	Label   string `json:"label"`
	Notes   string `json:"notes"`
}

type CaseEdgeItem struct {
	SourceAddress string `json:"source_address"`
	TargetAddress string `json:"target_address"`
	TxHash        string `json:"tx_hash"`
	Notes         string `json:"notes"`
}

type InvestigationCase struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Status      string         `json:"status"` // ACTIVE, DRAFT, CLOSED
	Tags        []string       `json:"tags"`
	Nodes       []CaseNodeItem `json:"nodes"`
	Edges       []CaseEdgeItem `json:"edges"`
	CreatedBy   string         `json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type Service struct {
	mu    sync.RWMutex
	cases map[string]InvestigationCase
}

func NewService() *Service {
	s := &Service{
		cases: make(map[string]InvestigationCase),
	}
	s.seedDefaultCase()
	return s
}

func (s *Service) seedDefaultCase() {
	cID := "CASE-SEPOLIA-001"
	s.cases[cID] = InvestigationCase{
		ID:          cID,
		Title:       "Sepolia Testnet Suspicious Deposit Investigation",
		Description: "Initial investigation tracking high-value transfers across Sepolia testnet contracts.",
		Status:      "ACTIVE",
		Tags:        []string{"Testnet", "Suspicious", "Uniswap"},
		Nodes: []CaseNodeItem{
			{Address: "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", Label: "Uniswap V2 Router", Notes: "Initial target contract"},
		},
		Edges:     []CaseEdgeItem{},
		CreatedBy: "Lead Analyst",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (s *Service) CreateCase(ctx context.Context, title string, desc string, tags []string) (*InvestigationCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cID := fmt.Sprintf("CASE-%s", uuid.New().String()[:8])
	c := InvestigationCase{
		ID:          cID,
		Title:       title,
		Description: desc,
		Status:      "ACTIVE",
		Tags:        tags,
		Nodes:       []CaseNodeItem{},
		Edges:       []CaseEdgeItem{},
		CreatedBy:   "Analyst",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	s.cases[cID] = c
	return &c, nil
}

func (s *Service) GetCase(ctx context.Context, id string) (*InvestigationCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.cases[id]
	if !ok {
		return nil, fmt.Errorf("case not found: %s", id)
	}
	return &c, nil
}

func (s *Service) ListCases(ctx context.Context) []InvestigationCase {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []InvestigationCase
	for _, c := range s.cases {
		result = append(result, c)
	}
	return result
}

func (s *Service) ExportReport(ctx context.Context, caseID string, format string) (string, []byte, string, error) {
	c, err := s.GetCase(ctx, caseID)
	if err != nil {
		return "", nil, "", err
	}

	switch format {
	case "JSON":
		data, _ := json.MarshalIndent(c, "", "  ")
		return fmt.Sprintf("%s_dossier.json", caseID), data, "application/json", nil
	case "CSV":
		csvContent := fmt.Sprintf("Case ID,Title,Status,Created At\n%s,\"%s\",%s,%s\n", c.ID, c.Title, c.Status, c.CreatedAt.Format(time.RFC3339))
		return fmt.Sprintf("%s_dossier.csv", caseID), []byte(csvContent), "text/csv", nil
	default:
		data, _ := json.MarshalIndent(c, "", "  ")
		return fmt.Sprintf("%s_dossier.json", caseID), data, "application/json", nil
	}
}

type stringWriter struct {
	buf []byte
}

func (sw *stringWriter) Write(p []byte) (n int, err error) {
	sw.buf = append(sw.buf, p...)
	return len(p), nil
}
