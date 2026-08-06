package cases

import (
	"context"
	"testing"
)

func TestCaseService(t *testing.T) {
	service := NewService()
	ctx := context.Background()

	// Verify seeded case
	casesList := service.ListCases(ctx)
	if len(casesList) == 0 {
		t.Fatalf("expected seeded investigation case")
	}

	// Create new case
	newCase, err := service.CreateCase(ctx, "Test Sepolia Audit", "Audit of testnet transactions", []string{"Test", "Audit"})
	if err != nil {
		t.Fatalf("failed to create case: %v", err)
	}

	if newCase.ID == "" {
		t.Errorf("expected generated case ID")
	}

	// Retrieve case by ID
	retrieved, err := service.GetCase(ctx, newCase.ID)
	if err != nil {
		t.Fatalf("failed to retrieve case: %v", err)
	}
	if retrieved.Title != "Test Sepolia Audit" {
		t.Errorf("expected Title 'Test Sepolia Audit', got %s", retrieved.Title)
	}

	// Test Export JSON
	filenameJSON, contentJSON, mimeJSON, err := service.ExportReport(ctx, newCase.ID, "JSON")
	if err != nil {
		t.Fatalf("failed to export JSON report: %v", err)
	}
	if mimeJSON != "application/json" || len(contentJSON) == 0 || filenameJSON == "" {
		t.Errorf("invalid JSON export output: %s, %s", filenameJSON, mimeJSON)
	}

	// Test Export CSV
	filenameCSV, contentCSV, mimeCSV, err := service.ExportReport(ctx, newCase.ID, "CSV")
	if err != nil {
		t.Fatalf("failed to export CSV report: %v", err)
	}
	if mimeCSV != "text/csv" || len(contentCSV) == 0 || filenameCSV == "" {
		t.Errorf("invalid CSV export output: %s, %s", filenameCSV, mimeCSV)
	}
}
