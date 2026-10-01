package simulation

import (
	"fmt"
	"sort"
)

type DependencyGraph struct {
	Dependents    map[string][]string
	Prerequisites map[string][]string
	Order         []string
	Cyclic        []string
	Criticality   map[string]int
}

func NewDependencyGraph(nodes map[string]*InfrastructureNode) *DependencyGraph {
	g := &DependencyGraph{
		Dependents:    make(map[string][]string, len(nodes)),
		Prerequisites: make(map[string][]string, len(nodes)),
		Criticality:   make(map[string]int, len(nodes)),
	}
	for id := range nodes {
		g.Dependents[id] = []string{}
		g.Prerequisites[id] = []string{}
	}
	for id, node := range nodes {
		seen := make(map[string]bool, len(node.Dependencies))
		prereqs := make([]string, 0, len(node.Dependencies))
		for _, dep := range node.Dependencies {
			if dep == id || seen[dep] {
				continue
			}
			seen[dep] = true
			prereqs = append(prereqs, dep)
		}
		g.Prerequisites[id] = prereqs
		for _, dep := range prereqs {
			g.Dependents[dep] = append(g.Dependents[dep], id)
		}
	}
	g.Order, g.Cyclic = g.topologicalOrder()
	g.computeCriticality()
	return g
}
func (graph *DependencyGraph) topologicalOrder() (order []string, cyclic []string) {
	indeg := make(map[string]int, len(graph.Prerequisites))
	for id, prereqs := range graph.Prerequisites {
		indeg[id] = len(prereqs)
	}
	ready := make([]string, 0, len(indeg))
	for nodeId, inDegree := range indeg {
		if inDegree == 0 {
			ready = append(ready, nodeId)
		}
	}
	sort.Strings(ready)
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		deps := append([]string(nil), graph.Dependents[id]...)
		sort.Strings(deps)
		for _, dep := range deps {
			indeg[dep]--
			if indeg[dep] == 0 {
				ready = append(ready, dep)
			}
		}
		sort.Strings(ready)
	}
	remaining := make([]string, 0)
	for nodeId, inDegree := range indeg {
		if inDegree > 0 {
			remaining = append(remaining, nodeId)
		}
	}
	sort.Strings(remaining)
	return order, remaining
}
func (graph *DependencyGraph) computeCriticality() {
	ids := make([]string, 0, len(graph.Prerequisites))
	for id := range graph.Prerequisites {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, root := range ids {
		visited := make(map[string]bool, len(ids))
		queue := append([]string(nil), graph.Dependents[root]...)
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if visited[cur] {
				continue
			}
			visited[cur] = true
			queue = append(queue, graph.Dependents[cur]...)
		}
		graph.Criticality[root] = len(visited)
	}
}
func (graph *DependencyGraph) Reachable(from, to string) bool {
	visited := map[string]bool{}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == to && cur != from {
			return true
		}
		if visited[cur] {
			continue
		}
		visited[cur] = true
		queue = append(queue, graph.Dependents[cur]...)
	}
	return false
}

type depClass int

const (
	depEssential depClass = iota
	depRedundant
)

type dependencyVerdict struct {
	ID        string
	Satisfied bool
	Reason    string
	Depth     int
	Causes    []string
}

func classify(nodes map[string]*InfrastructureNode, prereqs []string) (essential, redundant []string) {
	var power, traffic, comms, other []string
	for _, prereqId := range prereqs {
		node, ok := nodes[prereqId]
		if !ok {
			continue
		}
		switch node.Type {
		case NodeCellTower, NodeRadioMesh:
			comms = append(comms, prereqId)
		case NodeRoadIntersection:
			traffic = append(traffic, prereqId)
		case NodePowerSubstation:
			power = append(power, prereqId)
		default:
			other = append(other, prereqId)
		}
	}
	essential = append(essential, power...)
	essential = append(essential, other...)
	if len(comms) > 0 {
		if len(comms) == 1 {
			essential = append(essential, comms[0])
		} else {
			redundant = append(redundant, comms...)
		}
	}
	if len(traffic) > 0 {
		if len(traffic) == 1 {
			essential = append(essential, traffic[0])
		} else {
			redundant = append(redundant, traffic...)
		}
	}
	return
}
func (graph *DependencyGraph) EvaluateDependencies(
	nodes map[string]*InfrastructureNode,
	connections map[string]*Connection,
	floodZone map[string]bool,
) []dependencyVerdict {
	verdicts := make([]dependencyVerdict, 0, len(nodes))
	failures := make([]dependencyVerdict, 0)
	inducedBlocks := make([]string, 0)
	depths := make(map[string]int, len(nodes))
	up := func(id string) bool {
		n, ok := nodes[id]
		return ok && n.Operational
	}
	depthOf := func(id string) int {
		if d, ok := depths[id]; ok {
			return d
		}
		n, ok := nodes[id]
		if !ok {
			return 0
		}
		return n.CascadeDep
	}
	for _, id := range graph.Order {
		node := nodes[id]
		if node == nil {
			continue
		}
		essential, redundant := classify(nodes, graph.Prerequisites[id])
		v := dependencyVerdict{ID: id, Satisfied: true}
		var causeDepth int
		record := func(reason string, cause string) {
			v.Satisfied = false
			v.Reason = reason
			v.Causes = append(v.Causes, cause)
			if d := depthOf(cause) + 1; d > causeDepth {
				causeDepth = d
			}
		}
		for _, prereqId := range essential {
			if !up(prereqId) {
				record(reasonFor(nodes[prereqId].Type), prereqId)
			}
		}
		if v.Satisfied && len(redundant) > 0 {
			anyUp := false
			for _, prereqId := range redundant {
				if up(prereqId) {
					anyUp = true
					break
				}
			}
			if !anyUp {
				reason, cause := worstRedundantCause(nodes, redundant)
				record(reason, cause)
			}
		}
		if !v.Satisfied {
			v.Depth = causeDepth
			if v.Depth < 1 {
				v.Depth = 1
			}
			depths[id] = v.Depth
			if node.Operational {
				node.Operational = false
				node.Health = 0
				node.CascadeDep = v.Depth
				node.Reason = v.Reason
				failures = append(failures, v)
			}
		}
		verdicts = append(verdicts, v)
	}
	for pass := 0; pass < len(graph.Cyclic); pass++ {
		progressed := false
		for _, id := range graph.Cyclic {
			node := nodes[id]
			if node == nil || !node.Operational {
				continue
			}
			essential, redundant := classify(nodes, graph.Prerequisites[id])
			down := false
			var causeDepth int
			for _, prereqId := range essential {
				if !up(prereqId) {
					down = true
					if d := depthOf(prereqId) + 1; d > causeDepth {
						causeDepth = d
					}
				}
			}
			if !down && len(redundant) > 0 {
				anyUp := false
				for _, prereqId := range redundant {
					if up(prereqId) {
						anyUp = true
						break
					}
				}
				if !anyUp {
					down = true
					causeDepth = 1
				}
			}
			if down {
				if causeDepth < 1 {
					causeDepth = 1
				}
				node.Operational = false
				node.Health = 0
				node.CascadeDep = causeDepth
				node.Reason = "CYCLE_DEPENDENCY"
				depths[id] = causeDepth
				failures = append(failures, dependencyVerdict{ID: id, Reason: "CYCLE_DEPENDENCY", Depth: causeDepth})
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	for _, failure := range failures {
		node := nodes[failure.ID]
		if node == nil || node.Type != NodeRoadIntersection {
			continue
		}
		for _, eID := range controlledSegments(connections, failure.ID) {
			conn, ok := connections[eID]
			if !ok || conn.Blocked {
				continue
			}
			if !floodZone[eID] && conn.Risk() < 0.5 {
				continue
			}
			conn.Blocked = true
			conn.Active = false
			conn.Failures++
			inducedBlocks = append(inducedBlocks, eID)
		}
	}
	_ = inducedBlocks
	_ = verdicts
	return failures
}
func reasonFor(nodeType NodeType) string {
	switch nodeType {
	case NodePowerSubstation:
		return "POWER_LOSS"
	case NodeCellTower:
		return "COMMS_BACKHAUL_LOST"
	case NodeRadioMesh:
		return "MESH_RELAY_LOST"
	case NodeRoadIntersection:
		return "TRAFFIC_CONTROL_LOSS"
	default:
		return "DEPENDENCY_LOSS"
	}
}
func worstRedundantCause(nodes map[string]*InfrastructureNode, ids []string) (reason, cause string) {
	priority := map[string]int{
		"POWER_LOSS": 4, "TRAFFIC_CONTROL_LOSS": 3,
		"COMMS_BACKHAUL_LOST": 2, "MESH_RELAY_LOST": 1,
	}
	reason, cause = "DEPENDENCY_LOSS", ids[0]
	best := -1
	for _, id := range ids {
		node, ok := nodes[id]
		if !ok {
			continue
		}
		r := reasonFor(node.Type)
		if p, ok := priority[r]; ok && p > best {
			best, reason, cause = p, r, id
		}
	}
	return reason, cause
}
func controlledSegments(connections map[string]*Connection, intersectionID string) []string {
	out := make([]string, 0, 4)
	for id, conn := range connections {
		if conn.Type != EdgeRoad {
			continue
		}
		owner, _ := conn.Metadata["controlled_by"].(string)
		if owner == intersectionID {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
func DependencyEdgeList(nodes map[string]*InfrastructureNode, conns map[string]*Connection) []DependencyEdge {
	out := make([]DependencyEdge, 0, len(conns))
	for id := range nodes {
		for _, node := range nodes[id].Dependencies {
			if _, ok := nodes[node]; !ok {
				continue
			}
			out = append(out, DependencyEdge{
				ID:     "dep:" + node + "->" + id,
				Source: node,
				Target: id,
				Kind:   "DEPENDENCY",
				Active: nodes[node].Operational && nodes[id].Operational,
			})
		}
	}
	for id, conn := range conns {
		if _, ok := nodes[conn.From]; !ok {
			continue
		}
		if _, ok := nodes[conn.To]; !ok {
			continue
		}
		active := conn.Active && !conn.Blocked && nodes[conn.From].Operational && nodes[conn.To].Operational
		out = append(out, DependencyEdge{
			ID:     "conn:" + id,
			Source: conn.From,
			Target: conn.To,
			Kind:   string(conn.Type),
			Active: active,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func Describe(node *InfrastructureNode, depth int) string {
	verb := "LOST"
	if depth >= 2 {
		verb = "CASCADE"
	}
	return fmt.Sprintf("%s %s | %s (depth %d)", verb, node.Name, node.Reason, depth)
}
