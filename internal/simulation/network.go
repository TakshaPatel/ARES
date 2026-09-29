package simulation

import (
	"container/heap"
	"math"
	"sort"
)

const (
	meshHopLatencyPenalty = 9.0
	meshLossFloor         = 1.4
	baseLossPercent       = 0.15
	latencyLossCoeff      = 0.02
	healthLossCoeff       = 0.015
)

type CommsLink struct {
	ID       string
	From     string
	To       string
	Latency  float64
	LossPct  float64
	Mesh     bool
	Capacity float64
}
type NetworkAnalysis struct {
	Latency        map[string]float64
	Delivery       map[string]float64
	Path           map[string][]string
	Clusters       [][]string
	ClusterOf      map[string]int
	CommsReach     map[string]bool
	PeerReach      map[string]bool
	Isolated       []string
	MeshBackoff    []string
	MeshBackoffSet map[string]bool
	ActiveLinks    int
	NodesOffline   []string
	AvgPacketLoss  float64
}

func commsAdj(nodes map[string]*InfrastructureNode, conns map[string]*Connection) map[string][]CommsLink {
	adj := make(map[string][]CommsLink, len(nodes))
	ok := func(id string) bool {
		n, exist := nodes[id]
		return exist && n.Operational
	}
	push := func(c *Connection, from, to string) {
		if !c.Active || c.Blocked || c.Capacity <= 0 {
			return
		}
		if !ok(from) || !ok(to) {
			return
		}
		hf, ht := nodes[from].Health, nodes[to].Health
		worst := math.Min(hf, ht)
		mesh := isMeshHop(nodes, c)
		lat := c.Latency
		loss := baseLossPercent + lat*latencyLossCoeff + (100-worst)*healthLossCoeff
		if mesh {
			lat += meshHopLatencyPenalty
			if loss < meshLossFloor {
				loss = meshLossFloor
			}
		}
		adj[from] = append(adj[from], CommsLink{
			ID: c.ID, From: from, To: to, Latency: lat, LossPct: loss, Mesh: mesh, Capacity: c.Capacity,
		})
	}
	for _, c := range conns {
		if c.Type != EdgeMeshLink {
			continue
		}
		if _, exists := nodes[c.From]; !exists {
			continue
		}
		if _, exists := nodes[c.To]; !exists {
			continue
		}
		push(c, c.From, c.To)
		push(c, c.To, c.From)
	}
	for k := range adj {
		links := adj[k]
		sort.Slice(links, func(i, j int) bool { return links[i].To < links[j].To })
		adj[k] = links
	}
	return adj
}
func isMeshHop(nodes map[string]*InfrastructureNode, c *Connection) bool {
	if to, ok := nodes[c.To]; ok && to.Type == NodeRadioMesh {
		return true
	}
	if from, ok := nodes[c.From]; ok && from.Type == NodeRadioMesh {
		return true
	}
	return c.Meta("bandwidth_kbps", 0) < 256
}

type dsu struct{ parent map[string]string }

func newDSU(ids []string) *dsu {
	d := &dsu{parent: make(map[string]string, len(ids))}
	for _, id := range ids {
		d.parent[id] = id
	}
	return d
}
func (d *dsu) find(x string) string {
	if _, ok := d.parent[x]; !ok {
		d.parent[x] = x
		return x
	}
	root := x
	for d.parent[root] != root {
		root = d.parent[root]
	}
	for d.parent[x] != root {
		d.parent[x], x = root, d.parent[x]
	}
	return root
}
func (d *dsu) union(a, b string) {
	ra, rb := d.find(a), d.find(b)
	if ra != rb {
		if ra < rb {
			d.parent[rb] = ra
		} else {
			d.parent[ra] = rb
		}
	}
}
func AnalyseNetwork(nodes map[string]*InfrastructureNode, conns map[string]*Connection) *NetworkAnalysis {
	res := &NetworkAnalysis{
		Latency:        map[string]float64{},
		Delivery:       map[string]float64{},
		Path:           map[string][]string{},
		ClusterOf:      make(map[string]int, len(nodes)),
		CommsReach:     map[string]bool{},
		PeerReach:      map[string]bool{},
		MeshBackoffSet: map[string]bool{},
	}
	adj := commsAdj(nodes, conns)
	res.ActiveLinks = 0
	for _, links := range adj {
		res.ActiveLinks += len(links)
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	d := newDSU(ids)
	for from, links := range adj {
		for _, l := range links {
			d.union(from, l.To)
		}
	}
	groups := map[string][]string{}
	for _, id := range ids {
		root := d.find(id)
		groups[root] = append(groups[root], id)
	}
	roots := make([]string, 0, len(groups))
	for r := range groups {
		roots = append(roots, r)
	}
	sort.Strings(roots)
	for i, r := range roots {
		members := groups[r]
		sort.Strings(members)
		hasFacility := false
		for _, m := range members {
			if nodes[m] != nil && nodes[m].Type.IsFacility() {
				hasFacility = true
				break
			}
		}
		if !hasFacility {
			continue
		}
		res.Clusters = append(res.Clusters, members)
		for _, m := range members {
			res.ClusterOf[m] = i
		}
	}
	var eocs []string
	for _, id := range ids {
		if n := nodes[id]; n != nil && n.Type == NodeEOC {
			eocs = append(eocs, id)
		}
	}
	facilities := make([]string, 0, len(ids))
	for _, id := range ids {
		if n := nodes[id]; n != nil && n.Type.IsFacility() {
			facilities = append(facilities, id)
		}
	}
	for _, f := range facilities {
		best := math.Inf(1)
		bestEOC := ""
		for _, e := range eocs {
			if e == f {
				continue
			}
			sp := dijkstra(adj, e)
			if d, ok := sp.dist[f]; ok && d < best {
				best, bestEOC = d, e
			}
		}
		if bestEOC != "" {
			res.CommsReach[f] = true
			res.Latency[f] = best
			chosen := dijkstra(adj, bestEOC)
			path := chosen.path(f)
			res.Path[f] = path
			res.Delivery[f] = deliveryAlong(nodes, path, adj)
			if usesMesh(nodes, path) {
				res.MeshBackoff = append(res.MeshBackoff, f)
				res.MeshBackoffSet[f] = true
			}
		} else if f == eocs[0] {
			res.CommsReach[f] = true
			res.Latency[f] = 0
			res.Delivery[f] = 100
			res.Path[f] = []string{f}
		}
		peer := false
		if idx, ok := res.ClusterOf[f]; ok {
			for _, m := range res.Clusters[idx] {
				if m != f && nodes[m] != nil && nodes[m].Type.IsFacility() {
					peer = true
					break
				}
			}
		}
		res.PeerReach[f] = peer
		if !res.CommsReach[f] && !peer {
			res.Isolated = append(res.Isolated, f)
		}
	}
	for _, id := range ids {
		if n := nodes[id]; n != nil && !n.Operational {
			res.NodesOffline = append(res.NodesOffline, id)
		}
	}
	var lossSum float64
	var lossCount int
	for from := range adj {
		for _, l := range adj[from] {
			lossSum += l.LossPct
			lossCount++
		}
	}
	if lossCount > 0 {
		res.AvgPacketLoss = lossSum / float64(lossCount)
	}
	sort.Strings(res.MeshBackoff)
	sort.Strings(res.Isolated)
	return res
}
func deliveryAlong(nodes map[string]*InfrastructureNode, path []string, adj map[string][]CommsLink) float64 {
	if len(path) == 0 {
		return 0
	}
	if len(path) == 1 {
		return 100
	}
	success := 1.0
	for i := 0; i+1 < len(path); i++ {
		var link *CommsLink
		for k := range adj[path[i]] {
			if adj[path[i]][k].To == path[i+1] {
				link = &adj[path[i]][k]
				break
			}
		}
		if link == nil {
			return 0
		}
		success *= (100 - link.LossPct) / 100
	}
	return success * 100
}
func usesMesh(nodes map[string]*InfrastructureNode, path []string) bool {
	for _, id := range path {
		if n, ok := nodes[id]; ok && n.Type == NodeRadioMesh {
			return true
		}
	}
	return false
}

type shortestPath struct {
	dist     map[string]float64
	prev     map[string]string
	prevEdge map[string]string
}

func (s *shortestPath) path(to string) []string {
	if _, ok := s.dist[to]; !ok {
		return nil
	}
	out := []string{to}
	cur := to
	for {
		p, ok := s.prev[cur]
		if !ok {
			break
		}
		out = append([]string{p}, out...)
		cur = p
	}
	return out
}

type pqItem struct {
	id   string
	dist float64
}
type pq []pqItem

func (p pq) Len() int            { return len(p) }
func (p pq) Less(i, j int) bool  { return p[i].dist < p[j].dist }
func (p pq) Swap(i, j int)       { p[i], p[j] = p[j], p[i] }
func (p *pq) Push(x interface{}) { *p = append(*p, x.(pqItem)) }
func (p *pq) Pop() interface{} {
	old := *p
	n := len(old)
	it := old[n-1]
	*p = old[:n-1]
	return it
}
func dijkstra(adj map[string][]CommsLink, source string) *shortestPath {
	sp := &shortestPath{
		dist:     map[string]float64{source: 0},
		prev:     map[string]string{},
		prevEdge: map[string]string{},
	}
	q := &pq{{id: source, dist: 0}}
	heap.Init(q)
	for q.Len() > 0 {
		item := heap.Pop(q).(pqItem)
		if cur, ok := sp.dist[item.id]; !ok || cur > item.dist+1e-9 {
			continue
		}
		for _, l := range adj[item.id] {
			nd := item.dist + l.Latency
			if old, ok := sp.dist[l.To]; !ok || nd < old-1e-9 {
				sp.dist[l.To] = nd
				sp.prev[l.To] = l.From
				sp.prevEdge[l.To] = l.ID
				heap.Push(q, pqItem{id: l.To, dist: nd})
			}
		}
	}
	return sp
}
