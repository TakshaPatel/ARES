package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ares-sim/ares/internal/scenario"
	"github.com/ares-sim/ares/internal/simulation"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	sc := &simulation.Scenario{
		ID: "t1", Name: "T1",
		Nodes: []*simulation.InfrastructureNode{
			{ID: "h", Type: simulation.NodeHospital, Lat: 40, Lng: -74},
			{ID: "e", Type: simulation.NodeEOC, Lat: 41, Lng: -75, Dependencies: []string{"h"}},
		},
		Connections: []*simulation.Connection{
			{ID: "c1", From: "h", To: "e", Type: simulation.EdgeRoad, Capacity: 10, Distance: 1},
		},
	}
	if err := scenario.Validate(sc); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	l := scenario.NewLoader()
	if err := l.Add(sc, "test"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	s := NewServer(l)
	s.SetEngine(simulation.NewEngine(sc), sc.ID)
	return s
}

func getJSON(t *testing.T, s *Server, path string) map[string]any {
	t.Helper()
	mux := http.NewServeMux()
	rec := httptest.NewRecorder()
	s.Routes(mux)
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return out
}

func TestPresetsNeverNull(t *testing.T) {
	s := newTestServer(t)
	body := getJSON(t, s, "/api/simulation/presets")
	v, present := body["presets"]
	if !present {
		t.Fatal("presets key missing")
	}
	if v == nil {
		t.Fatal("presets must be [] not null, the UI map()s it")
	}
	if _, ok := v.([]any); !ok {
		t.Fatalf("presets is %T, want []any", v)
	}
}

func TestStateArraysNeverNull(t *testing.T) {
	s := newTestServer(t)
	body := getJSON(t, s, "/api/simulation/state")
	for _, k := range []string{"isolatedFacilities", "activeAlerts", "activeEmsRoutes"} {
		if body[k] == nil {
			t.Errorf("%s must not be null", k)
		}
	}
	for _, k := range []string{"nodes", "connections"} {
		if body[k] == nil {
			t.Errorf("%s must not be null", k)
		}
	}
	d, ok := body["diagnostics"].(map[string]any)
	if !ok {
		t.Fatal("diagnostics missing")
	}
	for _, k := range []string{"events", "emsRoutes", "meshBackoff", "dependencyEdges", "blockedEdges"} {
		if d[k] == nil {
			t.Errorf("diagnostics.%s must not be null", k)
		}
	}
}

func TestInlineLoadUsesScenarioOwnID(t *testing.T) {
	s := newTestServer(t)
	inline := `{"id":"inline-probe","name":"Inline Probe",
	  "nodes":[{"id":"h","type":"HOSPITAL","lat":1,"lng":1},
	           {"id":"e","type":"EMERGENCY_OPS_CENTER","lat":2,"lng":2}],
	  "connections":[{"id":"c","from":"h","to":"e","type":"ROAD_SEGMENT","capacity":5,"distance":1}]}`
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	s.Routes(mux)
	body := strings.NewReader(`{"scenarioId":"","inline":` + mustJSON(t, inline) + `}`)
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/scenarios/load", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("inline load = %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["scenarioId"] != "inline-probe" {
		t.Fatalf("scenarioId = %v, want inline-probe", out["scenarioId"])
	}
}

func TestInlineLoadRejectsMissingEOC(t *testing.T) {
	s := newTestServer(t)
	inline := `{"id":"bad","name":"Bad",
	  "nodes":[{"id":"h","type":"HOSPITAL","lat":1,"lng":1}],
	  "connections":[]}`
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	s.Routes(mux)
	body := strings.NewReader(`{"inline":` + mustJSON(t, inline) + `}`)
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/scenarios/load", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
