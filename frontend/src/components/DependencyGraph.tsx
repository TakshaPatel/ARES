import { useCallback, useEffect, useMemo } from 'react'
import {
  Background,
  Controls,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useSimStore } from '../store/useSimStore'
import { C, NODE_COLOR, NODE_EMOJI } from '../lib/theme'
import type { InfrastructureNode, NodeType } from '../types/simulation'

type ArenaNode = Node<{ label: string; type: NodeType; down: boolean; depth: number; health: number; selected: boolean }>

function ArenaNodeView({ data, selected }: NodeProps<ArenaNode>) {
  const color = data.down ? C.danger : NODE_COLOR[data.type]
  return (
    <div
      className="rounded border px-2 py-1.5 text-center transition"
      style={{
        width: NODE_W,
        borderColor: selected ? '#ffffff' : color,
        background: data.down ? 'rgba(40,16,16,0.9)' : 'rgba(22,23,26,0.95)',
        boxShadow: data.down ? `0 0 12px ${color}55` : selected ? `0 0 0 1px ${color}66` : 'none',
      }}
    >
      <Handle type="target" position={Position.Left} className="!h-1.5 !w-1.5 !border-0 !bg-neutral-600" />
      <div className="text-sm leading-none">{NODE_EMOJI[data.type]}</div>
      <div className="mt-0.5 truncate text-xs font-medium text-neutral-100">{data.label}</div>
      <div className="mt-0.5 h-0.5 w-full overflow-hidden rounded bg-neutral-800">
        <div
          className="h-full transition-all"
          style={{ width: `${Math.max(0, Math.min(100, data.health))}%`, background: color }}
        />
      </div>
      {data.depth > 0 && (
        <div className="absolute -right-1 -top-1 rounded-full bg-orange-500 px-1 font-mono text-xs leading-tight text-black">
          {data.depth}
        </div>
      )}
      <Handle type="source" position={Position.Right} className="!h-1.5 !w-1.5 !border-0 !bg-neutral-600" />
    </div>
  )
}

const nodeTypes = { arena: ArenaNodeView }

const NODE_W = 150
const GAP_X = 96
const NODE_H = 74
const GAP_Y = 26

const TYPE_RANK: Record<NodeType, number> = {
  POWER_SUBSTATION: 0,
  CELL_TOWER: 1,
  RADIO_MESH: 2,
  ROAD_INTERSECTION: 3,
  FIRE_STATION: 4,
  HOSPITAL: 4,
  EMERGENCY_OPS_CENTER: 5,
}

function layout(nodes: InfrastructureNode[]): Map<string, { x: number; y: number }> {
  const pos = new Map<string, { x: number; y: number }>()
  if (nodes.length === 0) return pos

  const byId = new Map(nodes.map((n) => [n.id, n]))
  const depth = new Map<string, number>()
  const visiting = new Set<string>()

  const resolve = (id: string): number => {
    const cached = depth.get(id)
    if (cached !== undefined) return cached
    if (visiting.has(id)) return 0
    visiting.add(id)
    let d = 0
    for (const dep of byId.get(id)?.dependencies ?? []) {
      if (byId.has(dep)) {
        const child = resolve(dep)
        if (child + 1 > d) d = child + 1
      }
    }
    visiting.delete(id)
    depth.set(id, d)
    return d
  }

  for (const n of nodes) resolve(n.id)

  const columns = new Map<number, InfrastructureNode[]>()
  for (const n of nodes) {
    const d = depth.get(n.id) ?? 0
    const bucket = columns.get(d)
    if (bucket) bucket.push(n)
    else columns.set(d, [n])
  }

  const tallest = Math.max(...[...columns.values()].map((c) => c.length))
  const fullHeight = (tallest - 1) * (NODE_H + GAP_Y)

  for (const [d, bucket] of columns) {
    bucket.sort(
      (a, b) => TYPE_RANK[a.type] - TYPE_RANK[b.type] || a.id.localeCompare(b.id),
    )
    const height = (bucket.length - 1) * (NODE_H + GAP_Y)
    const offset = (fullHeight - height) / 2
    bucket.forEach((n, i) => {
      pos.set(n.id, {
        x: d * (NODE_W + GAP_X),
        y: offset + i * (NODE_H + GAP_Y) - fullHeight / 2,
      })
    })
  }
  return pos
}

export default function DependencyGraph({ active = true }: { active?: boolean }) {
  return (
    <ReactFlowProvider>
      <GraphInner active={active} />
    </ReactFlowProvider>
  )
}

function GraphInner({ active }: { active: boolean }) {
  const state = useSimStore((s) => s.state)
  const pulseIds = useSimStore((s) => s.pulseIds)
  const selectNode = useSimStore((s) => s.selectNode)
  const selectedNodeId = useSimStore((s) => s.selectedNodeId)

  const rfNodes = useMemo<ArenaNode[]>(() => {
    if (!state) return []
    const list = Object.values(state.nodes)
    const pos = layout(list)
    return list.map((n) => ({
      id: n.id,
      type: 'arena',
      position: pos.get(n.id) ?? { x: 0, y: 0 },
      data: {
        label: n.name,
        type: n.type,
        down: !n.operational,
        depth: n.cascadeDepth,
        health: n.health,
        selected: n.id === selectedNodeId,
      },
    }))
  }, [state, selectedNodeId])

  const rfEdges = useMemo<Edge[]>(() => {
    if (!state) return []
    const edges = state.diagnostics?.dependencyEdges ?? []
    return edges
      .filter((e) => e.kind === 'DEPENDENCY' || e.kind === 'ROAD_SEGMENT')
      .map((e) => {
        const pulsing = pulseIds.includes(e.source) || pulseIds.includes(e.target)
        const depTargetDown = state.nodes[e.target] && !state.nodes[e.target].operational
        const color = e.kind === 'DEPENDENCY'
          ? depTargetDown
            ? C.danger
            : C.comms
          : state.connections[e.id.replace('conn:', '')]?.blocked
            ? C.danger
            : C.road
        return {
          id: e.id,
          source: e.source,
          target: e.target,
          type: 'smoothstep',
          animated: pulsing || depTargetDown,
          style: {
            stroke: pulsing ? C.danger : color,
            strokeWidth: pulsing ? 2.6 : e.kind === 'DEPENDENCY' ? 1.3 : 0.9,
            strokeDasharray: e.kind === 'DEPENDENCY' ? undefined : '4 3',
            opacity: pulsing ? 1 : 0.55,
          },
        }
      })
  }, [state, pulseIds])

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      selectNode(node.id)
    },
    [selectNode],
  )

  useEffect(() => {
    if (selectedNodeId && rfNodes.some((n) => n.id === selectedNodeId)) {
      const el = document.querySelector(`[data-id="${selectedNodeId}"]`)
      el?.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' })
    }
  }, [selectedNodeId, rfNodes])

  const { fitView } = useReactFlow()

  useEffect(() => {
    if (!active) return
    let raf = 0
    let tries = 0
    const attempt = () => {
      const el = document.querySelector('.react-flow')
      if (!el || el.clientWidth < 40 || el.clientHeight < 40) {
        if (tries++ < 30) raf = requestAnimationFrame(attempt)
        return
      }
      void fitView({ padding: 0.18, duration: 260 })
    }
    raf = requestAnimationFrame(attempt)
    return () => cancelAnimationFrame(raf)
  }, [active, fitView])

  const stats = useMemo(() => {
    if (!state) return { total: 0, down: 0, edges: 0, depth: 0 }
    const list = Object.values(state.nodes)
    return {
      total: list.length,
      down: list.filter((n) => !n.operational).length,
      edges: (state.diagnostics?.dependencyEdges ?? []).filter(
        (e) => e.kind === 'DEPENDENCY' || e.kind === 'ROAD_SEGMENT',
      ).length,
      depth: Math.max(0, ...list.map((n) => n.cascadeDepth)),
    }
  }, [state])

  if (!state) {
    return <div className="flex h-full items-center justify-center text-sm text-neutral-500">Loading graph…</div>
  }

  return (
    <div className="relative h-full w-full">
      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        fitView={false}
        minZoom={0.15}
        maxZoom={2.2}
        proOptions={{ hideAttribution: true }}
        nodesDraggable
        nodesConnectable={false}
        elementsSelectable
      >
        <Background color={C.grid} gap={22} size={1} />
        <Controls showInteractive={false} className="!bg-neutral-900 !border-neutral-700" />
        <MiniMap
          pannable
          zoomable
          className="!bg-neutral-900 !border-neutral-700"
          nodeColor={(n) => {
            const d = n.data as ArenaNode['data']
            return d.down ? C.danger : NODE_COLOR[d.type]
          }}
          maskColor="rgba(2,6,23,0.75)"
        />
      </ReactFlow>

      <div className="pointer-events-none absolute left-3 top-3 flex flex-col gap-1 text-xs">
        <div className="rounded border border-ares-border bg-neutral-900/85 px-2 py-1.5 text-neutral-400 backdrop-blur">
          <span className="font-mono tabular-nums">{stats.total}</span> nodes ·{' '}
          <span className="font-mono tabular-nums">{stats.edges}</span> edges ·{' '}
          <span className={`font-mono tabular-nums ${stats.down ? 'text-red-300' : 'text-neutral-100'}`}>{stats.down} down</span> · max depth{' '}
          <span className="font-mono tabular-nums">{stats.depth}</span>
        </div>
        <div className="flex flex-wrap gap-2 rounded border border-ares-border bg-neutral-900/85 px-2 py-1.5 backdrop-blur">
          {(Object.keys(NODE_COLOR) as NodeType[]).map((t) => (
            <span key={t} className="flex items-center gap-1 text-neutral-400">
              <span className="h-1.5 w-1.5 rounded-full" style={{ background: NODE_COLOR[t] }} />
              {t.replace(/_/g, ' ').toLowerCase()}
            </span>
          ))}
        </div>
      </div>

      {pulseIds.length > 0 && (
        <div className="pointer-events-none absolute right-3 top-3 rounded border border-red-500/50 bg-red-500/10 px-2 py-1 text-xs font-medium text-red-200 backdrop-blur">
          Cascade pulse · <span className="font-mono tabular-nums">{pulseIds.length}</span> node{pulseIds.length === 1 ? '' : 's'}
        </div>
      )}
    </div>
  )
}
