package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

const testAddress = "0x7a250d5630b4cf539739df2c5dacb4c659f2488d"

func setupTestServer() (http.Handler, *Server) {
	registry := labels.NewRegistry()
	engine := tracing.NewEngine(nil, nil, nil, registry)
	return NewServer(nil, registry, engine, nil, "http://localhost:3000").Handler(), NewServer(nil, registry, engine, nil, "http://localhost:3000")
}

func TestHealthAPI(t *testing.T) {
	handler, _ := setupTestServer()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "healthy" {
		t.Fatalf("status = %q", body["status"])
	}
}

func TestCORSOnlyAllowsConfiguredOrigin(t *testing.T) {
	handler, _ := setupTestServer()
	for _, origin := range []string{"http://localhost:3000", "https://untrusted.example"} {
		request := httptest.NewRequest(http.MethodOptions, "/openchain.v1.LookupService/LookupAddress", nil)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d", response.Code)
		}
		if origin == "http://localhost:3000" && response.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("configured origin not allowed")
		}
		if origin != "http://localhost:3000" && response.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin allowed")
		}
	}
}

func TestLookupRejectsInvalidAddress(t *testing.T) {
	_, server := setupTestServer()
	_, err := (&connectLookupHandler{server: server}).LookupAddress(context.Background(), connect.NewRequest(&pb.LookupAddressRequest{Address: "invalid"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v", connect.CodeOf(err))
	}
}

func TestTraceWithoutDataSourceIsUnavailable(t *testing.T) {
	_, server := setupTestServer()
	response, err := (&connectTracingHandler{server: server}).TraceGraph(context.Background(), connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: testAddress}))
	_ = response
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("code = %v", connect.CodeOf(err))
	}
}
