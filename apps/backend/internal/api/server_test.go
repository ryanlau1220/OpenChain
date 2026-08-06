package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/cases"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

func setupTestServer() *http.ServeMux {
	evmClient := adapter.NewEVMClient("https://ethereum-sepolia-rpc.publicnode.com")
	labelRegistry := labels.NewRegistry()
	riskEvaluator := risk.NewEvaluator(labelRegistry)
	tracingEngine := tracing.NewEngine(evmClient, labelRegistry, riskEvaluator)
	caseService := cases.NewService()
	wsHub := NewHub()

	server := NewServer(evmClient, labelRegistry, riskEvaluator, tracingEngine, caseService, wsHub)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	return mux
}

func TestHealthAPI(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse health response: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("expected healthy status, got %s", resp["status"])
	}
}

func TestLookupAddressAPI(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/lookup/address?address=0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse lookup response: %v", err)
	}
	if resp["summary"] == nil {
		t.Errorf("expected summary field in response")
	}
}

func TestTraceGraphAPI(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/tracing/graph?seed_address=0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D&max_hops=2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse trace graph response: %v", err)
	}
	if resp["seed_address"] == nil {
		t.Errorf("expected seed_address field in trace response")
	}
}

func TestLabelsAPI(t *testing.T) {
	mux := setupTestServer()

	// GET labels
	req := httptest.NewRequest("GET", "/api/v1/labels?address=0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// POST label
	postBody := `{"address":"0x1111111111111111111111111111111111111111","network":"ETHEREUM_SEPOLIA","category":"DeFi","label":"Test Protocol","confidence":1.0}`
	reqPost := httptest.NewRequest("POST", "/api/v1/labels", strings.NewReader(postBody))
	reqPost.Header.Set("Content-Type", "application/json")
	wPost := httptest.NewRecorder()
	mux.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusOK {
		t.Fatalf("expected status 200 on POST label, got %d", wPost.Code)
	}
}

func TestCasesAPI(t *testing.T) {
	mux := setupTestServer()

	// GET cases
	req := httptest.NewRequest("GET", "/api/v1/cases", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on GET cases, got %d", w.Code)
	}

	// Export Case
	reqExport := httptest.NewRequest("GET", "/api/v1/cases/export?case_id=CASE-SEPOLIA-001&format=JSON", nil)
	wExport := httptest.NewRecorder()
	mux.ServeHTTP(wExport, reqExport)

	if wExport.Code != http.StatusOK {
		t.Fatalf("expected status 200 on export, got %d", wExport.Code)
	}
}
