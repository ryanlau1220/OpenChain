package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageStatusIsIncompleteWhenAPageHasMore(t *testing.T) {
	if PageStatus(SourceStatus{IsComplete: true}, true).IsComplete {
		t.Fatal("paged transfer history was marked complete")
	}
	if !PageStatus(SourceStatus{IsComplete: true}, false).IsComplete {
		t.Fatal("complete final page was changed")
	}
}

func TestWithEVMChainHeadRecordsTheConfirmationHeight(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"jsonrpc":"2.0","id":1,"result":"0x100"}`)
	}))
	defer rpc.Close()

	status := withEVMChainHead(context.Background(), SourceStatus{Source: "test"}, NewEVMClient(rpc.URL))
	if status.LatestChainBlock != 256 || status.Warning != "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestWithEVMChainHeadKeepsObservationsProvisionalWithoutAHeight(t *testing.T) {
	status := withEVMChainHead(context.Background(), SourceStatus{Source: "test"}, nil)
	if status.LatestChainBlock != 0 || status.Warning == "" {
		t.Fatalf("status = %#v", status)
	}
}
