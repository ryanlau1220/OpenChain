package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	EthereumMainnetRPCURL string
	EtherscanAPIKey       string
	WebOrigin             string
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
	etherscanAPIKey := os.Getenv("ETHERSCAN_API_KEY")

	webOrigin := os.Getenv("WEB_ORIGIN")
	if webOrigin == "" {
		webOrigin = "http://localhost:3000"
	}

	return &Config{
		Port:                  port,
		DatabaseURL:           dbURL,
		EthereumMainnetRPCURL: ethRPC,
		EtherscanAPIKey:       etherscanAPIKey,
		WebOrigin:             webOrigin,
	}
}

func (c *Config) Validate() error {
	if c.EthereumMainnetRPCURL == "" {
		return fmt.Errorf("ETHEREUM_MAINNET_RPC_URL is required")
	}
	if c.EtherscanAPIKey == "" {
		return fmt.Errorf("ETHERSCAN_API_KEY is required")
	}
	return nil
}
