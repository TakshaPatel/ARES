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
	for _, conn := range conns {
		if conn.Type != EdgeMeshLink {
			continue
		}
		if _, exists := nodes[conn.From]; !exists {
			continue
		}
		if _, exists := nodes[conn.To]; !exists {
			continue
		}
		push(conn, conn.From, conn.To)
		push(conn, conn.To, conn.From)
	}
	for nodeId := range adj {
		links := adj[nodeId]
		sort.Slice(links, func(i, j int) bool { return links[i].To < links[j].To })
		adj[nodeId] = links
	}
	return adj
}
func isMeshHop(nodes map[string]*InfrastructureNode, conn *Connection) bool {
	if to, ok := nodes[conn.To]; ok && to.Type == NodeRadioMesh {
		return true
	}
	if from, ok := nodes[conn.From]; ok && from.Type == NodeRadioMesh {
		return true
	}
	return conn.Meta("bandwidth_kbps", 0) < 256
}

type dsu struct{ parent map[string]string }

func newDSU(ids []string) *dsu {
	d := &dsu{parent: make(map[string]string, len(ids))}
	for _, id := range ids {
		d.parent[id] = id
	}
	return d
}
func (set *dsu) find(x string) string {
	if _, ok := set.parent[x]; !ok {
		set.parent[x] = x
		return x
	}
	root := x
	for set.parent[root] != root {
		root = set.parent[root]
	}
	for set.parent[x] != root {
		set.parent[x], x = root, set.parent[x]
	}
	return root
}
func (set *dsu) union(a, b string) {
	ra, rb := set.find(a), set.find(b)
	if ra != rb {
		if ra < rb {
			set.parent[rb] = ra
		} else {
			set.parent[ra] = rb
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
		for _, link := range links {
			d.union(from, link.To)
		}
	}
	groups := map[string][]string{}
	for _, id := range ids {
		root := d.find(id)
		groups[root] = append(groups[root], id)
	}
	roots := make([]string, 0, len(groups))
	for rootId := range groups {
		roots = append(roots, rootId)
	}
	sort.Strings(roots)
	for i, rootId := range roots {
		members := groups[rootId]
		sort.Strings(members)
		hasFacility := false
		for _, memberId := range members {
			if nodes[memberId] != nil && nodes[memberId].Type.IsFacility() {
				hasFacility = true
				break
			}
		}
		if !hasFacility {
			continue
		}
		res.Clusters = append(res.Clusters, members)
		for _, memberId := range members {
			res.ClusterOf[memberId] = i
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
	for _, facilityId := range facilities {
		best := math.Inf(1)
		bestEOC := ""
		for _, eocId := range eocs {
			if eocId == facilityId {
				continue
			}
			sp := dijkstra(adj, eocId)
			if d, ok := sp.dist[facilityId]; ok && d < best {
				best, bestEOC = d, eocId
			}
		}
		if bestEOC != "" {
			res.CommsReach[facilityId] = true
			res.Latency[facilityId] = best
			chosen := dijkstra(adj, bestEOC)
			path := chosen.path(facilityId)
			res.Path[facilityId] = path
			res.Delivery[facilityId] = deliveryAlong(nodes, path, adj)
			if usesMesh(nodes, path) {
				res.MeshBackoff = append(res.MeshBackoff, facilityId)
				res.MeshBackoffSet[facilityId] = true
			}
		} else if facilityId == eocs[0] {
			res.CommsReach[facilityId] = true
			res.Latency[facilityId] = 0
			res.Delivery[facilityId] = 100
			res.Path[facilityId] = []string{facilityId}
		}
		peer := false
		if idx, ok := res.ClusterOf[facilityId]; ok {
			for _, memberId := range res.Clusters[idx] {
				if memberId != facilityId && nodes[memberId] != nil && nodes[memberId].Type.IsFacility() {
					peer = true
					break
				}
			}
		}
		res.PeerReach[facilityId] = peer
		if !res.CommsReach[facilityId] && !peer {
			res.Isolated = append(res.Isolated, facilityId)
		}
	}
	for _, id := range ids {
		if n := nodes[id]; n != nil && !n.Operational {
			res.NodesOffline = append(res.NodesOffline, id)
		}
	}
	var lossSum float64
	var lossCount int
	for nodeId := range adj {
		for _, link := range adj[nodeId] {
			lossSum += link.LossPct
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
		for nodeId := range adj[path[i]] {
			if adj[path[i]][nodeId].To == path[i+1] {
				link = &adj[path[i]][nodeId]
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

func (sp *shortestPath) path(to string) []string {
	if _, ok := sp.dist[to]; !ok {
		return nil
	}
	out := []string{to}
	cur := to
	for {
		p, ok := sp.prev[cur]
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

func (queue pq) Len() int            { return len(queue) }
func (queue pq) Less(i, j int) bool  { return queue[i].dist < queue[j].dist }
func (queue pq) Swap(i, j int)       { queue[i], queue[j] = queue[j], queue[i] }
func (queue *pq) Push(x interface{}) { *queue = append(*queue, x.(pqItem)) }
func (queue *pq) Pop() interface{} {
	old := *queue
	n := len(old)
	it := old[n-1]
	*queue = old[:n-1]
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
		for _, link := range adj[item.id] {
			nd := item.dist + link.Latency
			if old, ok := sp.dist[link.To]; !ok || nd < old-1e-9 {
				sp.dist[link.To] = nd
				sp.prev[link.To] = link.From
				sp.prevEdge[link.To] = link.ID
				heap.Push(q, pqItem{id: link.To, dist: nd})
			}
		}
	}
	return sp
}
