package config

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear env vars to test defaults
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("ETH_SEPOLIA_RPC_URL")

	cfg := LoadConfig()
	if cfg.Port != "8081" {
		t.Errorf("expected default Port 8081, got %s", cfg.Port)
	}

	if cfg.EthSepoliaRPCURL != "https://ethereum-sepolia-rpc.publicnode.com" {
		t.Errorf("expected default EthSepoliaRPCURL, got %s", cfg.EthSepoliaRPCURL)
	}
}

func TestLoadConfigCustomEnv(t *testing.T) {
	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("ETH_SEPOLIA_RPC_URL", "https://custom-rpc.example.com")
	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("ETH_SEPOLIA_RPC_URL")
	}()

	cfg := LoadConfig()
	if cfg.Port != "9090" {
		t.Errorf("expected custom Port 9090, got %s", cfg.Port)
	}

	if cfg.EthSepoliaRPCURL != "https://custom-rpc.example.com" {
		t.Errorf("expected custom RPC URL, got %s", cfg.EthSepoliaRPCURL)
	}
}
