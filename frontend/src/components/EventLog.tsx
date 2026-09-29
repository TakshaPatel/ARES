import { useMemo, useState } from 'react'
import { Eraser, ListFilter, ScrollText } from 'lucide-react'
import { useSimStore } from '../store/useSimStore'
import type { EventSeverity } from '../types/simulation'

const TONE: Record<EventSeverity, { border: string; text: string; label: string }> = {
  FAILURE: { border: 'border-l-red-500', text: 'text-red-300', label: 'FAILURE' },
  CASCADE: { border: 'border-l-orange-500', text: 'text-orange-300', label: 'CASCADE' },
  REROUTE: { border: 'border-l-sky-500', text: 'text-sky-300', label: 'REROUTE' },
  WARNING: { border: 'border-l-amber-500', text: 'text-amber-300', label: 'WARNING' },
  SUCCESS: { border: 'border-l-emerald-500', text: 'text-emerald-300', label: 'RESTORE' },
  INFO: { border: 'border-l-slate-600', text: 'text-slate-400', label: 'INFO' },
}

type Filter = 'ALL' | EventSeverity

export default function EventLog() {
  const state = useSimStore((s) => s.state)
  const [filter, setFilter] = useState<Filter>('ALL')
  const [showCategories, setShowCategories] = useState(false)

  const events = useMemo(() => {
    const all = state?.diagnostics?.events ?? []
    if (filter === 'ALL') return all
    return all.filter((e) => e.severity === filter)
  }, [state, filter])

  const counts = useMemo(() => {
    const all = state?.diagnostics?.events ?? []
    const c: Record<string, number> = {}
    for (const e of all) c[e.severity] = (c[e.severity] ?? 0) + 1
    return c
  }, [state])

  const filters: Filter[] = ['ALL', 'FAILURE', 'CASCADE', 'REROUTE', 'WARNING', 'SUCCESS', 'INFO']

  return (
    <section className="flex min-h-0 flex-1 flex-col border-t border-ares-border bg-ares-panel/60">
      <header className="flex shrink-0 items-center justify-between gap-2 border-b border-ares-border px-3 py-2">
        <h2 className="flex items-center gap-1.5 font-mono text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">
          <ScrollText className="h-3.5 w-3.5" />
          Cascade Log
          <span className="ml-1 rounded bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-500">
            {events.length}
          </span>
        </h2>
        <div className="flex items-center gap-1">
          <ListFilter className="h-3 w-3 text-slate-600" />
          <button
            onClick={() => setShowCategories((v) => !v)}
            className="rounded border border-slate-700 px-1.5 py-0.5 font-mono text-[9px] uppercase text-slate-500 hover:text-slate-300"
          >
            {showCategories ? 'Hide' : 'Cats'}
          </button>
        </div>
      </header>

      <div className="flex shrink-0 flex-wrap gap-1 border-b border-ares-border/60 px-3 py-1.5">
        {filters.map((f) => {
          const n = f === 'ALL' ? Object.values(counts).reduce((a, b) => a + b, 0) : counts[f] ?? 0
          return (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`rounded px-1.5 py-0.5 font-mono text-[9px] uppercase tracking-wider transition ${
                filter === f
                  ? 'bg-cyan-500/20 text-cyan-300'
                  : 'bg-slate-800/50 text-slate-500 hover:text-slate-300'
              }`}
            >
              {f} <span className="text-slate-600">{n}</span>
            </button>
          )
        })}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {events.length === 0 && (
          <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
            <Eraser className="h-6 w-6 text-slate-700" />
            <p className="font-mono text-[11px] text-slate-600">
              No events match the current filter
            </p>
          </div>
        )}
        {events.map((e) => {
          const tone = TONE[e.severity] ?? TONE.INFO
          return (
            <article
              key={e.id}
              className={`border-b border-l-2 border-b-slate-800/50 ${tone.border} px-3 py-1.5 transition hover:bg-slate-800/40`}
            >
              <div className="flex items-baseline gap-2">
                <span className="shrink-0 font-mono text-[10px] text-slate-600">
                  {e.clock}
                </span>
                <span className={`shrink-0 font-mono text-[9px] font-bold uppercase tracking-wider ${tone.text}`}>
                  {tone.label}
                </span>
                {showCategories && (
                  <span className="shrink-0 rounded bg-slate-800 px-1 font-mono text-[8px] uppercase text-slate-500">
                    {e.category}
                  </span>
                )}
                {e.depth > 0 && (
                  <span className="shrink-0 rounded bg-orange-500/20 px-1 font-mono text-[8px] text-orange-300">
                    d{e.depth}
                  </span>
                )}
                <span className="ml-auto shrink-0 font-mono text-[9px] text-slate-700">
                  {e.source}
                </span>
              </div>
              <p className="mt-0.5 text-[11px] leading-snug text-slate-300">{e.message}</p>
            </article>
          )
        })}
      </div>
    </section>
  )
}
