package adapter

// NetworkCapabilities declares the evidence OpenChain can produce for a
// network today. A false value means unsupported, not unavailable data.
type NetworkCapabilities struct {
	NativeTransfers      bool `json:"native_transfers"`
	TokenTransfers       bool `json:"token_transfers"`
	InternalTransfers    bool `json:"internal_transfers"`
	HistoricalPagination bool `json:"historical_pagination"`
	Finality             bool `json:"finality"`
	TransactionSuccess   bool `json:"transaction_success"`
	EntityClassification bool `json:"entity_classification"`
	BridgeEvidence       bool `json:"bridge_evidence"`
	ExactRawProvenance   bool `json:"exact_raw_provenance"`
}

func evmCapabilities() NetworkCapabilities {
	return NetworkCapabilities{
		NativeTransfers:      true,
		TokenTransfers:       true,
		InternalTransfers:    true,
		HistoricalPagination: true,
		Finality:             true,
		EntityClassification: true,
		ExactRawProvenance:   true,
	}
}
