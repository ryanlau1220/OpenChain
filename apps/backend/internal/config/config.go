package config

import (
	"os"
)

type Config struct {
	Port              string
	EthSepoliaRPCURL  string
	BaseSepoliaRPCURL string
}

func LoadConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
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
		EthSepoliaRPCURL:  ethRPC,
		BaseSepoliaRPCURL: baseRPC,
	}
}
