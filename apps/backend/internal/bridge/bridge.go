// Package bridge contains protocol-specific, deterministic cross-chain
// evidence adapters. It deliberately does not attempt address clustering or
// ownership inference.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"golang.org/x/crypto/sha3"
)

const (
	EthereumMainnet = "ethereum-mainnet"
	BaseMainnet     = "base-mainnet"

	baseL1Messenger = "0x866e82a600a1414e583f7f13623f1ac5d58b0afa"
	baseL1Bridge    = "0x3154cf16ccdb4c6d922629664174b904d80f2c35"
	baseL2Messenger = "0x4200000000000000000000000000000000000007"
	baseL2Bridge    = "0x4200000000000000000000000000000000000010"

	ethereumWETH = "0xc02aa39b223fe8d0a0e5c4f27ead9083c756cc2"
	baseWETH     = "0x4200000000000000000000000000000000000006"
)

type Lifecycle string

const (
	LifecycleInitiated  Lifecycle = "initiated"
	LifecycleRelayed    Lifecycle = "relayed"
	LifecycleFinalized  Lifecycle = "finalized"
	LifecycleFailed     Lifecycle = "failed"
	LifecycleUnresolved Lifecycle = "unresolved"
)

// Capabilities is declared per bridge protocol, rather than inferred from a
// network logo. Future non-EVM bridges receive their own adapter contract.
type Capabilities struct {
	Directions         []string
	Assets             []string
	MessageIDType      string
	FinalityMethod     string
	HistoricalCoverage string
	ProvenanceQuality  string
}

// Adapter is the explicit contract for one bridge protocol. It receives only
// normalized observed transfers and the EVM clients required by this OP Stack
// protocol. Other bridge families should implement a different adapter rather
// than force non-EVM data through this interface.
type Adapter interface {
	Protocol() string
	Capabilities() Capabilities
	Resolve(context.Context, string, []adapter.TransferItem) ([]Transition, error)
}

// Transition contains independently verifiable bridge records. It never
// represents ownership equivalence between the source and destination address.
type Transition struct {
	ID, Protocol, BridgeName                              string
	SourceNetwork, DestinationNetwork                     string
	Lifecycle                                             Lifecycle
	MessageID                                             string
	SourceTransferID, DestinationTransferID               string
	SourceTransactionHash, DestinationTransactionHash     string
	SourceLogReference, DestinationLogReference           string
	SourceBridgeAddress, DestinationBridgeAddress         string
	CanonicalSourceToken, CanonicalDestinationToken       string
	Recipient, AmountBaseUnits, Limitations               string
	Asset                                                 adapter.Asset
	SourceBlockNumber, DestinationBlockNumber             uint64
	SourceBlockHash, DestinationBlockHash                 string
	SourceTimestamp, DestinationTimestamp                 time.Time
	SourceConfirmed, DestinationConfirmed                 bool
	SourceAcquisitionHashes, DestinationAcquisitionHashes []string
	SourceAcquisitionIDs, DestinationAcquisitionIDs       []int64
}

type route struct {
	SourceNetwork, DestinationNetwork string
	SourceToken, DestinationToken     string
	Asset                             adapter.Asset
}

type endpoint struct {
	Network, Bridge, Messenger string
	Confirmations              uint64
}

// OPStackStandardBridge implements the Base Standard Bridge only. It follows
// the protocol message hash through SentMessage and Relayed/FailedRelayedMessage
// and validates the exact StandardBridge finalization call and registered route.
type OPStackStandardBridge struct {
	clients map[string]*adapter.EVMClient
	routes  []route
}

func NewBaseStandardBridge(clients map[string]*adapter.EVMClient) *OPStackStandardBridge {
	return &OPStackStandardBridge{
		clients: clients,
		routes: []route{
			{SourceNetwork: EthereumMainnet, DestinationNetwork: BaseMainnet, Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}},
			{SourceNetwork: BaseMainnet, DestinationNetwork: EthereumMainnet, Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}},
			{SourceNetwork: EthereumMainnet, DestinationNetwork: BaseMainnet, SourceToken: ethereumWETH, DestinationToken: baseWETH, Asset: adapter.Asset{Kind: "ERC20", ContractAddress: ethereumWETH, Symbol: "WETH", Decimals: 18}},
			{SourceNetwork: BaseMainnet, DestinationNetwork: EthereumMainnet, SourceToken: baseWETH, DestinationToken: ethereumWETH, Asset: adapter.Asset{Kind: "ERC20", ContractAddress: baseWETH, Symbol: "WETH", Decimals: 18}},
		},
	}
}

func (a *OPStackStandardBridge) Protocol() string { return "base-standard-bridge" }

func (a *OPStackStandardBridge) Capabilities() Capabilities {
	return Capabilities{
		Directions:         []string{EthereumMainnet + "→" + BaseMainnet, BaseMainnet + "→" + EthereumMainnet},
		Assets:             []string{"native ETH", "WETH (canonical route registry)"},
		MessageIDType:      "OP Stack versioned cross-domain message hash (v1)",
		FinalityMethod:     "both source and destination chain heads meet the configured confirmation threshold",
		HistoricalCoverage: "candidate bridge transfers in the retrieved trace scope; destination is queried by exact message hash",
		ProvenanceQuality:  "exact EVM receipt, log-query, and block-header acquisition snapshots",
	}
}

func (a *OPStackStandardBridge) Resolve(ctx context.Context, sourceNetwork string, transfers []adapter.TransferItem) ([]Transition, error) {
	source, destination, ok := baseEndpoints(sourceNetwork)
	if !ok || a.clients[source.Network] == nil || a.clients[destination.Network] == nil {
		return []Transition{}, nil
	}
	candidates := make([]adapter.TransferItem, 0)
	seen := make(map[string]struct{})
	for _, transfer := range transfers {
		if !sameAddress(transfer.To, source.Bridge) || transfer.Hash == "" {
			continue
		}
		if _, exists := seen[transfer.Hash]; exists {
			continue
		}
		seen[transfer.Hash] = struct{}{}
		candidates = append(candidates, transfer)
	}
	result := make([]Transition, 0)
	for _, candidate := range candidates {
		transitions, err := a.resolveCandidate(ctx, source, destination, candidate)
		if err != nil {
			return nil, err
		}
		result = append(result, transitions...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func baseEndpoints(network string) (endpoint, endpoint, bool) {
	switch network {
	case EthereumMainnet:
		return endpoint{Network: EthereumMainnet, Bridge: baseL1Bridge, Messenger: baseL1Messenger, Confirmations: 64}, endpoint{Network: BaseMainnet, Bridge: baseL2Bridge, Messenger: baseL2Messenger, Confirmations: 120}, true
	case BaseMainnet:
		return endpoint{Network: BaseMainnet, Bridge: baseL2Bridge, Messenger: baseL2Messenger, Confirmations: 120}, endpoint{Network: EthereumMainnet, Bridge: baseL1Bridge, Messenger: baseL1Messenger, Confirmations: 64}, true
	default:
		return endpoint{}, endpoint{}, false
	}
}

func (a *OPStackStandardBridge) resolveCandidate(ctx context.Context, source, destination endpoint, candidate adapter.TransferItem) ([]Transition, error) {
	sourceClient := a.clients[source.Network]
	destinationClient := a.clients[destination.Network]
	before := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	logs, err := sourceClient.GetTransactionReceiptLogs(ctx, candidate.Hash)
	if err != nil {
		return []Transition{unresolvedTransition(source, destination, candidate, "Source bridge receipt is unavailable; no cross-chain continuation was inferred.", nil)}, nil
	}
	afterReceipt := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	sourceHashes := newHashes(before, afterReceipt)
	initiations := decodeInitiations(logs, source.Bridge)
	if len(initiations) == 0 {
		return []Transition{}, nil
	}
	messages := decodeSentMessages(logs, source.Messenger)
	for i := range messages {
		messages[i].Value = sentMessageValue(logs, source.Messenger, messages[i])
	}
	result := make([]Transition, 0, len(initiations))
	for _, initiated := range initiations {
		sent, found := matchingMessage(messages, initiated, source, destination)
		if !found {
			result = append(result, unresolvedTransition(source, destination, candidate, "A StandardBridge initiation was observed, but no matching version-1 cross-domain message record was present in the same source receipt.", sourceHashes))
			continue
		}
		route, found := a.canonicalRoute(source.Network, destination.Network, initiated)
		if !found {
			result = append(result, unresolvedTransition(source, destination, candidate, "The bridge message is protocol-shaped, but its token route is not in OpenChain's canonical Base Standard Bridge route registry.", sourceHashes))
			continue
		}
		messageID, err := sent.messageID()
		if err != nil {
			result = append(result, unresolvedTransition(source, destination, candidate, "The bridge message uses an unsupported cross-domain message version; no continuation was inferred.", sourceHashes))
			continue
		}
		transition := Transition{
			Protocol: a.Protocol(), BridgeName: "Base Standard Bridge", SourceNetwork: source.Network, DestinationNetwork: destination.Network,
			Lifecycle: LifecycleInitiated, MessageID: messageID, SourceTransactionHash: strings.ToLower(candidate.Hash), SourceLogReference: logReference(initiated.log),
			SourceBridgeAddress: source.Bridge, DestinationBridgeAddress: destination.Bridge, CanonicalSourceToken: route.SourceToken, CanonicalDestinationToken: route.DestinationToken,
			Recipient: initiated.To, AmountBaseUnits: initiated.Amount.String(), Asset: route.Asset, SourceBlockNumber: logBlock(initiated.log), SourceBlockHash: initiated.log.BlockHash,
			SourceTimestamp: candidate.Timestamp.UTC(), Limitations: limitation(), SourceAcquisitionHashes: sourceHashes,
		}
		if err := a.completeDestination(ctx, &transition, initiated, source, destination, destinationClient); err != nil {
			return nil, err
		}
		transition.ID = transitionID(transition)
		result = append(result, transition)
	}
	return result, nil
}

func (a *OPStackStandardBridge) canonicalRoute(sourceNetwork, destinationNetwork string, initiation bridgeEvent) (route, bool) {
	for _, route := range a.routes {
		if route.SourceNetwork != sourceNetwork || route.DestinationNetwork != destinationNetwork || route.SourceToken != initiation.LocalToken || route.DestinationToken != initiation.RemoteToken {
			continue
		}
		return route, true
	}
	return route{}, false
}

func (a *OPStackStandardBridge) completeDestination(ctx context.Context, transition *Transition, initiation bridgeEvent, source, destination endpoint, client *adapter.EVMClient) error {
	before := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	successes, successErr := client.GetLogs(ctx, adapter.LogFilter{Address: destination.Messenger, Topics: []interface{}{relayedMessageTopic, transition.MessageID}})
	failures, failureErr := client.GetLogs(ctx, adapter.LogFilter{Address: destination.Messenger, Topics: []interface{}{failedRelayedMessageTopic, transition.MessageID}})
	afterLogs := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	transition.DestinationAcquisitionHashes = newHashes(before, afterLogs)
	if successErr != nil || failureErr != nil {
		transition.Lifecycle = LifecycleUnresolved
		transition.Limitations = "The source bridge message is exact, but the destination messenger could not be queried by message identifier. It remains unresolved evidence."
		return nil
	}
	if len(successes) == 0 && len(failures) == 0 {
		return a.applyConfirmations(ctx, transition, source, destination)
	}
	if len(successes) == 0 {
		failed := failures[0]
		transition.Lifecycle = LifecycleFailed
		transition.DestinationTransactionHash = strings.ToLower(failed.TransactionHash)
		transition.DestinationLogReference = logReference(failed)
		transition.DestinationBlockNumber = logBlock(failed)
		transition.DestinationBlockHash = failed.BlockHash
		transition.DestinationTimestamp, _ = client.GetBlockTimestamp(ctx, transition.DestinationBlockNumber)
		transition.DestinationAcquisitionHashes = newHashes(before, adapter.AcquisitionResponseHashes(acquisitions(ctx)))
		transition.Limitations = "A destination FailedRelayedMessage was observed for this exact message identifier. A later relay may still succeed; this does not establish address ownership or intent."
		return a.applyConfirmations(ctx, transition, source, destination)
	}
	relayed := successes[0]
	transition.Lifecycle = LifecycleRelayed
	transition.DestinationTransactionHash = strings.ToLower(relayed.TransactionHash)
	transition.DestinationLogReference = logReference(relayed)
	transition.DestinationBlockNumber = logBlock(relayed)
	transition.DestinationBlockHash = relayed.BlockHash
	transition.DestinationTimestamp, _ = client.GetBlockTimestamp(ctx, transition.DestinationBlockNumber)
	receipt, err := client.GetTransactionReceiptLogs(ctx, relayed.TransactionHash)
	transition.DestinationAcquisitionHashes = newHashes(before, adapter.AcquisitionResponseHashes(acquisitions(ctx)))
	if err != nil {
		transition.Lifecycle = LifecycleUnresolved
		transition.Limitations = "A destination relay event was observed, but its receipt could not be retrieved to validate the StandardBridge finalization."
		return nil
	}
	if !hasMatchingFinalization(receipt, destination.Bridge, initiation, source, destination) {
		transition.Lifecycle = LifecycleUnresolved
		transition.Limitations = "A relay event was observed, but no matching canonical StandardBridge finalization was found in that destination receipt."
		return nil
	}
	if err := a.applyConfirmations(ctx, transition, source, destination); err != nil {
		return err
	}
	if transition.SourceConfirmed && transition.DestinationConfirmed {
		transition.Lifecycle = LifecycleFinalized
	}
	return nil
}

func (a *OPStackStandardBridge) applyConfirmations(ctx context.Context, transition *Transition, source, destination endpoint) error {
	beforeSource := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	sourceHead, sourceErr := a.clients[source.Network].GetLatestBlockNumber(ctx)
	afterSource := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	transition.SourceAcquisitionHashes = appendUnique(transition.SourceAcquisitionHashes, newHashes(beforeSource, afterSource)...)
	beforeDestination := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	destinationHead, destinationErr := a.clients[destination.Network].GetLatestBlockNumber(ctx)
	afterDestination := adapter.AcquisitionResponseHashes(acquisitions(ctx))
	transition.DestinationAcquisitionHashes = appendUnique(transition.DestinationAcquisitionHashes, newHashes(beforeDestination, afterDestination)...)
	if sourceErr != nil || destinationErr != nil {
		transition.Limitations = "Bridge records are exact, but one or both current chain heights are unavailable; the lifecycle is not confirmation-complete."
		return nil
	}
	transition.SourceConfirmed = confirmed(sourceHead, transition.SourceBlockNumber, source.Confirmations)
	transition.DestinationConfirmed = transition.DestinationBlockNumber > 0 && confirmed(destinationHead, transition.DestinationBlockNumber, destination.Confirmations)
	return nil
}

func confirmed(head, block, threshold uint64) bool {
	return block > 0 && head >= block && head-block >= threshold
}

func unresolvedTransition(source, destination endpoint, candidate adapter.TransferItem, reason string, hashes []string) Transition {
	identifier := sha256.Sum256([]byte(strings.ToLower(candidate.Hash) + "|" + reason))
	return Transition{ID: "base-standard-bridge:unresolved:" + hex.EncodeToString(identifier[:16]), Protocol: "base-standard-bridge", BridgeName: "Base Standard Bridge", SourceNetwork: source.Network, DestinationNetwork: destination.Network, Lifecycle: LifecycleUnresolved, SourceTransactionHash: strings.ToLower(candidate.Hash), SourceBridgeAddress: source.Bridge, DestinationBridgeAddress: destination.Bridge, SourceTimestamp: candidate.Timestamp.UTC(), Limitations: reason, SourceAcquisitionHashes: hashes}
}

func limitation() string {
	return "This continuation is limited to the exact Base Standard Bridge message, canonical token route, and retrieved chain records. It does not infer cross-chain address ownership, control, or intent."
}

func transitionID(value Transition) string {
	material := strings.Join([]string{value.Protocol, value.MessageID, string(value.Lifecycle), value.SourceTransactionHash, value.DestinationTransactionHash, value.DestinationLogReference}, "|")
	hash := sha256.Sum256([]byte(material))
	return value.Protocol + ":" + hex.EncodeToString(hash[:16])
}

func acquisitions(ctx context.Context) []adapter.RawAcquisition {
	// A nil recorder is valid in isolated adapter tests. The bridge can still
	// decode facts, but persistence will report no raw acquisition links.
	return adapter.AcquisitionItems(ctx)
}

func newHashes(before, after []string) []string {
	start := len(before)
	if start > len(after) {
		start = 0
	}
	return appendUnique(nil, after[start:]...)
}

func appendUnique(values []string, more ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(more))
	result := make([]string, 0, len(values)+len(more))
	for _, value := range append(values, more...) {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func sameAddress(left, right string) bool { return strings.EqualFold(left, right) }

func logBlock(log adapter.LogItem) uint64 {
	value, ok := new(big.Int).SetString(strings.TrimPrefix(log.BlockNumber, "0x"), 16)
	if !ok || !value.IsUint64() {
		return 0
	}
	return value.Uint64()
}

func logReference(log adapter.LogItem) string {
	if log.TransactionHash == "" {
		return ""
	}
	return strings.ToLower(log.TransactionHash) + ":log:" + strings.ToLower(strings.TrimPrefix(log.LogIndex, "0x"))
}

func keccak(value string) string {
	hash := sha3.NewLegacyKeccak256()
	_, _ = hash.Write([]byte(value))
	return "0x" + hex.EncodeToString(hash.Sum(nil))
}

var (
	ethBridgeInitiatedTopic   = keccak("ETHBridgeInitiated(address,address,uint256,bytes)")
	erc20BridgeInitiatedTopic = keccak("ERC20BridgeInitiated(address,address,address,address,uint256,bytes)")
	ethBridgeFinalizedTopic   = keccak("ETHBridgeFinalized(address,address,uint256,bytes)")
	erc20BridgeFinalizedTopic = keccak("ERC20BridgeFinalized(address,address,address,address,uint256,bytes)")
	sentMessageTopic          = keccak("SentMessage(address,address,bytes,uint256,uint256)")
	sentMessageExtensionTopic = keccak("SentMessageExtension1(address,uint256)")
	relayedMessageTopic       = keccak("RelayedMessage(bytes32)")
	failedRelayedMessageTopic = keccak("FailedRelayedMessage(bytes32)")
	finalizeETHSelector       = selector("finalizeBridgeETH(address,address,uint256,bytes)")
	finalizeERC20Selector     = selector("finalizeBridgeERC20(address,address,address,address,uint256,bytes)")
)

func selector(signature string) []byte {
	decoded, _ := hex.DecodeString(strings.TrimPrefix(keccak(signature), "0x"))
	return decoded[:4]
}

type bridgeEvent struct {
	log                               adapter.LogItem
	IsERC20                           bool
	LocalToken, RemoteToken, From, To string
	Amount                            *big.Int
}

func decodeInitiations(logs []adapter.LogItem, bridgeAddress string) []bridgeEvent {
	result := make([]bridgeEvent, 0)
	for _, log := range logs {
		if !sameAddress(log.Address, bridgeAddress) || len(log.Topics) == 0 {
			continue
		}
		switch strings.ToLower(log.Topics[0]) {
		case ethBridgeInitiatedTopic:
			if event, ok := decodeETHBridge(log); ok {
				result = append(result, event)
			}
		case erc20BridgeInitiatedTopic:
			if event, ok := decodeERC20Bridge(log); ok {
				result = append(result, event)
			}
		}
	}
	return result
}

func hasMatchingFinalization(logs []adapter.LogItem, bridgeAddress string, expected bridgeEvent, source, destination endpoint) bool {
	for _, log := range logs {
		if !sameAddress(log.Address, bridgeAddress) || len(log.Topics) == 0 {
			continue
		}
		var event bridgeEvent
		var ok bool
		if expected.IsERC20 && strings.EqualFold(log.Topics[0], erc20BridgeFinalizedTopic) {
			event, ok = decodeERC20Bridge(log)
		}
		if !expected.IsERC20 && strings.EqualFold(log.Topics[0], ethBridgeFinalizedTopic) {
			event, ok = decodeETHBridge(log)
		}
		if !ok || event.IsERC20 != expected.IsERC20 || !sameAddress(event.From, expected.From) || !sameAddress(event.To, expected.To) || event.Amount.Cmp(expected.Amount) != 0 {
			continue
		}
		if event.IsERC20 && (!sameAddress(event.LocalToken, expected.RemoteToken) || !sameAddress(event.RemoteToken, expected.LocalToken)) {
			continue
		}
		return true
	}
	return false
}

func decodeETHBridge(log adapter.LogItem) (bridgeEvent, bool) {
	if len(log.Topics) != 3 {
		return bridgeEvent{}, false
	}
	words, ok := abiWords(log.Data)
	if !ok || len(words) < 2 {
		return bridgeEvent{}, false
	}
	amount := wordInt(words[0])
	if amount == nil {
		return bridgeEvent{}, false
	}
	return bridgeEvent{log: log, From: wordAddress(topicWord(log.Topics[1])), To: wordAddress(topicWord(log.Topics[2])), Amount: amount}, true
}

func decodeERC20Bridge(log adapter.LogItem) (bridgeEvent, bool) {
	if len(log.Topics) != 4 {
		return bridgeEvent{}, false
	}
	words, ok := abiWords(log.Data)
	if !ok || len(words) < 3 {
		return bridgeEvent{}, false
	}
	amount := wordInt(words[1])
	if amount == nil {
		return bridgeEvent{}, false
	}
	return bridgeEvent{log: log, IsERC20: true, LocalToken: wordAddress(topicWord(log.Topics[1])), RemoteToken: wordAddress(topicWord(log.Topics[2])), From: wordAddress(topicWord(log.Topics[3])), To: wordAddress(words[0]), Amount: amount}, true
}

type sentMessage struct {
	log                        adapter.LogItem
	Target, Sender             string
	Message, Nonce, Gas, Value []byte
}

func decodeSentMessages(logs []adapter.LogItem, messenger string) []sentMessage {
	result := make([]sentMessage, 0)
	for _, log := range logs {
		if !sameAddress(log.Address, messenger) || len(log.Topics) != 2 || !strings.EqualFold(log.Topics[0], sentMessageTopic) {
			continue
		}
		words, ok := abiWords(log.Data)
		if !ok || len(words) < 4 {
			continue
		}
		message, ok := dynamicBytes(log.Data, words[1])
		if !ok {
			continue
		}
		result = append(result, sentMessage{log: log, Target: wordAddress(topicWord(log.Topics[1])), Sender: wordAddress(words[0]), Message: message, Nonce: words[2], Gas: words[3]})
	}
	return result
}

func sentMessageValue(logs []adapter.LogItem, messenger string, message sentMessage) []byte {
	for _, log := range logs {
		if !sameAddress(log.Address, messenger) || len(log.Topics) != 2 || !strings.EqualFold(log.Topics[0], sentMessageExtensionTopic) || !sameAddress(wordAddress(topicWord(log.Topics[1])), message.Sender) {
			continue
		}
		words, ok := abiWords(log.Data)
		if ok && len(words) == 1 {
			return words[0]
		}
	}
	return nil
}

func matchingMessage(messages []sentMessage, initiated bridgeEvent, source, destination endpoint) (sentMessage, bool) {
	for _, message := range messages {
		if !sameAddress(message.Sender, source.Bridge) || !sameAddress(message.Target, destination.Bridge) || len(message.Value) != 32 {
			continue
		}
		if bridgeFinalizationMatches(message.Message, initiated, source, destination) {
			return message, true
		}
	}
	return sentMessage{}, false
}

func bridgeFinalizationMatches(data []byte, initiated bridgeEvent, source, destination endpoint) bool {
	if len(data) < 4 {
		return false
	}
	words, ok := abiWords("0x" + hex.EncodeToString(data[4:]))
	if !ok {
		return false
	}
	if !initiated.IsERC20 && string(data[:4]) == string(finalizeETHSelector) {
		return len(words) >= 4 && sameAddress(wordAddress(words[0]), initiated.From) && sameAddress(wordAddress(words[1]), initiated.To) && wordInt(words[2]) != nil && wordInt(words[2]).Cmp(initiated.Amount) == 0
	}
	if initiated.IsERC20 && string(data[:4]) == string(finalizeERC20Selector) {
		return len(words) >= 6 && sameAddress(wordAddress(words[0]), initiated.RemoteToken) && sameAddress(wordAddress(words[1]), initiated.LocalToken) && sameAddress(wordAddress(words[2]), initiated.From) && sameAddress(wordAddress(words[3]), initiated.To) && wordInt(words[4]) != nil && wordInt(words[4]).Cmp(initiated.Amount) == 0
	}
	return false
}

func (message sentMessage) messageID() (string, error) {
	if len(message.Nonce) != 32 || len(message.Gas) != 32 || len(message.Value) != 32 {
		return "", fmt.Errorf("missing message fields")
	}
	// Version is stored in the high two bytes of the nonce. This adapter only
	// supports current v1 encoding; legacy v0 needs a separate deterministic codec.
	if message.Nonce[0] != 0 || message.Nonce[1] != 1 {
		return "", fmt.Errorf("unsupported message nonce version")
	}
	encoded := abiEncode(message.Nonce, addressWord(message.Sender), addressWord(message.Target), message.Value, message.Gas, dynamicWord(message.Message))
	hash := sha3.NewLegacyKeccak256()
	_, _ = hash.Write(encoded)
	return "0x" + hex.EncodeToString(hash.Sum(nil)), nil
}

func abiEncode(parts ...[]byte) []byte {
	result := make([]byte, 0)
	for _, part := range parts {
		result = append(result, part...)
	}
	return result
}

func dynamicWord(value []byte) []byte {
	// The five static words preceding this dynamic parameter plus its offset.
	offset := make([]byte, 32)
	offset[31] = 0xc0
	length := make([]byte, 32)
	new(big.Int).SetInt64(int64(len(value))).FillBytes(length)
	padded := append([]byte(nil), value...)
	if mod := len(padded) % 32; mod != 0 {
		padded = append(padded, make([]byte, 32-mod)...)
	}
	return append(append(offset, length...), padded...)
}

func addressWord(address string) []byte {
	value := make([]byte, 32)
	decoded, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(address), "0x"))
	if err == nil && len(decoded) == 20 {
		copy(value[12:], decoded)
	}
	return value
}

func abiWords(value string) ([][]byte, bool) {
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	if err != nil || len(decoded) == 0 || len(decoded)%32 != 0 || len(decoded) > 1<<20 {
		return nil, false
	}
	words := make([][]byte, 0, len(decoded)/32)
	for offset := 0; offset < len(decoded); offset += 32 {
		words = append(words, decoded[offset:offset+32])
	}
	return words, true
}

func dynamicBytes(data string, offsetWord []byte) ([]byte, bool) {
	decoded, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	if err != nil || len(offsetWord) != 32 {
		return nil, false
	}
	offset := new(big.Int).SetBytes(offsetWord)
	if !offset.IsInt64() || offset.Int64() < 0 || offset.Int64()+32 > int64(len(decoded)) {
		return nil, false
	}
	start := int(offset.Int64())
	length := new(big.Int).SetBytes(decoded[start : start+32])
	if !length.IsInt64() || length.Int64() < 0 || length.Int64() > 1<<20 || start+32+int(length.Int64()) > len(decoded) {
		return nil, false
	}
	return append([]byte(nil), decoded[start+32:start+32+int(length.Int64())]...), true
}

func topicWord(value string) []byte {
	decoded, _ := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	return decoded
}
func wordAddress(word []byte) string {
	if len(word) != 32 {
		return ""
	}
	return "0x" + hex.EncodeToString(word[12:])
}
func wordInt(word []byte) *big.Int {
	if len(word) != 32 {
		return nil
	}
	return new(big.Int).SetBytes(word)
}
