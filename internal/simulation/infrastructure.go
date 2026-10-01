package simulation

import (
	"math"
	"sort"
	"strings"
)

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

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

func (nodeType NodeType) IsFacility() bool {
	switch nodeType {
	case NodeHospital, NodeFireStation, NodeEOC:
		return true
	}
	return false
}
func (nodeType NodeType) IsComms() bool {
	return nodeType == NodeCellTower || nodeType == NodeRadioMesh
}
func (nodeType NodeType) IsPower() bool {
	return nodeType == NodePowerSubstation
}
func (nodeType NodeType) IsRoadControl() bool {
	return nodeType == NodeRoadIntersection
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

func (node *InfrastructureNode) Meta(key string, fallback float64) float64 {
	if node.Metadata == nil {
		return fallback
	}
	switch v := node.Metadata[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return fallback
}
func (node *InfrastructureNode) MetaBool(key string, fallback bool) bool {
	if node.Metadata == nil {
		return fallback
	}
	if v, ok := node.Metadata[key].(bool); ok {
		return v
	}
	return fallback
}
func (node *InfrastructureNode) StatusFor(isolated bool) InfrastructureStatus {
	switch {
	case !node.Operational || node.Health <= healthCritical:
		return StatusFailed
	case node.Repairing:
		return StatusRestoring
	case isolated:
		return StatusIsolated
	case node.Health < healthImpaired:
		return StatusImpaired
	case node.Health < healthDegraded:
		return StatusDegraded
	case node.Operational:
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

func (conn *Connection) Meta(key string, fallback float64) float64 {
	if conn.Metadata == nil {
		return fallback
	}
	switch v := conn.Metadata[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return fallback
}
func (conn *Connection) Usable() bool {
	return conn.Active && !conn.Blocked
}
func (conn *Connection) Speed() float64 {
	if conn.SpeedKPH > 0 {
		return conn.SpeedKPH
	}
	if s := conn.Meta("speed_kph", 0); s > 0 {
		return s
	}
	return 50
}
func (conn *Connection) Risk() float64 {
	return conn.Meta("flood_risk", 0.25)
}
func (conn *Connection) MetaBool(key string, fallback bool) bool {
	if conn.Metadata == nil {
		return fallback
	}
	if v, ok := conn.Metadata[key].(bool); ok {
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

func (state *SimulationState) Clone() *SimulationState {
	out := &SimulationState{
		Tick:                state.Tick,
		Running:             state.Running,
		PowerGridHealth:     state.PowerGridHealth,
		CommsCoverage:       state.CommsCoverage,
		RoadAccessibility:   state.RoadAccessibility,
		MessageDeliveryRate: state.MessageDeliveryRate,
		AverageLatency:      state.AverageLatency,
		ConnectedFacilities: state.ConnectedFacilities,
		TotalFacilities:     state.TotalFacilities,
	}
	if state.Nodes != nil {
		out.Nodes = make(map[string]*InfrastructureNode, len(state.Nodes))
		for id, node := range state.Nodes {
			c := *node
			c.Dependencies = copyStrings(node.Dependencies)
			if node.Metadata != nil {
				c.Metadata = make(map[string]interface{}, len(node.Metadata))
				for key, value := range node.Metadata {
					c.Metadata[key] = value
				}
			}
			out.Nodes[id] = &c
		}
	}
	if state.Connections != nil {
		out.Connections = make(map[string]*Connection, len(state.Connections))
		for id, conn := range state.Connections {
			c := *conn
			if conn.Metadata != nil {
				c.Metadata = make(map[string]interface{}, len(conn.Metadata))
				for key, value := range conn.Metadata {
					c.Metadata[key] = value
				}
			}
			out.Connections[id] = &c
		}
	}
	out.IsolatedFacilities = append([]string{}, state.IsolatedFacilities...)
	out.ActiveAlerts = append([]string{}, state.ActiveAlerts...)
	out.ActiveEMSRoutes = make(map[string][]string, len(state.ActiveEMSRoutes))
	for key, value := range state.ActiveEMSRoutes {
		out.ActiveEMSRoutes[key] = append([]string{}, value...)
	}
	if state.Diagnostics != nil {
		d := *state.Diagnostics
		d.Events = append([]EventLogEntry{}, state.Diagnostics.Events...)
		d.MeshBackoff = append([]string{}, state.Diagnostics.MeshBackoff...)
		d.BlockedEdges = append([]string{}, state.Diagnostics.BlockedEdges...)
		d.DependencyEdge = append([]DependencyEdge{}, state.Diagnostics.DependencyEdge...)
		d.CommsClusters = make([][]string, 0, len(state.Diagnostics.CommsClusters))
		for _, cluster := range state.Diagnostics.CommsClusters {
			d.CommsClusters = append(d.CommsClusters, append([]string{}, cluster...))
		}
		d.EmsRoutes = make([]EmsRoute, 0, len(state.Diagnostics.EmsRoutes))
		for _, route := range state.Diagnostics.EmsRoutes {
			rr := route
			rr.Nodes = append([]string{}, route.Nodes...)
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
