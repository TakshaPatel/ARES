package simulation

import (
	"sort"
)

type RepairUnit struct {
	ID       string  `json:"id"`
	NodeID   string  `json:"nodeId"`
	Kind     string  `json:"kind"`
	Progress float64 `json:"progress"`
	ETA      int     `json:"eta"`
	Crew     string  `json:"crew"`
}

type RecoveryReport struct {
	Repaired  []string
	Progress  []RepairUnit
	Unblocked []string
	Stalled   []string
}

func RepairRate(node *InfrastructureNode) float64 {
	base := 3.2
	switch node.Type {
	case NodePowerSubstation:
		base = 2.1
	case NodeCellTower:
		base = 5.0
	case NodeRadioMesh:
		base = 6.5
	case NodeRoadIntersection:
		base = 7.0
	}
	if node.MetaBool("backup_power", false) {
		base *= 1.15
	}
	if node.Health < 20 {
		base *= 0.6
	}
	return base
}

func StartRepair(node *InfrastructureNode) bool {
	if node.Operational || node.Health > 0 {
		return false
	}
	node.Repairing = true
	return true
}

func StepRecovery(
	nodes map[string]*InfrastructureNode,
	conns map[string]*Connection,
	graph *DependencyGraph,
	hz *Hazard,
	autoRepair bool,
) RecoveryReport {
	var rep RecoveryReport

	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		n := nodes[id]
		if n.Operational {
			if n.Health < 100 {
				n.Health = minFloat(100, n.Health+1.4)
			}
			if n.Health >= 99.5 {
				n.Health = 100
			}
			continue
		}
		if !n.Repairing && autoRepair {
			if blocking := blockingPrerequisites(graph, nodes, id); len(blocking) == 0 {
				if StartRepair(n) {
					rep.Repaired = append(rep.Repaired, id)
				}
			}
		}
		if !n.Repairing {
			continue
		}
		if blocking := blockingPrerequisites(graph, nodes, id); len(blocking) > 0 {
			n.Reason = "AWAITING_PREREQ:" + blocking[0]
			rep.Stalled = append(rep.Stalled, id)
			continue
		}
		if hz != nil && hz.Active && n.Meta("exposure", 0.3) >= 0.9 && hz.Intensity > 0.85 {
			n.Reason = "REPAIR_ABORTED_STORM"
			rep.Stalled = append(rep.Stalled, id)
			continue
		}
		rate := RepairRate(n)
		before := n.Health
		n.Health = minFloat(100, n.Health+rate)
		rep.Progress = append(rep.Progress, RepairUnit{
			ID: n.ID, NodeID: n.ID, Kind: string(n.Type), Progress: n.Health, Crew: crewFor(n),
		})
		if n.Health >= 62 {
			n.Operational = true
			n.Reason = "SERVICE_RESTORED"
			n.Repairing = false
			n.CascadeDep = 0
			for _, seg := range reactivateLinks(conns, n.ID) {
				rep.Unblocked = append(rep.Unblocked, seg)
			}
		} else if before < 30 && n.Health >= 30 {
			n.Status = StatusRestoring
		}
	}

	segIDs := make([]string, 0, len(conns))
	for id := range conns {
		segIDs = append(segIDs, id)
	}
	sort.Strings(segIDs)
	if hz != nil && !hz.Active {
		for _, id := range segIDs {
			c := conns[id]
			if c.Type == EdgeRoad && c.Blocked && c.Risk() < 0.7 {
				UnblockConnection(conns, id)
				rep.Unblocked = append(rep.Unblocked, id)
			}
		}
	}
	sort.Strings(rep.Repaired)
	sort.Strings(rep.Unblocked)
	return rep
}

func crewFor(node *InfrastructureNode) string {
	switch node.Type {
	case NodePowerSubstation:
		return "LINE_CREW_ALPHA"
	case NodeCellTower:
		return "COMMS_TEAM_2"
	case NodeRadioMesh:
		return "MESH_TEAM_1"
	case NodeRoadIntersection:
		return "PUBLIC_WORKS_3"
	case NodeHospital:
		return "MEDICAL_TEAM"
	default:
		return "OPS_TEAM"
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
