package webui

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/qsoa-cloud/qdevrunner/logstore"
	"github.com/qsoa-cloud/qdevrunner/metricsstore"
	"github.com/qsoa-cloud/qdevrunner/tracer"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Hub manages WebSocket connections and broadcasts data from stores.
type Hub struct {
	tracerStore  *tracer.Store
	logStore     *logstore.Store
	metricsStore *metricsstore.Store
	mu           sync.Mutex
	clients      map[*wsClient]bool
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

func NewHub(ts *tracer.Store, ls *logstore.Store, ms *metricsstore.Store) *Hub {
	return &Hub{
		tracerStore:  ts,
		logStore:     ls,
		metricsStore: ms,
		clients:      make(map[*wsClient]bool),
	}
}

// Run starts background broadcast goroutines.
func (h *Hub) Run() {
	// Trace subscriber.
	go func() {
		_, ch := h.tracerStore.Subscribe(256)
		for record := range ch {
			h.broadcast(WSMessage{Type: "span", Payload: record})
		}
	}()

	// Log subscriber.
	go func() {
		_, ch := h.logStore.Subscribe(512)
		for entry := range ch {
			h.broadcast(WSMessage{Type: "log", Payload: entry})
		}
	}()

	// Metrics subscriber.
	go func() {
		_, ch := h.metricsStore.Subscribe(64)
		for snapshot := range ch {
			h.broadcast(WSMessage{Type: "metrics", Payload: snapshot})
		}
	}()
}

// BroadcastServiceEvent sends a service status change to all clients.
func (h *Hub) BroadcastServiceEvent(event interface{}) {
	h.broadcast(WSMessage{Type: "service_event", Payload: event})
}

func (h *Hub) broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client too slow, drop.
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// ServeWS handles WebSocket upgrade requests.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 2048),
	}

	// Register client so live broadcasts start queuing.
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	// Writer goroutine.
	go func() {
		defer conn.Close()
		for msg := range client.send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	// Reader goroutine (handles client commands + keepalive).
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, client)
			h.mu.Unlock()
			close(client.send)
			conn.Close()
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			var cmd struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(msg, &cmd) == nil && cmd.Type == "backlog" {
				h.sendBacklog(client)
			}
		}
	}()
}

// sendBacklog queues recent logs, spans, and metrics into a client's send channel.
func (h *Hub) sendBacklog(client *wsClient) {
	send := func(msg WSMessage) {
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		select {
		case client.send <- data:
		default:
		}
	}

	for _, entry := range h.logStore.Recent(500, "") {
		send(WSMessage{Type: "log", Payload: entry})
	}
	for _, record := range h.tracerStore.RecentSpans(500) {
		send(WSMessage{Type: "span", Payload: record})
	}
	for _, snapshot := range h.metricsStore.Recent(100, "") {
		send(WSMessage{Type: "metrics", Payload: snapshot})
	}
}
