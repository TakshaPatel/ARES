import { useEffect, useState } from 'react'
import { Activity, AlertTriangle, Cpu, Link2, RotateCcw, Radio, ShieldAlert } from 'lucide-react'
import { useSimStore, post } from '../store/useSimStore'

function Badge({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone: 'ok' | 'warn' | 'danger' | 'muted' | 'accent'
}) {
  const tones: Record<string, string> = {
    ok: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300',
    warn: 'border-amber-500/40 bg-amber-500/10 text-amber-300',
    danger: 'border-red-500/40 bg-red-500/10 text-red-300',
    muted: 'border-slate-600/40 bg-slate-800/50 text-slate-300',
    accent: 'border-cyan-500/40 bg-cyan-500/10 text-cyan-300',
  }
  return (
    <div
      className={`flex items-center gap-2 rounded-md border px-3 py-1.5 font-mono text-xs uppercase tracking-wider ${tones[tone]}`}
    >
      <span className="text-slate-500">{label}</span>
      <span className="font-semibold">{value}</span>
    </div>
  )
}

export default function SystemHeader() {
  const state = useSimStore((s) => s.state)
  const runState = useSimStore((s) => s.runState)
  const connected = useSimStore((s) => s.connected)
  const activeScenario = useSimStore((s) => s.activeScenario)
  const resetHistory = useSimStore((s) => s.resetHistory)
  const [resetting, setResetting] = useState(false)

  const elapsed = state?.diagnostics?.elapsedMinutes ?? 0
  const clock = state?.diagnostics?.clock ?? '00:00'
  const hh = String(Math.floor(elapsed / 60)).padStart(2, '0')
  const mm = String(elapsed % 60).padStart(2, '0')

  useEffect(() => {
    if (runState === 'RUNNING') document.title = 'ARES :: RUNNING'
    else document.title = 'ARES :: PAUSED'
  }, [runState])

  const runTone =
    runState === 'RUNNING' ? 'ok' : runState === 'PAUSED' ? 'warn' : connected ? 'accent' : 'danger'

  async function handleReset() {
    setResetting(true)
    resetHistory()
    await post('/api/simulation/reset', {})
    window.setTimeout(() => setResetting(false), 400)
  }

  const alertCount = state?.activeAlerts.length ?? 0
  const isolated = state?.isolatedFacilities.length ?? 0

  return (
    <header className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-ares-border bg-ares-panel/80 px-4 py-3 backdrop-blur">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-md border border-cyan-500/40 bg-cyan-500/10">
          <ShieldAlert className="h-5 w-5 text-cyan-400" />
        </div>
        <div>
          <h1 className="text-lg font-bold leading-none tracking-[0.2em] text-slate-100">ARES</h1>
          <p className="mt-0.5 font-mono text-[10px] uppercase tracking-[0.18em] text-slate-500">
            Adaptive Resilience &amp; Emergency Simulation
          </p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Badge label="Tick" value={`T+${hh}:${mm} / ${clock}`} tone="muted" />
        <Badge
          label="State"
          value={runState}
          tone={runTone}
        />
        <Badge
          label="Link"
          value={connected ? 'ONLINE' : 'DOWN'}
          tone={connected ? 'ok' : 'danger'}
        />
        <Badge
          label="Alerts"
          value={String(alertCount)}
          tone={alertCount === 0 ? 'ok' : alertCount > 4 ? 'danger' : 'warn'}
        />
        <Badge label="Isolated" value={String(isolated)} tone={isolated === 0 ? 'ok' : 'danger'} />
      </div>

      <div className="flex items-center gap-3">
        <div className="hidden items-center gap-3 font-mono text-[10px] uppercase tracking-widest text-slate-500 lg:flex">
          <span className="flex items-center gap-1">
            <Cpu className="h-3 w-3" />
            {Object.values(state?.nodes ?? {}).filter((n) => n.operational).length ?? 0} online
          </span>
          <span className="flex items-center gap-1">
            <Link2 className="h-3 w-3" />
            {Object.values(state?.connections ?? {}).filter((c) => !c.blocked).length ?? 0} links
          </span>
          {activeScenario && (
            <span className="flex items-center gap-1 text-slate-400">
              <Radio className="h-3 w-3" />
              {activeScenario}
            </span>
          )}
        </div>

        <button
          onClick={handleReset}
          disabled={resetting}
          className="flex items-center gap-2 rounded-md border border-slate-600/50 bg-slate-800/60 px-3 py-2 font-mono text-xs uppercase tracking-wider text-slate-200 transition hover:border-cyan-500/50 hover:text-cyan-300 disabled:opacity-50"
        >
          <RotateCcw className={`h-3.5 w-3.5 ${resetting ? 'animate-spin' : ''}`} />
          Reset T+0
        </button>

        {alertCount > 0 && (
          <div className="flex items-center gap-1.5 rounded-md border border-amber-500/40 bg-amber-500/10 px-2.5 py-2 font-mono text-xs text-amber-300">
            <AlertTriangle className="h-3.5 w-3.5" />
            {alertCount}
          </div>
        )}
        <Activity className="hidden h-4 w-4 text-slate-600 sm:block" />
      </div>
    </header>
  )
}
