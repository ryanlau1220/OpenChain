package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	EthereumMainnetRPCURL   string
	BaseMainnetRPCURL       string
	EtherscanAPIKey         string
	BlockscoutAPIKey        string
	WebOrigin               string
	PublicRequestsPerMinute int
	MaxQueuedTraceJobs      int
	TrustProxy              bool
	validationError         error
}

func LoadConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://openchain:openchain_secret@localhost:5432/openchain?sslmode=disable"
	}

	ethRPC := os.Getenv("ETHEREUM_MAINNET_RPC_URL")
	baseRPC := os.Getenv("BASE_MAINNET_RPC_URL")
	etherscanAPIKey := os.Getenv("ETHERSCAN_API_KEY")
	blockscoutAPIKey := os.Getenv("BLOCKSCOUT_API_KEY")

	webOrigin := os.Getenv("WEB_ORIGIN")
	if webOrigin == "" {
		webOrigin = "http://localhost:3000"
	}
	publicRequestsPerMinute, requestLimitError := positiveEnv("PUBLIC_REQUESTS_PER_MINUTE", 30)
	maxQueuedTraceJobs, queueLimitError := positiveEnv("MAX_QUEUED_TRACE_JOBS", 25)
	trustProxy, trustProxyError := boolEnv("TRUST_PROXY", false)
	validationError := requestLimitError
	if validationError == nil {
		validationError = queueLimitError
	}
	if validationError == nil {
		validationError = trustProxyError
	}

	return &Config{
		Port:                    port,
		DatabaseURL:             dbURL,
		EthereumMainnetRPCURL:   ethRPC,
		BaseMainnetRPCURL:       baseRPC,
		EtherscanAPIKey:         etherscanAPIKey,
		BlockscoutAPIKey:        blockscoutAPIKey,
		WebOrigin:               webOrigin,
		PublicRequestsPerMinute: publicRequestsPerMinute,
		MaxQueuedTraceJobs:      maxQueuedTraceJobs,
		TrustProxy:              trustProxy,
		validationError:         validationError,
	}
}

func (c *Config) Validate() error {
	if c.validationError != nil {
		return c.validationError
	}
	for _, rpc := range []struct {
		name, value string
	}{{"ETHEREUM_MAINNET_RPC_URL", c.EthereumMainnetRPCURL}, {"BASE_MAINNET_RPC_URL", c.BaseMainnetRPCURL}} {
		if rpc.value == "" {
			return fmt.Errorf("%s is required", rpc.name)
		}
		if endpoint, err := url.Parse(rpc.value); err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
			return fmt.Errorf("%s must be an https URL", rpc.name)
		}
	}
	if c.EtherscanAPIKey == "" {
		return fmt.Errorf("ETHERSCAN_API_KEY is required")
	}
	if c.BlockscoutAPIKey == "" {
		return fmt.Errorf("BLOCKSCOUT_API_KEY is required")
	}
	return nil
}

func positiveEnv(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func boolEnv(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}
