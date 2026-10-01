import type { CityView } from '../types/simulation'

export interface CityNode {
  id?: string
  name?: string
  type?: string
  lat?: number
  lng?: number
  dependencies?: string[]
}

interface ConnectionLike {
  id?: string
  from?: string
  to?: string
  type?: string
  latency?: number
}

export interface ParsedCity {
  kind: 'scenario' | 'profile'
  label: string
  source: string
  raw: string
  scenario: Record<string, unknown> | null
  view: CityView
  nodeCount: number
  connectionCount: number
  bounds: { minLat: number; maxLat: number; minLng: number; maxLng: number } | null
  warnings: string[]
  blocking: string[]
}

const VALID_TYPES = new Set([
  'POWER_SUBSTATION',
  'CELL_TOWER',
  'RADIO_MESH',
  'ROAD_INTERSECTION',
  'HOSPITAL',
  'FIRE_STATION',
  'EMERGENCY_OPS_CENTER',
])

const FACILITY_TYPES = new Set(['HOSPITAL', 'FIRE_STATION', 'EMERGENCY_OPS_CENTER'])
const VALID_EDGE_TYPES = new Set(['POWER_LINE', 'MESH_LINK', 'ROAD_SEGMENT'])

function num(v: unknown): number | null {
  const n = typeof v === 'string' ? Number(v) : v
  return typeof n === 'number' && Number.isFinite(n) ? n : null
}

function validLatLng(lat: unknown, lng: unknown): boolean {
  const la = num(lat)
  const ln = num(lng)
  return la !== null && ln !== null && la >= -90 && la <= 90 && ln >= -180 && ln <= 180
}

function readCenter(src: Record<string, unknown>): [number, number] | null {
  const c = src.center
  if (c && typeof c === 'object' && validLatLng((c as Record<string, unknown>).lat, (c as Record<string, unknown>).lng)) {
    const cc = c as Record<string, unknown>
    return [num(cc.lng) as number, num(cc.lat) as number]
  }
  if (Array.isArray(c) && c.length === 2 && validLatLng(c[1], c[0])) {
    return [num(c[0]) as number, num(c[1]) as number]
  }
  if (validLatLng(src.lat, src.lng)) {
    return [num(src.lng) as number, num(src.lat) as number]
  }
  return null
}

function boundsOf(nodes: CityNode[]) {
  let minLat = 90
  let maxLat = -90
  let minLng = 180
  let maxLng = -180
  let n = 0
  for (const node of nodes) {
    const la = num(node.lat)
    const ln = num(node.lng)
    if (la === null || ln === null) continue
    n++
    if (la < minLat) minLat = la
    if (la > maxLat) maxLat = la
    if (ln < minLng) minLng = ln
    if (ln > maxLng) maxLng = ln
  }
  return n === 0 ? null : { minLat, maxLat, minLng, maxLng }
}

function zoomForSpan(b: { minLat: number; maxLat: number; minLng: number; maxLng: number }) {
  const spanLat = Math.max(b.maxLat - b.minLat, 1e-6)
  const spanLng = Math.max(b.maxLng - b.minLng, 1e-6)
  const latZ = Math.log2(360 / spanLat)
  const lngZ = Math.log2(360 / (spanLng * Math.cos(((b.minLat + b.maxLat) / 2) * (Math.PI / 180))))
  return Math.max(6, Math.min(16, Math.min(latZ, lngZ) - 0.6))
}

export function parseCityData(text: string, source: string): ParsedCity {
  let src: Record<string, unknown>
  try {
    src = JSON.parse(text) as Record<string, unknown>
  } catch (e) {
    throw new Error(`Not valid JSON: ${(e as Error).message}`)
  }
  if (!src || typeof src !== 'object' || Array.isArray(src)) {
    throw new Error('Top level of the file must be a JSON object')
  }

  const warnings: string[] = []
  const blocking: string[] = []
  const nodes = Array.isArray(src.nodes) ? (src.nodes as CityNode[]) : []
  const connections = Array.isArray(src.connections) ? (src.connections as ConnectionLike[]) : []
  const isScenario = nodes.length > 0 && connections.length > 0
  const b = boundsOf(nodes)

  if (isScenario) {
    const ids = new Set<string>()
    const dupes = new Set<string>()
    for (const n of nodes) {
      const id = typeof n.id === 'string' ? n.id : ''
      if (!id) blocking.push('Every node needs a non-empty id.')
      else if (ids.has(id)) dupes.add(id)
      ids.add(id)
    }
    if (dupes.size > 0) blocking.push(`Duplicate node id(s): ${[...dupes].join(', ')}.`)

    let facilities = 0
    let eocs = 0
    for (const n of nodes) {
      if (FACILITY_TYPES.has(String(n.type))) facilities++
      if (n.type === 'EMERGENCY_OPS_CENTER') eocs++
    }
    if (facilities === 0) blocking.push('No emergency facilities (hospital, fire station or EOC).')
    if (eocs === 0) blocking.push('No emergency operations center - the server rejects a scenario without one.')

    const connIds = new Set<string>()
    const connDupes = new Set<string>()
    let badEndpoints = 0
    let selfLoops = 0
    let badEdgeType = 0
    let meshNoLatency = 0
    for (const c of connections) {
      const id = typeof c.id === 'string' ? c.id : ''
      if (!id) blocking.push('Every connection needs a non-empty id.')
      else if (connIds.has(id)) connDupes.add(id)
      connIds.add(id)
      const from = typeof c.from === 'string' ? c.from : ''
      const to = typeof c.to === 'string' ? c.to : ''
      if (!ids.has(from) || !ids.has(to)) badEndpoints++
      else if (from === to) selfLoops++
      if (!VALID_EDGE_TYPES.has(String(c.type))) badEdgeType++
      if (c.type === 'MESH_LINK' && !(num(c.latency) && num(c.latency)! > 0)) meshNoLatency++
    }
    if (connDupes.size > 0) blocking.push(`Duplicate connection id(s): ${[...connDupes].join(', ')}.`)
    if (badEndpoints > 0) blocking.push(`${badEndpoints} connection(s) point at a node id that does not exist.`)
    if (selfLoops > 0) blocking.push(`${selfLoops} connection(s) are a self loop.`)
    if (badEdgeType > 0) {
      warnings.push(
        `${badEdgeType} connection(s) need a type of POWER_LINE, MESH_LINK or ROAD_SEGMENT.`,
      )
    }
    if (meshNoLatency > 0) warnings.push(`${meshNoLatency} MESH_LINK connection(s) need a latency above 0.`)

    const dangling = nodes.filter(
      (n) => Array.isArray(n.dependencies) && n.dependencies.some((d) => !ids.has(d as string)),
    )
    if (dangling.length > 0) {
      warnings.push(
        `${dangling.length} node(s) declare a dependency on an id that does not exist (${dangling
          .map((n) => n.id ?? '?')
          .join(', ')}).`,
      )
    }
  }

  if (nodes.length > 0 && !connections.length) {
    warnings.push('File has nodes but no connections - treated as a view profile, not a scenario.')
  }

  let missing = 0
  for (const n of nodes) {
    if (!validLatLng(n.lat, n.lng)) missing++
  }
  if (missing > 0) {
    warnings.push(`${missing} node(s) have no usable lat/lng and cannot be placed on the map.`)
  }
  const badType = nodes.filter((n) => n.type && !VALID_TYPES.has(n.type)).length
  if (badType > 0) {
    warnings.push(`${badType} node(s) use an unrecognised type and may fail validation.`)
  }

  const explicit = readCenter(src)
  let center = explicit
  let zoom = num(src.zoom)
  if (!center && b) {
    center = [(b.minLng + b.maxLng) / 2, (b.minLat + b.maxLat) / 2]
  }
  if (zoom === null && b) zoom = zoomForSpan(b)
  if (zoom !== null && (zoom < 1 || zoom > 22)) {
    warnings.push(`Zoom ${zoom} is out of range, clamped.`)
    zoom = Math.max(1, Math.min(22, zoom))
  }

  const label =
    (typeof src.name === 'string' && src.name.trim()) ||
    (typeof src.city === 'string' && src.city.trim()) ||
    (typeof src.label === 'string' && src.label.trim()) ||
    source

  if (isScenario && !explicit && b) {
    warnings.push('No explicit center supplied - derived it from the node bounds.')
  }

  return {
    kind: isScenario ? 'scenario' : 'profile',
    label,
    source,
    raw: text,
    scenario: isScenario ? src : null,
    view: {
      center,
      zoom,
      label,
      source,
    },
    nodeCount: nodes.length,
    connectionCount: connections.length,
    bounds: b,
    warnings,
    blocking,
  }
}

export const SAMPLE_CITY_DATA = `{
  "id": "north-brunswick-hurricane-profile",
  "name": "North Brunswick Township, NJ - Real Facilities",
  "description": "Township of North Brunswick / Middlesex County profile built on published locations: PSE&G Brunswick and Adams substations, RWJBarnabas Health's Robert Wood Johnson University Hospital and Saint Peter's University Hospital, the township's four volunteer fire companies and First Aid & Rescue Squad, and the Municipal Building operating as the emergency operations centre.",
  "hazardModel": "MAJOR_HURRICANE",
  "tickMinutes": 5,
  "startHour": 6,
  "startMinute": 15,
  "center": {
    "lat": 40.459458,
    "lng": -74.472968
  },
  "zoom": 12,
  "nodes": [
    {
      "id": "substation-01",
      "name": "PSE&G Brunswick Substation",
      "type": "POWER_SUBSTATION",
      "lat": 40.4463,
      "lng": -74.46102,
      "dependencies": []
    },
    {
      "id": "substation-03",
      "name": "PSE&G Adams Substation",
      "type": "POWER_SUBSTATION",
      "lat": 40.4583,
      "lng": -74.48014,
      "dependencies": []
    },
    {
      "id": "tower-03",
      "name": "Jersey Avenue Tower",
      "type": "CELL_TOWER",
      "lat": 40.45983,
      "lng": -74.43483,
      "dependencies": [
        "substation-01"
      ]
    },
    {
      "id": "mesh-01",
      "name": "Milltown Mesh Relay",
      "type": "RADIO_MESH",
      "lat": 40.45319,
      "lng": -74.43514,
      "dependencies": [
        "substation-01"
      ]
    },
    {
      "id": "intersection-01",
      "name": "Hermann Road & Ridgewood Avenue",
      "type": "ROAD_INTERSECTION",
      "lat": 40.4672,
      "lng": -74.4672,
      "dependencies": [
        "substation-01"
      ]
    },
    {
      "id": "intersection-02",
      "name": "US-130 & Station Avenue (Adams)",
      "type": "ROAD_INTERSECTION",
      "lat": 40.4423,
      "lng": -74.49901,
      "dependencies": [
        "substation-03"
      ]
    },
    {
      "id": "tower-04",
      "name": "Kendall Park Tower (South Brunswick)",
      "type": "CELL_TOWER",
      "lat": 40.41389,
      "lng": -74.56598,
      "dependencies": [
        "substation-03"
      ]
    },
    {
      "id": "hospital-01",
      "name": "Robert Wood Johnson University Hospital",
      "type": "HOSPITAL",
      "lat": 40.4953,
      "lng": -74.44951,
      "dependencies": [
        "substation-01",
        "tower-03"
      ]
    },
    {
      "id": "hospital-02",
      "name": "Saint Peter's University Hospital",
      "type": "HOSPITAL",
      "lat": 40.50061,
      "lng": -74.45949,
      "dependencies": [
        "substation-01",
        "tower-03"
      ]
    },
    {
      "id": "fire-01",
      "name": "North Brunswick Volunteer Fire Co. 1",
      "type": "FIRE_STATION",
      "lat": 40.46472,
      "lng": -74.45772,
      "dependencies": [
        "substation-01",
        "intersection-01"
      ]
    },
    {
      "id": "fire-02",
      "name": "North Brunswick Volunteer Fire Co. 2",
      "type": "FIRE_STATION",
      "lat": 40.43986,
      "lng": -74.47887,
      "dependencies": [
        "substation-03",
        "intersection-02"
      ]
    },
    {
      "id": "fire-03",
      "name": "North Brunswick Volunteer Fire Co. 3",
      "type": "FIRE_STATION",
      "lat": 40.45996,
      "lng": -74.50488,
      "dependencies": [
        "substation-03"
      ]
    },
    {
      "id": "fire-04",
      "name": "North Brunswick First Aid & Rescue Squad",
      "type": "FIRE_STATION",
      "lat": 40.46419,
      "lng": -74.4699,
      "dependencies": [
        "substation-01",
        "mesh-01"
      ]
    },
    {
      "id": "eoc-01",
      "name": "North Brunswick Township Municipal Building (EOC)",
      "type": "EMERGENCY_OPS_CENTER",
      "lat": 40.46676,
      "lng": -74.45786,
      "dependencies": [
        "substation-01",
        "tower-03"
      ]
    }
  ],
  "connections": [
    {
      "id": "pl-01",
      "from": "substation-01",
      "to": "tower-03",
      "type": "POWER_LINE",
      "capacity": 30,
      "latency": 0,
      "distance": 2.68
    },
    {
      "id": "pl-02",
      "from": "substation-01",
      "to": "mesh-01",
      "type": "POWER_LINE",
      "capacity": 12,
      "latency": 0,
      "distance": 2.32
    },
    {
      "id": "pl-03",
      "from": "substation-01",
      "to": "intersection-01",
      "type": "POWER_LINE",
      "capacity": 45,
      "latency": 0,
      "distance": 2.38
    },
    {
      "id": "pl-04",
      "from": "substation-03",
      "to": "intersection-02",
      "type": "POWER_LINE",
      "capacity": 45,
      "latency": 0,
      "distance": 2.39
    },
    {
      "id": "pl-05",
      "from": "substation-03",
      "to": "tower-04",
      "type": "POWER_LINE",
      "capacity": 30,
      "latency": 0,
      "distance": 8.78
    },
    {
      "id": "pl-06",
      "from": "substation-03",
      "to": "fire-03",
      "type": "POWER_LINE",
      "capacity": 40,
      "latency": 0,
      "distance": 2.1
    },
    {
      "id": "ml-01",
      "from": "tower-03",
      "to": "hospital-01",
      "type": "MESH_LINK",
      "capacity": 600,
      "latency": 24,
      "distance": 4.13
    },
    {
      "id": "ml-02",
      "from": "tower-03",
      "to": "eoc-01",
      "type": "MESH_LINK",
      "capacity": 700,
      "latency": 21,
      "distance": 2.1
    },
    {
      "id": "ml-03",
      "from": "mesh-01",
      "to": "eoc-01",
      "type": "MESH_LINK",
      "capacity": 96,
      "latency": 78,
      "distance": 2.44
    },
    {
      "id": "ml-04",
      "from": "mesh-01",
      "to": "fire-04",
      "type": "MESH_LINK",
      "capacity": 64,
      "latency": 90,
      "distance": 3.19
    },
    {
      "id": "ml-05",
      "from": "tower-04",
      "to": "intersection-02",
      "type": "MESH_LINK",
      "capacity": 200,
      "latency": 62,
      "distance": 6.49
    },
    {
      "id": "ml-06",
      "from": "tower-03",
      "to": "hospital-02",
      "type": "MESH_LINK",
      "capacity": 500,
      "latency": 33,
      "distance": 4.99
    },
    {
      "id": "rd-01",
      "from": "intersection-01",
      "to": "fire-01",
      "type": "ROAD_SEGMENT",
      "capacity": 35,
      "latency": 0,
      "distance": 0.85
    },
    {
      "id": "rd-02",
      "from": "intersection-01",
      "to": "fire-04",
      "type": "ROAD_SEGMENT",
      "capacity": 45,
      "latency": 0,
      "distance": 0.41
    },
    {
      "id": "rd-03",
      "from": "intersection-01",
      "to": "eoc-01",
      "type": "ROAD_SEGMENT",
      "capacity": 40,
      "latency": 0,
      "distance": 0.79
    },
    {
      "id": "rd-04",
      "from": "intersection-01",
      "to": "hospital-01",
      "type": "ROAD_SEGMENT",
      "capacity": 30,
      "latency": 0,
      "distance": 3.46
    },
    {
      "id": "rd-05",
      "from": "intersection-02",
      "to": "fire-02",
      "type": "ROAD_SEGMENT",
      "capacity": 25,
      "latency": 0,
      "distance": 1.73
    },
    {
      "id": "rd-06",
      "from": "intersection-02",
      "to": "fire-03",
      "type": "ROAD_SEGMENT",
      "capacity": 35,
      "latency": 0,
      "distance": 2.03
    },
    {
      "id": "rd-07",
      "from": "intersection-02",
      "to": "tower-04",
      "type": "ROAD_SEGMENT",
      "capacity": 55,
      "latency": 0,
      "distance": 6.49
    },
    {
      "id": "rd-08",
      "from": "intersection-01",
      "to": "hospital-02",
      "type": "ROAD_SEGMENT",
      "capacity": 40,
      "latency": 0,
      "distance": 3.77
    }
  ]
}`
