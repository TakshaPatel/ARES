import { useCallback, useEffect, useMemo } from 'react'
import {
  Background,
  Controls,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useSimStore } from '../store/useSimStore'
import type { InfrastructureNode, NodeType } from '../types/simulation'

const TYPE_COLOR: Record<NodeType, string> = {
  POWER_SUBSTATION: '#fbbf24',
  CELL_TOWER: '#22d3ee',
  RADIO_MESH: '#818cf8',
  ROAD_INTERSECTION: '#94a3b8',
  HOSPITAL: '#34d399',
  FIRE_STATION: '#fb923c',
  EMERGENCY_OPS_CENTER: '#f472b6',
}

const TYPE_GLYPH: Record<NodeType, string> = {
  POWER_SUBSTATION: '⚡',
  CELL_TOWER: '📡',
  RADIO_MESH: '📶',
  ROAD_INTERSECTION: '🚦',
  HOSPITAL: '🏥',
  FIRE_STATION: '🚒',
  EMERGENCY_OPS_CENTER: '🏢',
}

type ArenaNode = Node<{ label: string; type: NodeType; down: boolean; depth: number; health: number; selected: boolean }>

function ArenaNodeView({ data, selected }: NodeProps<ArenaNode>) {
  const color = data.down ? '#ef4444' : TYPE_COLOR[data.type]
  return (
    <div
      className="min-w-[118px] rounded-md border px-2 py-1.5 text-center transition"
      style={{
        borderColor: selected ? '#22d3ee' : color,
        background: data.down ? 'rgba(127,29,29,0.35)' : 'rgba(13,19,27,0.94)',
        boxShadow: data.down ? `0 0 12px ${color}55` : selected ? `0 0 0 1px ${color}66` : 'none',
      }}
    >
      <Handle type="target" position={Position.Left} className="!h-1.5 !w-1.5 !border-0 !bg-slate-600" />
      <div className="text-sm leading-none">{TYPE_GLYPH[data.type]}</div>
      <div className="mt-0.5 truncate font-mono text-[9px] font-semibold text-slate-200">{data.label}</div>
      <div className="mt-0.5 h-0.5 w-full overflow-hidden rounded bg-slate-800">
        <div
          className="h-full transition-all"
          style={{ width: `${Math.max(0, Math.min(100, data.health))}%`, background: color }}
        />
      </div>
      {data.depth > 0 && (
        <div className="absolute -right-1 -top-1 rounded-full bg-orange-500 px-1 font-mono text-[8px] text-black">
          {data.depth}
        </div>
      )}
      <Handle type="source" position={Position.Right} className="!h-1.5 !w-1.5 !border-0 !bg-slate-600" />
    </div>
  )
}

const nodeTypes = { arena: ArenaNodeView }

function layout(nodes: InfrastructureNode[]): Map<string, { x: number; y: number }> {
  const byType: NodeType[] = [
    'POWER_SUBSTATION',
    'CELL_TOWER',
    'RADIO_MESH',
    'ROAD_INTERSECTION',
    'FIRE_STATION',
    'HOSPITAL',
    'EMERGENCY_OPS_CENTER',
  ]
  const pos = new Map<string, { x: number; y: number }>()
  byType.forEach((t, ti) => {
    const members = nodes.filter((n) => n.type === t).sort((a, b) => a.id.localeCompare(b.id))
    const perCol = Math.max(1, Math.ceil(members.length / 2))
    members.forEach((n, i) => {
      const col = Math.floor(i / perCol)
      const row = i % perCol
      pos.set(n.id, {
        x: col * 250 + ti * 40,
        y: row * 96 + ti * 22,
      })
    })
  })
  return pos
}

export default function DependencyGraph() {
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
            ? '#ef4444'
            : '#22d3ee'
          : state.connections[e.id.replace('conn:', '')]?.blocked
            ? '#ef4444'
            : '#475569'
        return {
          id: e.id,
          source: e.source,
          target: e.target,
          type: 'smoothstep',
          animated: pulsing || depTargetDown,
          style: {
            stroke: pulsing ? '#ef4444' : color,
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

  const stats = useMemo(() => {
    if (!state) return { total: 0, down: 0, edges: 0, depth: 0 }
    const list = Object.values(state.nodes)
    return {
      total: list.length,
      down: list.filter((n) => !n.operational).length,
      edges: state.diagnostics?.dependencyEdges.length ?? 0,
      depth: Math.max(0, ...list.map((n) => n.cascadeDepth)),
    }
  }, [state])

  if (!state) {
    return <div className="flex h-full items-center justify-center font-mono text-[11px] text-slate-600">Loading graph…</div>
  }

  return (
    <div className="relative h-full w-full">
      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        fitView
        fitViewOptions={{ padding: 0.18 }}
        minZoom={0.15}
        maxZoom={2.2}
        proOptions={{ hideAttribution: true }}
        nodesDraggable
        nodesConnectable={false}
        elementsSelectable
      >
        <Background color="#1e2936" gap={22} size={1} />
        <Controls showInteractive={false} className="!bg-slate-900 !border-ares-border" />
        <MiniMap
          pannable
          zoomable
          className="!bg-slate-900 !border-ares-border"
          nodeColor={(n) => {
            const d = n.data as ArenaNode['data']
            return d.down ? '#ef4444' : TYPE_COLOR[d.type]
          }}
          maskColor="rgba(2,6,23,0.75)"
        />
      </ReactFlow>

      <div className="pointer-events-none absolute left-3 top-3 flex flex-col gap-1 font-mono text-[9px] uppercase tracking-wider">
        <div className="rounded-md border border-ares-border bg-ares-panel/85 px-2 py-1.5 text-slate-400 backdrop-blur">
          {stats.total} nodes · {stats.edges} edges ·{' '}
          <span className={stats.down ? 'text-red-400' : 'text-emerald-400'}>{stats.down} down</span> · max depth{' '}
          {stats.depth}
        </div>
        <div className="flex flex-wrap gap-2 rounded-md border border-ares-border bg-ares-panel/85 px-2 py-1.5 backdrop-blur">
          {(Object.keys(TYPE_COLOR) as NodeType[]).map((t) => (
            <span key={t} className="flex items-center gap-1 text-slate-400">
              <span className="h-1.5 w-1.5 rounded-full" style={{ background: TYPE_COLOR[t] }} />
              {t.replace(/_/g, ' ').slice(0, 10)}
            </span>
          ))}
        </div>
      </div>

      {pulseIds.length > 0 && (
        <div className="pointer-events-none absolute right-3 top-3 rounded-md border border-red-500/50 bg-red-500/10 px-2 py-1 font-mono text-[9px] uppercase tracking-wider text-red-300 backdrop-blur">
          Cascade pulse · {pulseIds.length} node{pulseIds.length === 1 ? '' : 's'}
        </div>
      )}
    </div>
  )
}
