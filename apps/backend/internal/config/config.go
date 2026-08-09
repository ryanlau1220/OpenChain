package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	EthereumMainnetRPCURL string
	TrueBlocksAPIURL      string
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
	trueBlocksURL := os.Getenv("TRUEBLOCKS_API_URL")

	webOrigin := os.Getenv("WEB_ORIGIN")
	if webOrigin == "" {
		webOrigin = "http://localhost:3000"
	}

	return &Config{
		Port:                  port,
		DatabaseURL:           dbURL,
		EthereumMainnetRPCURL: ethRPC,
		TrueBlocksAPIURL:      trueBlocksURL,
		WebOrigin:             webOrigin,
	}
}

func (c *Config) Validate() error {
	if c.EthereumMainnetRPCURL == "" {
		return fmt.Errorf("ETHEREUM_MAINNET_RPC_URL is required")
	}
	if c.TrueBlocksAPIURL == "" {
		return fmt.Errorf("TRUEBLOCKS_API_URL is required")
	}
	return nil
}
