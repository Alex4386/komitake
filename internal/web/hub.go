package web

import (
	"encoding/json"
	"sync"
)

// Hub fans out JSON events to connected WebSocket clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan []byte]struct{})}
}

// Subscribe registers a client buffer. The caller must Unsubscribe when done.
func (h *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Broadcast sends payload to every subscriber; slow clients are dropped from
// that message (non-blocking send).
func (h *Hub) Broadcast(payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (h *Hub) BroadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.Broadcast(b)
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
