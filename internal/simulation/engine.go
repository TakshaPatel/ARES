package simulation

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Preset struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Kind        string  `json:"kind"`
	TargetID    string  `json:"targetId,omitempty"`
	Severity    string  `json:"severity"`
	Intensity   float64 `json:"intensity,omitempty"`
}

type Scenario struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	HazardModel string                `json:"hazardModel"`
	TickMinutes int                   `json:"tickMinutes"`
	StartHour   int                   `json:"startHour"`
	StartMinute int                   `json:"startMinute"`
	Nodes       []*InfrastructureNode `json:"nodes"`
	Connections []*Connection         `json:"connections"`
	Presets     []Preset              `json:"presets"`
}

func (s *Scenario) Normalize() {
	if s.TickMinutes <= 0 {
		s.TickMinutes = 1
	}
	if s.StartHour < 0 || s.StartHour > 23 {
		s.StartHour = 6
	}
	if s.StartMinute < 0 || s.StartMinute > 59 {
		s.StartMinute = 0
	}
	for _, n := range s.Nodes {
		if n.Health <= 0 {
			n.Health = 100
		}
		if n.Health > 100 {
			n.Health = 100
		}
		if !n.Operational && n.Health > 0 {
			n.Operational = true
		}
		if n.Status == "" {
			n.Status = StatusOperational
		}
	}
	idx := map[string]*InfrastructureNode{}
	for _, n := range s.Nodes {
		idx[n.ID] = n
	}
	for _, c := range s.Connections {
		if !c.Active && !c.Blocked && c.Capacity > 0 {
			c.Active = true
		}
		if c.SpeedKPH <= 0 {
			if v, ok := c.Metadata["speed_kph"]; ok {
				if f, ok2 := v.(float64); ok2 {
					c.SpeedKPH = f
				}
			}
		}
		if c.SpeedKPH <= 0 {
			if c.Type == EdgeRoad {
				c.SpeedKPH = 50
			}
		}
		if c.Distance <= 0 {
			from, ok1 := idx[c.From]
			to, ok2 := idx[c.To]
			if ok1 && ok2 {
				c.Distance = round2(Haversine(from.Lat, from.Lng, to.Lat, to.Lng))
			}
		}
		if c.Metadata == nil {
			c.Metadata = map[string]interface{}{}
		}
	}
}

type Engine struct {
	mu sync.RWMutex

	scenario *Scenario
	graph    *DependencyGraph
	nodes    map[string]*InfrastructureNode
	conns    map[string]*Connection

	baseNodes map[string]*InfrastructureNode
	baseConns map[string]*Connection

	tick                int
	running             bool
	hz                  *Hazard
	events              []EventLogEntry
	eventSeq            int
	revision            int64
	lastRoutes          map[string][]string
	lastTotalFacilities int

	listeners map[int]func(*SimulationState)
	nextSub   int

	stop chan struct{}
	done chan struct{}
}

func NewEngine(sc *Scenario) *Engine {
	sc.Normalize()
	e := &Engine{
		scenario:  sc,
		nodes:     make(map[string]*InfrastructureNode, len(sc.Nodes)),
		conns:     make(map[string]*Connection, len(sc.Connections)),
		hz:        NewHazard(),
		listeners: map[int]func(*SimulationState){},
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	e.loadLocked()
	return e
}

func (e *Engine) loadLocked() {
	e.nodes = make(map[string]*InfrastructureNode, len(e.scenario.Nodes))
	e.conns = make(map[string]*Connection, len(e.scenario.Connections))
	for _, n := range e.scenario.Nodes {
		c := *n
		c.Dependencies = append([]string(nil), n.Dependencies...)
		if n.Metadata != nil {
			c.Metadata = make(map[string]interface{}, len(n.Metadata))
			for k, v := range n.Metadata {
				c.Metadata[k] = v
			}
		}
		c.Operational = true
		c.Health = 100
		c.Status = StatusOperational
		c.Reason = ""
		c.CascadeDep = 0
		c.Repairing = false
		e.nodes[c.ID] = &c
	}
	for _, cn := range e.scenario.Connections {
		c := *cn
		c.Blocked = false
		c.Active = true
		c.Failures = 0
		c.Flooded = false
		if cn.Metadata != nil {
			c.Metadata = make(map[string]interface{}, len(cn.Metadata))
			for k, v := range cn.Metadata {
				c.Metadata[k] = v
			}
		}
		e.conns[c.ID] = &c
	}
	e.graph = NewDependencyGraph(e.nodes)
	e.tick = 0
	e.hz = NewHazard()
	e.events = nil
	e.eventSeq = 0
	e.lastRoutes = map[string][]string{}
	e.revision++
	e.captureBase()
}

func (e *Engine) captureBase() {
	e.baseNodes = make(map[string]*InfrastructureNode, len(e.nodes))
	for id, n := range e.nodes {
		c := *n
		e.baseNodes[id] = &c
	}
	e.baseConns = make(map[string]*Connection, len(e.conns))
	for id, c := range e.conns {
		cc := *c
		e.baseConns[id] = &cc
	}
}

func (e *Engine) Scenario() *Scenario { return e.scenario }

func (e *Engine) Running() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

func (e *Engine) Tick() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.tick
}

func (e *Engine) Presets() []Preset { return e.scenario.Presets }

func (e *Engine) clock() string {
	return ClockString(e.tick, e.scenario.TickMinutes, e.scenario.StartHour, e.scenario.StartMinute)
}

func (e *Engine) logEvent(sev EventSeverity, cat EventCategory, source, nodeID, msg string, depth int) {
	e.eventSeq++
	e.events = append([]EventLogEntry{{
		ID: e.eventSeq, Tick: e.tick, Clock: e.clock(), Severity: sev,
		Category: cat, Source: source, NodeID: nodeID, Depth: depth, Message: msg,
	}}, e.events...)
	if len(e.events) > 400 {
		e.events = e.events[:400]
	}
}

func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stop = make(chan struct{})
	e.done = make(chan struct{})
	stop, done := e.stop, e.done
	e.logEvent(EventInfo, CatCommand, "COMMAND", "", "SIMULATION STARTED", 0)
	e.mu.Unlock()
	e.publish()

	go func() {
		defer close(done)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				e.TickOnce()
			}
		}
	}()
}

func (e *Engine) Pause() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stop)
	e.logEvent(EventWarning, CatCommand, "COMMAND", "", "SIMULATION PAUSED", 0)
	e.mu.Unlock()
	<-e.done
	e.publish()
}

func (e *Engine) Shutdown() {
	e.mu.Lock()
	if e.running {
		e.running = false
		close(e.stop)
	}
	e.mu.Unlock()
}

func (e *Engine) TickOnce() {
	e.mu.Lock()
	e.advanceLocked()
	state := e.buildStateLocked()
	listeners := e.collectListenersLocked()
	e.mu.Unlock()
	notify(listeners, state)
}

func (e *Engine) advanceLocked() {
	e.tick++
	e.graph = NewDependencyGraph(e.nodes)

	hzOut := AdvanceHazard(e.hz, e.nodes, e.conns, e.tick)
	for _, id := range hzOut.Failed {
		n := e.nodes[id]
		e.logEvent(EventFailure, CatPower, string(SourceHazard), id,
			fmt.Sprintf("%s destroyed by %s at intensity %.0f%%", n.Name, e.hz.Name, e.hz.Intensity*100), 0)
	}
	for _, id := range hzOut.Blocked {
		e.logEvent(EventFailure, CatRoad, string(SourceHazard), id,
			fmt.Sprintf("%s impassable: storm surge", labelFor(e.conns, id)), 1)
	}

	failures := e.graph.EvaluateDependencies(e.nodes, e.conns, e.hz.Flooded)
	for _, f := range failures {
		n := e.nodes[f.ID]
		sev := EventCascade
		if f.Depth <= 1 {
			sev = EventCascade
		}
		cat := categoryFor(n.Type)
		e.logEvent(sev, cat, string(SourceCascade), n.ID,
			fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", n.Name, f.Reason, f.Depth), f.Depth)
	}

	DecayHealth(e.nodes, e.hz, e.tick)
	rec := StepRecovery(e.nodes, e.conns, e.graph, e.hz, e.tick > 2)
	for _, id := range rec.Repaired {
		e.logEvent(EventInfo, CatCommand, string(SourceAutoRepair), id,
			fmt.Sprintf("Repair crew dispatched to %s", e.nodes[id].Name), 0)
	}
	for _, id := range rec.Unblocked {
		e.logEvent(EventSuccess, CatRoad, string(SourceAutoRepair), id,
			fmt.Sprintf("%s cleared and reopened", labelFor(e.conns, id)), 0)
	}
	e.applyRoutingLocked()
}

func (e *Engine) applyRoutingLocked() {
	net := AnalyseNetwork(e.nodes, e.conns)
	rr := ComputeEMSRoutes(e.nodes, e.conns, net, e.lastRoutes)
	for _, r := range rr.Changed {
		e.logEvent(EventReroute, CatRouting, "ROUTING_ENGINE", r.Origin,
			DescribeRoute(r, "dynamic obstruction"), 0)
	}
	for _, h := range rr.Unreachable {
		e.logEvent(EventFailure, CatRouting, "ROUTING_ENGINE", h,
			fmt.Sprintf("No road route to %s from any responding unit", e.nodes[h].Name), 0)
	}
	e.lastRoutes = rr.Active
}

func (e *Engine) buildStateLocked() *SimulationState {
	net := AnalyseNetwork(e.nodes, e.conns)
	rr := ComputeEMSRoutes(e.nodes, e.conns, net, e.lastRoutes)

	facilities := make([]string, 0, len(e.nodes))
	for id, n := range e.nodes {
		if n.Type.IsFacility() {
			facilities = append(facilities, id)
		}
	}
	sort.Strings(facilities)

	roadAdj := BuildRoadAdjacency(e.nodes, e.conns)
	roadReach := roadReachability(e.nodes, roadAdj)

	isolated := make([]string, 0)
	connected := 0
	commsCov := e.commsCoverage(net)
	for _, f := range facilities {
		commsOK := net.CommsReach[f] || net.PeerReach[f]
		roadOK := roadReach[f]
		if commsOK || roadOK {
			connected++
		}
		if !commsOK && !roadOK {
			isolated = append(isolated, f)
		}
	}

	delivery, latency := e.deliveryMetrics(net, facilities)
	roadAcc := e.roadAccessibility(facilities, roadReach)

	alerts := e.buildAlerts(net, facilities, isolated, delivery, latency)
	meshBackoff := net.MeshBackoff
	blockedish := make([]string, 0)
	for id, c := range e.conns {
		if c.Blocked {
			blockedish = append(blockedish, id)
		}
	}
	sort.Strings(blockedish)

	for _, n := range e.nodes {
		n.MeshLink = net.MeshBackoffSet[n.ID]
		n.Status = n.StatusFor(contains(isolated, n.ID))
	}

	state := &SimulationState{
		Tick:                e.tick,
		Running:             e.running,
		Nodes:               e.nodes,
		Connections:         e.conns,
		PowerGridHealth:     e.powerGridHealth(),
		CommsCoverage:       commsCov,
		RoadAccessibility:   roadAcc,
		MessageDeliveryRate: delivery,
		AverageLatency:      latency,
		ConnectedFacilities: connected,
		TotalFacilities:     len(facilities),
		IsolatedFacilities:  isolated,
		ActiveAlerts:        alerts,
		ActiveEMSRoutes:     rr.Active,
	}
	powerCritical := 0.0
	for id, n := range e.nodes {
		if n.Type == NodePowerSubstation {
			powerCritical += float64(e.graph.Criticality[id])
		}
	}
	state.Diagnostics = &Diagnostics{
		Events:         append([]EventLogEntry{}, e.events...),
		CommsClusters:  net.Clusters,
		EmsRoutes:      rr.Routes,
		MeshBackoff:    meshBackoff,
		DependencyEdge: DependencyEdgeList(e.nodes, e.conns),
		BlockedEdges:   blockedish,
		PowerCritical:  powerCritical,
		ElapsedMinutes: e.tick * e.scenario.TickMinutes,
		Clock:          e.clock(),
		Revision:       e.revision,
	}
	return state
}

func (e *Engine) powerGridHealth() float64 {
	var num, den float64
	for id, n := range e.nodes {
		if n.Type != NodePowerSubstation {
			continue
		}
		w := float64(e.graph.Criticality[id])
		if w < 1 {
			w = 1
		}
		num += w * n.Health
		den += w
	}
	if den == 0 {
		return 100
	}
	return round2(num / den)
}

func (e *Engine) commsCoverage(net *NetworkAnalysis) float64 {
	var total, covered int
	for id, n := range e.nodes {
		if !n.Type.IsComms() {
			continue
		}
		total++
		if !n.Operational {
			continue
		}
		if idx, ok := net.ClusterOf[id]; ok {
			for _, m := range net.Clusters[idx] {
				if mn, ok2 := e.nodes[m]; ok2 && mn.Type == NodeEOC {
					covered++
					break
				}
			}
		}
	}
	if total == 0 {
		return 0
	}
	return round2(float64(covered) / float64(total) * 100)
}

func (e *Engine) roadAccessibility(facilities []string, roadReach map[string]bool) float64 {
	var total, open int
	for _, c := range e.conns {
		if c.Type != EdgeRoad {
			continue
		}
		total++
		if !c.Blocked {
			open++
		}
	}
	segPct := 100.0
	if total > 0 {
		segPct = float64(open) / float64(total) * 100
	}
	reachCount := 0
	for _, f := range facilities {
		if roadReach[f] {
			reachCount++
		}
	}
	facPct := 100.0
	if len(facilities) > 0 {
		facPct = float64(reachCount) / float64(len(facilities)) * 100
	}
	return round2(segPct*0.55 + facPct*0.45)
}

func (e *Engine) deliveryMetrics(net *NetworkAnalysis, facilities []string) (float64, float64) {
	if len(facilities) == 0 {
		return 0, 0
	}
	var dsum, lsum float64
	var lcount int
	for _, f := range facilities {
		if v, ok := net.Delivery[f]; ok && v > 0 {
			dsum += v
		}
		if v, ok := net.Latency[f]; ok && v > 0 {
			lsum += v
			lcount++
		}
	}
	delivery := dsum / float64(len(facilities))
	latency := 0.0
	if lcount > 0 {
		latency = lsum / float64(lcount)
	}
	return round2(delivery), round2(latency)
}

func (e *Engine) buildAlerts(net *NetworkAnalysis, facilities, isolated []string, delivery, latency float64) []string {
	alerts := make([]string, 0, 8)
	for id, n := range e.nodes {
		if n.Type == NodePowerSubstation && !n.Operational {
			alerts = append(alerts, "GRID: "+n.Name+" OFFLINE ("+n.Reason+")")
		}
		if n.Type.IsComms() && !n.Operational {
			alerts = append(alerts, "COMMS: "+n.Name+" OFFLINE")
		}
		if n.Type == NodeRoadIntersection && !n.Operational {
			alerts = append(alerts, "TRAFFIC: "+n.Name+" CONTROL LOST")
		}
		_ = id
	}
	for _, f := range isolated {
		alerts = append(alerts, "ISOLATION: "+e.nodes[f].Name+" has no comms or road path to command")
	}
	if len(net.MeshBackoff) > 0 {
		alerts = append(alerts, fmt.Sprintf("MESH: %d facilities on low-bandwidth fallback", len(net.MeshBackoff)))
	}
	if delivery < 85 && delivery > 0 {
		alerts = append(alerts, fmt.Sprintf("DELIVERY: packet delivery degraded to %.1f%%", delivery))
	}
	if latency > 220 {
		alerts = append(alerts, fmt.Sprintf("LATENCY: mesh backhaul at %.0f ms", latency))
	}
	sort.Strings(alerts)
	if len(alerts) > 24 {
		alerts = alerts[:24]
	}
	return alerts
}

func categoryFor(t NodeType) EventCategory {
	switch t {
	case NodePowerSubstation:
		return CatPower
	case NodeCellTower, NodeRadioMesh:
		return CatComms
	case NodeRoadIntersection:
		return CatRoad
	case NodeHospital, NodeFireStation, NodeEOC:
		return CatRouting
	}
	return CatCommand
}

func labelFor(conns map[string]*Connection, id string) string {
	if c, ok := conns[id]; ok {
		return c.ID
	}
	return id
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func (e *Engine) collectListenersLocked() []func(*SimulationState) {
	out := make([]func(*SimulationState), 0, len(e.listeners))
	for _, fn := range e.listeners {
		out = append(out, fn)
	}
	return out
}

func notify(listeners []func(*SimulationState), state *SimulationState) {
	payload := state.Clone()
	for _, fn := range listeners {
		fn(payload)
	}
}

func (e *Engine) publish() {
	e.mu.RLock()
	state := e.buildStateLocked()
	listeners := e.collectListenersLocked()
	e.mu.RUnlock()
	notify(listeners, state)
}

func (e *Engine) Subscribe(fn func(*SimulationState)) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextSub++
	id := e.nextSub
	e.listeners[id] = fn
	return id
}

func (e *Engine) Unsubscribe(id int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.listeners, id)
}

func (e *Engine) Snapshot() *SimulationState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.buildStateLocked().Clone()
}

func (e *Engine) InjectNodeFailure(targetID string) (string, error) {
	e.mu.Lock()
	n, ok := FailNode(e.nodes, e.conns, targetID, SourceOperator)
	if !ok {
		current := e.nodes[targetID]
		if current == nil {
			e.mu.Unlock()
			return "", fmt.Errorf("unknown node %q", targetID)
		}
		e.logEvent(EventWarning, CatCommand, "COMMAND", targetID, n.Name+" already offline", 0)
		e.mu.Unlock()
		e.publish()
		return n.ID, nil
	}
	e.logEvent(EventFailure, categoryFor(n.Type), "OPERATOR", n.ID,
		fmt.Sprintf("%s taken offline by operator command (%s)", n.Name, n.Type), 0)
	graph := NewDependencyGraph(e.nodes)
	failures := graph.EvaluateDependencies(e.nodes, e.conns, e.hz.Flooded)
	for _, f := range failures {
		fn := e.nodes[f.ID]
		e.logEvent(EventCascade, categoryFor(fn.Type), string(SourceCascade), fn.ID,
			fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", fn.Name, f.Reason, f.Depth), f.Depth)
	}
	e.applyRoutingLocked()
	e.mu.Unlock()
	e.publish()
	return n.ID, nil
}

func (e *Engine) InjectRoadBlock(connID string) (string, error) {
	e.mu.Lock()
	c, changed := BlockConnection(e.conns, connID)
	if !changed {
		if c == nil {
			e.mu.Unlock()
			return "", fmt.Errorf("unknown connection %q", connID)
		}
		e.logEvent(EventWarning, CatRoad, "COMMAND", connID, c.ID+" already blocked", 0)
		e.mu.Unlock()
		e.publish()
		return c.ID, nil
	}
	e.logEvent(EventFailure, CatRoad, "OPERATOR", connID,
		fmt.Sprintf("Road segment %s blocked (obstruction reported)", connID), 0)
	e.applyRoutingLocked()
	e.mu.Unlock()
	e.publish()
	return c.ID, nil
}

func (e *Engine) RestoreNodeByID(targetID string) (string, error) {
	e.mu.Lock()
	n, repaired, ok := RestoreNode(e.nodes, e.conns, e.graph, targetID)
	if !ok {
		if n == nil {
			e.mu.Unlock()
			return "", fmt.Errorf("unknown node %q", targetID)
		}
		blocking := blockingPrerequisites(e.graph, e.nodes, targetID)
		e.logEvent(EventWarning, CatCommand, "COMMAND", targetID,
			fmt.Sprintf("Restore of %s denied: prerequisite %s still offline", n.Name, blocking[0]), 0)
		e.mu.Unlock()
		e.publish()
		return n.ID, nil
	}
	e.logEvent(EventSuccess, categoryFor(n.Type), "OPERATOR", n.ID,
		fmt.Sprintf("%s restored to service; %d links re-energised", n.Name, len(repaired)), 0)
	graph := NewDependencyGraph(e.nodes)
	for _, f := range graph.EvaluateDependencies(e.nodes, e.conns, e.hz.Flooded) {
		fn := e.nodes[f.ID]
		e.logEvent(EventSuccess, categoryFor(fn.Type), string(SourceCascade), fn.ID,
			fmt.Sprintf("%s back online after prerequisite restoration", fn.Name), f.Depth)
	}
	e.applyRoutingLocked()
	e.mu.Unlock()
	e.publish()
	return n.ID, nil
}

func (e *Engine) ApplyPreset(id string) error {
	e.mu.Lock()
	var preset *Preset
	for i := range e.scenario.Presets {
		if e.scenario.Presets[i].ID == id {
			preset = &e.scenario.Presets[i]
			break
		}
	}
	if preset == nil {
		e.mu.Unlock()
		return fmt.Errorf("unknown preset %q", id)
	}
	kind := preset.Kind
	target := preset.TargetID
	e.logEvent(EventWarning, CatCommand, "OPERATOR", target,
		fmt.Sprintf("PRESET ACTIVATED: %s (%s)", preset.Label, preset.Description), 0)
	switch kind {
	case "HAZARD":
		ActivateHurricane(e.hz, e.conns, e.tick, preset.Intensity)
		graph := NewDependencyGraph(e.nodes)
		for _, f := range graph.EvaluateDependencies(e.nodes, e.conns, e.hz.Flooded) {
			fn := e.nodes[f.ID]
			e.logEvent(EventCascade, categoryFor(fn.Type), string(SourceCascade), fn.ID,
				fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", fn.Name, f.Reason, f.Depth), f.Depth)
		}
		e.applyRoutingLocked()
	case "NODE_FAILURE":
		ids := strings.Split(target, ",")
		changed := false
		for _, raw := range ids {
			tid := strings.TrimSpace(raw)
			if tid == "" {
				continue
			}
			n, ok := FailNode(e.nodes, e.conns, tid, SourceOperator)
			if n == nil {
				e.mu.Unlock()
				return fmt.Errorf("unknown preset target %q", tid)
			}
			if ok {
				changed = true
				e.logEvent(EventFailure, categoryFor(n.Type), "OPERATOR", n.ID,
					fmt.Sprintf("%s taken offline by preset %s", n.Name, preset.Label), 0)
			}
		}
		if !changed {
			e.mu.Unlock()
			e.publish()
			return nil
		}
		graph := NewDependencyGraph(e.nodes)
		for _, f := range graph.EvaluateDependencies(e.nodes, e.conns, e.hz.Flooded) {
			fn := e.nodes[f.ID]
			e.logEvent(EventCascade, categoryFor(fn.Type), string(SourceCascade), fn.ID,
				fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", fn.Name, f.Reason, f.Depth), f.Depth)
		}
		e.applyRoutingLocked()
	case "ROAD_BLOCK":
		if c, changed := BlockConnection(e.conns, target); changed {
			e.logEvent(EventFailure, CatRoad, "OPERATOR", target,
				fmt.Sprintf("Road segment %s flooded by preset %s", target, preset.Label), 0)
			_ = c
		}
		e.applyRoutingLocked()
	case "FLOOD_ZONE":
		ids := make([]string, 0, len(e.conns))
		for id := range e.conns {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		sealed := 0
		for _, id := range ids {
			c := e.conns[id]
			if c.Type != EdgeRoad || c.Blocked || c.Risk() < 0.6 {
				continue
			}
			c.Blocked = true
			c.Active = false
			c.Flooded = true
			c.Failures++
			e.hz.Flooded[id] = true
			e.logEvent(EventFailure, CatRoad, "OPERATOR", id,
				fmt.Sprintf("Storm surge seals %s (flood risk %.0f%%)", id, c.Risk()*100), 1)
			sealed++
		}
		e.applyRoutingLocked()
	}
	e.mu.Unlock()
	e.publish()
	return nil
}

func (e *Engine) Reset() {
	e.mu.Lock()
	if e.running {
		e.running = false
		close(e.stop)
		<-e.done
	}
	e.loadLocked()
	e.logEvent(EventInfo, CatCommand, "COMMAND", "",
		fmt.Sprintf("RESET to T+00:00 :: %s", e.scenario.Name), 0)
	e.mu.Unlock()
	e.publish()
}
