package tracing

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/rules"
)

const bridgeCorrelationWindow = 7 * 24 * time.Hour

const crossChainTransitionLimitation = "This is matching bridge-transfer evidence within retrieved provider pages based on known bridge contracts, reported asset kind/symbol/decimals, raw amount, recipient, and timing. It does not establish cross-chain address ownership, token equivalence, or intent."

type bridgeRoute struct {
	name, sourceNetwork, destinationNetwork string
	sourceBridge, destinationBridge         string
}

// These are the canonical ERC-20 Standard Bridge contracts published for the
// OP Stack chains that OpenChain can currently trace with its configured
// providers. A route is only emitted when all on-chain transfer facts match.
var opStackBridgeRoutes = []bridgeRoute{
	{"Base Standard Bridge", "ethereum-mainnet", "base-mainnet", "0x3154cf16ccdb4c6d922629664174b904d80f2c35", "0x4200000000000000000000000000000000000010"},
	{"Optimism Standard Bridge", "ethereum-mainnet", "optimism-mainnet", "0x99c9fc46f92e8a1c0dec1f919a3f9fa9f7b8c8fc", "0x4200000000000000000000000000000000000010"},
}

type bridgeEvidence struct {
	Candidate    rules.BridgeCandidate
	Transition   CrossChainTransition
	Scope        db.AcquisitionScope
	Addresses    []db.Address
	Transfers    []db.Transfer
	Acquisitions []adapter.RawAcquisition
}

// CrossChainTransition describes a qualified bridge observation without adding
// a synthetic wallet-to-wallet graph edge across networks.
type CrossChainTransition struct {
	ID, BridgeName, SourceNetwork, DestinationNetwork string
	Source, Destination                               db.Transfer
	SourceBridgeAddress, DestinationBridgeAddress     string
	Limitations                                       string
}

type BridgeCorrelator struct {
	chains map[string]adapter.ChainAdapter
}

func NewBridgeCorrelator(chains map[string]adapter.ChainAdapter) *BridgeCorrelator {
	copy := make(map[string]adapter.ChainAdapter, len(chains))
	for network, chain := range chains {
		copy[network] = chain
	}
	return &BridgeCorrelator{chains: copy}
}

func (c *BridgeCorrelator) Correlate(ctx context.Context, sourceNetwork string, sourceTransfers []db.Transfer) []bridgeEvidence {
	if c == nil || sourceNetwork != "ethereum-mainnet" {
		return nil
	}
	result := make([]bridgeEvidence, 0)
	for _, route := range opStackBridgeRoutes {
		chain := c.chains[route.destinationNetwork]
		if chain == nil {
			continue
		}
		byOwner := make(map[string][]db.Transfer)
		for _, source := range sourceTransfers {
			if source.Provisional || !strings.EqualFold(source.ToAddress, route.sourceBridge) || source.FromAddress == "" {
				continue
			}
			byOwner[strings.ToLower(source.FromAddress)] = append(byOwner[strings.ToLower(source.FromAddress)], source)
		}
		owners := make([]string, 0, len(byOwner))
		for owner := range byOwner {
			owners = append(owners, owner)
		}
		sort.Strings(owners)
		for _, owner := range owners {
			sources := byOwner[owner]
			acquisitionContext, recorder := adapter.WithAcquisitionRecorder(ctx)
			page, err := chain.ListTransfers(acquisitionContext, owner, maxPageSize, "")
			if err != nil {
				continue
			}
			retrievedAt := page.SourceStatus.RetrievedAt
			if retrievedAt.IsZero() {
				retrievedAt = time.Now().UTC()
			}
			scope := db.AcquisitionScope{Network: route.destinationNetwork, Address: owner, RetrievedAt: retrievedAt}
			for _, source := range sources {
				for _, item := range page.Transfers {
					if !strings.EqualFold(item.From, route.destinationBridge) || !strings.EqualFold(item.To, owner) || !matchingBridgeAsset(source.Asset, item.Asset) || item.AmountBaseUnits != source.AmountBaseUnits || item.Timestamp.Before(source.BlockTimestamp) || item.Timestamp.Sub(source.BlockTimestamp) > bridgeCorrelationWindow {
						continue
					}
					destination := db.Transfer{ID: transferID(route.destinationNetwork, item.Hash, item.EventID), Network: route.destinationNetwork, TransactionHash: item.Hash, EventID: item.EventID, TransferKind: item.TransferKind, FromAddress: strings.ToLower(item.From), ToAddress: strings.ToLower(item.To), Asset: item.Asset, AmountBaseUnits: item.AmountBaseUnits, BlockNumber: item.BlockNumber, BlockHash: item.BlockHash, BlockTimestamp: item.Timestamp, Provisional: isProvisional(route.destinationNetwork, item.Timestamp, page.SourceStatus.RetrievedAt), Source: page.SourceStatus.Source, RetrievedAt: page.SourceStatus.RetrievedAt}
					if destination.Provisional {
						continue
					}
					candidate := rules.BridgeCandidate{BridgeName: route.name, DestinationNetwork: route.destinationNetwork, Source: source, Destination: destination}
					result = append(result, bridgeEvidence{Candidate: candidate, Transition: crossChainTransition(candidate, route.sourceBridge, route.destinationBridge), Scope: scope, Addresses: []db.Address{{Network: route.destinationNetwork, Address: destination.FromAddress, Label: shortAddress(destination.FromAddress), EntityType: "BRIDGE"}, {Network: route.destinationNetwork, Address: destination.ToAddress, Label: shortAddress(destination.ToAddress), EntityType: "EOA"}}, Transfers: []db.Transfer{destination}, Acquisitions: recorder.Items()})
					break
				}
			}
		}
	}
	return result
}

func matchingBridgeAsset(source, destination adapter.Asset) bool {
	return source.Kind == destination.Kind && source.Symbol != "" && strings.EqualFold(source.Symbol, destination.Symbol) && source.Decimals == destination.Decimals
}

func crossChainTransition(candidate rules.BridgeCandidate, sourceBridge, destinationBridge string) CrossChainTransition {
	return CrossChainTransition{
		ID:                       "cross-chain:" + candidate.Source.ID + ":" + candidate.Destination.ID,
		BridgeName:               candidate.BridgeName,
		SourceNetwork:            candidate.Source.Network,
		DestinationNetwork:       candidate.Destination.Network,
		Source:                   candidate.Source,
		Destination:              candidate.Destination,
		SourceBridgeAddress:      sourceBridge,
		DestinationBridgeAddress: destinationBridge,
		Limitations:              crossChainTransitionLimitation,
	}
}
