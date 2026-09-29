package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/ares-sim/ares/internal/scenario"
	"github.com/ares-sim/ares/internal/simulation"
)

type Server struct {
	hub    *Hub
	loader *scenario.Loader

	mu      sync.RWMutex
	engine  *simulation.Engine
	current string
	execID  int64
}

func NewServer(loader *scenario.Loader) *Server {
	s := &Server{loader: loader, hub: NewHub()}
	s.bindDefault()
	return s
}

func (s *Server) Engine() *simulation.Engine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine
}

func (s *Server) SetEngine(e *simulation.Engine, id string) {
	s.mu.Lock()
	prev := s.engine
	s.engine = e
	s.current = id
	s.mu.Unlock()
	if prev != nil && prev != e {
		prev.Shutdown()
	}
	e.Subscribe(s.hub.Publish)
}

func (s *Server) bindDefault() {
	list := s.loader.List()
	for _, sum := range list {
		if sc, ok := s.loader.Get(sum.ID); ok {
			e := simulation.NewEngine(sc)
			s.mu.Lock()
			s.engine = e
			s.current = sc.ID
			s.mu.Unlock()
			e.Subscribe(s.hub.Publish)
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode: %v", err)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/scenarios", s.handleListScenarios)
	mux.HandleFunc("/api/scenarios/load", s.handleLoadScenario)
	mux.HandleFunc("/api/simulation/state", s.handleState)
	mux.HandleFunc("/api/simulation/reset", s.handleReset)
	mux.HandleFunc("/api/simulation/control", s.handleControl)
	mux.HandleFunc("/api/simulation/inject", s.handleInject)
	mux.HandleFunc("/api/simulation/presets", s.handlePresets)
	mux.HandleFunc("/api/simulation/apply-preset", s.handleApplyPreset)
	mux.HandleFunc("/api/simulation/restore", s.handleRestore)
	mux.HandleFunc("/api/simulation/executions", s.handleExecutions)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		s.hub.ServeWS(w, r, s)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	s.mu.RLock()
	cur := s.current
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok", "scenarioId": cur, "tick": e.Tick(), "running": e.Running(),
	})
}

func (s *Server) handleListScenarios(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list := s.loader.List()
	s.mu.RLock()
	cur := s.current
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"scenarios": list, "active": cur})
}

func (s *Server) handleLoadScenario(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ScenarioID string `json:"scenarioId"`
		Inline     string `json:"inline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Inline) != "" {
		if err := s.loader.LoadFromBytes([]byte(body.Inline), "inline"); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.ScenarioID == "" {
			body.ScenarioID = "inline"
		}
	}
	sc, ok := s.loader.Get(body.ScenarioID)
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown scenario "+body.ScenarioID)
		return
	}
	e := simulation.NewEngine(sc)
	id, _ := s.loader.StartExecution(sc.ID)
	s.SetEngine(e, sc.ID)
	s.mu.Lock()
	s.execID = id
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scenarioId": sc.ID, "name": sc.Name, "state": e.Snapshot(),
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	writeJSON(w, http.StatusOK, e.Snapshot())
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	e.Reset()
	writeJSON(w, http.StatusOK, e.Snapshot())
}

type controlRequest struct {
	Action string `json:"action"`
	Steps  int    `json:"steps"`
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req controlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	switch strings.ToUpper(req.Action) {
	case "START":
		e.Start()
	case "PAUSE":
		e.Pause()
	case "STEP":
		steps := req.Steps
		if steps <= 0 {
			steps = 1
		}
		if steps > 60 {
			steps = 60
		}
		for i := 0; i < steps; i++ {
			e.TickOnce()
		}
	default:
		writeErr(w, http.StatusBadRequest, "unknown action "+req.Action)
		return
	}
	writeJSON(w, http.StatusOK, e.Snapshot())
}

type injectRequest struct {
	Action       string `json:"action"`
	TargetID     string `json:"targetId"`
	TargetType   string `json:"targetType"`
	ConnectionID string `json:"connectionId"`
	RequestID    string `json:"requestId"`
}

func (s *Server) handleInject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req injectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	var (
		id  string
		err error
	)
	switch strings.ToUpper(req.Action) {
	case "INJECT_FAILURE":
		if req.TargetID == "" {
			writeErr(w, http.StatusBadRequest, "targetId is required")
			return
		}
		id, err = e.InjectNodeFailure(req.TargetID)
	case "INJECT_ROAD_BLOCK":
		cid := req.ConnectionID
		if cid == "" {
			cid = req.TargetID
		}
		if cid == "" {
			writeErr(w, http.StatusBadRequest, "connectionId is required")
			return
		}
		id, err = e.InjectRoadBlock(cid)
	case "RESTORE_NODE":
		if req.TargetID == "" {
			writeErr(w, http.StatusBadRequest, "targetId is required")
			return
		}
		id, err = e.RestoreNodeByID(req.TargetID)
	default:
		writeErr(w, http.StatusBadRequest, "unknown action "+req.Action)
		return
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "targetId": id, "requestId": req.RequestID, "state": e.Snapshot(),
	})
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"presets": e.Presets()})
}

func (s *Server) handleApplyPreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		PresetID string `json:"presetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	if err := e.ApplyPreset(req.PresetID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e.Snapshot())
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		TargetID string `json:"targetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	e := s.Engine()
	if e == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scenario bound")
		return
	}
	if _, err := e.RestoreNodeByID(req.TargetID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e.Snapshot())
}

func (s *Server) handleExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := r.URL.Query().Get("scenarioId")
	if id == "" {
		s.mu.RLock()
		id = s.current
		s.mu.RUnlock()
	}
	recs, err := s.loader.Executions(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"executions": recs})
}
