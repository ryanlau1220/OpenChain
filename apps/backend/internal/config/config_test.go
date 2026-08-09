package config

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear env vars to test defaults
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("ETHEREUM_MAINNET_RPC_URL")
	_ = os.Unsetenv("TRUEBLOCKS_API_URL")

	cfg := LoadConfig()
	if cfg.Port != "8081" {
		t.Errorf("expected default Port 8081, got %s", cfg.Port)
	}

	if cfg.Validate() == nil {
		t.Error("expected missing mainnet settings to be rejected")
	}
}

func TestLoadConfigCustomEnv(t *testing.T) {
	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("ETHEREUM_MAINNET_RPC_URL", "https://custom-rpc.example.com")
	_ = os.Setenv("TRUEBLOCKS_API_URL", "http://trueblocks.internal:8080")
	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("ETHEREUM_MAINNET_RPC_URL")
		_ = os.Unsetenv("TRUEBLOCKS_API_URL")
	}()

	cfg := LoadConfig()
	if cfg.Port != "9090" {
		t.Errorf("expected custom Port 9090, got %s", cfg.Port)
	}

	if cfg.EthereumMainnetRPCURL != "https://custom-rpc.example.com" {
		t.Errorf("expected custom RPC URL, got %s", cfg.EthereumMainnetRPCURL)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got %v", err)
	}
}
