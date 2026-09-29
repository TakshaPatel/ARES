package simulation

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
)

type RoadArc struct {
	ID       string
	From     string
	To       string
	Distance float64
	Minutes  float64
	Blocked  bool
	Flooded  bool
}
type RoadPath struct {
	Nodes      []string
	Segments   []string
	DistanceKM float64
	Minutes    float64
}

func (p *RoadPath) Full() bool { return p != nil && len(p.Nodes) > 0 }
func BuildRoadAdjacency(nodes map[string]*InfrastructureNode, conns map[string]*Connection) map[string][]RoadArc {
	adj := make(map[string][]RoadArc, len(nodes))
	add := func(c *Connection, from, to string) {
		if !c.Usable() {
			return
		}
		if c.MetaBool("one_way", false) && from != c.From {
			return
		}
		speed := c.Speed()
		if speed <= 0 {
			speed = 30
		}
		dist := c.Distance
		if dist <= 0 {
			dist = Haversine(nodes[from].Lat, nodes[from].Lng, nodes[to].Lat, nodes[to].Lng)
		}
		adj[from] = append(adj[from], RoadArc{
			ID: c.ID, From: from, To: to,
			Distance: dist,
			Minutes:  dist / speed * 60,
			Blocked:  c.Blocked,
			Flooded:  c.Flooded,
		})
	}
	ids := make([]string, 0, len(conns))
	for id := range conns {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := conns[id]
		if c.Type != EdgeRoad {
			continue
		}
		if _, ok := nodes[c.From]; !ok {
			continue
		}
		if _, ok := nodes[c.To]; !ok {
			continue
		}
		add(c, c.From, c.To)
		add(c, c.To, c.From)
	}
	for k := range adj {
		arcs := adj[k]
		sort.Slice(arcs, func(i, j int) bool { return arcs[i].To < arcs[j].To })
		adj[k] = arcs
	}
	return adj
}

type roadHeapItem struct {
	id    string
	f     float64
	g     float64
	steps int
}
type roadHeap []roadHeapItem

func (h roadHeap) Len() int           { return len(h) }
func (h roadHeap) Less(i, j int) bool { return h[i].f < h[j].f }
func (h roadHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *roadHeap) Push(x interface{}) {
	*h = append(*h, x.(roadHeapItem))
}
func (h *roadHeap) Pop() interface{} {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}
func junctionPassable(nodes map[string]*InfrastructureNode, id string) bool {
	n, ok := nodes[id]
	if !ok {
		return false
	}
	if n.Type == NodeRoadIntersection && !n.Operational {
		return false
	}
	return true
}

func AstarRoad(
	nodes map[string]*InfrastructureNode,
	adj map[string][]RoadArc,
	source, goal string,
) *RoadPath {
	if source == goal {
		if _, ok := nodes[source]; ok {
			return &RoadPath{Nodes: []string{source}}
		}
		return nil
	}
	if _, ok := nodes[goal]; !ok {
		return nil
	}
	maxSpeed := 1.0
	for _, arcs := range adj {
		for _, a := range arcs {
			if m := a.Minutes / math.Max(a.Distance, 0.001); m > 0 {
				if s := 60 / m; s > maxSpeed {
					maxSpeed = s
				}
			}
		}
	}
	if maxSpeed < 1 {
		maxSpeed = 50
	}
	h := func(id string) float64 {
		n, ok := nodes[id]
		if !ok {
			return 0
		}
		g, ok2 := nodes[goal]
		if !ok2 {
			return 0
		}
		return Haversine(n.Lat, n.Lng, g.Lat, g.Lng) / maxSpeed * 60
	}
	open := &roadHeap{{id: source, f: h(source), g: 0, steps: 0}}
	heap.Init(open)
	gScore := map[string]float64{source: 0}
	prevNode := map[string]string{}
	prevEdge := map[string]string{}
	closed := map[string]bool{}
	for open.Len() > 0 {
		cur := heap.Pop(open).(roadHeapItem)
		if closed[cur.id] {
			continue
		}
		if cur.id == goal {
			break
		}
		closed[cur.id] = true
		for _, arc := range adj[cur.id] {
			if closed[arc.To] || arc.Blocked {
				continue
			}
			if !junctionPassable(nodes, arc.To) {
				continue
			}
			cost := arc.Minutes
			ng := gScore[cur.id] + cost
			if old, ok := gScore[arc.To]; ok && ng >= old-1e-9 {
				continue
			}
			gScore[arc.To] = ng
			prevNode[arc.To] = cur.id
			prevEdge[arc.To] = arc.ID
			heap.Push(open, roadHeapItem{id: arc.To, g: ng, f: ng + h(arc.To), steps: cur.steps + 1})
		}
	}
	if _, ok := gScore[goal]; !ok {
		return nil
	}
	nodesPath := []string{goal}
	segments := []string{}
	cur := goal
	for {
		p, ok := prevNode[cur]
		if !ok {
			break
		}
		segments = append([]string{prevEdge[cur]}, segments...)
		nodesPath = append([]string{p}, nodesPath...)
		cur = p
	}
	return &RoadPath{Nodes: nodesPath, Segments: segments}
}
func finalisePath(adj map[string][]RoadArc, p *RoadPath) *RoadPath {
	if p == nil {
		return nil
	}
	for i := 0; i+1 < len(p.Nodes); i++ {
		from, to := p.Nodes[i], p.Nodes[i+1]
		for _, a := range adj[from] {
			if a.To == to {
				p.DistanceKM += a.Distance
				p.Minutes += a.Minutes
				break
			}
		}
	}
	return p
}

type RoutingResult struct {
	Routes      []EmsRoute
	Active      map[string][]string
	Changed     []EmsRoute
	Unreachable []string
	RoadReach   map[string]bool
}

func ComputeEMSRoutes(
	nodes map[string]*InfrastructureNode,
	conns map[string]*Connection,
	net *NetworkAnalysis,
	prior map[string][]string,
) *RoutingResult {
	adj := BuildRoadAdjacency(nodes, conns)
	res := &RoutingResult{Active: map[string][]string{}, RoadReach: roadReachability(nodes, adj)}
	var stations, hospitals []string
	for id, n := range nodes {
		switch n.Type {
		case NodeFireStation:
			stations = append(stations, id)
		case NodeHospital:
			hospitals = append(hospitals, id)
		}
	}
	sort.Strings(stations)
	sort.Strings(hospitals)
	// Every responding unit is dispatched to its fastest reachable hospital.
	// A hospital may receive inbound from several stations, which mirrors real
	// mutual-aid dispatch and guarantees every live unit is represented.
	type option struct {
		station  string
		hospital string
		minutes  float64
	}
	best := map[string]option{}
	for _, s := range stations {
		if !nodes[s].Operational {
			continue
		}
		for _, h := range hospitals {
			if !nodes[h].Operational {
				continue
			}
			p := finalisePath(adj, AstarRoad(nodes, adj, s, h))
			if p == nil {
				continue
			}
			cur, seen := best[s]
			if !seen || p.Minutes < cur.minutes {
				best[s] = option{station: s, hospital: h, minutes: p.Minutes}
			}
		}
	}
	order := make([]string, 0, len(best))
	for s := range best {
		order = append(order, s)
	}
	sort.Strings(order)

	primaryByHospital := map[string]EmsRoute{}
	for _, s := range order {
		opt := best[s]
		p := finalisePath(adj, AstarRoad(nodes, adj, s, opt.hospital))
		if p == nil {
			continue
		}
		route := EmsRoute{
			ID:          s + "->" + opt.hospital,
			Origin:      s,
			Destination: opt.hospital,
			Nodes:       append([]string{}, p.Nodes...),
			DistanceKM:  round2(p.DistanceKM),
			ETAMinutes:  round2(p.Minutes),
		}
		if net != nil {
			route.Degraded = net.MeshBackoffSet[s]
		}
		if old, had := prior[s]; had {
			route.PriorDistance = route.DistanceKM
			if !samePath(old, route.Nodes) {
				route.Changed = true
				res.Changed = append(res.Changed, route)
			}
		}
		res.Routes = append(res.Routes, route)
		res.Active[s] = append([]string{}, route.Nodes...)
		if cur, seen := primaryByHospital[opt.hospital]; !seen || route.ETAMinutes < cur.ETAMinutes {
			primaryByHospital[opt.hospital] = route
		}
	}
	for h, route := range primaryByHospital {
		res.Active[h] = append([]string{}, route.Nodes...)
	}

	assignedHospital := map[string]bool{}
	for _, s := range order {
		assignedHospital[best[s].hospital] = true
	}
	for _, s := range stations {
		if _, ok := best[s]; ok {
			continue
		}
		if n, alive := nodes[s]; alive && n.Operational {
			res.Unreachable = append(res.Unreachable, s)
		}
	}
	sort.Strings(res.Unreachable)
	return res
}
func roadReachability(nodes map[string]*InfrastructureNode, adj map[string][]RoadArc) map[string]bool {
	reach := map[string]bool{}
	var queue []string
	for id, n := range nodes {
		if n.Type == NodeEOC && n.Operational {
			reach[id] = true
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, a := range adj[cur] {
			if a.Blocked || reach[a.To] || !junctionPassable(nodes, a.To) {
				continue
			}
			reach[a.To] = true
			queue = append(queue, a.To)
		}
	}
	return reach
}
func samePath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
func DescribeRoute(r EmsRoute, cause string) string {
	if cause == "" {
		cause = "obstruction"
	}
	return fmt.Sprintf("REROUTE %s -> %s via %d segments (%.1f km, ETA %.1f min) due to %s",
		r.Origin, r.Destination, len(r.Nodes)-1, r.DistanceKM, r.ETAMinutes, cause)
}
