package api

import (
	"sync"
)

// StreamEvent defines case-partitioned events for ConnectRPC server-streaming
type StreamEvent struct {
	CaseID    string      `json:"case_id"`
	EventType string      `json:"event_type"` // graph_update, label_added, risk_eval
	Data      interface{} `json:"data"`
}

// PubSubEngine abstracts event distribution for Redis readiness
type PubSubEngine interface {
	Subscribe(caseID string) (chan StreamEvent, func())
	Publish(event StreamEvent)
}

// MemoryPubSub implements local in-memory PubSubEngine
type MemoryPubSub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan StreamEvent]struct{}
}

// NewMemoryPubSub initializes an in-memory PubSub hub
func NewMemoryPubSub() *MemoryPubSub {
	return &MemoryPubSub{
		subscribers: make(map[string]map[chan StreamEvent]struct{}),
	}
}

// Subscribe returns a channel and unsubscribe cleanup callback for a specific case_id room
func (m *MemoryPubSub) Subscribe(caseID string) (chan StreamEvent, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan StreamEvent, 100)
	if _, exists := m.subscribers[caseID]; !exists {
		m.subscribers[caseID] = make(map[chan StreamEvent]struct{})
	}
	m.subscribers[caseID][ch] = struct{}{}

	unsubscribe := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if subMap, ok := m.subscribers[caseID]; ok {
			delete(subMap, ch)
			close(ch)
			if len(subMap) == 0 {
				delete(m.subscribers, caseID)
			}
		}
	}

	return ch, unsubscribe
}

// Publish broadcasts an event strictly to subscribers of the matching case_id room
func (m *MemoryPubSub) Publish(event StreamEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if subMap, ok := m.subscribers[event.CaseID]; ok {
		for ch := range subMap {
			select {
			case ch <- event:
			default:
			}
		}
	}
}
