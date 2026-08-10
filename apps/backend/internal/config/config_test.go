package config

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear env vars to test defaults
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("ETHEREUM_MAINNET_RPC_URL")
	_ = os.Unsetenv("ETHERSCAN_API_KEY")
	_ = os.Unsetenv("PUBLIC_REQUESTS_PER_MINUTE")
	_ = os.Unsetenv("MAX_QUEUED_TRACE_JOBS")
	_ = os.Unsetenv("TRUST_PROXY")

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
	_ = os.Setenv("ETHERSCAN_API_KEY", "test-key")
	_ = os.Setenv("PUBLIC_REQUESTS_PER_MINUTE", "12")
	_ = os.Setenv("MAX_QUEUED_TRACE_JOBS", "7")
	_ = os.Setenv("TRUST_PROXY", "true")
	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("ETHEREUM_MAINNET_RPC_URL")
		_ = os.Unsetenv("ETHERSCAN_API_KEY")
		_ = os.Unsetenv("PUBLIC_REQUESTS_PER_MINUTE")
		_ = os.Unsetenv("MAX_QUEUED_TRACE_JOBS")
		_ = os.Unsetenv("TRUST_PROXY")
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
	if cfg.PublicRequestsPerMinute != 12 || cfg.MaxQueuedTraceJobs != 7 || !cfg.TrustProxy {
		t.Fatalf("public config = %#v", cfg)
	}
}

func TestValidateRejectsInsecureRPCURL(t *testing.T) {
	cfg := &Config{EthereumMainnetRPCURL: "http://127.0.0.1:8545", EtherscanAPIKey: "test-key"}
	if cfg.Validate() == nil {
		t.Fatal("insecure RPC URL was accepted")
	}
}
