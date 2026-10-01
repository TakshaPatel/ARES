import { useMemo, useState } from 'react'
import {
  Ban,
  ChevronDown,
  Layers,
  Pause,
  Play,
  RotateCcw,
  SkipForward,
  Wrench,
  Zap,
} from 'lucide-react'
import { useSimStore, send } from '../store/useSimStore'
import { PRESET_TONE } from '../lib/theme'
import type { NodeType } from '../types/simulation'
import { Button, SectionTitle } from './ui'

const TARGETS: { label: string; type: NodeType; ids: string[] }[] = [
  {
    label: 'Substations',
    type: 'POWER_SUBSTATION',
    ids: ['substation-01', 'substation-02', 'substation-03', 'substation-04'],
  },
  {
    label: 'Comms sites',
    type: 'CELL_TOWER',
    ids: ['tower-01', 'tower-02', 'tower-03', 'tower-04', 'tower-05', 'tower-06'],
  },
  {
    label: 'Intersections',
    type: 'ROAD_INTERSECTION',
    ids: [
      'intersection-01',
      'intersection-02',
      'intersection-03',
      'intersection-04',
      'intersection-05',
      'intersection-06',
    ],
  },
]

const ROAD_TARGETS = ['road-17', 'road-05', 'road-06', 'road-07', 'road-03']

export default function ControlPanel() {
  const state = useSimStore((s) => s.state)
  const runState = useSimStore((s) => s.runState)
  const presets = useSimStore((s) => s.presets)
  const setError = useSimStore((s) => s.setError)
  const selectNode = useSimStore((s) => s.selectNode)
  const [openGroup, setOpenGroup] = useState<string | null>('Substations')
  const [selected, setSelected] = useState<string[]>([])
  const [applying, setApplying] = useState(false)

  const offline = useMemo(
    () => (state ? Object.values(state.nodes).filter((n) => !n.operational) : []),
    [state],
  )

  function togglePreset(id: string) {
    setSelected((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]))
  }

  function applyStacked() {
    if (selected.length === 0) return
    setApplying(true)
    setError(null)
    send('APPLY_PRESETS', { presetIds: selected })
    window.setTimeout(() => {
      setApplying(false)
      setSelected([])
    }, 500)
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
    <section className="flex flex-col gap-4 bg-transparent p-3">
      <SectionTitle
        action={
          <span className="text-xs text-neutral-500">
            {presets.length} preset{presets.length === 1 ? '' : 's'}
          </span>
        }
      >
        Command console
      </SectionTitle>

      <div className="flex items-center gap-1.5">
        <Button
          variant={running ? 'warn' : 'primary'}
          className="flex-1"
          onClick={() => send(running ? 'PAUSE' : 'START')}
          icon={
            running ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />
          }
        >
          {running ? 'Pause' : 'Start'}
        </Button>
        <Button
          size="sm"
          className="px-2.5 py-2"
          title="Advance one tick"
          onClick={() => send('STEP', { steps: 1 })}
          icon={<SkipForward className="h-3.5 w-3.5" />}
        >
          1
        </Button>
        <Button
          size="sm"
          className="px-2.5 py-2"
          title="Advance five ticks"
          onClick={() => send('STEP', { steps: 5 })}
        >
          +5
        </Button>
      </div>

      <div>
        <div className="mb-2 flex items-center justify-between gap-2">
          <SectionTitle icon={<Zap className="h-3.5 w-3.5 text-neutral-500" />}>
            Stacked events
          </SectionTitle>
          <div className="flex gap-1">
            <Button
              size="sm"
              variant="subtle"
              onClick={() =>
                setSelected(selected.length === presets.length ? [] : presets.map((p) => p.id))
              }
            >
              {selected.length === presets.length && presets.length > 0 ? 'None' : 'All'}
            </Button>
            <Button
              size="sm"
              variant="subtle"
              onClick={() => setSelected([])}
              disabled={selected.length === 0}
            >
              Clear
            </Button>
          </div>
        </div>

        <div className="flex flex-col gap-1">
          {presets.map((p) => {
            const on = selected.includes(p.id)
            return (
              <button
                key={p.id}
                onClick={() => togglePreset(p.id)}
                title={p.description}
                aria-pressed={on}
                className={`flex items-start gap-2 rounded-md border px-2.5 py-2 text-left transition ${
                  on
                    ? 'border-neutral-500 bg-neutral-800 text-neutral-50'
                    : 'border-neutral-800 bg-neutral-900/40 text-neutral-400 hover:border-neutral-600 hover:text-neutral-100'
                }`}
              >
                <span
                  aria-hidden
                  className={`mt-0.5 flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-[3px] border ${
                    on ? 'border-neutral-100 bg-neutral-100' : 'border-neutral-700 bg-neutral-950'
                  }`}
                >
                  {on && <span className="h-1.5 w-1.5 rounded-[1px] bg-neutral-900" />}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-medium leading-tight">{p.label}</span>
                  <span className="mt-0.5 block text-xs leading-snug text-neutral-500">
                    {p.description}
                  </span>
                </span>
                <span
                  className={`shrink-0 rounded border px-1.5 py-0.5 font-mono text-xs ${
                    PRESET_TONE[p.severity] ?? PRESET_TONE.MEDIUM
                  }`}
                >
                  {p.severity.slice(0, 4)}
                </span>
              </button>
            )
          })}
        </div>

        <Button
          variant="warn"
          className="mt-2 w-full"
          onClick={applyStacked}
          disabled={selected.length === 0 || applying}
          icon={<Layers className="h-3.5 w-3.5" />}
        >
          {applying
            ? 'Stacking…'
            : selected.length === 0
              ? 'Select events to stack'
              : `Stack ${selected.length} event${selected.length === 1 ? '' : 's'}`}
        </Button>
        <p className="mt-1.5 text-xs leading-snug text-neutral-500">
          Selected events apply in order and compound: each one re-evaluates the cascade on top of
          the last.
        </p>
      </div>

      <div>
        <SectionTitle className="mb-2" icon={<Ban className="h-3.5 w-3.5 text-red-400" />}>
          Manual failure injection
        </SectionTitle>
        <div className="flex flex-col gap-1">
          {TARGETS.map((g) => (
            <div key={g.type} className="overflow-hidden rounded-md border border-neutral-800">
              <button
                onClick={() => setOpenGroup(openGroup === g.label ? null : g.label)}
                className="flex w-full items-center justify-between bg-neutral-900/60 px-2.5 py-1.5 text-sm text-neutral-200 transition hover:bg-neutral-800"
              >
                {g.label}
                <ChevronDown
                  className={`h-3.5 w-3.5 text-neutral-500 transition-transform ${
                    openGroup === g.label ? 'rotate-180' : ''
                  }`}
                />
              </button>
              {openGroup === g.label && (
                <div className="grid grid-cols-2 gap-1 p-1.5">
                  {g.ids.map((id) => {
                    const n = state?.nodes[id]
                    const down = n ? !n.operational : false
                    return (
                      <Button
                        key={id}
                        size="sm"
                        variant={down ? 'primary' : 'ghost'}
                        onClick={() => (down ? restore(id) : inject(id))}
                        icon={
                          down ? (
                            <Wrench className="h-3 w-3" />
                          ) : (
                            <Ban className="h-3 w-3" />
                          )
                        }
                      >
                        {id.split('-').slice(-1)[0]}
                      </Button>
                    )
                  })}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      <div>
        <SectionTitle className="mb-2" icon={<Ban className="h-3.5 w-3.5 text-neutral-500" />}>
          Flood road segment
        </SectionTitle>
        <div className="flex flex-wrap gap-1">
          {ROAD_TARGETS.map((id) => {
            const c = state?.connections[id]
            const blocked = c?.blocked ?? false
            return (
              <Button
                key={id}
                size="sm"
                variant={blocked ? 'danger' : 'ghost'}
                onClick={() => block(id)}
                disabled={blocked}
                className="font-mono"
              >
                {id}
              </Button>
            )
          })}
        </div>
      </div>

      {offline.length > 0 && (
        <div className="rounded-md border border-red-500/30 bg-red-500/5 p-2">
          <SectionTitle className="mb-1.5" icon={<RotateCcw className="h-3.5 w-3.5" />}>
            <span className="text-red-200">Offline assets ({offline.length})</span>
          </SectionTitle>
          <div className="flex max-h-28 flex-col gap-0.5 overflow-y-auto">
            {offline.slice(0, 12).map((n) => (
              <button
                key={n.id}
                onClick={() => restore(n.id)}
                className="flex items-center justify-between gap-2 rounded px-1.5 py-1 text-left text-xs text-neutral-400 transition hover:bg-emerald-500/10 hover:text-emerald-200"
              >
                <span className="truncate">{n.name}</span>
                <span className="shrink-0 font-mono text-neutral-600">{n.reason || n.status}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </section>
  )
}