package config

import (
	"os"
)

type Config struct {
	Port              string
	DatabaseURL       string
	TrueBlocksAPIURL  string
	EthSepoliaRPCURL  string
	BaseSepoliaRPCURL string
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

	tbURL := os.Getenv("TRUEBLOCKS_API_URL")
	if tbURL == "" {
		tbURL = "http://localhost:8085"
	}

	ethRPC := os.Getenv("ETH_SEPOLIA_RPC_URL")
	if ethRPC == "" {
		ethRPC = "https://ethereum-sepolia-rpc.publicnode.com"
	}

	baseRPC := os.Getenv("BASE_SEPOLIA_RPC_URL")
	if baseRPC == "" {
		baseRPC = "https://sepolia.base.org"
	}

	return &Config{
		Port:              port,
		DatabaseURL:       dbURL,
		TrueBlocksAPIURL:  tbURL,
		EthSepoliaRPCURL:  ethRPC,
		BaseSepoliaRPCURL: baseRPC,
	}
}

