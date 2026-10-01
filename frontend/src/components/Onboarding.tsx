import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowRight, Check, ChevronDown, ChevronUp, Play, SkipForward, X } from 'lucide-react'
import { send, useSimStore } from '../store/useSimStore'
import type { Preset, TabKey } from '../types/simulation'

const STORAGE_KEY = 'ares.onboarded.v1'

interface Step {
  title: string
  body: string
  tab: TabKey | null
  hint?: string
  /** Resolved to human-readable labels from the active scenario, so the tour can
   *  never point at an internal preset ID the user cannot see in the console. */
  tryPresetIds?: string[]
}

const STEPS: Step[] = [
  {
    title: 'ARES Emergency Operations Console',
    body: 'You are looking at a live cascading-failure simulator for a municipal utility, comms and road network. Everything you see is driven by a real dependency graph ticking the clock forward one step at a time.',
    tab: 'map',
  },
  {
    title: 'Run controls (top right)',
    body: 'At the top of the Command Console on the right: Start and Pause the run, or step the clock forward 1 or 5 ticks. Nothing degrades until you inject something, so you stay in control of the timeline.',
    tab: 'map',
    hint: 'Press Start and watch the clock, power grid and coverage counters move.',
  },
  {
    title: 'Map View',
    body: 'Satellite base with the network drawn on top. Power substation, cell tower, mesh node, road intersection, hospital, fire station and the EOC each use a distinct marker. Click any node to open its detail card.',
    tab: 'map',
    hint: 'Red pulses = nodes going offline. A red dashed line = a blocked link.',
  },
  {
    title: 'Stack events in one go',
    body: 'Scroll down the Command Console to Stacked events. Tick as many events as you like, then press Stack once. They apply in order and each one re-runs the cascade on top of the last, so two moderate events can produce a worse outcome than either alone.',
    tab: 'map',
    tryPresetIds: ['PRESET-SUB01', 'PRESET-FLOOD-ZONE'],
  },
  {
    title: 'Dependency DAG',
    body: 'The same network as a directed graph, laid out in dependency layers. It answers the question the map cannot: why is this node down? Follow an edge backwards to the root cause and forward to the facilities that lost service.',
    tab: 'graph',
    hint: 'Cascade depth shows how far the failure propagated.',
  },
  {
    title: 'Reports',
    body: 'A running after-action report. The left pane is the chronological event log with severity filters; the right pane is the generated report with key indicators, offline assets, blocked links, EMS routes and alerts.',
    tab: 'reports',
    hint: 'Export the report as .txt for the write-up, or .json for the raw snapshot.',
  },
  {
    title: 'Upload City Data',
    body: 'Load a different city as JSON. A file containing both nodes and connections becomes the active scenario; a lighter file with a name and a center simply sets the starting location and zoom for the map.',
    tab: 'upload',
    hint: 'Use the built-in sample to see the expected shape.',
  },
  {
    title: 'Run the guided demo',
    body: 'Let ARES drive itself: it starts the clock, then stacks a substation failure with a flood zone so you can watch the cascade propagate to the hospitals and fire stations it serves. You can pause at any point.',
    tab: 'map',
  },
]

const buildScript = (presets: Preset[]) => {
  const label = (id: string) => presets.find((p) => p.id === id)?.label ?? id
  const sub01 = label('PRESET-SUB01')
  const flood = label('PRESET-FLOOD-ZONE')
  return [
    { at: 0, run: () => send('RESET'), note: 'Resetting to a clean baseline…' },
    { at: 700, run: () => send('START'), note: 'Clock running at 5-minute ticks.' },
    {
      at: 2600,
      run: () => send('APPLY_PRESETS', { presetIds: ['PRESET-SUB01'] }),
      note: `Stacking event 1 of 2: ${sub01}.`,
    },
    {
      at: 4600,
      run: () => send('APPLY_PRESETS', { presetIds: ['PRESET-FLOOD-ZONE'] }),
      note: `Stacking event 2 of 2: ${flood}, on top of the first.`,
    },
    { at: 6400, run: () => send('PAUSE'), note: 'Paused so you can inspect the result.' },
    { at: 6400, run: () => useSimStore.getState().setTab('graph'), note: '' },
  ] satisfies { at: number; run: () => void; note: string }[]
}

export default function Onboarding() {
  const tourOpen = useSimStore((s) => s.tourOpen)
  const setTourOpen = useSimStore((s) => s.setTourOpen)
  const selectedNodeId = useSimStore((s) => s.selectedNodeId)
  const presets = useSimStore((s) => s.presets)
  const [step, setStep] = useState(0)
  const [demoRunning, setDemoRunning] = useState(false)
  const [peek, setPeek] = useState(false)
  const [demoNote, setDemoNote] = useState<string | null>(null)
  const timers = useRef<number[]>([])
  const autoOpened = useRef(false)

  useEffect(() => {
    if (autoOpened.current) return
    autoOpened.current = true
    if (window.localStorage.getItem(STORAGE_KEY) !== '1') setTourOpen(true)
  }, [setTourOpen])

  useEffect(() => {
    if (selectedNodeId) setPeek(true)
  }, [selectedNodeId])

  const clearTimers = useCallback(() => {
    for (const t of timers.current) window.clearTimeout(t)
    timers.current = []
  }, [])

  useEffect(() => clearTimers, [clearTimers])

  const runDemo = useCallback(() => {
    clearTimers()
    setDemoRunning(true)
    setDemoNote(null)
    for (const s of buildScript(presets)) {
      timers.current.push(
        window.setTimeout(() => {
          s.run()
          if (s.note) setDemoNote(s.note)
        }, s.at),
      )
    }
    timers.current.push(window.setTimeout(() => setDemoRunning(false), 7200))
  }, [clearTimers, presets])

  const close = useCallback(() => {
    clearTimers()
    setDemoRunning(false)
    window.localStorage.setItem(STORAGE_KEY, '1')
    setTourOpen(false)
  }, [clearTimers, setTourOpen])

  const isLast = step === STEPS.length - 1
  const current = STEPS[step]
  const goTab = useCallback((t: TabKey | null) => {
    if (t) useSimStore.getState().setTab(t)
  }, [])

  const progress = useMemo(() => `${step + 1} / ${STEPS.length}`, [step])

  const hint = useMemo(() => {
    if (current.tryPresetIds) {
      const labels = current.tryPresetIds
        .map((id) => presets.find((p) => p.id === id)?.label)
        .filter((l): l is string => Boolean(l))
      if (labels.length < 2) {
        return 'The active scenario does not define the presets this step uses. Load the built-in North Brunswick scenario to try stacking.'
      }
      return `Try stacking "${labels[0]}" and "${labels[1]}" together.`
    }
    return current.hint
  }, [current, presets])

  if (!tourOpen) return null

  if (peek) {
    return (
      <button
        onClick={() => setPeek(false)}
        className="pointer-events-auto fixed bottom-3 left-3 z-50 flex items-center gap-2 rounded-full border border-neutral-500 bg-ares-panel/95 px-3 py-1.5 text-sm font-medium text-neutral-200 shadow-xl backdrop-blur transition hover:border-neutral-300"
      >
        <ChevronUp className="h-3 w-3" />
        Tour {progress}
      </button>
    )
  }

  return (
    <div className="pointer-events-none fixed inset-0 z-50 flex items-end justify-start p-3">
      <div className="pointer-events-auto flex max-h-[70vh] w-full max-w-sm flex-col rounded-lg border border-neutral-500 bg-ares-panel/95 shadow-2xl backdrop-blur-sm">
        <div className="flex items-center justify-between border-b border-ares-border px-4 py-2.5">
          <div className="flex items-center gap-2">
            <span className="text-xs uppercase tracking-wide text-neutral-500">
              Onboarding
            </span>
            <span className="rounded bg-neutral-800 px-1.5 py-0.5 font-mono text-xs tabular-nums text-neutral-400">
              {progress}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={() => {
                clearTimers()
                setDemoRunning(false)
                setTourOpen(false)
                window.localStorage.setItem(STORAGE_KEY, '1')
              }}
              className="rounded px-2 py-1 text-xs font-medium text-neutral-500 transition hover:text-neutral-200"
            >
              Skip
            </button>
            <button
              onClick={() => setPeek(true)}
              title="Shrink to follow along"
              className="rounded p-1 text-neutral-500 transition hover:text-neutral-200"
            >
              <ChevronDown className="h-4 w-4" />
            </button>
            <button onClick={close} className="rounded p-1 text-neutral-500 transition hover:text-neutral-200">
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-3">
          <h3 className="text-base font-medium text-neutral-50">
            {current.title}
          </h3>
          <p className="mt-2 text-[12px] leading-relaxed text-neutral-300">{current.body}</p>
          {hint && (
            <p className="mt-3 flex items-start gap-1.5 rounded border border-neutral-700 bg-neutral-900/60 px-2.5 py-1.5 text-sm leading-relaxed text-neutral-400">
              <span className="mt-px text-amber-400">▸</span>
              {hint}
            </p>
          )}

          {isLast && (
            <div className="mt-4 rounded border border-neutral-700 bg-neutral-900/60 p-3">
              <button
                onClick={runDemo}
                disabled={demoRunning}
                className="flex w-full items-center justify-center gap-2 rounded border border-emerald-600/60 bg-emerald-700/25 px-3 py-2 text-sm font-medium text-emerald-100 transition hover:bg-emerald-700/40 disabled:opacity-50"
              >
                {demoRunning ? (
                  <>
                    <SkipForward className="h-3.5 w-3.5 animate-pulse" />
                    Demo running…
                  </>
                ) : (
                  <>
                    <Play className="h-3.5 w-3.5" />
                    Run guided demo
                  </>
                )}
              </button>
              {demoNote && (
                <p className="mt-2 text-sm leading-relaxed text-emerald-200/90">{demoNote}</p>
              )}
              {demoRunning && (
                <p className="mt-2 text-xs text-neutral-500">
                  Watch the map first, then the Dependency DAG.
                </p>
              )}
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2 border-t border-ares-border px-3 py-2">
          <div className="flex flex-1 gap-1">
            {STEPS.map((s, i) => (
              <button
                key={s.title}
                onClick={() => {
                  setStep(i)
                  goTab(s.tab)
                }}
                title={s.title}
                className={`h-1 flex-1 rounded-full transition ${
                  i === step ? 'bg-neutral-200' : i < step ? 'bg-neutral-600' : 'bg-neutral-800'
                }`}
              />
            ))}
          </div>
          <button
            onClick={() => setStep((s) => Math.max(0, s - 1))}
            disabled={step === 0}
            className="rounded border border-neutral-700 px-2.5 py-1 text-sm text-neutral-300 transition hover:bg-neutral-700 disabled:opacity-30"
          >
            Back
          </button>
          <button
            onClick={() => {
              if (isLast) {
                close()
                return
              }
              const n = step + 1
              setStep(n)
              setPeek(false)
              goTab(STEPS[n].tab)
            }}
            className="flex items-center gap-1.5 rounded border border-neutral-400 bg-neutral-700 px-3 py-1 text-sm font-medium text-white transition hover:bg-neutral-600"
          >
            {isLast ? (
              <>
                <Check className="h-3 w-3" />
                Done
              </>
            ) : (
              <>
                Next
                <ArrowRight className="h-3 w-3" />
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  )
}
