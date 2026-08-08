package api

import (
	"testing"
	"time"
)

func TestMemoryPubSubRoomIsolation(t *testing.T) {
	pubsub := NewMemoryPubSub()

	// Subscribe Client A to Case 1
	ch1, unsub1 := pubsub.Subscribe("case-1")
	defer unsub1()

	// Subscribe Client B to Case 2
	ch2, unsub2 := pubsub.Subscribe("case-2")
	defer unsub2()

	// Publish event to Case 1
	pubsub.Publish(StreamEvent{
		CaseID:    "case-1",
		EventType: "graph_update",
		Data:      "node-data",
	})

	select {
	case evt := <-ch1:
		if evt.CaseID != "case-1" {
			t.Errorf("expected event for case-1, got %s", evt.CaseID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("timeout waiting for case-1 event")
	}

	// Verify Client B (case-2) received NO cross-case data leakage
	select {
	case evt := <-ch2:
		t.Errorf("unexpected cross-case data leakage received by case-2: %v", evt)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event received
	}
}
