package main

import (
	"testing"

	"github.com/openchain/openchain/apps/backend/internal/config"
)

func TestConfigInitialization(t *testing.T) {
	cfg := config.LoadConfig()
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}
	if cfg.Port == "" {
		t.Errorf("expected port to be set")
	}
}
