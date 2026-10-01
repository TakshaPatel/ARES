package scenario

import (
	"testing"

	"github.com/ares-sim/ares/internal/simulation"
)

const inlineFixture = `{
  "id": "probe-city",
  "name": "Probe City",
  "center": {"lat": 40.4, "lng": -74.5},
  "nodes": [
    {"id": "sub", "name": "Sub", "type": "POWER_SUBSTATION", "lat": 40.40, "lng": -74.50},
    {"id": "hosp", "name": "Hosp", "type": "HOSPITAL", "lat": 40.41, "lng": -74.51, "dependencies": ["sub"]},
    {"id": "eoc", "name": "EOC", "type": "EMERGENCY_OPS_CENTER", "lat": 40.42, "lng": -74.52, "dependencies": ["hosp"]}
  ],
  "connections": [
    {"id": "c1", "from": "sub", "to": "hosp", "type": "POWER_LINE", "capacity": 45, "distance": 1.0},
    {"id": "c2", "from": "hosp", "to": "eoc", "type": "ROAD_SEGMENT", "capacity": 60, "distance": 1.2}
  ]
}`

func TestLoadFromBytesReturnsRegisteredID(t *testing.T) {
	l := NewLoader()
	id, err := l.LoadFromBytes([]byte(inlineFixture), "inline")
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	if id != "probe-city" {
		t.Fatalf("id = %q, want %q", id, "probe-city")
	}
	sc, ok := l.Get(id)
	if !ok {
		t.Fatalf("scenario %q not retrievable by the returned id", id)
	}
	if len(sc.Nodes) != 3 || len(sc.Connections) != 2 {
		t.Fatalf("got %d nodes / %d connections, want 3/2", len(sc.Nodes), len(sc.Connections))
	}
}

func TestLoadFromBytesRejectsInvalid(t *testing.T) {
	l := NewLoader()
	if _, err := l.LoadFromBytes([]byte(`{not json`), "inline"); err == nil {
		t.Fatal("expected error for malformed json")
	}
	noEOC := `{"id":"x","name":"X","nodes":[{"id":"a","type":"HOSPITAL","lat":1,"lng":1}],
	  "connections":[{"id":"c","from":"a","to":"a","type":"ROAD_SEGMENT"}]}`
	if _, err := l.LoadFromBytes([]byte(noEOC), "inline"); err == nil {
		t.Fatal("expected validation error for scenario without an EOC")
	}
	if _, ok := l.Get("x"); ok {
		t.Fatal("rejected scenario must not be registered")
	}
}

func TestLoadFromBytesReplacesSameID(t *testing.T) {
	l := NewLoader()
	if _, err := l.LoadFromBytes([]byte(inlineFixture), "inline"); err != nil {
		t.Fatal(err)
	}
	smaller := `{"id":"probe-city","name":"Probe City v2",
	  "nodes":[{"id":"a","type":"HOSPITAL","lat":1,"lng":1},
	           {"id":"e","type":"EMERGENCY_OPS_CENTER","lat":2,"lng":2}],
	  "connections":[{"id":"c","from":"a","to":"e","type":"ROAD_SEGMENT"}]}`
	if _, err := l.LoadFromBytes([]byte(smaller), "inline"); err != nil {
		t.Fatal(err)
	}
	sc, _ := l.Get("probe-city")
	if sc.Name != "Probe City v2" {
		t.Fatalf("name = %q, want replaced value", sc.Name)
	}
	if len(l.List()) != 1 {
		t.Fatalf("expected 1 scenario after replacement, got %d", len(l.List()))
	}
	if sc.Nodes[0].Type != simulation.NodeHospital {
		t.Fatalf("unexpected node type %q", sc.Nodes[0].Type)
	}
}
