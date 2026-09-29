export type NodeType =
  | 'POWER_SUBSTATION'
  | 'CELL_TOWER'
  | 'RADIO_MESH'
  | 'ROAD_INTERSECTION'
  | 'HOSPITAL'
  | 'FIRE_STATION'
  | 'EMERGENCY_OPS_CENTER'

export type InfrastructureStatus =
  | 'OPERATIONAL'
  | 'DEGRADED'
  | 'IMPAIRED'
  | 'ISOLATED'
  | 'FAILED'
  | 'RESTORING'
  | 'STANDBY'

export type EdgeType = 'POWER_LINE' | 'MESH_LINK' | 'ROAD_SEGMENT'

export type EventSeverity = 'INFO' | 'SUCCESS' | 'WARNING' | 'CASCADE' | 'FAILURE' | 'REROUTE'

export type EventCategory = 'POWER' | 'COMMS' | 'ROAD' | 'ROUTING' | 'COMMAND'

export interface InfrastructureNode {
  id: string
  name: string
  type: NodeType
  operational: boolean
  health: number
  lat: number
  lng: number
  dependencies: string[]
  metadata: Record<string, unknown>
  status: InfrastructureStatus
  reason?: string
  cascadeDepth: number
  meshFallback: boolean
  flooded: boolean
  repairing: boolean
}

export interface Connection {
  id: string
  from: string
  to: string
  type: EdgeType
  capacity: number
  latency: number
  distance: number
  blocked: boolean
  active: boolean
  metadata?: Record<string, unknown>
  failureCount: number
  speedKph: number
  flooded: boolean
}

export interface EventLogEntry {
  id: number
  tick: number
  clock: string
  severity: EventSeverity
  category: EventCategory
  source: string
  nodeId?: string
  depth: number
  message: string
}

export interface DependencyEdge {
  id: string
  source: string
  target: string
  kind: 'DEPENDENCY' | 'POWER_LINE' | 'MESH_LINK' | 'ROAD_SEGMENT'
  active: boolean
}

export interface EmsRoute {
  id: string
  origin: string
  destination: string
  nodes: string[]
  distanceKm: number
  etaMinutes: number
  degraded: boolean
  changed: boolean
  priorDistanceKm: number
}

export interface Diagnostics {
  events: EventLogEntry[]
  commsClusters: string[][]
  emsRoutes: EmsRoute[]
  meshBackoff: string[]
  dependencyEdges: DependencyEdge[]
  blockedEdges: string[]
  powerCriticality: number
  elapsedMinutes: number
  clock: string
  revision: number
  loadMs?: number
}

export interface SimulationState {
  tick: number
  running: boolean
  nodes: Record<string, InfrastructureNode>
  connections: Record<string, Connection>
  powerGridHealth: number
  commsCoverage: number
  roadAccessibility: number
  messageDeliveryRate: number
  averageLatency: number
  connectedFacilities: number
  totalFacilities: number
  isolatedFacilities: string[]
  activeAlerts: string[]
  activeEmsRoutes: Record<string, string[]>
  diagnostics?: Diagnostics
}

export interface Preset {
  id: string
  label: string
  description: string
  kind: string
  targetId?: string
  severity: string
  intensity?: number
}

export interface ScenarioSummary {
  id: string
  name: string
  description: string
  hazardModel: string
  nodes: number
  connections: number
  facilities: number
}

export type RunState = 'RUNNING' | 'PAUSED' | 'CONNECTING' | 'OFFLINE'

export interface MetricSample {
  tick: number
  clock: string
  powerGridHealth: number
  commsCoverage: number
  roadAccessibility: number
  messageDeliveryRate: number
  averageLatency: number
  connectedFacilities: number
  totalFacilities: number
  isolated: number
}

export type InboundAction =
  | 'START'
  | 'PAUSE'
  | 'STEP'
  | 'INJECT_FAILURE'
  | 'INJECT_ROAD_BLOCK'
  | 'RESTORE_NODE'
  | 'APPLY_PRESET'
  | 'RESET'

export interface NodeGlyph {
  label: string
  color: string
  shape: 'circle' | 'square' | 'triangle' | 'diamond' | 'hex'
}
