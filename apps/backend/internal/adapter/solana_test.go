package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func solanaTestKey(last byte, length int) string {
	value := make([]byte, length)
	value[len(value)-1] = last
	return encodeBase58(value)
}

func TestSolanaListsBoundedNativeTransfersWithoutSkippingInstructionEvents(t *testing.T) {
	address := solanaTestKey(1, 32)
	destination := solanaTestKey(2, 32)
	signature := solanaTestKey(3, 64)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		switch body.Method {
		case "getSignaturesForAddress":
			var options map[string]any
			_ = json.Unmarshal(body.Params[1], &options)
			if options["before"] != nil {
				_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","result":[]}`))
				return
			}
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","result":[{"signature":"` + signature + `","slot":9,"blockTime":100}]}`))
		case "getTransaction":
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","result":{"slot":9,"blockTime":100,"transaction":{"message":{"instructions":[{"program":"system","parsed":{"type":"transfer","info":{"source":"` + address + `","destination":"` + destination + `","lamports":42}}},{"program":"system","parsed":{"type":"transfer","info":{"source":"` + address + `","destination":"` + destination + `","lamports":7}}}]}}}}`))
		default:
			t.Fatalf("unexpected method %q", body.Method)
		}
	}))
	defer server.Close()

	adapter := NewSolanaAdapter("solana-mainnet", server.URL)
	first, err := adapter.ListTransfers(context.Background(), address, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Transfers) != 1 || first.Transfers[0].EventID != "instruction:0" || first.Transfers[0].Asset.Symbol != "SOL" || !first.HasMore {
		t.Fatalf("first page = %#v", first)
	}
	second, err := adapter.ListTransfers(context.Background(), address, 1, first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Transfers) != 1 || second.Transfers[0].EventID != "instruction:1" {
		t.Fatalf("second page = %#v", second)
	}
	if second.SourceStatus.IsComplete || second.SourceStatus.Warning == "" {
		t.Fatalf("source status = %#v", second.SourceStatus)
	}
}
