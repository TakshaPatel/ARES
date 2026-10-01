import { useEffect, useState } from 'react'
import { Cpu, HelpCircle, Link2, RotateCcw, ShieldAlert, Unlink, WifiOff } from 'lucide-react'
import { useSimStore, post } from '../store/useSimStore'
import { Badge } from './ui'

export default function SystemHeader() {
  const state = useSimStore((s) => s.state)
  const runState = useSimStore((s) => s.runState)
  const connected = useSimStore((s) => s.connected)
  const activeScenario = useSimStore((s) => s.activeScenario)
  const resetHistory = useSimStore((s) => s.resetHistory)
  const setTourOpen = useSimStore((s) => s.setTourOpen)
  const [resetting, setResetting] = useState(false)

  const elapsed = state?.diagnostics?.elapsedMinutes ?? 0
  const clock = state?.diagnostics?.clock ?? '00:00'
  const hh = String(Math.floor(elapsed / 60)).padStart(2, '0')
  const mm = String(elapsed % 60).padStart(2, '0')

  useEffect(() => {
    document.title = runState === 'RUNNING' ? 'ARES :: RUNNING' : 'ARES :: PAUSED'
  }, [runState])

  const runTone =
    runState === 'RUNNING'
      ? 'ok'
      : runState === 'PAUSED'
        ? 'warn'
        : connected
          ? 'accent'
          : 'danger'

  async function handleReset() {
    setResetting(true)
    resetHistory()
    await post('/api/simulation/reset', {})
    window.setTimeout(() => setResetting(false), 400)
  }

  const alertCount = state?.activeAlerts.length ?? 0
  const isolated = state?.isolatedFacilities.length ?? 0
  const online = Object.values(state?.nodes ?? {}).filter((n) => n.operational).length
  const links = Object.values(state?.connections ?? {}).filter((c) => !c.blocked).length

  return (
    <header className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-ares-border bg-ares-panel px-4 py-2.5">
      <div className="flex items-center gap-2.5">
        <div className="flex h-8 w-8 items-center justify-center rounded-md border border-neutral-700 bg-neutral-800">
          <ShieldAlert className="h-4 w-4 text-neutral-100" />
        </div>
        <div>
          <h1 className="text-base font-semibold leading-none tracking-tight text-neutral-50">ARES</h1>
          <p className="mt-0.5 text-xs leading-none text-neutral-500">
            Adaptive Resilience &amp; Emergency Simulation
          </p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-1.5">
        <Badge label="Tick" value={`T+${hh}:${mm} / ${clock}`} tone="muted" />
        <Badge label="State" value={runState} tone={runTone} />
        <Badge
          label="Alerts"
          value={String(alertCount)}
          tone={alertCount === 0 ? 'ok' : alertCount > 4 ? 'danger' : 'warn'}
        />
      </div>

      <div className="flex items-center gap-3">
        <div className="hidden items-center gap-3 text-xs text-neutral-500 lg:flex">
          <span className="flex items-center gap-1">
            <Cpu className="h-3 w-3" />
            <span className="font-mono tabular-nums">{online}</span> online
          </span>
          <span className="flex items-center gap-1">
            <Link2 className="h-3 w-3" />
            <span className="font-mono tabular-nums">{links}</span> links
          </span>
          <span
            className={`flex items-center gap-1 ${isolated > 0 ? 'text-amber-300' : ''}`}
            title="Facilities with no comms or road path to command"
          >
            {isolated > 0 ? <WifiOff className="h-3 w-3" /> : <Unlink className="h-3 w-3" />}
            <span className="font-mono tabular-nums">{isolated}</span> isolated
          </span>
          {activeScenario && <span className="max-w-[16rem] truncate text-neutral-400">{activeScenario}</span>}
        </div>

        <button
          onClick={() => setTourOpen(true)}
          title="Replay the guided tour"
          className="flex items-center gap-1.5 rounded-md border border-neutral-700 bg-neutral-900 px-2.5 py-1.5 text-xs font-medium text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <HelpCircle className="h-3.5 w-3.5" />
          Tour
        </button>

        <button
          onClick={handleReset}
          disabled={resetting}
          className="flex items-center gap-1.5 rounded-md border border-neutral-700 bg-neutral-900 px-2.5 py-1.5 text-xs font-medium text-neutral-300 transition hover:border-neutral-500 hover:text-white disabled:opacity-40"
        >
          <RotateCcw className={`h-3.5 w-3.5 ${resetting ? 'animate-spin' : ''}`} />
          Reset T+0
        </button>
      </div>
    </header>
  )
}