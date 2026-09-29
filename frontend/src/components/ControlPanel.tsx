import { useMemo, useState } from 'react'
import {
  AlertOctagon,
  Ban,
  ChevronDown,
  Pause,
  Play,
  RotateCcw,
  SkipForward,
  Skull,
  Wrench,
  Zap,
} from 'lucide-react'
import { useSimStore, send } from '../store/useSimStore'
import type { NodeType, Preset } from '../types/simulation'

const TARGETS: { label: string; type: NodeType; ids: string[] }[] = [
  { label: 'Substations', type: 'POWER_SUBSTATION', ids: ['substation-01', 'substation-02', 'substation-03', 'substation-04'] },
  { label: 'Comms Sites', type: 'CELL_TOWER', ids: ['tower-01', 'tower-02', 'tower-03', 'tower-04', 'tower-05', 'tower-06'] },
  { label: 'Intersections', type: 'ROAD_INTERSECTION', ids: ['intersection-01', 'intersection-02', 'intersection-03', 'intersection-04', 'intersection-05', 'intersection-06'] },
]

const ROAD_TARGETS = ['road-17', 'road-05', 'road-06', 'road-07', 'road-03']

const SEVERITY_TONE: Record<string, string> = {
  CATASTROPHIC: 'border-red-500/50 bg-red-500/10 text-red-300 hover:bg-red-500/20',
  CRITICAL: 'border-orange-500/50 bg-orange-500/10 text-orange-300 hover:bg-orange-500/20',
  HIGH: 'border-amber-500/50 bg-amber-500/10 text-amber-300 hover:bg-amber-500/20',
  MEDIUM: 'border-slate-500/50 bg-slate-700/40 text-slate-200 hover:bg-slate-700/70',
}

export default function ControlPanel() {
  const state = useSimStore((s) => s.state)
  const runState = useSimStore((s) => s.runState)
  const presets = useSimStore((s) => s.presets)
  const setError = useSimStore((s) => s.setError)
  const selectNode = useSimStore((s) => s.selectNode)
  const [busy, setBusy] = useState<string | null>(null)
  const [openGroup, setOpenGroup] = useState<string | null>('Substations')

  const offline = useMemo(() => {
    if (!state) return []
    return Object.values(state.nodes).filter((n) => !n.operational)
  }, [state])

  function applyPreset(p: Preset) {
    setBusy(p.id)
    setError(null)
    send('APPLY_PRESET', { presetId: p.id })
    window.setTimeout(() => setBusy((cur) => (cur === p.id ? null : cur)), 350)
  }

  function inject(targetId: string) {
    setError(null)
    send('INJECT_FAILURE', { targetId, targetType: 'NODE' })
    selectNode(targetId)
  }

  function block(connectionId: string) {
    setError(null)
    send('INJECT_ROAD_BLOCK', { connectionId })
  }

  function restore(targetId: string) {
    setError(null)
    send('RESTORE_NODE', { targetId })
  }

  const running = runState === 'RUNNING'

  return (
    <section className="flex flex-col gap-3 border-b border-ares-border bg-ares-panel/60 p-3">
      <header className="flex items-center justify-between">
        <h2 className="font-mono text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">
          Command Console
        </h2>
        <span className="font-mono text-[10px] text-slate-600">
          {presets.length} presets loaded
        </span>
      </header>

      <div className="flex items-center gap-1.5">
        <button
          onClick={() => send(running ? 'PAUSE' : 'START')}
          className={`flex flex-1 items-center justify-center gap-2 rounded-md border px-3 py-2 font-mono text-xs font-semibold uppercase tracking-wider transition ${
            running
              ? 'border-amber-500/50 bg-amber-500/10 text-amber-300 hover:bg-amber-500/20'
              : 'border-emerald-500/50 bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20'
          }`}
        >
          {running ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
          {running ? 'Pause' : 'Start'}
        </button>
        <button
          onClick={() => send('STEP', { steps: 1 })}
          className="flex items-center justify-center gap-2 rounded-md border border-cyan-500/40 bg-cyan-500/10 px-3 py-2 font-mono text-xs uppercase tracking-wider text-cyan-300 transition hover:bg-cyan-500/20"
        >
          <SkipForward className="h-3.5 w-3.5" />1
        </button>
        <button
          onClick={() => send('STEP', { steps: 5 })}
          className="flex items-center justify-center gap-2 rounded-md border border-cyan-500/40 bg-cyan-500/10 px-3 py-2 font-mono text-xs uppercase tracking-wider text-cyan-300 transition hover:bg-cyan-500/20"
        >
          +5
        </button>
      </div>

      <div>
        <h3 className="mb-1.5 flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.15em] text-slate-500">
          <Zap className="h-3 w-3 text-orange-400" />
          Hazard Presets
        </h3>
        <div className="grid grid-cols-1 gap-1.5">
          {presets.map((p) => (
            <button
              key={p.id}
              onClick={() => applyPreset(p)}
              disabled={busy === p.id}
              title={p.description}
              className={`rounded-md border px-2.5 py-2 text-left font-mono text-[11px] uppercase tracking-wider transition disabled:opacity-50 ${
                SEVERITY_TONE[p.severity] ?? SEVERITY_TONE.MEDIUM
              }`}
            >
              <span className="flex items-center gap-1.5 font-semibold">
                <AlertOctagon className="h-3 w-3 shrink-0" />
                {busy === p.id ? 'Applying…' : p.label}
              </span>
            </button>
          ))}
        </div>
      </div>

      <div>
        <h3 className="mb-1.5 flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.15em] text-slate-500">
          <Skull className="h-3 w-3 text-red-400" />
          Manual Failure Injection
        </h3>
        <div className="flex flex-col gap-1">
          {TARGETS.map((g) => (
            <div key={g.type} className="overflow-hidden rounded-md border border-slate-700/60">
              <button
                onClick={() => setOpenGroup(openGroup === g.label ? null : g.label)}
                className="flex w-full items-center justify-between bg-slate-800/40 px-2.5 py-1.5 font-mono text-[10px] uppercase tracking-wider text-slate-300 hover:bg-slate-800/70"
              >
                {g.label}
                <ChevronDown
                  className={`h-3 w-3 transition-transform ${openGroup === g.label ? 'rotate-180' : ''}`}
                />
              </button>
              {openGroup === g.label && (
                <div className="grid grid-cols-2 gap-1 p-1.5">
                  {g.ids.map((id) => {
                    const n = state?.nodes[id]
                    const down = n ? !n.operational : false
                    return (
                      <button
                        key={id}
                        onClick={() => (down ? restore(id) : inject(id))}
                        className={`rounded border px-1.5 py-1.5 font-mono text-[10px] transition ${
                          down
                            ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20'
                            : 'border-slate-600/50 bg-slate-800/30 text-slate-400 hover:border-red-500/50 hover:text-red-300'
                        }`}
                      >
                        {down ? <Wrench className="mr-1 inline h-2.5 w-2.5" /> : <Ban className="mr-1 inline h-2.5 w-2.5" />}
                        {id.split('-').slice(-1)[0]}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      <div>
        <h3 className="mb-1.5 flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.15em] text-slate-500">
          <Ban className="h-3 w-3 text-amber-400" />
          Flood Road Segment
        </h3>
        <div className="flex flex-wrap gap-1">
          {ROAD_TARGETS.map((id) => {
            const c = state?.connections[id]
            const blocked = c?.blocked ?? false
            return (
              <button
                key={id}
                onClick={() => block(id)}
                disabled={blocked}
                className={`rounded border px-2 py-1 font-mono text-[10px] transition ${
                  blocked
                    ? 'border-red-500/40 bg-red-500/10 text-red-300'
                    : 'border-slate-600/50 bg-slate-800/30 text-slate-400 hover:border-amber-500/50 hover:text-amber-300'
                }`}
              >
                {id}
              </button>
            )
          })}
        </div>
      </div>

      {offline.length > 0 && (
        <div className="rounded-md border border-red-500/30 bg-red-500/5 p-2">
          <h3 className="mb-1 flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.15em] text-red-300">
            <RotateCcw className="h-3 w-3" />
            Offline Assets ({offline.length})
          </h3>
          <div className="flex max-h-24 flex-col gap-0.5 overflow-y-auto">
            {offline.slice(0, 12).map((n) => (
              <button
                key={n.id}
                onClick={() => restore(n.id)}
                className="flex items-center justify-between gap-2 rounded px-1.5 py-1 text-left font-mono text-[10px] text-slate-400 transition hover:bg-emerald-500/10 hover:text-emerald-300"
              >
                <span className="truncate">{n.name}</span>
                <span className="shrink-0 text-slate-600">{n.reason || n.status}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </section>
  )
}
