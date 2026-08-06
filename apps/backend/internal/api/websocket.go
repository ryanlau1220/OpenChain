package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

type WSMessage struct {
	Type      string      `json:"type"`      // ping, subscribe_address, alert
	Address   string      `json:"address"`   // target address
	Data      interface{} `json:"data"`      // payload
	Timestamp int64       `json:"timestamp"`
}

type Hub struct {
	mu          sync.RWMutex
	connections map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{
		connections: make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow frontend dev origins
	})
	if err != nil {
		log.Printf("WebSocket accept error: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "connection closed")

	h.mu.Lock()
	h.connections[conn] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.connections, conn)
		h.mu.Unlock()
	}()

	// Send initial connection welcome message
	welcome := WSMessage{
		Type:      "connected",
		Data:      "OpenChain Realtime WebSocket Connected",
		Timestamp: time.Now().Unix(),
	}
	wData, _ := json.Marshal(welcome)
	_ = conn.Write(r.Context(), websocket.MessageText, wData)

	for {
		_, msgData, err := conn.Read(r.Context())
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgData, &msg); err == nil {
			if msg.Type == "ping" {
				pong := WSMessage{Type: "pong", Timestamp: time.Now().Unix()}
				pData, _ := json.Marshal(pong)
				_ = conn.Write(r.Context(), websocket.MessageText, pData)
			}
		}
	}
}

func (h *Hub) BroadcastAlert(msg WSMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for conn := range h.connections {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = conn.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}
