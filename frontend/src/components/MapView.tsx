import { useEffect, useMemo, useRef } from 'react'
import maplibregl, { type GeoJSONSource, type LngLatLike, type StyleSpecification } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useSimStore } from '../store/useSimStore'
import type { InfrastructureNode, NodeType } from '../types/simulation'

const CENTER: LngLatLike = [-82.478, 27.893]
const ZOOM = 11.4

const STYLE: StyleSpecification = {
  version: 8,
  sources: {
    base: {
      type: 'raster',
      tiles: [
        'https://a.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png',
        'https://b.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png',
        'https://c.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png',
      ],
      tileSize: 256,
      attribution: '© OpenStreetMap contributors © CARTO',
    },
  },
  layers: [{ id: 'base', type: 'raster', source: 'base' }],
}

const GLYPH: Record<NodeType, { label: string; color: string }> = {
  POWER_SUBSTATION: { label: '⚡', color: '#fbbf24' },
  CELL_TOWER: { label: '📡', color: '#22d3ee' },
  RADIO_MESH: { label: '📶', color: '#818cf8' },
  ROAD_INTERSECTION: { label: '🚦', color: '#cbd5e1' },
  HOSPITAL: { label: '🏥', color: '#34d399' },
  FIRE_STATION: { label: '🚒', color: '#fb923c' },
  EMERGENCY_OPS_CENTER: { label: '🏢', color: '#f472b6' },
}

type FeatureCollection = GeoJSON.FeatureCollection

function emptyFC(): FeatureCollection {
  return { type: 'FeatureCollection', features: [] }
}

function nodeFC(nodes: InfrastructureNode[]): FeatureCollection {
  return {
    type: 'FeatureCollection',
    features: nodes.map((n) => ({
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [n.lng, n.lat] },
      properties: {
        id: n.id,
        name: n.name,
        type: n.type,
        operational: n.operational,
        health: n.health,
        status: n.status,
        reason: n.reason ?? '',
        glyph: GLYPH[n.type].label,
        color: GLYPH[n.type].color,
        repairing: n.repairing,
        mesh: n.meshFallback,
      },
    })),
  }
}

function lineFC(
  state: ReturnType<typeof useSimStore.getState>['state'],
  type: 'POWER_LINE' | 'MESH_LINK' | 'ROAD_SEGMENT',
): FeatureCollection {
  if (!state) return emptyFC()
  const features = Object.values(state.connections)
    .filter((c) => c.type === type)
    .flatMap((c) => {
      const a = state.nodes[c.from]
      const b = state.nodes[c.to]
      if (!a || !b) return []
      const dead = c.blocked || !c.active || !a.operational || !b.operational
      return [
        {
          type: 'Feature' as const,
          geometry: {
            type: 'LineString' as const,
            coordinates: [
              [a.lng, a.lat],
              [b.lng, b.lat],
            ],
          },
          properties: {
            id: c.id,
            dead,
            blocked: c.blocked,
            label: (c.metadata?.label as string) ?? c.id,
            distance: c.distance,
            speed: c.speedKph,
          },
        },
      ]
    })
  return { type: 'FeatureCollection', features }
}

function routeFC(state: ReturnType<typeof useSimStore.getState>['state']): FeatureCollection {
  if (!state) return emptyFC()
  const seen = new Set<string>()
  const features = (state.diagnostics?.emsRoutes ?? []).flatMap((r) => {
    if (r.nodes.length < 2) return []
    const coords = r.nodes
      .map((id) => state.nodes[id])
      .filter((n): n is InfrastructureNode => Boolean(n))
      .map((n) => [n.lng, n.lat] as [number, number])
    if (coords.length < 2) return []
    const key = coords.map((c) => c.join(',')).join('|')
    if (seen.has(key)) return []
    seen.add(key)
    return [
      {
        type: 'Feature' as const,
        geometry: { type: 'LineString' as const, coordinates: coords },
        properties: {
          id: r.id,
          origin: r.origin,
          destination: r.destination,
          eta: r.etaMinutes,
          distance: r.distanceKm,
          changed: r.changed,
          degraded: r.degraded,
          label: `${r.origin} → ${r.destination}`,
        },
      },
    ]
  })
  return { type: 'FeatureCollection', features }
}

export default function MapView() {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const readyRef = useRef(false)
  const state = useSimStore((s) => s.state)
  const selectNode = useSimStore((s) => s.selectNode)
  const selectedNodeId = useSimStore((s) => s.selectedNodeId)
  const pulseIds = useSimStore((s) => s.pulseIds)

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: STYLE,
      center: CENTER,
      zoom: ZOOM,
      attributionControl: false,
    })
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
    map.addControl(new maplibregl.AttributionControl({ compact: true }), 'bottom-left')

    map.on('load', () => {
      map.addSource('power', { type: 'geojson', data: emptyFC() })
      map.addSource('mesh', { type: 'geojson', data: emptyFC() })
      map.addSource('roads', { type: 'geojson', data: emptyFC() })
      map.addSource('routes', { type: 'geojson', data: emptyFC() })
      map.addSource('nodes', { type: 'geojson', data: emptyFC() })

      map.addLayer({
        id: 'roads-case',
        type: 'line',
        source: 'roads',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: {
          'line-color': ['case', ['get', 'blocked'], '#ef4444', '#475569'],
          'line-width': ['interpolate', ['linear'], ['zoom'], 9, 2.5, 14, 7],
          'line-opacity': 0.85,
        },
      })
      map.addLayer({
        id: 'roads-x',
        type: 'symbol',
        source: 'roads',
        filter: ['==', ['get', 'blocked'], true],
        layout: {
          'text-field': '✕',
          'text-size': 13,
          'text-rotate': 0,
          'text-allow-overlap': true,
        },
        paint: { 'text-color': '#fecaca' },
      })
      map.addLayer({
        id: 'power-lines',
        type: 'line',
        source: 'power',
        layout: { 'line-cap': 'round' },
        paint: {
          'line-color': ['case', ['get', 'dead'], '#7f1d1d', '#facc15'],
          'line-width': ['interpolate', ['linear'], ['zoom'], 9, 1.4, 14, 3.4],
          'line-dasharray': [3, 2.5],
          'line-opacity': ['case', ['get', 'dead'], 0.4, 0.95],
        },
      })
      map.addLayer({
        id: 'mesh-links',
        type: 'line',
        source: 'mesh',
        layout: { 'line-cap': 'round' },
        paint: {
          'line-color': ['case', ['get', 'dead'], '#334155', '#22d3ee'],
          'line-width': ['interpolate', ['linear'], ['zoom'], 9, 0.8, 14, 2],
          'line-opacity': ['case', ['get', 'dead'], 0.25, 0.8],
        },
      })
      map.addLayer({
        id: 'routes-glow',
        type: 'line',
        source: 'routes',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: {
          'line-color': '#22c55e',
          'line-width': ['interpolate', ['linear'], ['zoom'], 9, 4, 14, 10],
          'line-opacity': 0.25,
          'line-blur': 3,
        },
      })
      map.addLayer({
        id: 'routes-line',
        type: 'line',
        source: 'routes',
        layout: { 'line-cap': 'round', 'line-join': 'round' },
        paint: {
          'line-color': ['case', ['get', 'degraded'], '#f59e0b', '#4ade80'],
          'line-width': ['interpolate', ['linear'], ['zoom'], 9, 1.6, 14, 3.6],
          'line-dasharray': [2.2, 1.6],
        },
      })
      map.addLayer({
        id: 'nodes-halo',
        type: 'circle',
        source: 'nodes',
        paint: {
          'circle-radius': ['case', ['get', 'operational'], 11, 13],
          'circle-color': ['case', ['get', 'operational'], '#22d3ee', '#ef4444'],
          'circle-opacity': 0.14,
          'circle-blur': 0.6,
        },
      })
      map.addLayer({
        id: 'nodes-core',
        type: 'circle',
        source: 'nodes',
        paint: {
          'circle-radius': ['case', ['==', ['get', 'type'], 'EMERGENCY_OPS_CENTER'], 9, 7],
          'circle-color': [
            'case',
            ['get', 'operational'],
            ['get', 'color'],
            '#7f1d1d',
          ],
          'circle-stroke-width': 1.4,
          'circle-stroke-color': ['case', ['get', 'operational'], '#0f172a', '#ef4444'],
          'circle-opacity': 0.95,
        },
      })
      map.addLayer({
        id: 'nodes-glyph',
        type: 'symbol',
        source: 'nodes',
        layout: {
          'text-field': ['get', 'glyph'],
          'text-size': 11,
          'text-allow-overlap': true,
          'text-ignore-placement': true,
        },
        paint: { 'text-color': '#0f172a' },
      })
      map.addLayer({
        id: 'nodes-label',
        type: 'symbol',
        source: 'nodes',
        minzoom: 12.2,
        layout: {
          'text-field': ['get', 'name'],
          'text-size': 10,
          'text-offset': [0, 1.5],
          'text-anchor': 'top',
          'text-allow-overlap': false,
        },
        paint: {
          'text-color': '#e2e8f0',
          'text-halo-color': '#020617',
          'text-halo-width': 1.4,
        },
      })

      map.on('click', 'nodes-core', (e) => {
        const id = e.features?.[0]?.properties?.id
        if (typeof id === 'string') selectNode(id)
      })
      map.on('mouseenter', 'nodes-core', () => {
        map.getCanvas().style.cursor = 'pointer'
      })
      map.on('mouseleave', 'nodes-core', () => {
        map.getCanvas().style.cursor = ''
      })

      readyRef.current = true
      readyRef.current = true
    })

    mapRef.current = map
    return () => {
      map.remove()
      mapRef.current = null
      readyRef.current = false
    }
  }, [selectNode])

  const nodes = useMemo(
    () => (state ? Object.values(state.nodes) : []),
    [state],
  )

  useEffect(() => {
    const map = mapRef.current
    if (!map || !readyRef.current) return
    const set = (id: string, data: FeatureCollection) => {
      const src = map.getSource(id) as GeoJSONSource | undefined
      src?.setData(data as never)
    }
    set('nodes', nodeFC(nodes))
    set('power', lineFC(state, 'POWER_LINE'))
    set('mesh', lineFC(state, 'MESH_LINK'))
    set('roads', lineFC(state, 'ROAD_SEGMENT'))
    set('routes', routeFC(state))
  }, [state, nodes])

  useEffect(() => {
    const map = mapRef.current
    if (!map || !readyRef.current || !pulseIds.length) return
    map.setFilter('nodes-halo', ['in', ['get', 'id'], ['literal', pulseIds]])
    map.setPaintProperty('nodes-halo', 'circle-radius', 20)
    map.setPaintProperty('nodes-halo', 'circle-opacity', 0.85)
    map.setPaintProperty('nodes-halo', 'circle-color', '#ef4444')
    const t = window.setTimeout(() => {
      if (!mapRef.current) return
      mapRef.current.setFilter('nodes-halo', null)
      mapRef.current.setPaintProperty('nodes-halo', 'circle-radius', ['case', ['get', 'operational'], 11, 13])
      mapRef.current.setPaintProperty('nodes-halo', 'circle-opacity', 0.14)
      mapRef.current.setPaintProperty('nodes-halo', 'circle-color', ['case', ['get', 'operational'], '#22d3ee', '#ef4444'])
    }, 1300)
    return () => window.clearTimeout(t)
  }, [pulseIds])

  useEffect(() => {
    const map = mapRef.current
    if (!map || !state || !selectedNodeId) return
    const n = state.nodes[selectedNodeId]
    if (!n) return
    map.easeTo({ center: [n.lng, n.lat], zoom: Math.max(map.getZoom(), 13), duration: 700 })
  }, [selectedNodeId, state])

  const selected = selectedNodeId && state ? state.nodes[selectedNodeId] : null

  return (
    <div className="relative h-full w-full">
      <div ref={containerRef} className="h-full w-full" />

      <div className="pointer-events-none absolute left-3 top-3 flex flex-col gap-1.5 font-mono text-[9px] uppercase tracking-wider">
        <div className="pointer-events-auto flex gap-2 rounded-md border border-ares-border bg-ares-panel/85 px-2 py-1.5 backdrop-blur">
          <LegendItem color="#facc15" label="Power" />
          <LegendItem color="#22d3ee" label="RescueNet" />
          <LegendItem color="#475569" label="Road" />
          <LegendItem color="#4ade80" label="EMS" />
        </div>
        {state && (
          <div className="pointer-events-auto rounded-md border border-ares-border bg-ares-panel/85 px-2 py-1.5 text-slate-400 backdrop-blur">
            {state.diagnostics?.blockedEdges.length ?? 0} blocked · {state.isolatedFacilities.length} isolated ·{' '}
            {state.diagnostics?.emsRoutes.length ?? 0} routes
          </div>
        )}
      </div>

      {selected && (
        <div className="pointer-events-auto absolute bottom-3 left-3 w-64 rounded-md border border-cyan-500/30 bg-ares-panel/95 p-2.5 backdrop-blur">
          <div className="flex items-center gap-2">
            <span className="text-base">{GLYPH[selected.type].label}</span>
            <div className="min-w-0">
              <p className="truncate text-[11px] font-semibold text-slate-100">{selected.name}</p>
              <p className="font-mono text-[9px] uppercase tracking-wider text-slate-500">
                {selected.type.replace(/_/g, ' ')}
              </p>
            </div>
          </div>
          <dl className="mt-1.5 grid grid-cols-2 gap-x-2 gap-y-0.5 font-mono text-[9px]">
            <dt className="text-slate-600">Status</dt>
            <dd className={selected.operational ? 'text-emerald-400' : 'text-red-400'}>{selected.status}</dd>
            <dt className="text-slate-600">Health</dt>
            <dd className="text-slate-300">{selected.health.toFixed(0)}%</dd>
            <dt className="text-slate-600">Deps</dt>
            <dd className="truncate text-slate-300">{selected.dependencies.length}</dd>
            {selected.reason && (
              <>
                <dt className="text-slate-600">Reason</dt>
                <dd className="truncate text-amber-400">{selected.reason}</dd>
              </>
            )}
            {selected.cascadeDepth > 0 && (
              <>
                <dt className="text-slate-600">Cascade</dt>
                <dd className="text-orange-400">depth {selected.cascadeDepth}</dd>
              </>
            )}
          </dl>
        </div>
      )}
    </div>
  )
}

function LegendItem({ color, label }: { color: string; label: string }) {
  return (
    <span className="flex items-center gap-1 text-slate-400">
      <span className="h-0.5 w-3.5" style={{ backgroundColor: color }} />
      {label}
    </span>
  )
}
