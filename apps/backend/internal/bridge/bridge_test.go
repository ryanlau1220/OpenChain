package bridge

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
)

func TestBaseBridgeDecoderMatchesOnlyExactMessageAndRoute(t *testing.T) {
	initiation := ethInitiatedLog("0xsource", "0x01", "0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "42")
	message := sentLog("0xsource", "0x02", baseL1Bridge, baseL2Bridge, finalETHData("0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "42"))
	initiated, ok := decodeETHBridge(initiation)
	if !ok {
		t.Fatal("expected ETH bridge initiation to decode")
	}
	messages := decodeSentMessages([]adapter.LogItem{message}, baseL1Messenger)
	if len(messages) != 1 {
		t.Fatalf("expected one sent message, got %d", len(messages))
	}
	messages[0].Value = word("0")
	if _, ok := matchingMessage(messages, initiated, endpoint{Bridge: baseL1Bridge}, endpoint{Bridge: baseL2Bridge}); !ok {
		t.Fatal("exact bridge finalization message should match")
	}
	nearMatch := sentLog("0xsource", "0x03", baseL1Bridge, baseL2Bridge, finalETHData("0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "43"))
	nearMessages := decodeSentMessages([]adapter.LogItem{nearMatch}, baseL1Messenger)
	nearMessages[0].Value = word("0")
	if _, ok := matchingMessage(nearMessages, initiated, endpoint{Bridge: baseL1Bridge}, endpoint{Bridge: baseL2Bridge}); ok {
		t.Fatal("equal timing and near amount must not match a bridge message")
	}
	if _, err := messages[0].messageID(); err != nil {
		t.Fatalf("expected v1 message ID: %v", err)
	}
}

func TestBaseBridgeControlledLifecycles(t *testing.T) {
	for _, fixture := range []struct {
		name string
		mode string
		want Lifecycle
	}{
		{"finalized", "finalized", LifecycleFinalized},
		{"pending", "pending", LifecycleInitiated},
		{"failed", "failed", LifecycleFailed},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			source, destination := baseEndpointsForTest()
			server := bridgeRPCFixture(t, fixture.mode, source, destination)
			defer server.Close()
			clients := map[string]*adapter.EVMClient{EthereumMainnet: adapter.NewEVMClient(server.URL), BaseMainnet: adapter.NewEVMClient(server.URL)}
			resolver := NewBaseStandardBridge(clients)
			ctx, recorder := adapter.WithAcquisitionRecorder(context.Background())
			transitions, err := resolver.Resolve(ctx, EthereumMainnet, []adapter.TransferItem{{Hash: "0xsource", To: baseL1Bridge, Timestamp: time.Unix(1000, 0).UTC()}})
			if err != nil {
				t.Fatalf("resolve bridge: %v", err)
			}
			if len(transitions) != 1 {
				t.Fatalf("expected one transition, got %d", len(transitions))
			}
			if transitions[0].Lifecycle != fixture.want {
				t.Fatalf("lifecycle = %s, want %s", transitions[0].Lifecycle, fixture.want)
			}
			if transitions[0].MessageID == "" {
				t.Fatal("expected deterministic message ID")
			}
			if fixture.want == LifecycleFinalized && (!transitions[0].SourceConfirmed || !transitions[0].DestinationConfirmed) {
				t.Fatal("finalized bridge requires both confirmation policies")
			}
			if len(recorder.Items()) == 0 || len(transitions[0].SourceAcquisitionHashes) == 0 {
				t.Fatal("bridge transition must retain raw acquisition provenance")
			}
		})
	}
}

func TestBaseBridgeDuplicateAmountDoesNotCreateSecondContinuation(t *testing.T) {
	source, destination := baseEndpointsForTest()
	server := bridgeRPCFixture(t, "duplicate", source, destination)
	defer server.Close()
	resolver := NewBaseStandardBridge(map[string]*adapter.EVMClient{EthereumMainnet: adapter.NewEVMClient(server.URL), BaseMainnet: adapter.NewEVMClient(server.URL)})
	transitions, err := resolver.Resolve(context.Background(), EthereumMainnet, []adapter.TransferItem{{Hash: "0xsource", To: baseL1Bridge, Timestamp: time.Unix(1000, 0).UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	var finalized, unresolved int
	for _, transition := range transitions {
		if transition.Lifecycle == LifecycleFinalized {
			finalized++
		}
		if transition.Lifecycle == LifecycleUnresolved {
			unresolved++
		}
	}
	if finalized != 1 || unresolved != 1 {
		t.Fatalf("duplicate amount fixture must produce one exact continuation and one unresolved initiation, got finalized=%d unresolved=%d", finalized, unresolved)
	}
}

func TestOptimismBridgeControlledLifecycles(t *testing.T) {
	for _, fixture := range []struct {
		name string
		mode string
		want Lifecycle
	}{
		{"finalized", "finalized", LifecycleFinalized},
		{"pending", "pending", LifecycleInitiated},
		{"failed", "failed", LifecycleFailed},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			source, destination := optimismEndpointsForTest()
			server := bridgeRPCFixture(t, fixture.mode, source, destination)
			defer server.Close()
			resolver := NewOptimismStandardBridge(map[string]*adapter.EVMClient{EthereumMainnet: adapter.NewEVMClient(server.URL), OptimismMainnet: adapter.NewEVMClient(server.URL)})
			transitions, err := resolver.Resolve(context.Background(), EthereumMainnet, []adapter.TransferItem{{Hash: "0xsource", To: source.Bridge, Timestamp: time.Unix(1000, 0).UTC()}})
			if err != nil || len(transitions) != 1 {
				t.Fatalf("resolve = %#v, %v", transitions, err)
			}
			transition := transitions[0]
			if transition.Protocol != "optimism-standard-bridge" || transition.SourceNetwork != EthereumMainnet || transition.DestinationNetwork != OptimismMainnet || transition.Lifecycle != fixture.want {
				t.Fatalf("transition = %#v", transition)
			}
			if fixture.want == LifecycleFinalized && (!transition.SourceConfirmed || !transition.DestinationConfirmed) {
				t.Fatal("finalized Optimism bridge requires both confirmation policies")
			}
		})
	}
}

func TestOptimismBridgeRejectsBaseMessengerNearMatch(t *testing.T) {
	source, destination := optimismEndpointsForTest()
	initiation, ok := decodeETHBridge(ethInitiatedLogAt(source.Bridge, "0xsource", "0x01", "0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "42"))
	if !ok {
		t.Fatal("expected Optimism bridge initiation")
	}
	message := sentLogAt(baseL1Messenger, "0xsource", "0x02", source.Bridge, destination.Bridge, finalETHData(initiation.From, initiation.To, "42"))
	messages := decodeSentMessages([]adapter.LogItem{message}, source.Messenger)
	if len(messages) != 0 {
		t.Fatal("a Base messenger event must not match the Optimism Standard Bridge")
	}
}

func baseEndpointsForTest() (endpoint, endpoint) {
	return endpoint{Network: EthereumMainnet, Bridge: baseL1Bridge, Messenger: baseL1Messenger, Confirmations: 64}, endpoint{Network: BaseMainnet, Bridge: baseL2Bridge, Messenger: baseL2Messenger, Confirmations: 120}
}

func optimismEndpointsForTest() (endpoint, endpoint) {
	return endpoint{Network: EthereumMainnet, Bridge: optimismL1Bridge, Messenger: optimismL1Messenger, Confirmations: 64}, endpoint{Network: OptimismMainnet, Bridge: optimismL2Bridge, Messenger: optimismL2Messenger, Confirmations: 120}
}

func bridgeRPCFixture(t *testing.T, mode string, source, destination endpoint) *httptest.Server {
	t.Helper()
	initiation := ethInitiatedLogAt(source.Bridge, "0xsource", "0x01", "0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "42")
	message := sentLogAt(source.Messenger, "0xsource", "0x02", source.Bridge, destination.Bridge, finalETHData("0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "42"))
	extension := adapter.LogItem{Address: source.Messenger, Topics: []string{sentMessageExtensionTopic, topicAddress(source.Bridge)}, Data: "0x" + hex.EncodeToString(word("0")), BlockNumber: "0x1", BlockHash: "0xsourceblock", TransactionHash: "0xsource", LogIndex: "0x03"}
	messageID, err := sentMessage{log: message, Target: destination.Bridge, Sender: source.Bridge, Message: finalETHData("0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "42"), Nonce: nonceV1(), Gas: word("100000"), Value: word("0")}.messageID()
	if err != nil {
		t.Fatal(err)
	}
	relayed := adapter.LogItem{Address: destination.Messenger, Topics: []string{relayedMessageTopic, messageID}, Data: "0x", BlockNumber: "0x2", BlockHash: "0xdestinationblock", TransactionHash: "0xdestination", LogIndex: "0x04"}
	failed := adapter.LogItem{Address: destination.Messenger, Topics: []string{failedRelayedMessageTopic, messageID}, Data: "0x", BlockNumber: "0x2", BlockHash: "0xdestinationblock", TransactionHash: "0xdestination", LogIndex: "0x04"}
	finalized := ethFinalizedLogAt(destination.Bridge, "0xdestination", "0x05", "0xaaa0000000000000000000000000000000000001", "0xbbb0000000000000000000000000000000000002", "42")
	if mode == "duplicate" {
		initiation2 := ethInitiatedLogAt(source.Bridge, "0xsource", "0x06", "0xaaa0000000000000000000000000000000000001", "0xccc0000000000000000000000000000000000003", "42")
		return rpcServer([]adapter.LogItem{initiation, message, extension, initiation2}, relayed, failed, finalized, mode)
	}
	return rpcServer([]adapter.LogItem{initiation, message, extension}, relayed, failed, finalized, mode)
}

func rpcServer(sourceReceipt []adapter.LogItem, relayed, failed, destinationReceipt adapter.LogItem, mode string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var rpc adapter.RPCRequest
		if err := json.NewDecoder(request.Body).Decode(&rpc); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		var result any
		switch rpc.Method {
		case "eth_getTransactionReceipt":
			hash, _ := rpc.Params[0].(string)
			if hash == "0xsource" {
				result = map[string]any{"logs": sourceReceipt}
			} else {
				result = map[string]any{"logs": []adapter.LogItem{destinationReceipt}}
			}
		case "eth_getLogs":
			filter, _ := rpc.Params[0].(map[string]any)
			topics, _ := filter["topics"].([]any)
			topic, _ := topics[0].(string)
			if mode == "pending" {
				result = []adapter.LogItem{}
			} else if topic == relayedMessageTopic && mode != "failed" {
				result = []adapter.LogItem{relayed}
			} else if topic == failedRelayedMessageTopic && mode == "failed" {
				result = []adapter.LogItem{failed}
			} else {
				result = []adapter.LogItem{}
			}
		case "eth_blockNumber":
			result = "0x1000"
		case "eth_getBlockByNumber":
			result = map[string]string{"timestamp": "0x3e9"}
		default:
			result = nil
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
}

func ethInitiatedLog(tx, index, from, to, amount string) adapter.LogItem {
	return ethInitiatedLogAt(baseL1Bridge, tx, index, from, to, amount)
}

func ethInitiatedLogAt(bridge, tx, index, from, to, amount string) adapter.LogItem {
	return adapter.LogItem{Address: bridge, Topics: []string{ethBridgeInitiatedTopic, topicAddress(from), topicAddress(to)}, Data: "0x" + hex.EncodeToString(append(append(word(amount), word("64")...), append(word("0"), make([]byte, 0)...)...)), BlockNumber: "0x1", BlockHash: "0xsourceblock", TransactionHash: tx, LogIndex: index}
}

func ethFinalizedLog(tx, index, from, to, amount string) adapter.LogItem {
	return ethFinalizedLogAt(baseL2Bridge, tx, index, from, to, amount)
}

func ethFinalizedLogAt(bridge, tx, index, from, to, amount string) adapter.LogItem {
	return adapter.LogItem{Address: bridge, Topics: []string{ethBridgeFinalizedTopic, topicAddress(from), topicAddress(to)}, Data: "0x" + hex.EncodeToString(append(word(amount), append(word("64"), word("0")...)...)), BlockNumber: "0x2", BlockHash: "0xdestinationblock", TransactionHash: tx, LogIndex: index}
}

func sentLog(tx, index, sender, target string, message []byte) adapter.LogItem {
	return sentLogAt(baseL1Messenger, tx, index, sender, target, message)
}

func sentLogAt(messenger, tx, index, sender, target string, message []byte) adapter.LogItem {
	data := append(addressWord(sender), word("128")...)
	data = append(data, nonceV1()...)
	data = append(data, word("100000")...)
	data = append(data, word(big.NewInt(int64(len(message))).String())...)
	padded := append([]byte(nil), message...)
	if remainder := len(padded) % 32; remainder != 0 {
		padded = append(padded, make([]byte, 32-remainder)...)
	}
	data = append(data, padded...)
	return adapter.LogItem{Address: messenger, Topics: []string{sentMessageTopic, topicAddress(target)}, Data: "0x" + hex.EncodeToString(data), BlockNumber: "0x1", BlockHash: "0xsourceblock", TransactionHash: tx, LogIndex: index}
}

func finalETHData(from, to, amount string) []byte {
	data := append([]byte(nil), finalizeETHSelector...)
	body := append(addressWord(from), addressWord(to)...)
	body = append(body, word(amount)...)
	body = append(body, word("128")...)
	body = append(body, word("0")...)
	return append(data, body...)
}

func word(value string) []byte {
	result := make([]byte, 32)
	parsed, _ := new(big.Int).SetString(value, 10)
	if parsed != nil {
		parsed.FillBytes(result)
	}
	return result
}
func nonceV1() []byte { value := make([]byte, 32); value[1] = 1; return value }
func topicAddress(value string) string {
	return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(strings.ToLower(value), "0x")
}
