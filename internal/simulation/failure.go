package simulation

import (
	"fmt"
	"sort"
)

type FailureSource string

const (
	SourceOperator   FailureSource = "OPERATOR"
	SourceHazard     FailureSource = "HAZARD"
	SourceCascade    FailureSource = "CASCADE"
	SourceAutoRepair FailureSource = "AUTO_REPAIR"
)

type Hazard struct {
	Name        string          `json:"name"`
	Active      bool            `json:"active"`
	Intensity   float64         `json:"intensity"`
	Target      float64         `json:"target"`
	RampPerTick float64         `json:"rampPerTick"`
	OnsetTick   int             `json:"onsetTick"`
	Flooded     map[string]bool `json:"flooded"`
}

func NewHazard() *Hazard {
	return &Hazard{Flooded: map[string]bool{}}
}

func (hazard *Hazard) Clone() *Hazard {
	n := *hazard
	n.Flooded = make(map[string]bool, len(hazard.Flooded))
	for key, value := range hazard.Flooded {
		n.Flooded[key] = value
	}
	return &n
}

func (hazard *Hazard) Reset() {
	hazard.Active = false
	hazard.Intensity = 0
	hazard.Flooded = map[string]bool{}
}

type FailureOutcome struct {
	Failed    []string
	Blocked   []string
	Unblocked []string
	Notes     []string
}

func FailNode(nodes map[string]*InfrastructureNode, conns map[string]*Connection, targetID string, source FailureSource) (*InfrastructureNode, bool) {
	n, ok := nodes[targetID]
	if !ok {
		return nil, false
	}
	if !n.Operational && n.Health <= 0 {
		return n, false
	}
	n.Operational = false
	n.Health = 0
	n.Reason = string(source) + "_INJECTED"
	n.CascadeDep = 0
	n.Repairing = false

	if n.Type == NodeRoadIntersection {
		for _, segID := range controlledSegments(conns, n.ID) {
			c := conns[segID]
			if c == nil || c.Blocked {
				continue
			}
			c.Blocked = true
			c.Active = false
			c.Failures++
		}
	}
	for id, conn := range conns {
		if !conn.Active {
			continue
		}
		if conn.From == n.ID || conn.To == n.ID {
			conn.Active = false
			_ = id
		}
	}
	return n, true
}

func RestoreNode(nodes map[string]*InfrastructureNode, conns map[string]*Connection, graph *DependencyGraph, targetID string) (*InfrastructureNode, []string, bool) {
	n, ok := nodes[targetID]
	if !ok {
		return nil, nil, false
	}
	if graph != nil {
		blocking := blockingPrerequisites(graph, nodes, targetID)
		if len(blocking) > 0 {
			n.Reason = "AWAITING_PREREQ:" + blocking[0]
			return n, blocking, false
		}
	}
	n.Operational = true
	n.Health = 55
	n.Reason = "MANUAL_RESTORE"
	n.CascadeDep = 0
	n.Repairing = true
	repaired := reactivateLinks(conns, n.ID)
	return n, repaired, true
}

func blockingPrerequisites(graph *DependencyGraph, nodes map[string]*InfrastructureNode, id string) []string {
	var out []string
	for _, prereqId := range graph.Prerequisites[id] {
		if n, ok := nodes[prereqId]; ok && !n.Operational {
			out = append(out, prereqId)
		}
	}
	sort.Strings(out)
	return out
}

func reactivateLinks(conns map[string]*Connection, nodeID string) []string {
	var out []string
	for id, conn := range conns {
		if conn.Blocked {
			continue
		}
		if conn.From == nodeID || conn.To == nodeID {
			conn.Active = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func BlockConnection(conns map[string]*Connection, connID string) (*Connection, bool) {
	c, ok := conns[connID]
	if !ok {
		return nil, false
	}
	if c.Blocked {
		return c, false
	}
	c.Blocked = true
	c.Active = false
	c.Flooded = true
	c.Failures++
	return c, true
}

func UnblockConnection(conns map[string]*Connection, connID string) (*Connection, bool) {
	c, ok := conns[connID]
	if !ok {
		return nil, false
	}
	if !c.Blocked {
		return c, false
	}
	c.Blocked = false
	c.Flooded = false
	c.Active = true
	return c, true
}

func ActivateHurricane(hz *Hazard, conns map[string]*Connection, tick int, intensity float64) FailureOutcome {
	var out FailureOutcome
	hz.Name = "MAJOR_HURRICANE"
	hz.Active = true
	hz.Intensity = intensity
	hz.Target = 1
	hz.RampPerTick = 0.06
	hz.OnsetTick = tick

	for id, conn := range conns {
		if conn.Type != EdgeRoad {
			continue
		}
		if conn.Risk() >= 0.45 && conn.Risk() < 0.9 {
			hz.Flooded[id] = true
		}
	}
	out.Notes = append(out.Notes, fmt.Sprintf("Hurricane active at intensity %.2f", intensity))
	return out
}

func AdvanceHazard(hz *Hazard, nodes map[string]*InfrastructureNode, conns map[string]*Connection, tick int) FailureOutcome {
	var out FailureOutcome
	if hz == nil || !hz.Active {
		return out
	}
	if hz.Intensity < hz.Target {
		hz.Intensity += hz.RampPerTick
		if hz.Intensity > hz.Target {
			hz.Intensity = hz.Target
		}
	}
	i := hz.Intensity

	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		n := nodes[id]
		if !n.Operational {
			continue
		}
		exposure := n.Meta("exposure", 0.3)
		threshold := windThreshold(n.Type, i)
		if exposure >= threshold {
			n.Operational = false
			n.Health = 0
			n.Reason = "WIND_DAMAGE"
			n.CascadeDep = 0
			out.Failed = append(out.Failed, n.ID)
		}
	}

	segIDs := make([]string, 0, len(conns))
	for id := range conns {
		segIDs = append(segIDs, id)
	}
	sort.Strings(segIDs)
	for _, id := range segIDs {
		c := conns[id]
		if c.Type != EdgeRoad || c.Blocked {
			continue
		}
		if c.Risk()*i >= 0.42 {
			c.Blocked = true
			c.Active = false
			c.Flooded = true
			c.Failures++
			hz.Flooded[id] = true
			out.Blocked = append(out.Blocked, id)
		}
	}
	if i >= 0.6 {
		for id := range hz.Flooded {
			if c, ok := conns[id]; ok && c != nil && !c.Blocked {
				c.Blocked = true
				c.Active = false
				c.Flooded = true
				out.Blocked = append(out.Blocked, id)
			}
		}
	}
	_ = tick
	return out
}

func windThreshold(nodeType NodeType, intensity float64) float64 {
	base := map[NodeType]float64{
		NodeCellTower:        0.62,
		NodeRadioMesh:        0.55,
		NodeRoadIntersection: 0.80,
		NodePowerSubstation:  0.88,
		NodeHospital:         0.95,
		NodeFireStation:      0.95,
		NodeEOC:              0.97,
	}
	b := base[nodeType]
	if b == 0 {
		b = 0.9
	}
	return b * (0.55 + intensity*0.75)
}

func DecayHealth(nodes map[string]*InfrastructureNode, hz *Hazard, tick int) {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := nodes[id]
		if !n.Operational || n.Repairing {
			continue
		}
		if n.Health >= 100 {
			continue
		}
		exposure := n.Meta("exposure", 0.25)
		intensity := 0.0
		if hz != nil && hz.Active {
			intensity = hz.Intensity
		}
		rate := 0.18 + exposure*intensity*1.4
		n.Health -= rate
		if n.Health < 8 {
			n.Health = 8
		}
	}
	_ = tick
}
