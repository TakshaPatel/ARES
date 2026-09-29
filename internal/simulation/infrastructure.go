package simulation

import (
	"math"
	"sort"
	"strings"
)

type NodeType string

const (
	NodePowerSubstation  NodeType = "POWER_SUBSTATION"
	NodeCellTower        NodeType = "CELL_TOWER"
	NodeRadioMesh        NodeType = "RADIO_MESH"
	NodeRoadIntersection NodeType = "ROAD_INTERSECTION"
	NodeHospital         NodeType = "HOSPITAL"
	NodeFireStation      NodeType = "FIRE_STATION"
	NodeEOC              NodeType = "EMERGENCY_OPS_CENTER"
)

type InfrastructureStatus string

const (
	StatusOperational InfrastructureStatus = "OPERATIONAL"
	StatusDegraded    InfrastructureStatus = "DEGRADED"
	StatusImpaired    InfrastructureStatus = "IMPAIRED"
	StatusIsolated    InfrastructureStatus = "ISOLATED"
	StatusFailed      InfrastructureStatus = "FAILED"
	StatusRestoring   InfrastructureStatus = "RESTORING"
	StatusStandby     InfrastructureStatus = "STANDBY"
)
const (
	healthCritical = 15.0
	healthImpaired = 45.0
	healthDegraded = 75.0
)

func (t NodeType) IsFacility() bool {
	switch t {
	case NodeHospital, NodeFireStation, NodeEOC:
		return true
	}
	return false
}
func (t NodeType) IsComms() bool {
	return t == NodeCellTower || t == NodeRadioMesh
}
func (t NodeType) IsPower() bool {
	return t == NodePowerSubstation
}
func (t NodeType) IsRoadControl() bool {
	return t == NodeRoadIntersection
}

type InfrastructureNode struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         NodeType               `json:"type"`
	Operational  bool                   `json:"operational"`
	Health       float64                `json:"health"`
	Lat          float64                `json:"lat"`
	Lng          float64                `json:"lng"`
	Dependencies []string               `json:"dependencies"`
	Metadata     map[string]interface{} `json:"metadata"`
	Status       InfrastructureStatus   `json:"status"`
	Reason       string                 `json:"reason,omitempty"`
	CascadeDep   int                    `json:"cascadeDepth"`
	MeshLink     bool                   `json:"meshFallback"`
	Flooded      bool                   `json:"flooded"`
	Repairing    bool                   `json:"repairing"`
}

func (n *InfrastructureNode) Meta(key string, fallback float64) float64 {
	if n.Metadata == nil {
		return fallback
	}
	switch v := n.Metadata[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return fallback
}
func (n *InfrastructureNode) MetaBool(key string, fallback bool) bool {
	if n.Metadata == nil {
		return fallback
	}
	if v, ok := n.Metadata[key].(bool); ok {
		return v
	}
	return fallback
}
func (n *InfrastructureNode) StatusFor(isolated bool) InfrastructureStatus {
	switch {
	case !n.Operational || n.Health <= healthCritical:
		return StatusFailed
	case n.Repairing:
		return StatusRestoring
	case isolated:
		return StatusIsolated
	case n.Health < healthImpaired:
		return StatusImpaired
	case n.Health < healthDegraded:
		return StatusDegraded
	case n.Operational:
		return StatusOperational
	default:
		return StatusStandby
	}
}

type EdgeType string

const (
	EdgePowerLine EdgeType = "POWER_LINE"
	EdgeMeshLink  EdgeType = "MESH_LINK"
	EdgeRoad      EdgeType = "ROAD_SEGMENT"
)

type Connection struct {
	ID       string                 `json:"id"`
	From     string                 `json:"from"`
	To       string                 `json:"to"`
	Type     EdgeType               `json:"type"`
	Capacity float64                `json:"capacity"`
	Latency  float64                `json:"latency"`
	Distance float64                `json:"distance"`
	Blocked  bool                   `json:"blocked"`
	Active   bool                   `json:"active"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Failures int                    `json:"failureCount"`
	SpeedKPH float64                `json:"speedKph"`
	Flooded  bool                   `json:"flooded"`
}

func (c *Connection) Meta(key string, fallback float64) float64 {
	if c.Metadata == nil {
		return fallback
	}
	switch v := c.Metadata[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return fallback
}
func (c *Connection) Usable() bool {
	return c.Active && !c.Blocked
}
func (c *Connection) Speed() float64 {
	if c.SpeedKPH > 0 {
		return c.SpeedKPH
	}
	if s := c.Meta("speed_kph", 0); s > 0 {
		return s
	}
	return 50
}
func (c *Connection) Risk() float64 {
	return c.Meta("flood_risk", 0.25)
}
func (c *Connection) MetaBool(key string, fallback bool) bool {
	if c.Metadata == nil {
		return fallback
	}
	if v, ok := c.Metadata[key].(bool); ok {
		return v
	}
	return fallback
}

type EventSeverity string

const (
	EventInfo    EventSeverity = "INFO"
	EventSuccess EventSeverity = "SUCCESS"
	EventWarning EventSeverity = "WARNING"
	EventCascade EventSeverity = "CASCADE"
	EventFailure EventSeverity = "FAILURE"
	EventReroute EventSeverity = "REROUTE"
)

type EventCategory string

const (
	CatPower   EventCategory = "POWER"
	CatComms   EventCategory = "COMMS"
	CatRoad    EventCategory = "ROAD"
	CatRouting EventCategory = "ROUTING"
	CatCommand EventCategory = "COMMAND"
)

type EventLogEntry struct {
	ID       int           `json:"id"`
	Tick     int           `json:"tick"`
	Clock    string        `json:"clock"`
	Severity EventSeverity `json:"severity"`
	Category EventCategory `json:"category"`
	Source   string        `json:"source"`
	NodeID   string        `json:"nodeId,omitempty"`
	Depth    int           `json:"depth"`
	Message  string        `json:"message"`
}
type DependencyEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Active bool   `json:"active"`
}
type EmsRoute struct {
	ID            string   `json:"id"`
	Origin        string   `json:"origin"`
	Destination   string   `json:"destination"`
	Nodes         []string `json:"nodes"`
	DistanceKM    float64  `json:"distanceKm"`
	ETAMinutes    float64  `json:"etaMinutes"`
	Degraded      bool     `json:"degraded"`
	Changed       bool     `json:"changed"`
	PriorDistance float64  `json:"priorDistanceKm"`
}
type Diagnostics struct {
	Events         []EventLogEntry  `json:"events"`
	CommsClusters  [][]string       `json:"commsClusters"`
	EmsRoutes      []EmsRoute       `json:"emsRoutes"`
	MeshBackoff    []string         `json:"meshBackoff"`
	DependencyEdge []DependencyEdge `json:"dependencyEdges"`
	BlockedEdges   []string         `json:"blockedEdges"`
	PowerCritical  float64          `json:"powerCriticality"`
	ElapsedMinutes int              `json:"elapsedMinutes"`
	Clock          string           `json:"clock"`
	Revision       int64            `json:"revision"`
	LoadMS         float64          `json:"loadMs"`
}
type SimulationState struct {
	Tick                int                            `json:"tick"`
	Running             bool                           `json:"running"`
	Nodes               map[string]*InfrastructureNode `json:"nodes"`
	Connections         map[string]*Connection         `json:"connections"`
	PowerGridHealth     float64                        `json:"powerGridHealth"`
	CommsCoverage       float64                        `json:"commsCoverage"`
	RoadAccessibility   float64                        `json:"roadAccessibility"`
	MessageDeliveryRate float64                        `json:"messageDeliveryRate"`
	AverageLatency      float64                        `json:"averageLatency"`
	ConnectedFacilities int                            `json:"connectedFacilities"`
	TotalFacilities     int                            `json:"totalFacilities"`
	IsolatedFacilities  []string                       `json:"isolatedFacilities"`
	ActiveAlerts        []string                       `json:"activeAlerts"`
	ActiveEMSRoutes     map[string][]string            `json:"activeEmsRoutes"`
	Diagnostics         *Diagnostics                   `json:"diagnostics,omitempty"`
}

func (s *SimulationState) Clone() *SimulationState {
	out := &SimulationState{
		Tick:                s.Tick,
		Running:             s.Running,
		PowerGridHealth:     s.PowerGridHealth,
		CommsCoverage:       s.CommsCoverage,
		RoadAccessibility:   s.RoadAccessibility,
		MessageDeliveryRate: s.MessageDeliveryRate,
		AverageLatency:      s.AverageLatency,
		ConnectedFacilities: s.ConnectedFacilities,
		TotalFacilities:     s.TotalFacilities,
	}
	if s.Nodes != nil {
		out.Nodes = make(map[string]*InfrastructureNode, len(s.Nodes))
		for id, n := range s.Nodes {
			c := *n
			c.Dependencies = append([]string(nil), n.Dependencies...)
			if n.Metadata != nil {
				c.Metadata = make(map[string]interface{}, len(n.Metadata))
				for k, v := range n.Metadata {
					c.Metadata[k] = v
				}
			}
			out.Nodes[id] = &c
		}
	}
	if s.Connections != nil {
		out.Connections = make(map[string]*Connection, len(s.Connections))
		for id, e := range s.Connections {
			c := *e
			if e.Metadata != nil {
				c.Metadata = make(map[string]interface{}, len(e.Metadata))
				for k, v := range e.Metadata {
					c.Metadata[k] = v
				}
			}
			out.Connections[id] = &c
		}
	}
	out.IsolatedFacilities = append([]string{}, s.IsolatedFacilities...)
	out.ActiveAlerts = append([]string{}, s.ActiveAlerts...)
	out.ActiveEMSRoutes = make(map[string][]string, len(s.ActiveEMSRoutes))
	for k, v := range s.ActiveEMSRoutes {
		out.ActiveEMSRoutes[k] = append([]string{}, v...)
	}
	if s.Diagnostics != nil {
		d := *s.Diagnostics
		d.Events = append([]EventLogEntry{}, s.Diagnostics.Events...)
		d.MeshBackoff = append([]string{}, s.Diagnostics.MeshBackoff...)
		d.BlockedEdges = append([]string{}, s.Diagnostics.BlockedEdges...)
		d.DependencyEdge = append([]DependencyEdge{}, s.Diagnostics.DependencyEdge...)
		d.CommsClusters = make([][]string, 0, len(s.Diagnostics.CommsClusters))
		for _, c := range s.Diagnostics.CommsClusters {
			d.CommsClusters = append(d.CommsClusters, append([]string{}, c...))
		}
		d.EmsRoutes = make([]EmsRoute, 0, len(s.Diagnostics.EmsRoutes))
		for _, r := range s.Diagnostics.EmsRoutes {
			rr := r
			rr.Nodes = append([]string{}, r.Nodes...)
			d.EmsRoutes = append(d.EmsRoutes, rr)
		}
		out.Diagnostics = &d
	}
	return out
}
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusKM * math.Asin(math.Sqrt(math.Min(1, a)))
}
func ClockString(tick, tickMinutes, startHour, startMinute int) string {
	total := tick*tickMinutes + startHour*60 + startMinute
	total = ((total % 1440) + 1440) % 1440
	return string(rune('0'+total/60/10)) + string(rune('0'+(total/60)%10)) + ":" +
		string(rune('0'+(total%60)/10)) + string(rune('0'+total%10))
}
func SortIDs(ids []string) []string {
	out := append([]string{}, ids...)
	sort.Strings(out)
	return out
}
func SanitizeLabel(id string) string {
	if i := strings.LastIndex(id, "-"); i >= 0 && i+1 < len(id) {
		return strings.ToUpper(id[i+1:])
	}
	return strings.ToUpper(id)
}
