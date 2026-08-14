package evidence

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

const (
	Format  = "openchain-evidence-package"
	Version = 3
)

type Package struct {
	Format   string   `json:"format"`
	Version  int      `json:"version"`
	Payload  Payload  `json:"payload"`
	Manifest Manifest `json:"manifest"`
}

type Manifest struct {
	Algorithm     string `json:"algorithm"`
	PayloadSHA256 string `json:"payload_sha256"`
}

type Payload struct {
	ExportedAt                   time.Time                     `json:"exported_at"`
	Case                         json.RawMessage               `json:"case"`
	Transfers                    []Transfer                    `json:"transfers"`
	Snapshots                    []Snapshot                    `json:"acquisition_snapshots"`
	Scopes                       []AcquisitionScope            `json:"acquisition_scopes"`
	ScopeTransfers               []ScopeTransfer               `json:"scope_transfers"`
	ScopeSnapshots               []ScopeSnapshot               `json:"scope_snapshots"`
	RuleRuns                     []RuleRun                     `json:"rule_runs"`
	Labels                       []Label                       `json:"labels"`
	BridgeTransitions            []BridgeTransition            `json:"bridge_transitions"`
	BridgeTransitionAcquisitions []BridgeTransitionAcquisition `json:"bridge_transition_acquisitions"`
}

type Transfer struct {
	ID              string    `json:"id"`
	Network         string    `json:"network"`
	TransactionHash string    `json:"transaction_hash"`
	EventID         string    `json:"event_id"`
	TransferKind    string    `json:"transfer_kind"`
	FromAddress     string    `json:"from_address"`
	ToAddress       string    `json:"to_address"`
	Asset           Asset     `json:"asset"`
	AmountBaseUnits string    `json:"amount_base_units"`
	BlockNumber     int64     `json:"block_number"`
	BlockHash       string    `json:"block_hash"`
	BlockTimestamp  time.Time `json:"block_timestamp"`
	Provisional     bool      `json:"provisional"`
	Source          string    `json:"source"`
	RetrievedAt     time.Time `json:"retrieved_at"`
}

type Asset struct {
	Kind            string `json:"kind"`
	ContractAddress string `json:"contract_address"`
	Symbol          string `json:"symbol"`
	Decimals        uint32 `json:"decimals"`
}

type Snapshot struct {
	ID                 int64     `json:"id"`
	Network            string    `json:"network"`
	Provider           string    `json:"provider"`
	RequestIdentity    string    `json:"request_identity"`
	ResponseSHA256     string    `json:"response_sha256"`
	ResponseBodyBase64 string    `json:"response_body_base64"`
	RetrievedAt        time.Time `json:"retrieved_at"`
}

// AcquisitionScope identifies the page of observations resolved by a trace.
// Snapshot links are page-scoped, rather than assertions about each transfer.
type AcquisitionScope struct {
	ID          int64     `json:"id"`
	Network     string    `json:"network"`
	Address     string    `json:"address"`
	Cursor      string    `json:"cursor"`
	RetrievedAt time.Time `json:"retrieved_at"`
}

type ScopeTransfer struct {
	ScopeID    int64  `json:"scope_id"`
	TransferID string `json:"transfer_id"`
}

type ScopeSnapshot struct {
	ScopeID       int64 `json:"scope_id"`
	AcquisitionID int64 `json:"acquisition_id"`
}

type RuleRun struct {
	ID               int64           `json:"id"`
	Network          string          `json:"network"`
	RuleID           string          `json:"rule_id"`
	RuleVersion      string          `json:"rule_version"`
	Parameters       json.RawMessage `json:"parameters"`
	InputTransferIDs json.RawMessage `json:"input_transfer_ids"`
	Result           json.RawMessage `json:"result"`
	StartedAt        time.Time       `json:"started_at"`
	CompletedAt      time.Time       `json:"completed_at"`
}

type Label struct {
	ID            string    `json:"id"`
	Network       string    `json:"network"`
	Address       string    `json:"address"`
	Category      string    `json:"category"`
	Value         string    `json:"label"`
	Confidence    float64   `json:"confidence"`
	EvidenceURL   string    `json:"evidence_url"`
	Source        string    `json:"source"`
	SourceVersion string    `json:"source_version"`
	Visibility    string    `json:"visibility"`
	TrustTier     uint32    `json:"trust_tier"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

type BridgeTransition struct {
	ID, Protocol, BridgeName, SourceNetwork, DestinationNetwork, Lifecycle, MessageID string
	SourceTransferID, DestinationTransferID                                           string
	SourceTransactionHash, DestinationTransactionHash                                 string
	SourceLogReference, DestinationLogReference                                       string
	SourceBridgeAddress, DestinationBridgeAddress                                     string
	CanonicalSourceToken, CanonicalDestinationToken, Recipient, AmountBaseUnits       string
	Asset                                                                             Asset
	SourceBlockNumber, DestinationBlockNumber                                         uint64
	SourceBlockHash, DestinationBlockHash                                             string
	SourceTimestamp, DestinationTimestamp                                             time.Time
	SourceConfirmed, DestinationConfirmed                                             bool
	Limitations                                                                       string
}

type BridgeTransitionAcquisition struct {
	TransitionID  string `json:"transition_id"`
	Side          string `json:"side"`
	AcquisitionID int64  `json:"acquisition_id"`
}

func Build(caseJSON []byte, exported *db.EvidenceExport, now time.Time) (*Package, error) {
	if !json.Valid(caseJSON) || exported == nil {
		return nil, fmt.Errorf("invalid evidence package input")
	}
	payload := Payload{Case: caseJSON, ExportedAt: now.UTC()}
	for _, transfer := range exported.Transfers {
		payload.Transfers = append(payload.Transfers, Transfer{ID: transfer.ID, Network: transfer.Network, TransactionHash: transfer.TransactionHash, EventID: transfer.EventID, TransferKind: transfer.TransferKind, FromAddress: transfer.FromAddress, ToAddress: transfer.ToAddress, Asset: Asset{Kind: transfer.Asset.Kind, ContractAddress: transfer.Asset.ContractAddress, Symbol: transfer.Asset.Symbol, Decimals: transfer.Asset.Decimals}, AmountBaseUnits: transfer.AmountBaseUnits, BlockNumber: transfer.BlockNumber, BlockHash: transfer.BlockHash, BlockTimestamp: transfer.BlockTimestamp.UTC(), Provisional: transfer.Provisional, Source: transfer.Source, RetrievedAt: transfer.RetrievedAt.UTC()})
	}
	for _, snapshot := range exported.Snapshots {
		payload.Snapshots = append(payload.Snapshots, Snapshot{ID: snapshot.ID, Network: snapshot.Network, Provider: snapshot.Provider, RequestIdentity: snapshot.RequestIdentity, ResponseSHA256: snapshot.Hash, ResponseBodyBase64: base64.StdEncoding.EncodeToString(snapshot.Response), RetrievedAt: snapshot.RetrievedAt.UTC()})
	}
	for _, scope := range exported.Scopes {
		payload.Scopes = append(payload.Scopes, AcquisitionScope{ID: scope.ID, Network: scope.Network, Address: scope.Address, Cursor: scope.Cursor, RetrievedAt: scope.RetrievedAt.UTC()})
	}
	for _, link := range exported.ScopeTransfers {
		payload.ScopeTransfers = append(payload.ScopeTransfers, ScopeTransfer{ScopeID: link.ScopeID, TransferID: link.TransferID})
	}
	for _, link := range exported.ScopeSnapshots {
		payload.ScopeSnapshots = append(payload.ScopeSnapshots, ScopeSnapshot{ScopeID: link.ScopeID, AcquisitionID: link.AcquisitionID})
	}
	for _, run := range exported.RuleRuns {
		payload.RuleRuns = append(payload.RuleRuns, RuleRun{ID: run.ID, Network: run.Network, RuleID: run.RuleID, RuleVersion: run.RuleVersion, Parameters: run.Parameters, InputTransferIDs: run.InputTransferIDs, Result: run.Result, StartedAt: run.StartedAt.UTC(), CompletedAt: run.CompletedAt.UTC()})
	}
	for _, label := range exported.Labels {
		payload.Labels = append(payload.Labels, Label{ID: label.ID, Network: label.Network, Address: label.Address, Category: label.Category, Value: label.Label, Confidence: label.Confidence, EvidenceURL: label.EvidenceURL, Source: label.Source, SourceVersion: label.SourceVersion, Visibility: label.Visibility, TrustTier: label.TrustTier, CreatedBy: label.CreatedBy, CreatedAt: label.CreatedAt.UTC()})
	}
	for _, transition := range exported.BridgeTransitions {
		payload.BridgeTransitions = append(payload.BridgeTransitions, BridgeTransition{ID: transition.ID, Protocol: transition.Protocol, BridgeName: transition.BridgeName, SourceNetwork: transition.SourceNetwork, DestinationNetwork: transition.DestinationNetwork, Lifecycle: transition.Lifecycle, MessageID: transition.MessageID, SourceTransferID: transition.SourceTransferID, DestinationTransferID: transition.DestinationTransferID, SourceTransactionHash: transition.SourceTransactionHash, DestinationTransactionHash: transition.DestinationTransactionHash, SourceLogReference: transition.SourceLogReference, DestinationLogReference: transition.DestinationLogReference, SourceBridgeAddress: transition.SourceBridgeAddress, DestinationBridgeAddress: transition.DestinationBridgeAddress, CanonicalSourceToken: transition.CanonicalSourceToken, CanonicalDestinationToken: transition.CanonicalDestinationToken, Recipient: transition.Recipient, AmountBaseUnits: transition.AmountBaseUnits, Asset: Asset{Kind: transition.Asset.Kind, ContractAddress: transition.Asset.ContractAddress, Symbol: transition.Asset.Symbol, Decimals: transition.Asset.Decimals}, SourceBlockNumber: transition.SourceBlockNumber, DestinationBlockNumber: transition.DestinationBlockNumber, SourceBlockHash: transition.SourceBlockHash, DestinationBlockHash: transition.DestinationBlockHash, SourceTimestamp: transition.SourceTimestamp.UTC(), DestinationTimestamp: transition.DestinationTimestamp.UTC(), SourceConfirmed: transition.SourceConfirmed, DestinationConfirmed: transition.DestinationConfirmed, Limitations: transition.Limitations})
	}
	for _, link := range exported.BridgeTransitionAcquisitions {
		payload.BridgeTransitionAcquisitions = append(payload.BridgeTransitionAcquisitions, BridgeTransitionAcquisition{TransitionID: link.TransitionID, Side: link.Side, AcquisitionID: link.AcquisitionID})
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode evidence package payload: %w", err)
	}
	hash := sha256.Sum256(payloadJSON)
	return &Package{Format: Format, Version: Version, Payload: payload, Manifest: Manifest{Algorithm: "SHA-256", PayloadSHA256: hex.EncodeToString(hash[:])}}, nil
}

func Marshal(caseJSON []byte, exported *db.EvidenceExport, now time.Time) ([]byte, error) {
	packageFile, err := Build(caseJSON, exported, now)
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(packageFile)
	if err != nil {
		return nil, fmt.Errorf("encode evidence package: %w", err)
	}
	return result, nil
}
