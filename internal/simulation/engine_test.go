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
	for _, node := range sc.Nodes {
		if node.Type.IsFacility() {
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
	for _, node := range sc.Nodes {
		counts[node.Type]++
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
	sim := NewEngine(loadSeed(t))
	st := sim.Snapshot()
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
	sim := NewEngine(loadSeed(t))
	before := sim.Snapshot()
	routeBefore := append([]string(nil), before.ActiveEMSRoutes["fire-02"]...)
	if len(routeBefore) == 0 {
		t.Fatal("expected a baseline route for fire-02")
	}
	if routeBefore[len(routeBefore)-1] != "hospital-01" {
		t.Fatalf("baseline fire-02 route does not terminate at RWJ University Hospital: %v", routeBefore)
	}
	usedArtery := false
	for i := 0; i+1 < len(routeBefore); i++ {
		if routeBefore[i] == "intersection-02" && routeBefore[i+1] == "intersection-01" {
			usedArtery = true
		}
	}
	if !usedArtery {
		t.Fatalf("baseline fire-02 route does not use the NJ-27 / Hermann Road artery: %v", routeBefore)
	}

	if _, err := sim.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	st := sim.Snapshot()

	for _, id := range []string{"tower-04", "intersection-03", "hospital-01"} {
		if st.Nodes[id].Operational {
			t.Errorf("%s still operational after substation-01 loss", id)
		}
	}
	if st.Connections["road-04"].Blocked != true {
		t.Error("road-04 (US-130 Bypass at Cozzens Lane) was not blocked by the intersection-03 cascade")
	}
	routeAfter := st.ActiveEMSRoutes["fire-02"]
	if samePath(routeBefore, routeAfter) {
		t.Errorf("fire-02 route did not reroute: still %v", routeAfter)
	}
	for _, hop := range routeAfter {
		if hop == "intersection-03" {
			t.Errorf("rerouted fire-02 path still crosses the failed junction: %v", routeAfter)
		}
	}
	if routeAfter[len(routeAfter)-1] == "hospital-01" {
		t.Errorf("fire-02 still routed to the hospital that lost power: %v", routeAfter)
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
	sim := NewEngine(loadSeed(t))
	if _, err := sim.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := sim.RestoreNodeByID("substation-01"); err != nil {
		t.Fatal(err)
	}
	st := sim.Snapshot()
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
	sim := NewEngine(loadSeed(t))
	if _, err := sim.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := sim.RestoreNodeByID("tower-04"); err != nil {
		t.Fatal(err)
	}
	st := sim.Snapshot()
	if st.Nodes["tower-04"].Operational {
		t.Error("tower-04 revived while its only power prerequisite was still offline")
	}
	if !strings.Contains(st.Nodes["tower-04"].Reason, "AWAITING_PREREQ") {
		t.Errorf("unexpected reason %q", st.Nodes["tower-04"].Reason)
	}
}

func TestRoadBlockReroute(t *testing.T) {
	sim := NewEngine(loadSeed(t))
	before := sim.Snapshot().ActiveEMSRoutes["fire-02"]
	if _, err := sim.InjectRoadBlock("road-01"); err != nil {
		t.Fatal(err)
	}
	after := sim.Snapshot().ActiveEMSRoutes["fire-02"]
	if samePath(before, after) {
		t.Error("blocking road-01 did not change the fire-02 route")
	}
}

func TestRoutesNeverTraverseFailedJunctions(t *testing.T) {
	sim := NewEngine(loadSeed(t))
	if _, err := sim.InjectNodeFailure("substation-01"); err != nil {
		t.Fatal(err)
	}
	snap := sim.Snapshot()
	for _, id := range []string{"tower-04", "intersection-03"} {
		n, ok := snap.Nodes[id]
		if !ok {
			t.Fatalf("expected node %s in snapshot", id)
		}
		if n.Operational {
			t.Fatalf("test precondition: %s should be failed after the substation-01 cascade", id)
		}
	}
	for _, route := range snap.Diagnostics.EmsRoutes {
		for _, node := range route.Nodes {
			if n, ok := snap.Nodes[node]; ok && n.Type == NodeRoadIntersection && !n.Operational {
				t.Errorf("route %s traverses failed junction %s", route.ID, node)
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
	sim := NewEngine(loadSeed(t))
	if err := sim.ApplyPreset("PRESET-HURRICANE"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		sim.TickOnce()
	}
	st := sim.Snapshot()
	if st.PowerGridHealth >= 100 {
		t.Error("hurricane produced no grid degradation")
	}
	if len(st.Diagnostics.BlockedEdges) == 0 {
		t.Error("hurricane blocked no road segments")
	}
	if st.RoadAccessibility >= 100 {
		t.Error("road accessibility did not degrade under flooding")
	}
}

func TestTopologicalOrderIsComplete(t *testing.T) {
	sc := loadSeed(t)
	nodes := map[string]*InfrastructureNode{}
	for _, node := range sc.Nodes {
		c := *node
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
		for _, prereqId := range g.Prerequisites[id] {
			if pos[prereqId] > pos[id] {
				t.Errorf("node %s ordered before its prerequisite %s", id, prereqId)
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
	for _, node := range sc.Nodes {
		c := *node
		nodes[c.ID] = &c
	}
	for _, cn := range sc.Connections {
		c := *cn
		conns[c.ID] = &c
	}
	adj := BuildRoadAdjacency(nodes, conns)
	viaArtery := AstarRoad(nodes, adj, "fire-02", "hospital-01")
	if viaArtery == nil {
		t.Fatal("no baseline road route fire-02 -> hospital-01")
	}
	conns["road-02"].Blocked = true
	conns["road-02"].Active = false
	adj2 := BuildRoadAdjacency(nodes, conns)
	detour := AstarRoad(nodes, adj2, "fire-02", "hospital-01")
	if detour == nil {
		t.Fatal("no detour available once Jersey Avenue is blocked")
	}
	if samePath(viaArtery.Nodes, detour.Nodes) {
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
	for _, node := range sc.Nodes {
		c := *node
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
	sim := NewEngine(loadSeed(t))
	a := sim.Snapshot()
	a.Nodes["substation-01"].Health = 0
	a.Nodes["substation-01"].Operational = false
	b := sim.Snapshot()
	if !b.Nodes["substation-01"].Operational {
		t.Error("mutating a snapshot leaked into engine state")
	}
}

func TestSnapshotEmitsEmptyArraysNotNull(t *testing.T) {
	sc := loadSeed(t)
	sim := NewEngine(sc)
	raw, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Nodes map[string]struct {
			Dependencies []string `json:"dependencies"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	if len(probe.Nodes) == 0 {
		t.Fatal("expected nodes in snapshot")
	}
	for id, node := range probe.Nodes {
		if node.Dependencies == nil {
			t.Fatalf("node %q serialized dependencies as null", id)
		}
	}
	if strings.Contains(string(raw), `"dependencies":null`) {
		t.Fatal("snapshot contains a null dependencies field")
	}
}

func TestStackedPresetsCompound(t *testing.T) {
	sc := loadSeed(t)
	sim := NewEngine(sc)
	base := sim.Snapshot()

	if err := sim.ApplyPresets([]string{"PRESET-SUB01"}); err != nil {
		t.Fatal(err)
	}
	one := sim.Snapshot()
	if one.Nodes["substation-01"].Operational {
		t.Fatal("substation-01 should be offline after PRESET-SUB01")
	}

	if err := sim.ApplyPresets([]string{"PRESET-TOWER04", "PRESET-INT03", "PRESET-FLOOD-ZONE"}); err != nil {
		t.Fatal(err)
	}
	stacked := sim.Snapshot()

	down := func(s *SimulationState) int {
		n := 0
		for _, node := range s.Nodes {
			if !node.Operational {
				n++
			}
		}
		return n
	}
	if got, prev := down(stacked), down(one); got < prev {
		t.Fatalf("stacking did not compound: %d down after stack vs %d after first preset", got, prev)
	}
	if down(base) != 0 {
		t.Fatalf("baseline should be fully operational, got %d down", down(base))
	}
	if stacked.Nodes["tower-04"].Operational {
		t.Fatal("tower-04 should be offline after stacking")
	}
	if stacked.Nodes["intersection-03"].Operational {
		t.Fatal("intersection-03 should be offline after stacking")
	}
	if len(stacked.Diagnostics.BlockedEdges) <= len(one.Diagnostics.BlockedEdges) {
		t.Fatal("FLOOD-ZONE stacking should add blocked edges")
	}
}

func TestApplyPresetsRejectsUnknownWithoutAbortingBatch(t *testing.T) {
	sc := loadSeed(t)
	sim := NewEngine(sc)
	err := sim.ApplyPresets([]string{"PRESET-SUB01", "NOPE-NOT-REAL", "PRESET-TOWER04"})
	if err == nil {
		t.Fatal("expected an error for the unknown preset")
	}
	s := sim.Snapshot()
	if s.Nodes["substation-01"].Operational || s.Nodes["tower-04"].Operational {
		t.Fatal("valid presets on either side of a bad id should still apply")
	}
}
