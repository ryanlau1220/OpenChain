package config

import (
	"os"
)

type Config struct {
	Port              string
	DatabaseURL       string
	ValkeyURL         string
	EthSepoliaRPCURL  string
	BaseSepoliaRPCURL string
	ZitadelIssuer     string
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

	valkeyURL := os.Getenv("VALKEY_URL")
	if valkeyURL == "" {
		valkeyURL = "localhost:6379"
	}

	ethRPC := os.Getenv("ETH_SEPOLIA_RPC_URL")
	if ethRPC == "" {
		ethRPC = "https://ethereum-sepolia-rpc.publicnode.com"
	}

	baseRPC := os.Getenv("BASE_SEPOLIA_RPC_URL")
	if baseRPC == "" {
		baseRPC = "https://sepolia.base.org"
	}

	zitadelIssuer := os.Getenv("ZITADEL_ISSUER")
	if zitadelIssuer == "" {
		zitadelIssuer = "http://localhost:8080"
	}

	return &Config{
		Port:              port,
		DatabaseURL:       dbURL,
		ValkeyURL:         valkeyURL,
		EthSepoliaRPCURL:  ethRPC,
		BaseSepoliaRPCURL: baseRPC,
		ZitadelIssuer:     zitadelIssuer,
	}
}
