import { create } from 'zustand'
import type {
  CityView,
  InboundAction,
  TabKey,
  MetricSample,
  Preset,
  ScenarioSummary,
  RunState,
  SimulationState,
} from '../types/simulation'

const HISTORY_LIMIT = 180

const arr = <T,>(v: T[] | null | undefined): T[] => (Array.isArray(v) ? v : [])
const rec = <T,>(v: Record<string, T> | null | undefined): Record<string, T> =>
  v && typeof v === 'object' ? v : {}

function normalizeState(s: SimulationState): SimulationState {
  const nodes = rec(s.nodes)
  for (const id of Object.keys(nodes)) {
    const n = nodes[id]
    if (!n) continue
    nodes[id] = { ...n, dependencies: arr(n.dependencies), metadata: rec(n.metadata) }
  }
  for (const id of Object.keys(rec(s.connections))) {
    const c = s.connections[id]
    if (c) s.connections[id] = { ...c, metadata: rec(c.metadata) }
  }
  const d = s.diagnostics
  return {
    ...s,
    nodes,
    connections: rec(s.connections),
    isolatedFacilities: arr(s.isolatedFacilities),
    activeAlerts: arr(s.activeAlerts),
    activeEmsRoutes: rec(s.activeEmsRoutes),
    diagnostics: d
      ? {
          ...d,
          events: arr(d.events),
          commsClusters: arr(d.commsClusters).map(arr),
          emsRoutes: arr(d.emsRoutes).map((r) => ({ ...r, nodes: arr(r.nodes) })),
          meshBackoff: arr(d.meshBackoff),
          dependencyEdges: arr(d.dependencyEdges),
          blockedEdges: arr(d.blockedEdges),
        }
      : d,
  }
}

interface SimStore {
  state: SimulationState | null
  runState: RunState
  connected: boolean
  history: MetricSample[]
  presets: Preset[]
  scenarios: ScenarioSummary[]
  activeScenario: string
  tab: TabKey
  view: CityView
  tourOpen: boolean
  lastError: string | null
  selectedNodeId: string | null
  pulseIds: string[]

  applyState: (s: SimulationState) => void
  setRunState: (r: RunState) => void
  setConnected: (c: boolean) => void
  setPresets: (p: Preset[]) => void
  setScenarios: (s: ScenarioSummary[], active: string) => void
  setTab: (t: TabKey) => void
  setView: (v: Partial<CityView>) => void
  setTourOpen: (b: boolean) => void
  setError: (e: string | null) => void
  selectNode: (id: string | null) => void
  setPulse: (ids: string[]) => void
  resetHistory: () => void
}

let lastRevision = -1
let lastEventId = 0

function appendSample(history: MetricSample[], sample: MetricSample): MetricSample[] {
  const next = [...history, sample]
  return next.length > HISTORY_LIMIT ? next.slice(next.length - HISTORY_LIMIT) : next
}

function sampleFrom(s: SimulationState): MetricSample {
  return {
    tick: s.tick,
    clock: s.diagnostics?.clock ?? '--:--',
    powerGridHealth: s.powerGridHealth,
    commsCoverage: s.commsCoverage,
    roadAccessibility: s.roadAccessibility,
    messageDeliveryRate: s.messageDeliveryRate,
    averageLatency: s.averageLatency,
    connectedFacilities: s.connectedFacilities,
    totalFacilities: s.totalFacilities,
    isolated: s.isolatedFacilities.length,
  }
}

function pulseTargets(s: SimulationState): string[] {
  const fresh = s.diagnostics?.events?.filter((e) => e.depth > 0) ?? []
  if (fresh.length === 0) return []
  const newest = fresh.reduce((max, e) => Math.max(max, e.id), 0)
  if (newest !== lastEventId) {
    lastEventId = newest
    return [...new Set(fresh.map((e) => e.nodeId).filter((x): x is string => Boolean(x)))]
  }
  return []
}

export const useSimStore = create<SimStore>((set, get) => ({
  state: null,
  runState: 'CONNECTING',
  connected: false,
  history: [],
  presets: [],
  scenarios: [],
  activeScenario: '',
  tab: 'map',
  view: { center: null, zoom: null, label: null, source: null },
  tourOpen: false,
  lastError: null,
  selectedNodeId: null,
  pulseIds: [],

  applyState: (raw) => {
    const s = normalizeState(raw)
    const rev = s.diagnostics?.revision ?? 0
    const isNew = rev !== lastRevision
    lastRevision = rev
    const pulse = isNew ? pulseTargets(s) : []
    set({
      state: s,
      history: isNew ? appendSample(get().history, sampleFrom(s)) : get().history,
      runState: s.running ? 'RUNNING' : get().connected ? 'PAUSED' : get().runState,
      ...(pulse.length ? { pulseIds: pulse } : {}),
    })
    if (pulse.length) {
      window.setTimeout(() => {
        if (get().pulseIds === pulse) set({ pulseIds: [] })
      }, 1400)
    }
  },

  setRunState: (runState) => set({ runState }),
  setConnected: (connected) =>
    set({ connected, runState: connected ? get().runState : 'OFFLINE' }),
  setPresets: (presets) => set({ presets }),
  setScenarios: (scenarios, active) => set({ scenarios, activeScenario: active }),
  setTab: (tab) => set({ tab }),
  setView: (v) => set({ view: { ...get().view, ...v } }),
  setTourOpen: (tourOpen) => set({ tourOpen }),
  setError: (lastError) => set({ lastError }),
  selectNode: (selectedNodeId) => set({ selectedNodeId }),
  setPulse: (pulseIds) => set({ pulseIds }),
  resetHistory: () => {
    lastRevision = -1
    lastEventId = 0
    set({ history: [], pulseIds: [] })
  },
}))

let socket: WebSocket | null = null
let reconnectTimer: number | null = null
let reconnectDelay = 1000

export function send(action: InboundAction, payload: Record<string, unknown> = {}) {
  if (!socket || socket.readyState !== WebSocket.OPEN) {
    useSimStore.getState().setError('WebSocket not connected')
    return
  }
  socket.send(JSON.stringify({ action, payload }))
}

export function connect(): () => void {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${window.location.host}/ws`
  const store = useSimStore.getState()

  if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
    return () => undefined
  }

  store.setRunState('CONNECTING')
  socket = new WebSocket(url)

  socket.onopen = () => {
    reconnectDelay = 1000
    useSimStore.getState().setConnected(true)
    useSimStore.getState().setError(null)
    void loadPresets()
  }

  socket.onmessage = (ev) => {
    let msg: { type?: string; state?: SimulationState; error?: string }
    try {
      msg = JSON.parse(ev.data as string)
    } catch {
      return
    }
    if (msg.type === 'STATE' && msg.state) {
      useSimStore.getState().applyState(msg.state)
    } else if (msg.type === 'ERROR' && msg.error) {
      useSimStore.getState().setError(msg.error)
    }
  }

  socket.onclose = () => {
    useSimStore.getState().setConnected(false)
    if (reconnectTimer) window.clearTimeout(reconnectTimer)
    reconnectTimer = window.setTimeout(() => {
      reconnectDelay = Math.min(reconnectDelay * 2, 15000)
      connect()
    }, reconnectDelay)
  }

  socket.onerror = () => {
    useSimStore.getState().setError('WebSocket error')
  }

  return () => {
    if (reconnectTimer) window.clearTimeout(reconnectTimer)
    socket?.close()
    socket = null
  }
}

export async function loadPresets() {
  try {
    const r = await fetch('/api/simulation/presets')
    if (!r.ok) return
    const data = (await r.json()) as { presets: Preset[] }
    useSimStore.getState().setPresets(data.presets ?? [])
  } catch {
    /* server not up yet */
  }
}

export async function loadScenarios() {
  try {
    const r = await fetch('/api/scenarios')
    if (!r.ok) return
    const data = (await r.json()) as { scenarios: ScenarioSummary[]; active: string }
    useSimStore.getState().setScenarios(data.scenarios ?? [], data.active ?? '')
  } catch {
    /* ignore */
  }
}

export async function post(path: string, body: unknown) {
  try {
    const r = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!r.ok) {
      const err = (await r.json().catch(() => ({}))) as { error?: string }
      useSimStore.getState().setError(err.error ?? `HTTP ${r.status}`)
      return null
    }
    return await r.json()
  } catch (e) {
    useSimStore.getState().setError(String(e))
    return null
  }
}
