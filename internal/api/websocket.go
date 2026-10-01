package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ares-sim/ares/internal/simulation"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 25 * time.Second
	maxMessageSize = 1 << 16
	sendBuffer     = 8
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type InboundMessage struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

type ActionPayload struct {
	TargetID     string   `json:"targetId"`
	TargetType   string   `json:"targetType"`
	ConnectionID string   `json:"connectionId"`
	Steps        int      `json:"steps"`
	PresetID     string   `json:"presetId"`
	PresetIDs    []string `json:"presetIds"`
	RequestID    string   `json:"requestId"`
}

type OutboundMessage struct {
	Type      string                      `json:"type"`
	RequestID string                      `json:"requestId,omitempty"`
	Payload   *simulation.SimulationState `json:"state,omitempty"`
	Error     string                      `json:"error,omitempty"`
}

type client struct {
	conn  *websocket.Conn
	send  chan []byte
	hub   *Hub
	close atomic.Bool
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
	latest  []byte
}

func NewHub() *Hub {
	return &Hub{clients: map[*client]struct{}{}}
}

func (h *Hub) Publish(state *simulation.SimulationState) {
	raw, err := json.Marshal(OutboundMessage{Type: "STATE", Payload: state})
	if err != nil {
		log.Printf("ws marshal: %v", err)
		return
	}
	h.mu.Lock()
	h.latest = raw
	targets := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		targets = append(targets, c)
	}
	h.mu.Unlock()
	for _, c := range targets {
		c.enqueue(raw)
	}
}

func (c *client) enqueue(raw []byte) {
	if c.close.Load() {
		return
	}
	select {
	case c.send <- raw:
	default:
		c.close.Store(true)
		_ = c.conn.Close()
	}
}

func (h *Hub) broadcastCtrl(kind, requestID, errMsg string) {
	raw, err := json.Marshal(OutboundMessage{Type: kind, RequestID: requestID, Error: errMsg})
	if err != nil {
		return
	}
	h.mu.RLock()
	targets := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		c.enqueue(raw)
	}
}

func (s *Server) currentEngine() *simulation.Engine { return s.Engine() }

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, srv *Server) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	c := &client{conn: conn, send: make(chan []byte, sendBuffer), hub: h}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	latest := h.latest
	h.mu.Unlock()

	go c.writePump()
	if len(latest) > 0 {
		c.enqueue(latest)
	} else if e := srv.currentEngine(); e != nil {
		h.Publish(e.Snapshot())
	}
	c.readPump(srv)
}

func (c *client) readPump(srv *Server) {
	defer func() {
		c.close.Store(true)
		_ = c.conn.Close()
		c.hub.mu.Lock()
		delete(c.hub.clients, c)
		c.hub.mu.Unlock()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read: %v", err)
			}
			return
		}
		var msg InboundMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.hub.broadcastCtrl("ERROR", "", "malformed json: "+err.Error())
			continue
		}
		pl := ActionPayload{}
		if len(msg.Payload) > 0 {
			_ = json.Unmarshal(msg.Payload, &pl)
		}
		c.dispatch(srv, msg.Action, pl)
	}
}

func (c *client) dispatch(srv *Server, action string, pl ActionPayload) {
	e := srv.currentEngine()
	if e == nil {
		c.hub.broadcastCtrl("ERROR", pl.RequestID, "no scenario bound")
		return
	}
	switch action {
	case "START":
		e.Start()
	case "PAUSE":
		e.Pause()
	case "STEP":
		steps := pl.Steps
		if steps <= 0 {
			steps = 1
		}
		if steps > 60 {
			steps = 60
		}
		for i := 0; i < steps; i++ {
			e.TickOnce()
		}
	case "INJECT_FAILURE":
		if _, err := e.InjectNodeFailure(pl.TargetID); err != nil {
			c.hub.broadcastCtrl("ERROR", pl.RequestID, err.Error())
			return
		}
	case "INJECT_ROAD_BLOCK":
		cid := pl.ConnectionID
		if cid == "" {
			cid = pl.TargetID
		}
		if _, err := e.InjectRoadBlock(cid); err != nil {
			c.hub.broadcastCtrl("ERROR", pl.RequestID, err.Error())
			return
		}
	case "RESTORE_NODE":
		if _, err := e.RestoreNodeByID(pl.TargetID); err != nil {
			c.hub.broadcastCtrl("ERROR", pl.RequestID, err.Error())
			return
		}
	case "APPLY_PRESET":
		if err := e.ApplyPreset(pl.PresetID); err != nil {
			c.hub.broadcastCtrl("ERROR", pl.RequestID, err.Error())
			return
		}
	case "APPLY_PRESETS":
		ids := pl.PresetIDs
		if len(ids) == 0 && pl.PresetID != "" {
			ids = []string{pl.PresetID}
		}
		if err := e.ApplyPresets(ids); err != nil {
			c.hub.broadcastCtrl("ERROR", pl.RequestID, err.Error())
			return
		}
	case "RESET":
		e.Reset()
	case "PING":
		raw, _ := json.Marshal(OutboundMessage{Type: "PONG", RequestID: pl.RequestID})
		c.enqueue(raw)
	default:
		c.hub.broadcastCtrl("ERROR", pl.RequestID, "unknown action "+action)
	}
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
