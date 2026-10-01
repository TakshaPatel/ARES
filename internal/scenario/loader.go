package scenario

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ares-sim/ares/internal/simulation"
)

//go:embed seed/*.json schema.sql
var seedFS embed.FS

type Summary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	HazardModel string  `json:"hazardModel"`
	Nodes       int     `json:"nodes"`
	Connections int     `json:"connections"`
	Facilities  int     `json:"facilities"`
	Source      string  `json:"source"`
	PowerHealth float64 `json:"powerHealth"`
	CommsCover  float64 `json:"commsCoverage"`
}

type Loader struct {
	mu        sync.RWMutex
	scenarios map[string]*simulation.Scenario
	order     []string
	db        *sql.DB
}

func NewLoader() *Loader {
	return &Loader{scenarios: map[string]*simulation.Scenario{}}
}

func (l *Loader) OpenDB(path string) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		return err
	}
	l.db = db
	schema, err := os.ReadFile("sqlite_schema.sql")
	if err != nil {
		schema, err = seedFS.ReadFile("schema.sql")
		if err != nil {
			return nil
		}
	}
	_, err = db.Exec(string(schema))
	return err
}

func (l *Loader) Close() error {
	if l.db != nil {
		return l.db.Close()
	}
	return nil
}

func (l *Loader) Add(sc *simulation.Scenario, source string) error {
	if sc == nil {
		return fmt.Errorf("nil scenario")
	}
	if sc.ID == "" {
		return fmt.Errorf("scenario id is required")
	}
	sc.Normalize()
	if err := Validate(sc); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.scenarios[sc.ID]; !exists {
		l.order = append(l.order, sc.ID)
	}
	l.scenarios[sc.ID] = sc
	return l.persist(sc, source)
}

func (l *Loader) Get(id string) (*simulation.Scenario, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	sc, ok := l.scenarios[id]
	return sc, ok
}

func (l *Loader) List() []Summary {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Summary, 0, len(l.order))
	for _, id := range l.order {
		sc := l.scenarios[id]
		if sc == nil {
			continue
		}
		s := Summary{
			ID: sc.ID, Name: sc.Name, Description: sc.Description,
			HazardModel: sc.HazardModel, Nodes: len(sc.Nodes), Connections: len(sc.Connections),
		}
		for _, n := range sc.Nodes {
			if n.Type.IsFacility() {
				s.Facilities++
			}
		}
		if s.Facilities == 0 {
			s.Facilities = 0
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (l *Loader) LoadEmbedded() error {
	entries, err := seedFS.ReadDir("seed")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := seedFS.ReadFile("seed/" + e.Name())
		if err != nil {
			return err
		}
		var sc simulation.Scenario
		if err := json.Unmarshal(raw, &sc); err != nil {
			return fmt.Errorf("seed %s: %w", e.Name(), err)
		}
		if err := l.Add(&sc, "embedded"); err != nil {
			return fmt.Errorf("seed %s: %w", e.Name(), err)
		}
	}
	return nil
}

func (l *Loader) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var sc simulation.Scenario
		if err := json.Unmarshal(raw, &sc); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if err := l.Add(&sc, "filesystem:"+dir); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

func (l *Loader) LoadFromBytes(raw []byte, source string) (string, error) {
	var sc simulation.Scenario
	if err := json.Unmarshal(raw, &sc); err != nil {
		return "", err
	}
	if err := l.Add(&sc, source); err != nil {
		return "", err
	}
	return sc.ID, nil
}

func (l *Loader) persist(sc *simulation.Scenario, source string) error {
	if l.db == nil {
		return nil
	}
	raw, err := json.Marshal(sc)
	if err != nil {
		return err
	}
	facilities := 0
	for _, n := range sc.Nodes {
		if n.Type.IsFacility() {
			facilities++
		}
	}
	_, err = l.db.Exec(`
		INSERT INTO scenarios (id, name, description, hazard_model, nodes, connections, facilities, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, description=excluded.description, hazard_model=excluded.hazard_model,
			nodes=excluded.nodes, connections=excluded.connections, facilities=excluded.facilities,
			payload=excluded.payload`,
		sc.ID, sc.Name, sc.Description, sc.HazardModel, len(sc.Nodes), len(sc.Connections), facilities, string(raw))
	return err
}

func (l *Loader) DB() *sql.DB { return l.db }

func Validate(sc *simulation.Scenario) error {
	if len(sc.Nodes) == 0 {
		return fmt.Errorf("scenario has no nodes")
	}
	if len(sc.Connections) == 0 {
		return fmt.Errorf("scenario has no connections")
	}
	ids := make(map[string]bool, len(sc.Nodes))
	coords := make(map[string][2]float64, len(sc.Nodes))
	for _, n := range sc.Nodes {
		if n.ID == "" {
			return fmt.Errorf("node with empty id")
		}
		if ids[n.ID] {
			return fmt.Errorf("duplicate node id %q", n.ID)
		}
		ids[n.ID] = true
		if n.Name == "" {
			n.Name = n.ID
		}
		if n.Lat < -90 || n.Lat > 90 || n.Lng < -180 || n.Lng > 180 {
			return fmt.Errorf("node %q has invalid coordinates %v,%v", n.ID, n.Lat, n.Lng)
		}
		if n.Health <= 0 {
			n.Health = 100
		}
		coords[n.ID] = [2]float64{n.Lat, n.Lng}
		switch n.Type {
		case simulation.NodePowerSubstation, simulation.NodeCellTower, simulation.NodeRadioMesh,
			simulation.NodeRoadIntersection, simulation.NodeHospital, simulation.NodeFireStation,
			simulation.NodeEOC:
		default:
			return fmt.Errorf("node %q has unknown type %q", n.ID, n.Type)
		}
	}
	for _, n := range sc.Nodes {
		for _, d := range n.Dependencies {
			if !ids[d] {
				return fmt.Errorf("node %q depends on unknown node %q", n.ID, d)
			}
		}
	}
	connIDs := make(map[string]bool, len(sc.Connections))
	for _, c := range sc.Connections {
		if c.ID == "" {
			return fmt.Errorf("connection with empty id")
		}
		if connIDs[c.ID] {
			return fmt.Errorf("duplicate connection id %q", c.ID)
		}
		connIDs[c.ID] = true
		if !ids[c.From] || !ids[c.To] {
			return fmt.Errorf("connection %q references unknown endpoint (%s -> %s)", c.ID, c.From, c.To)
		}
		if c.From == c.To {
			return fmt.Errorf("connection %q is a self loop", c.ID)
		}
		switch c.Type {
		case simulation.EdgePowerLine, simulation.EdgeMeshLink, simulation.EdgeRoad:
		default:
			return fmt.Errorf("connection %q has unknown type %q", c.ID, c.Type)
		}
		if c.Type == simulation.EdgeMeshLink && c.Latency <= 0 {
			return fmt.Errorf("mesh link %q must declare a positive latency", c.ID)
		}
	}
	var hasFacility bool
	var hasEOC bool
	for _, n := range sc.Nodes {
		if n.Type.IsFacility() {
			hasFacility = true
		}
		if n.Type == simulation.NodeEOC {
			hasEOC = true
		}
	}
	if !hasFacility {
		return fmt.Errorf("scenario defines no emergency facilities")
	}
	if !hasEOC {
		return fmt.Errorf("scenario defines no emergency operations center")
	}
	seenPreset := map[string]bool{}
	for _, p := range sc.Presets {
		if p.ID == "" || p.Label == "" {
			return fmt.Errorf("preset requires id and label")
		}
		if seenPreset[p.ID] {
			return fmt.Errorf("duplicate preset id %q", p.ID)
		}
		seenPreset[p.ID] = true
		switch p.Kind {
		case "HAZARD", "FLOOD_ZONE":
		case "NODE_FAILURE", "ROAD_BLOCK":
			for _, t := range strings.Split(p.TargetID, ",") {
				t = strings.TrimSpace(t)
				if t == "" {
					return fmt.Errorf("preset %q has an empty target", p.ID)
				}
				if p.Kind == "NODE_FAILURE" && !ids[t] {
					return fmt.Errorf("preset %q targets unknown node %q", p.ID, t)
				}
				if p.Kind == "ROAD_BLOCK" && !connIDs[t] {
					return fmt.Errorf("preset %q targets unknown connection %q", p.ID, t)
				}
			}
		default:
			return fmt.Errorf("preset %q has unknown kind %q", p.ID, p.Kind)
		}
	}
	return nil
}

type ExecutionRecord struct {
	ID         int64   `json:"id"`
	ScenarioID string  `json:"scenarioId"`
	FinalTick  int     `json:"finalTick"`
	PeakPower  float64 `json:"peakPowerLoss"`
	PeakComms  float64 `json:"peakCommsLoss"`
	Isolated   int     `json:"isolated"`
	StartedAt  string  `json:"startedAt"`
}

func (l *Loader) StartExecution(scenarioID string) (int64, error) {
	if l.db == nil {
		return 0, nil
	}
	res, err := l.db.Exec(`INSERT INTO executions (scenario_id) VALUES (?)`, scenarioID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (l *Loader) FinishExecution(id int64, sc *simulation.SimulationState) error {
	if l.db == nil || id == 0 {
		return nil
	}
	tx, err := l.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE executions SET ended_at=CURRENT_TIMESTAMP, final_tick=?,
		peak_power_loss=?, peak_comms_loss=?, isolated_facility_count=? WHERE id=?`,
		sc.Tick, 100-sc.PowerGridHealth, 100-sc.CommsCoverage, len(sc.IsolatedFacilities), id); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO metrics_samples
		(execution_id, tick, power_grid_health, comms_coverage, road_accessibility, message_delivery, average_latency, connected_facilities)
		VALUES (?,?,?,?,?,?,?,?)`,
		id, sc.Tick, sc.PowerGridHealth, sc.CommsCoverage, sc.RoadAccessibility,
		sc.MessageDeliveryRate, sc.AverageLatency, sc.ConnectedFacilities); err != nil {
		return err
	}
	for _, e := range sc.Diagnostics.Events {
		if _, err := tx.Exec(`INSERT INTO execution_events (execution_id, tick, severity, source, node_id, message)
			VALUES (?,?,?,?,?,?)`, id, e.Tick, e.Severity, e.Source, e.NodeID, e.Message); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (l *Loader) Executions(scenarioID string) ([]ExecutionRecord, error) {
	if l.db == nil {
		return nil, nil
	}
	rows, err := l.db.Query(`SELECT id, scenario_id, final_tick, peak_power_loss, peak_comms_loss,
		isolated_facility_count, started_at FROM executions WHERE scenario_id=? ORDER BY id DESC LIMIT 25`, scenarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExecutionRecord{}
	for rows.Next() {
		var r ExecutionRecord
		var started sql.NullString
		if err := rows.Scan(&r.ID, &r.ScenarioID, &r.FinalTick, &r.PeakPower, &r.PeakComms, &r.Isolated, &started); err != nil {
			return nil, err
		}
		r.StartedAt = started.String
		out = append(out, r)
	}
	return out, rows.Err()
}
