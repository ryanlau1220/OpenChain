package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                             string
	DatabaseURL                      string
	EthereumMainnetRPCURL            string
	BaseMainnetRPCURL                string
	SolanaMainnetRPCURL              string
	EtherscanAPIKey                  string
	BlockscoutAPIKey                 string
	AlchemyAPIKey                    string
	TronGridAPIKey                   string
	TonAPIKey                        string
	BlockfrostProjectID              string
	WebOrigin                        string
	PublicRequestsPerMinute          int
	MaxQueuedTraceJobsPerNetwork     int
	MaxQueuedJobsPerClientPerNetwork int
	QueueClientSecret                string
	TrustProxy                       bool
	validationError                  error
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
	solanaRPC := os.Getenv("SOLANA_MAINNET_RPC_URL")
	etherscanAPIKey := os.Getenv("ETHERSCAN_API_KEY")
	blockscoutAPIKey := os.Getenv("BLOCKSCOUT_API_KEY")
	alchemyAPIKey := os.Getenv("ALCHEMY_API_KEY")
	tronGridAPIKey := os.Getenv("TRONGRID_API_KEY")
	tonAPIKey := os.Getenv("TONAPI_KEY")
	blockfrostProjectID := os.Getenv("BLOCKFROST_PROJECT_ID")
	queueClientSecret := os.Getenv("QUEUE_CLIENT_SECRET")

	webOrigin := os.Getenv("WEB_ORIGIN")
	if webOrigin == "" {
		webOrigin = "http://localhost:3000"
	}
	publicRequestsPerMinute, requestLimitError := positiveEnv("PUBLIC_REQUESTS_PER_MINUTE", 30)
	maxQueuedTraceJobs, queueLimitError := positiveEnv("MAX_QUEUED_TRACE_JOBS_PER_NETWORK", 25)
	maxQueuedJobsPerClient, clientQueueLimitError := positiveEnv("MAX_QUEUED_TRACE_JOBS_PER_CLIENT_PER_NETWORK", 3)
	trustProxy, trustProxyError := boolEnv("TRUST_PROXY", false)
	validationError := requestLimitError
	if validationError == nil {
		validationError = queueLimitError
	}
	if validationError == nil {
		validationError = clientQueueLimitError
	}
	if validationError == nil {
		validationError = trustProxyError
	}

	return &Config{
		Port:                             port,
		DatabaseURL:                      dbURL,
		EthereumMainnetRPCURL:            ethRPC,
		BaseMainnetRPCURL:                baseRPC,
		SolanaMainnetRPCURL:              solanaRPC,
		EtherscanAPIKey:                  etherscanAPIKey,
		BlockscoutAPIKey:                 blockscoutAPIKey,
		AlchemyAPIKey:                    alchemyAPIKey,
		TronGridAPIKey:                   tronGridAPIKey,
		TonAPIKey:                        tonAPIKey,
		BlockfrostProjectID:              blockfrostProjectID,
		WebOrigin:                        webOrigin,
		PublicRequestsPerMinute:          publicRequestsPerMinute,
		MaxQueuedTraceJobsPerNetwork:     maxQueuedTraceJobs,
		MaxQueuedJobsPerClientPerNetwork: maxQueuedJobsPerClient,
		QueueClientSecret:                queueClientSecret,
		TrustProxy:                       trustProxy,
		validationError:                  validationError,
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
	if err := validateHeliusURL(c.SolanaMainnetRPCURL); err != nil {
		return err
	}
	if c.EtherscanAPIKey == "" {
		return fmt.Errorf("ETHERSCAN_API_KEY is required")
	}
	if c.BlockscoutAPIKey == "" {
		return fmt.Errorf("BLOCKSCOUT_API_KEY is required")
	}
	if c.AlchemyAPIKey == "" {
		return fmt.Errorf("ALCHEMY_API_KEY is required")
	}
	if c.TronGridAPIKey == "" {
		return fmt.Errorf("TRONGRID_API_KEY is required")
	}
	if c.TonAPIKey == "" || c.BlockfrostProjectID == "" {
		return fmt.Errorf("TONAPI_KEY and BLOCKFROST_PROJECT_ID are required")
	}
	if len(c.QueueClientSecret) < 32 {
		return fmt.Errorf("QUEUE_CLIENT_SECRET must be at least 32 bytes")
	}
	return nil
}

func validateHeliusURL(value string) error {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Scheme != "https" || !strings.HasSuffix(strings.ToLower(endpoint.Hostname()), ".helius-rpc.com") || endpoint.Query().Get("api-key") == "" {
		return fmt.Errorf("SOLANA_MAINNET_RPC_URL must be a Helius https URL with an api-key")
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
