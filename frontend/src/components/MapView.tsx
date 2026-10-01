import { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle } from 'lucide-react'
import maplibregl, { type GeoJSONSource, type StyleSpecification } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useSimStore } from '../store/useSimStore'
import { CENTER, FALLBACK_STYLE, SATELLITE_STYLE, ZOOM } from '../lib/maptiler'
import { C, NODE_COLOR, NODE_LETTER } from '../lib/theme'
import type { InfrastructureNode, NodeType } from '../types/simulation'

const GLYPH: Record<NodeType, { label: string; color: string }> = {
  POWER_SUBSTATION: { label: NODE_LETTER.POWER_SUBSTATION, color: NODE_COLOR.POWER_SUBSTATION },
  CELL_TOWER: { label: NODE_LETTER.CELL_TOWER, color: NODE_COLOR.CELL_TOWER },
  RADIO_MESH: { label: NODE_LETTER.RADIO_MESH, color: NODE_COLOR.RADIO_MESH },
  ROAD_INTERSECTION: { label: NODE_LETTER.ROAD_INTERSECTION, color: NODE_COLOR.ROAD_INTERSECTION },
  HOSPITAL: { label: NODE_LETTER.HOSPITAL, color: NODE_COLOR.HOSPITAL },
  FIRE_STATION: { label: NODE_LETTER.FIRE_STATION, color: NODE_COLOR.FIRE_STATION },
  EMERGENCY_OPS_CENTER: {
    label: NODE_LETTER.EMERGENCY_OPS_CENTER,
    color: NODE_COLOR.EMERGENCY_OPS_CENTER,
  },
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

export default function MapView({ active = true }: { active?: boolean }) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const readyRef = useRef(false)
  const state = useSimStore((s) => s.state)
  const selectNode = useSimStore((s) => s.selectNode)
  const selectedNodeId = useSimStore((s) => s.selectedNodeId)
  const pulseIds = useSimStore((s) => s.pulseIds)
  const viewCenter = useSimStore((s) => s.view.center)
  const viewZoom = useSimStore((s) => s.view.zoom)
  const viewLabel = useSimStore((s) => s.view.label)
  const [mapError, setMapError] = useState<string | null>(null)

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return
    const canFallback = SATELLITE_STYLE !== null
    let map: maplibregl.Map
    try {
      map = new maplibregl.Map({
        container: containerRef.current,
        style: SATELLITE_STYLE ?? FALLBACK_STYLE,
        center: viewCenter ?? CENTER,
        zoom: viewZoom ?? ZOOM,
        attributionControl: false,
      })
    } catch (err) {
      console.warn('map init failed', err)
      setMapError('Map rendering is unavailable in this browser (WebGL disabled). The rest of the console still works.')
      return
    }
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
    map.addControl(new maplibregl.AttributionControl({ compact: true }), 'bottom-left')

    let fellBack = false
    map.on('error', (e) => {
      if (!fellBack && canFallback) {
        fellBack = true
        map.setStyle(FALLBACK_STYLE as StyleSpecification)
        return
      }
      console.warn('map error', e)
    })

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
          'line-color': ['case', ['get', 'blocked'], C.danger, C.road],
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
          'text-field': 'x',
          'text-font': ['Noto Sans Regular'],
          'text-size': 13,
          'text-rotate': 0,
          'text-allow-overlap': true,
        },
        paint: { 'text-color': C.ink },
      })
      map.addLayer({
        id: 'power-lines',
        type: 'line',
        source: 'power',
        layout: { 'line-cap': 'round' },
        paint: {
          'line-color': ['case', ['get', 'dead'], C.deadLine, NODE_COLOR.POWER_SUBSTATION],
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
          'line-color': ['case', ['get', 'dead'], C.dead, NODE_COLOR.CELL_TOWER],
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
          'line-color': C.ink,
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
          'line-color': ['case', ['get', 'degraded'], C.warn, C.ink],
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
          'circle-color': ['case', ['get', 'operational'], C.comms, C.danger],
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
            C.dangerFill,
          ],
          'circle-stroke-width': 1.4,
          'circle-stroke-color': ['case', ['get', 'operational'], C.darkInk, C.danger],
          'circle-opacity': 0.95,
        },
      })
      map.addLayer({
        id: 'nodes-glyph',
        type: 'symbol',
        source: 'nodes',
        layout: {
          'text-field': ['get', 'glyph'],
          'text-font': ['Noto Sans Regular'],
          'text-size': 10,
          'text-allow-overlap': true,
          'text-ignore-placement': true,
        },
        paint: { 'text-color': C.darkInk },
      })
      map.addLayer({
        id: 'nodes-label',
        type: 'symbol',
        source: 'nodes',
        minzoom: 12.2,
        layout: {
          'text-field': ['get', 'name'],
          'text-font': ['Noto Sans Regular'],
          'text-size': 10,
          'text-offset': [0, 1.5],
          'text-anchor': 'top',
          'text-allow-overlap': false,
        },
        paint: {
          'text-color': C.ink,
          'text-halo-color': C.halo,
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
    map.setPaintProperty('nodes-halo', 'circle-color', C.danger)
    const t = window.setTimeout(() => {
      if (!mapRef.current) return
      mapRef.current.setFilter('nodes-halo', null)
      mapRef.current.setPaintProperty('nodes-halo', 'circle-radius', ['case', ['get', 'operational'], 11, 13])
      mapRef.current.setPaintProperty('nodes-halo', 'circle-opacity', 0.14)
      mapRef.current.setPaintProperty('nodes-halo', 'circle-color', ['case', ['get', 'operational'], C.comms, C.danger])
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

  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    if (viewCenter) map.jumpTo({ center: viewCenter, zoom: viewZoom ?? map.getZoom() })
    else map.jumpTo({ center: CENTER, zoom: ZOOM })
  }, [viewCenter, viewZoom])

  useEffect(() => {
    if (!active) return
    const map = mapRef.current
    const el = containerRef.current
    if (!map || !el) return
    const apply = () => {
      if (el.clientWidth > 0 && el.clientHeight > 0) map.resize()
    }
    apply()
    const ro = new ResizeObserver(apply)
    ro.observe(el)
    return () => ro.disconnect()
  }, [active])

  const selected = selectedNodeId && state ? state.nodes[selectedNodeId] : null

  return (
    <div className="relative h-full w-full">
      <div ref={containerRef} className="h-full w-full" />

      {mapError && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-ares-bg p-6 text-center">
          <AlertTriangle className="h-6 w-6 text-amber-400" />
          <p className="max-w-sm text-sm leading-relaxed text-neutral-400">{mapError}</p>
          <p className="text-xs text-neutral-600">
            Use the Dependency DAG, Reports and Upload tabs in the meantime.
          </p>
        </div>
      )}

      <div className="pointer-events-none absolute left-3 top-3 flex flex-col gap-1.5 text-xs">
        <div className="pointer-events-auto flex gap-2 rounded border border-neutral-800 bg-neutral-900/85 px-2 py-1.5 backdrop-blur">
          <LegendItem color={NODE_COLOR.POWER_SUBSTATION} label="Power" />
          <LegendItem color={NODE_COLOR.CELL_TOWER} label="Comms" />
          <LegendItem color={C.road} label="Road" />
          <LegendItem color={NODE_COLOR.HOSPITAL} label="EMS" />
        </div>
        {viewLabel && (
          <div className="pointer-events-auto max-w-[220px] truncate rounded border border-neutral-700 bg-neutral-900/85 px-2 py-1.5 text-xs text-neutral-300 backdrop-blur">
            {viewLabel}
          </div>
        )}
        {state && (
          <div className="pointer-events-auto rounded border border-neutral-800 bg-neutral-900/85 px-2 py-1.5 text-neutral-400 backdrop-blur">
            {state.diagnostics?.blockedEdges.length ?? 0} blocked · {state.isolatedFacilities.length} isolated ·{' '}
            {state.diagnostics?.emsRoutes.length ?? 0} routes
          </div>
        )}
      </div>

      {selected && (
        <div className="pointer-events-auto absolute bottom-3 left-3 w-64 rounded border border-neutral-600 bg-neutral-900/95 p-2.5 backdrop-blur">
          <div className="flex items-center gap-2">
            <span className="text-base">{GLYPH[selected.type].label}</span>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-neutral-50">{selected.name}</p>
              <p className="text-xs text-neutral-500">
                {selected.type.replace(/_/g, ' ')}
              </p>
            </div>
          </div>
          <dl className="mt-2 grid grid-cols-2 gap-x-2 gap-y-0.5 text-xs">
            <dt className="text-neutral-600">Status</dt>
            <dd className={selected.operational ? 'text-emerald-400' : 'text-red-400'}>{selected.status}</dd>
            <dt className="text-neutral-600">Health</dt>
            <dd className="text-neutral-300">{selected.health.toFixed(0)}%</dd>
            <dt className="text-neutral-600">Deps</dt>
            <dd className="truncate text-neutral-300">{selected.dependencies.length}</dd>
            {selected.reason && (
              <>
                <dt className="text-neutral-600">Reason</dt>
                <dd className="truncate text-amber-400">{selected.reason}</dd>
              </>
            )}
            {selected.cascadeDepth > 0 && (
              <>
                <dt className="text-neutral-600">Cascade</dt>
                <dd className="text-orange-300">depth {selected.cascadeDepth}</dd>
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
    <span className="flex items-center gap-1 text-neutral-400">
      <span className="h-0.5 w-3.5" style={{ backgroundColor: color }} />
      {label}
    </span>
  )
}
