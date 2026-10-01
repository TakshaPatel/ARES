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

func (scenario *Scenario) Normalize() {
	if scenario.TickMinutes <= 0 {
		scenario.TickMinutes = 1
	}
	if scenario.StartHour < 0 || scenario.StartHour > 23 {
		scenario.StartHour = 6
	}
	if scenario.StartMinute < 0 || scenario.StartMinute > 59 {
		scenario.StartMinute = 0
	}
	for _, node := range scenario.Nodes {
		if node.Health <= 0 {
			node.Health = 100
		}
		if node.Health > 100 {
			node.Health = 100
		}
		if !node.Operational && node.Health > 0 {
			node.Operational = true
		}
		if node.Status == "" {
			node.Status = StatusOperational
		}
	}
	idx := map[string]*InfrastructureNode{}
	for _, node := range scenario.Nodes {
		idx[node.ID] = node
	}
	for _, conn := range scenario.Connections {
		if !conn.Active && !conn.Blocked && conn.Capacity > 0 {
			conn.Active = true
		}
		if conn.SpeedKPH <= 0 {
			if v, ok := conn.Metadata["speed_kph"]; ok {
				if f, ok2 := v.(float64); ok2 {
					conn.SpeedKPH = f
				}
			}
		}
		if conn.SpeedKPH <= 0 {
			if conn.Type == EdgeRoad {
				conn.SpeedKPH = 50
			}
		}
		if conn.Distance <= 0 {
			from, ok1 := idx[conn.From]
			to, ok2 := idx[conn.To]
			if ok1 && ok2 {
				conn.Distance = round2(Haversine(from.Lat, from.Lng, to.Lat, to.Lng))
			}
		}
		if conn.Metadata == nil {
			conn.Metadata = map[string]interface{}{}
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
	eng := &Engine{
		scenario:  sc,
		nodes:     make(map[string]*InfrastructureNode, len(sc.Nodes)),
		conns:     make(map[string]*Connection, len(sc.Connections)),
		hz:        NewHazard(),
		listeners: map[int]func(*SimulationState){},
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	eng.loadLocked()
	return eng
}

func (eng *Engine) loadLocked() {
	eng.nodes = make(map[string]*InfrastructureNode, len(eng.scenario.Nodes))
	eng.conns = make(map[string]*Connection, len(eng.scenario.Connections))
	for _, node := range eng.scenario.Nodes {
		c := *node
		c.Dependencies = copyStrings(node.Dependencies)
		if node.Metadata != nil {
			c.Metadata = make(map[string]interface{}, len(node.Metadata))
			for key, value := range node.Metadata {
				c.Metadata[key] = value
			}
		}
		c.Operational = true
		c.Health = 100
		c.Status = StatusOperational
		c.Reason = ""
		c.CascadeDep = 0
		c.Repairing = false
		eng.nodes[c.ID] = &c
	}
	for _, cn := range eng.scenario.Connections {
		c := *cn
		c.Blocked = false
		c.Active = true
		c.Failures = 0
		c.Flooded = false
		if cn.Metadata != nil {
			c.Metadata = make(map[string]interface{}, len(cn.Metadata))
			for key, value := range cn.Metadata {
				c.Metadata[key] = value
			}
		}
		eng.conns[c.ID] = &c
	}
	eng.graph = NewDependencyGraph(eng.nodes)
	eng.tick = 0
	eng.hz = NewHazard()
	eng.events = nil
	eng.eventSeq = 0
	eng.lastRoutes = map[string][]string{}
	eng.revision++
	eng.captureBase()
}

func (eng *Engine) captureBase() {
	eng.baseNodes = make(map[string]*InfrastructureNode, len(eng.nodes))
	for id, node := range eng.nodes {
		c := *node
		eng.baseNodes[id] = &c
	}
	eng.baseConns = make(map[string]*Connection, len(eng.conns))
	for id, conn := range eng.conns {
		cc := *conn
		eng.baseConns[id] = &cc
	}
}

func (eng *Engine) Scenario() *Scenario { return eng.scenario }

func (eng *Engine) Running() bool {
	eng.mu.RLock()
	defer eng.mu.RUnlock()
	return eng.running
}

func (eng *Engine) Tick() int {
	eng.mu.RLock()
	defer eng.mu.RUnlock()
	return eng.tick
}

func (eng *Engine) Presets() []Preset { return eng.scenario.Presets }

func (eng *Engine) clock() string {
	return ClockString(eng.tick, eng.scenario.TickMinutes, eng.scenario.StartHour, eng.scenario.StartMinute)
}

func (eng *Engine) logEvent(sev EventSeverity, cat EventCategory, source, nodeID, msg string, depth int) {
	eng.eventSeq++
	eng.events = append([]EventLogEntry{{
		ID: eng.eventSeq, Tick: eng.tick, Clock: eng.clock(), Severity: sev,
		Category: cat, Source: source, NodeID: nodeID, Depth: depth, Message: msg,
	}}, eng.events...)
	if len(eng.events) > 400 {
		eng.events = eng.events[:400]
	}
}

func (eng *Engine) Start() {
	eng.mu.Lock()
	if eng.running {
		eng.mu.Unlock()
		return
	}
	eng.running = true
	eng.stop = make(chan struct{})
	eng.done = make(chan struct{})
	stop, done := eng.stop, eng.done
	eng.logEvent(EventInfo, CatCommand, "COMMAND", "", "SIMULATION STARTED", 0)
	eng.mu.Unlock()
	eng.publish()

	go func() {
		defer close(done)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				eng.TickOnce()
			}
		}
	}()
}

func (eng *Engine) Pause() {
	eng.mu.Lock()
	if !eng.running {
		eng.mu.Unlock()
		return
	}
	eng.running = false
	close(eng.stop)
	eng.logEvent(EventWarning, CatCommand, "COMMAND", "", "SIMULATION PAUSED", 0)
	eng.mu.Unlock()
	<-eng.done
	eng.publish()
}

func (eng *Engine) Shutdown() {
	eng.mu.Lock()
	if eng.running {
		eng.running = false
		close(eng.stop)
	}
	eng.mu.Unlock()
}

func (eng *Engine) TickOnce() {
	eng.mu.Lock()
	eng.advanceLocked()
	state := eng.buildStateLocked()
	listeners := eng.collectListenersLocked()
	eng.mu.Unlock()
	notify(listeners, state)
}

func (eng *Engine) advanceLocked() {
	eng.tick++
	eng.graph = NewDependencyGraph(eng.nodes)

	hzOut := AdvanceHazard(eng.hz, eng.nodes, eng.conns, eng.tick)
	for _, id := range hzOut.Failed {
		n := eng.nodes[id]
		eng.logEvent(EventFailure, CatPower, string(SourceHazard), id,
			fmt.Sprintf("%s destroyed by %s at intensity %.0f%%", n.Name, eng.hz.Name, eng.hz.Intensity*100), 0)
	}
	for _, id := range hzOut.Blocked {
		eng.logEvent(EventFailure, CatRoad, string(SourceHazard), id,
			fmt.Sprintf("%s impassable: floodwater", labelFor(eng.conns, id)), 1)
	}

	failures := eng.graph.EvaluateDependencies(eng.nodes, eng.conns, eng.hz.Flooded)
	for _, failure := range failures {
		n := eng.nodes[failure.ID]
		sev := EventCascade
		if failure.Depth <= 1 {
			sev = EventCascade
		}
		cat := categoryFor(n.Type)
		eng.logEvent(sev, cat, string(SourceCascade), n.ID,
			fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", n.Name, failure.Reason, failure.Depth), failure.Depth)
	}

	DecayHealth(eng.nodes, eng.hz, eng.tick)
	rec := StepRecovery(eng.nodes, eng.conns, eng.graph, eng.hz, eng.tick > 2)
	for _, id := range rec.Repaired {
		eng.logEvent(EventInfo, CatCommand, string(SourceAutoRepair), id,
			fmt.Sprintf("Repair crew dispatched to %s", eng.nodes[id].Name), 0)
	}
	for _, id := range rec.Unblocked {
		eng.logEvent(EventSuccess, CatRoad, string(SourceAutoRepair), id,
			fmt.Sprintf("%s cleared and reopened", labelFor(eng.conns, id)), 0)
	}
	eng.applyRoutingLocked()
}

func (eng *Engine) applyRoutingLocked() {
	net := AnalyseNetwork(eng.nodes, eng.conns)
	rr := ComputeEMSRoutes(eng.nodes, eng.conns, net, eng.lastRoutes)
	for _, route := range rr.Changed {
		eng.logEvent(EventReroute, CatRouting, "ROUTING_ENGINE", route.Origin,
			DescribeRoute(route, "dynamic obstruction"), 0)
	}
	for _, hospitalId := range rr.Unreachable {
		eng.logEvent(EventFailure, CatRouting, "ROUTING_ENGINE", hospitalId,
			fmt.Sprintf("No road route to %s from any responding unit", eng.nodes[hospitalId].Name), 0)
	}
	eng.lastRoutes = rr.Active
}

func (eng *Engine) buildStateLocked() *SimulationState {
	net := AnalyseNetwork(eng.nodes, eng.conns)
	rr := ComputeEMSRoutes(eng.nodes, eng.conns, net, eng.lastRoutes)

	facilities := make([]string, 0, len(eng.nodes))
	for id, node := range eng.nodes {
		if node.Type.IsFacility() {
			facilities = append(facilities, id)
		}
	}
	sort.Strings(facilities)

	roadAdj := BuildRoadAdjacency(eng.nodes, eng.conns)
	roadReach := roadReachability(eng.nodes, roadAdj)

	isolated := make([]string, 0)
	connected := 0
	commsCov := eng.commsCoverage(net)
	for _, facilityId := range facilities {
		commsOK := net.CommsReach[facilityId] || net.PeerReach[facilityId]
		roadOK := roadReach[facilityId]
		if commsOK || roadOK {
			connected++
		}
		if !commsOK && !roadOK {
			isolated = append(isolated, facilityId)
		}
	}

	delivery, latency := eng.deliveryMetrics(net, facilities)
	roadAcc := eng.roadAccessibility(facilities, roadReach)

	alerts := eng.buildAlerts(net, facilities, isolated, delivery, latency)
	meshBackoff := net.MeshBackoff
	blockedish := make([]string, 0)
	for id, conn := range eng.conns {
		if conn.Blocked {
			blockedish = append(blockedish, id)
		}
	}
	sort.Strings(blockedish)

	for _, node := range eng.nodes {
		node.MeshLink = net.MeshBackoffSet[node.ID]
		node.Status = node.StatusFor(contains(isolated, node.ID))
	}

	state := &SimulationState{
		Tick:                eng.tick,
		Running:             eng.running,
		Nodes:               eng.nodes,
		Connections:         eng.conns,
		PowerGridHealth:     eng.powerGridHealth(),
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
	for id, node := range eng.nodes {
		if node.Type == NodePowerSubstation {
			powerCritical += float64(eng.graph.Criticality[id])
		}
	}
	state.Diagnostics = &Diagnostics{
		Events:         append([]EventLogEntry{}, eng.events...),
		CommsClusters:  net.Clusters,
		EmsRoutes:      rr.Routes,
		MeshBackoff:    meshBackoff,
		DependencyEdge: DependencyEdgeList(eng.nodes, eng.conns),
		BlockedEdges:   blockedish,
		PowerCritical:  powerCritical,
		ElapsedMinutes: eng.tick * eng.scenario.TickMinutes,
		Clock:          eng.clock(),
		Revision:       eng.revision,
	}
	return state
}

func (eng *Engine) powerGridHealth() float64 {
	var num, den float64
	for id, node := range eng.nodes {
		if node.Type != NodePowerSubstation {
			continue
		}
		w := float64(eng.graph.Criticality[id])
		if w < 1 {
			w = 1
		}
		num += w * node.Health
		den += w
	}
	if den == 0 {
		return 100
	}
	return round2(num / den)
}

func (eng *Engine) commsCoverage(net *NetworkAnalysis) float64 {
	var total, covered int
	for id, node := range eng.nodes {
		if !node.Type.IsComms() {
			continue
		}
		total++
		if !node.Operational {
			continue
		}
		if idx, ok := net.ClusterOf[id]; ok {
			for _, memberId := range net.Clusters[idx] {
				if mn, ok2 := eng.nodes[memberId]; ok2 && mn.Type == NodeEOC {
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

func (eng *Engine) roadAccessibility(facilities []string, roadReach map[string]bool) float64 {
	var total, open int
	for _, conn := range eng.conns {
		if conn.Type != EdgeRoad {
			continue
		}
		total++
		if !conn.Blocked {
			open++
		}
	}
	segPct := 100.0
	if total > 0 {
		segPct = float64(open) / float64(total) * 100
	}
	reachCount := 0
	for _, facilityId := range facilities {
		if roadReach[facilityId] {
			reachCount++
		}
	}
	facPct := 100.0
	if len(facilities) > 0 {
		facPct = float64(reachCount) / float64(len(facilities)) * 100
	}
	return round2(segPct*0.55 + facPct*0.45)
}

func (eng *Engine) deliveryMetrics(net *NetworkAnalysis, facilities []string) (float64, float64) {
	if len(facilities) == 0 {
		return 0, 0
	}
	var dsum, lsum float64
	var lcount int
	for _, facilityId := range facilities {
		if v, ok := net.Delivery[facilityId]; ok && v > 0 {
			dsum += v
		}
		if v, ok := net.Latency[facilityId]; ok && v > 0 {
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

func (eng *Engine) buildAlerts(net *NetworkAnalysis, facilities, isolated []string, delivery, latency float64) []string {
	alerts := make([]string, 0, 8)
	for id, node := range eng.nodes {
		if node.Type == NodePowerSubstation && !node.Operational {
			alerts = append(alerts, "GRID: "+node.Name+" OFFLINE ("+node.Reason+")")
		}
		if node.Type.IsComms() && !node.Operational {
			alerts = append(alerts, "COMMS: "+node.Name+" OFFLINE")
		}
		if node.Type == NodeRoadIntersection && !node.Operational {
			alerts = append(alerts, "TRAFFIC: "+node.Name+" CONTROL LOST")
		}
		_ = id
	}
	for _, facilityId := range isolated {
		alerts = append(alerts, "ISOLATION: "+eng.nodes[facilityId].Name+" has no comms or road path to command")
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

func categoryFor(nodeType NodeType) EventCategory {
	switch nodeType {
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

func contains(list []string, value string) bool {
	for _, entry := range list {
		if entry == value {
			return true
		}
	}
	return false
}

func (eng *Engine) collectListenersLocked() []func(*SimulationState) {
	out := make([]func(*SimulationState), 0, len(eng.listeners))
	for _, fn := range eng.listeners {
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

func (eng *Engine) publish() {
	eng.mu.Lock()
	eng.revision++
	state := eng.buildStateLocked()
	listeners := eng.collectListenersLocked()
	eng.mu.Unlock()
	notify(listeners, state)
}

func (eng *Engine) Subscribe(fn func(*SimulationState)) int {
	eng.mu.Lock()
	defer eng.mu.Unlock()
	eng.nextSub++
	id := eng.nextSub
	eng.listeners[id] = fn
	return id
}

func (eng *Engine) Unsubscribe(id int) {
	eng.mu.Lock()
	defer eng.mu.Unlock()
	delete(eng.listeners, id)
}

func (eng *Engine) Snapshot() *SimulationState {
	eng.mu.RLock()
	defer eng.mu.RUnlock()
	return eng.buildStateLocked().Clone()
}

func (eng *Engine) InjectNodeFailure(targetID string) (string, error) {
	eng.mu.Lock()
	n, ok := FailNode(eng.nodes, eng.conns, targetID, SourceOperator)
	if !ok {
		current := eng.nodes[targetID]
		if current == nil {
			eng.mu.Unlock()
			return "", fmt.Errorf("unknown node %q", targetID)
		}
		eng.logEvent(EventWarning, CatCommand, "COMMAND", targetID, n.Name+" already offline", 0)
		eng.mu.Unlock()
		eng.publish()
		return n.ID, nil
	}
	eng.logEvent(EventFailure, categoryFor(n.Type), "OPERATOR", n.ID,
		fmt.Sprintf("%s taken offline by operator command (%s)", n.Name, n.Type), 0)
	graph := NewDependencyGraph(eng.nodes)
	failures := graph.EvaluateDependencies(eng.nodes, eng.conns, eng.hz.Flooded)
	for _, failure := range failures {
		fn := eng.nodes[failure.ID]
		eng.logEvent(EventCascade, categoryFor(fn.Type), string(SourceCascade), fn.ID,
			fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", fn.Name, failure.Reason, failure.Depth), failure.Depth)
	}
	eng.applyRoutingLocked()
	eng.mu.Unlock()
	eng.publish()
	return n.ID, nil
}

func (eng *Engine) InjectRoadBlock(connID string) (string, error) {
	eng.mu.Lock()
	c, changed := BlockConnection(eng.conns, connID)
	if !changed {
		if c == nil {
			eng.mu.Unlock()
			return "", fmt.Errorf("unknown connection %q", connID)
		}
		eng.logEvent(EventWarning, CatRoad, "COMMAND", connID, c.ID+" already blocked", 0)
		eng.mu.Unlock()
		eng.publish()
		return c.ID, nil
	}
	eng.logEvent(EventFailure, CatRoad, "OPERATOR", connID,
		fmt.Sprintf("Road segment %s blocked (obstruction reported)", connID), 0)
	eng.applyRoutingLocked()
	eng.mu.Unlock()
	eng.publish()
	return c.ID, nil
}

func (eng *Engine) RestoreNodeByID(targetID string) (string, error) {
	eng.mu.Lock()
	n, repaired, ok := RestoreNode(eng.nodes, eng.conns, eng.graph, targetID)
	if !ok {
		if n == nil {
			eng.mu.Unlock()
			return "", fmt.Errorf("unknown node %q", targetID)
		}
		blocking := blockingPrerequisites(eng.graph, eng.nodes, targetID)
		eng.logEvent(EventWarning, CatCommand, "COMMAND", targetID,
			fmt.Sprintf("Restore of %s denied: prerequisite %s still offline", n.Name, blocking[0]), 0)
		eng.mu.Unlock()
		eng.publish()
		return n.ID, nil
	}
	eng.logEvent(EventSuccess, categoryFor(n.Type), "OPERATOR", n.ID,
		fmt.Sprintf("%s restored to service; %d links re-energised", n.Name, len(repaired)), 0)
	graph := NewDependencyGraph(eng.nodes)
	for _, verdict := range graph.EvaluateDependencies(eng.nodes, eng.conns, eng.hz.Flooded) {
		fn := eng.nodes[verdict.ID]
		eng.logEvent(EventSuccess, categoryFor(fn.Type), string(SourceCascade), fn.ID,
			fmt.Sprintf("%s back online after prerequisite restoration", fn.Name), verdict.Depth)
	}
	eng.applyRoutingLocked()
	eng.mu.Unlock()
	eng.publish()
	return n.ID, nil
}

func (eng *Engine) ApplyPreset(id string) error {
	eng.mu.Lock()
	err := eng.applyPresetLocked(id)
	eng.mu.Unlock()
	if err != nil {
		return err
	}
	eng.publish()
	return nil
}

func (eng *Engine) ApplyPresets(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	eng.mu.Lock()
	var errs []string
	for _, id := range ids {
		if err := eng.applyPresetLocked(id); err != nil {
			errs = append(errs, err.Error())
		}
	}
	eng.logEvent(EventWarning, CatCommand, "OPERATOR", "",
		fmt.Sprintf("STACKED %d EVENT(S): %s", len(ids), strings.Join(ids, " + ")), 0)
	eng.mu.Unlock()
	eng.publish()
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (eng *Engine) applyPresetLocked(id string) error {
	var preset *Preset
	for i := range eng.scenario.Presets {
		if eng.scenario.Presets[i].ID == id {
			preset = &eng.scenario.Presets[i]
			break
		}
	}
	if preset == nil {
		return fmt.Errorf("unknown preset %q", id)
	}
	kind := preset.Kind
	target := preset.TargetID
	eng.logEvent(EventWarning, CatCommand, "OPERATOR", target,
		fmt.Sprintf("PRESET ACTIVATED: %s (%s)", preset.Label, preset.Description), 0)
	switch kind {
	case "HAZARD":
		ActivateHurricane(eng.hz, eng.conns, eng.tick, preset.Intensity)
		graph := NewDependencyGraph(eng.nodes)
		for _, verdict := range graph.EvaluateDependencies(eng.nodes, eng.conns, eng.hz.Flooded) {
			fn := eng.nodes[verdict.ID]
			eng.logEvent(EventCascade, categoryFor(fn.Type), string(SourceCascade), fn.ID,
				fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", fn.Name, verdict.Reason, verdict.Depth), verdict.Depth)
		}
		eng.applyRoutingLocked()
	case "NODE_FAILURE":
		ids := strings.Split(target, ",")
		changed := false
		for _, raw := range ids {
			tid := strings.TrimSpace(raw)
			if tid == "" {
				continue
			}
			n, ok := FailNode(eng.nodes, eng.conns, tid, SourceOperator)
			if n == nil {
				return fmt.Errorf("unknown preset target %q", tid)
			}
			if ok {
				changed = true
				eng.logEvent(EventFailure, categoryFor(n.Type), "OPERATOR", n.ID,
					fmt.Sprintf("%s taken offline by preset %s", n.Name, preset.Label), 0)
			}
		}
		if !changed {
			return nil
		}
		graph := NewDependencyGraph(eng.nodes)
		for _, verdict := range graph.EvaluateDependencies(eng.nodes, eng.conns, eng.hz.Flooded) {
			fn := eng.nodes[verdict.ID]
			eng.logEvent(EventCascade, categoryFor(fn.Type), string(SourceCascade), fn.ID,
				fmt.Sprintf("%s OFFLINE via %s (cascade depth %d)", fn.Name, verdict.Reason, verdict.Depth), verdict.Depth)
		}
		eng.applyRoutingLocked()
	case "ROAD_BLOCK":
		if c, changed := BlockConnection(eng.conns, target); changed {
			eng.logEvent(EventFailure, CatRoad, "OPERATOR", target,
				fmt.Sprintf("Road segment %s flooded by preset %s", target, preset.Label), 0)
			_ = c
		}
		eng.applyRoutingLocked()
	case "FLOOD_ZONE":
		ids := make([]string, 0, len(eng.conns))
		for id := range eng.conns {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		sealed := 0
		for _, id := range ids {
			c := eng.conns[id]
			if c.Type != EdgeRoad || c.Blocked || c.Risk() < 0.6 {
				continue
			}
			c.Blocked = true
			c.Active = false
			c.Flooded = true
			c.Failures++
			eng.hz.Flooded[id] = true
			eng.logEvent(EventFailure, CatRoad, "OPERATOR", id,
				fmt.Sprintf("Flooding seals %s (flood risk %.0f%%)", id, c.Risk()*100), 1)
			sealed++
		}
		eng.applyRoutingLocked()
	}
	return nil
}

func (eng *Engine) Reset() {
	eng.mu.Lock()
	if eng.running {
		eng.running = false
		close(eng.stop)
		<-eng.done
	}
	eng.loadLocked()
	eng.logEvent(EventInfo, CatCommand, "COMMAND", "",
		fmt.Sprintf("RESET to T+00:00 :: %s", eng.scenario.Name), 0)
	eng.mu.Unlock()
	eng.publish()
}
