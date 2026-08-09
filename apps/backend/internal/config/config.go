package config

import (
	"os"
)

type Config struct {
	Port             string
	DatabaseURL      string
	EthSepoliaRPCURL string
	WebOrigin        string
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

	ethRPC := os.Getenv("ETH_SEPOLIA_RPC_URL")
	if ethRPC == "" {
		ethRPC = "https://ethereum-sepolia-rpc.publicnode.com"
	}

	webOrigin := os.Getenv("WEB_ORIGIN")
	if webOrigin == "" {
		webOrigin = "http://localhost:3000"
	}

	return &Config{
		Port:             port,
		DatabaseURL:      dbURL,
		EthSepoliaRPCURL: ethRPC,
		WebOrigin:        webOrigin,
	}
}
