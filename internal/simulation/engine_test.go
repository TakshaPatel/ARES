package simulation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadSeed(t *testing.T) *Scenario {
	t.Helper()
	raw, err := os.ReadFile("../../internal/scenario/seed/hurricaneScenario.json")
	if err != nil {
		t.Fatal(err)
	}
	var sc Scenario
	if err := json.Unmarshal(raw, &sc); err != nil {
		t.Fatal(err)
	}
	sc.Normalize()
	return &sc
}

func TestScenarioShape(t *testing.T) {
	sc := loadSeed(t)
	if got := len(sc.Nodes); got != 30 {
		t.Fatalf("nodes = %d, want 30", got)
	}
	facilities, infra := 0, 0
	for _, n := range sc.Nodes {
		if n.Type.IsFacility() {
			facilities++
		} else {
			infra++
		}
	}
	if infra != 18 {
		t.Errorf("infrastructure nodes = %d, want 18", infra)
	}
	if facilities != 12 {
		t.Errorf("facilities = %d, want 12", facilities)
	}
	counts := map[NodeType]int{}
	for _, n := range sc.Nodes {
		counts[n.Type]++
	}
	if counts[NodePowerSubstation] != 4 {
		t.Errorf("substations = %d, want 4", counts[NodePowerSubstation])
	}
	if counts[NodeCellTower]+counts[NodeRadioMesh] != 8 {
		t.Errorf("comms nodes = %d, want 8", counts[NodeCellTower]+counts[NodeRadioMesh])
	}
	if counts[NodeRoadIntersection] != 6 {
		t.Errorf("intersections = %d, want 6", counts[NodeRoadIntersection])
	}
	if counts[NodeHospital] < 2 || counts[NodeFireStation] < 3 || counts[NodeEOC] != 1 {
		t.Errorf("primary facility mix unexpected: %v", counts)
	}
}

func TestBaselineIsHealthy(t *testing.T) {
	e := NewEngine(loadSeed(t))
	st := e.Snapshot()
	if st.PowerGridHealth < 99 {
		t.Errorf("baseline power health = %.2f, want ~100", st.PowerGridHealth)
	}
	if st.CommsCoverage < 99 {
		t.Errorf("baseline comms coverage = %.2f, want ~100", st.CommsCoverage)
	}
	if st.MessageDeliveryRate < 90 {
		t.Errorf("baseline delivery = %.2f, want >= 90", st.MessageDeliveryRate)
	}
	if len(st.IsolatedFacilities) != 0 {
		t.Errorf("baseline isolated = %v, want none", st.IsolatedFacilities)
	}
	if st.ConnectedFacilities != st.TotalFacilities {
		t.Errorf("baseline connected %d/%d", st.ConnectedFacilities, st.TotalFacilities)
	}
	if len(st.ActiveEMSRoutes) == 0 {
		t.Error("baseline produced no EMS routes")
	}
}

func TestSubstation01Cascade(t *testing.T) {
	e := NewEngine(loadSeed(t))
	before := e.Snapshot()
	routeBefore := append([]string(nil), before.ActiveEMSRoutes["fire-02"]...)
	if len(routeBefore) == 0 {
		t.Fatal("expected a baseline route for fire-02")
	}
	usedBridge := false
	for i := 0; i+1 < len(routeBefore); i++ {
		if routeBefore[i] == "intersection-02" && routeBefore[i+1] == "intersection-05" {
			usedBridge = true
		}
	}
	if !usedBridge {
		t.Fatalf("baseline fire-02 route does not use the bay bridge: %v", routeBefore)
	}

	if _, err := e.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	st := e.Snapshot()

	for _, id := range []string{"tower-04", "intersection-03"} {
		if st.Nodes[id].Operational {
			t.Errorf("%s still operational after substation-01 loss", id)
		}
	}
	if st.Connections["road-17"].Blocked != true {
		t.Error("road-17 (Bay Bridge) was not blocked by the intersection-03 cascade")
	}
	routeAfter := st.ActiveEMSRoutes["fire-02"]
	if samePath(routeBefore, routeAfter) {
		t.Errorf("fire-02 route did not reroute: still %v", routeAfter)
	}
	stillBridge := false
	for i := 0; i+1 < len(routeAfter); i++ {
		if routeAfter[i] == "intersection-02" && routeAfter[i+1] == "intersection-05" {
			stillBridge = true
		}
	}
	if stillBridge {
		t.Errorf("rerouted fire-02 path still crosses the blocked bridge: %v", routeAfter)
	}

	var maxDepth int
	sawCascade := false
	for _, ev := range st.Diagnostics.Events {
		if ev.Depth > maxDepth {
			maxDepth = ev.Depth
		}
		if ev.Severity == EventCascade && ev.Source == string(SourceCascade) {
			sawCascade = true
		}
	}
	if !sawCascade {
		t.Error("no cascade events logged")
	}
	if maxDepth < 2 {
		t.Errorf("expected multi-hop cascade, max depth = %d", maxDepth)
	}
}

func TestRestoreUnblocksPrerequisite(t *testing.T) {
	e := NewEngine(loadSeed(t))
	if _, err := e.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RestoreNodeByID("substation-01"); err != nil {
		t.Fatal(err)
	}
	st := e.Snapshot()
	if !st.Nodes["substation-01"].Operational {
		t.Fatal("substation-01 not operational after restore")
	}
	if st.Nodes["tower-04"].Operational {
		t.Error("tower-04 did not recover after its prerequisite was restored")
	}
	if st.Nodes["intersection-03"].Operational {
		t.Error("intersection-03 did not recover after its prerequisite was restored")
	}
}

func TestRestoreDeniedWhilePrereqDown(t *testing.T) {
	e := NewEngine(loadSeed(t))
	if _, err := e.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RestoreNodeByID("tower-04"); err != nil {
		t.Fatal(err)
	}
	st := e.Snapshot()
	if st.Nodes["tower-04"].Operational {
		t.Error("tower-04 revived while its only power prerequisite was still offline")
	}
	if !strings.Contains(st.Nodes["tower-04"].Reason, "AWAITING_PREREQ") {
		t.Errorf("unexpected reason %q", st.Nodes["tower-04"].Reason)
	}
}

func TestRoadBlockReroute(t *testing.T) {
	e := NewEngine(loadSeed(t))
	before := e.Snapshot().ActiveEMSRoutes["fire-02"]
	if _, err := e.InjectRoadBlock("road-17"); err != nil {
		t.Fatal(err)
	}
	after := e.Snapshot().ActiveEMSRoutes["fire-02"]
	if samePath(before, after) {
		t.Error("blocking road-17 did not change the fire-02 route")
	}
}

func TestRoutesNeverTraverseFailedJunctions(t *testing.T) {
	e := NewEngine(loadSeed(t))
	if _, err := e.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	snap := e.Snapshot()
	for _, id := range []string{"tower-04", "intersection-03"} {
		n, ok := snap.Nodes[id]
		if !ok {
			t.Fatalf("expected node %s in snapshot", id)
		}
		if n.Operational {
			t.Fatalf("test precondition: %s should be failed after the substation-01 cascade", id)
		}
	}
	for _, r := range snap.Diagnostics.EmsRoutes {
		for _, hop := range r.Nodes {
			if n, ok := snap.Nodes[hop]; ok && n.Type == NodeRoadIntersection && !n.Operational {
				t.Errorf("route %s traverses failed junction %s", r.ID, hop)
			}
		}
	}
	for origin, path := range snap.ActiveEMSRoutes {
		for _, hop := range path {
			if n, ok := snap.Nodes[hop]; ok && n.Type == NodeRoadIntersection && !n.Operational {
				t.Errorf("active route from %s traverses failed junction %s", origin, hop)
			}
		}
	}
}

func TestHurricaneProgressesAndIsolates(t *testing.T) {
	e := NewEngine(loadSeed(t))
	if err := e.ApplyPreset("PRESET-HURRICANE"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		e.TickOnce()
	}
	st := e.Snapshot()
	if st.PowerGridHealth >= 100 {
		t.Error("hurricane produced no grid degradation")
	}
	if len(st.Diagnostics.BlockedEdges) == 0 {
		t.Error("hurricane blocked no road segments")
	}
	if st.RoadAccessibility >= 100 {
		t.Error("road accessibility did not degrade under storm surge")
	}
}

func TestTopologicalOrderIsComplete(t *testing.T) {
	sc := loadSeed(t)
	nodes := map[string]*InfrastructureNode{}
	for _, n := range sc.Nodes {
		c := *n
		nodes[c.ID] = &c
	}
	g := NewDependencyGraph(nodes)
	if len(g.Order)+len(g.Cyclic) != len(nodes) {
		t.Fatalf("topological order lost nodes: %d ordered + %d cyclic != %d",
			len(g.Order), len(g.Cyclic), len(nodes))
	}
	pos := map[string]int{}
	for i, id := range g.Order {
		pos[id] = i
	}
	for _, id := range g.Order {
		for _, p := range g.Prerequisites[id] {
			if pos[p] > pos[id] {
				t.Errorf("node %s ordered before its prerequisite %s", id, p)
			}
		}
	}
	for id := range nodes {
		direct := len(g.Dependents[id])
		if g.Criticality[id] < direct {
			t.Errorf("criticality for %s (%d) is below its direct dependent count (%d)",
				id, g.Criticality[id], direct)
		}
		reachable := 0
		for other := range nodes {
			if other != id && g.Reachable(id, other) {
				reachable++
			}
		}
		if g.Criticality[id] != reachable {
			t.Errorf("criticality for %s = %d but BFS reachability found %d",
				id, g.Criticality[id], reachable)
		}
	}
	if g.Criticality["substation-01"] < 6 {
		t.Errorf("substation-01 blast radius = %d, expected >= 6", g.Criticality["substation-01"])
	}
}

func TestCycleTerminates(t *testing.T) {
	nodes := map[string]*InfrastructureNode{
		"a": {ID: "a", Type: NodeCellTower, Operational: true, Health: 100, Dependencies: []string{"b"}},
		"b": {ID: "b", Type: NodeCellTower, Operational: true, Health: 100, Dependencies: []string{"c"}},
		"c": {ID: "c", Type: NodeCellTower, Operational: true, Health: 100, Dependencies: []string{"a"}},
	}
	g := NewDependencyGraph(nodes)
	if len(g.Cyclic) != 3 {
		t.Fatalf("expected 3 cyclic nodes, got %v", g.Cyclic)
	}
	done := make(chan struct{})
	go func() {
		g.EvaluateDependencies(nodes, map[string]*Connection{}, map[string]bool{})
		close(done)
	}()
	<-done
}

func TestAstarAvoidsBlockedSegments(t *testing.T) {
	sc := loadSeed(t)
	nodes := map[string]*InfrastructureNode{}
	conns := map[string]*Connection{}
	for _, n := range sc.Nodes {
		c := *n
		nodes[c.ID] = &c
	}
	for _, cn := range sc.Connections {
		c := *cn
		conns[c.ID] = &c
	}
	adj := BuildRoadAdjacency(nodes, conns)
	viaBridge := AstarRoad(nodes, adj, "fire-02", "hospital-02")
	if viaBridge == nil {
		t.Fatal("no baseline road route fire-02 -> hospital-02")
	}
	conns["road-17"].Blocked = true
	conns["road-17"].Active = false
	adj2 := BuildRoadAdjacency(nodes, conns)
	detour := AstarRoad(nodes, adj2, "fire-02", "hospital-02")
	if detour == nil {
		t.Fatal("no detour available once the bridge is blocked")
	}
	if samePath(viaBridge.Nodes, detour.Nodes) {
		t.Error("A* returned the blocked path")
	}
	finalisePath(adj2, detour)
	if detour.DistanceKM <= 0 {
		t.Error("detour distance not computed")
	}
}

func TestNetworkAnalysisFindsMeshFallback(t *testing.T) {
	sc := loadSeed(t)
	nodes := map[string]*InfrastructureNode{}
	conns := map[string]*Connection{}
	for _, n := range sc.Nodes {
		c := *n
		nodes[c.ID] = &c
	}
	for _, cn := range sc.Connections {
		c := *cn
		conns[c.ID] = &c
	}
	net := AnalyseNetwork(nodes, conns)
	if len(net.Clusters) == 0 {
		t.Fatal("no comms clusters discovered")
	}
	if !net.CommsReach["eoc-01"] {
		t.Error("EOC not marked as reachable")
	}
	if net.Latency["hospital-01"] <= 0 {
		t.Error("no latency computed for hospital-01")
	}

	_, _ = FailNode(nodes, conns, "tower-01", SourceOperator)
	net2 := AnalyseNetwork(nodes, conns)
	if net2.MeshBackoffSet["hospital-01"] && net2.Latency["hospital-01"] == 0 {
		t.Error("mesh fallback flagged but no alternate path exists")
	}
}

func TestStateCloneIsDeep(t *testing.T) {
	e := NewEngine(loadSeed(t))
	a := e.Snapshot()
	a.Nodes["substation-01"].Health = 0
	a.Nodes["substation-01"].Operational = false
	b := e.Snapshot()
	if !b.Nodes["substation-01"].Operational {
		t.Error("mutating a snapshot leaked into engine state")
	}
}
