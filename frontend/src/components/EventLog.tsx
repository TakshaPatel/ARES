import { useMemo, useState } from 'react'
import { Eraser, ListFilter, ScrollText } from 'lucide-react'
import { useSimStore } from '../store/useSimStore'
import { SEVERITY_TONE } from '../lib/theme'
import type { EventSeverity } from '../types/simulation'
import { Chip, SectionTitle } from './ui'

type Filter = 'ALL' | EventSeverity

const FILTERS: Filter[] = ['ALL', 'FAILURE', 'CASCADE', 'REROUTE', 'WARNING', 'SUCCESS', 'INFO']

export default function EventLog() {
  const state = useSimStore((s) => s.state)
  const [filter, setFilter] = useState<Filter>('ALL')
  const [showCategories, setShowCategories] = useState(false)

  const events = useMemo(() => {
    const all = state?.diagnostics?.events ?? []
    return filter === 'ALL' ? all : all.filter((e) => e.severity === filter)
  }, [state, filter])

  const counts = useMemo(() => {
    const all = state?.diagnostics?.events ?? []
    const acc: Record<string, number> = {}
    for (const e of all) acc[e.severity] = (acc[e.severity] ?? 0) + 1
    return acc
  }, [state])

  return (
    <section className="flex min-h-0 flex-[1.1] flex-col overflow-hidden border-t border-neutral-800">
      <header className="flex shrink-0 items-center justify-between gap-2 border-b border-neutral-800 px-3 py-2">
        <SectionTitle icon={<ScrollText className="h-3.5 w-3.5" />} count={events.length}>
          Cascade log
        </SectionTitle>
        <ButtonToggle active={showCategories} onClick={() => setShowCategories((v) => !v)}>
          <ListFilter className="h-3 w-3" />
          {showCategories ? 'Hide categories' : 'Categories'}
        </ButtonToggle>
      </header>

      <div className="flex shrink-0 flex-wrap gap-1 border-b border-neutral-800 px-3 py-1.5">
        {FILTERS.map((f) => {
          const n = f === 'ALL' ? Object.values(counts).reduce((a, b) => a + b, 0) : counts[f] ?? 0
          return (
            <Chip key={f} active={filter === f} onClick={() => setFilter(f)}>
              {f === 'ALL' ? 'All' : f.toLowerCase()}{' '}
              <span className="ml-0.5 font-mono tabular-nums text-neutral-500">{n}</span>
            </Chip>
          )
        })}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {events.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
            <Eraser className="h-6 w-6 text-neutral-700" />
            <p className="text-sm text-neutral-500">No events match the current filter</p>
          </div>
        ) : (
          events.map((e) => {
            const tone = SEVERITY_TONE[e.severity] ?? SEVERITY_TONE.INFO
            return (
              <article
                key={e.id}
                className={`border-b border-l-2 border-b-neutral-800/60 ${tone.border} px-3 py-2 transition hover:bg-neutral-800/30`}
              >
                <div className="flex items-baseline gap-2">
                  <span className="shrink-0 font-mono text-xs tabular-nums text-neutral-500">
                    {e.clock}
                  </span>
                  <span
                    className={`shrink-0 text-xs font-semibold uppercase tracking-wide ${tone.text}`}
                  >
                    {tone.label}
                  </span>
                  {showCategories && (
                    <span className="shrink-0 rounded bg-neutral-800 px-1.5 py-0.5 text-xs text-neutral-400">
                      {e.category}
                    </span>
                  )}
                  {e.depth > 0 && (
                    <span className="shrink-0 rounded bg-orange-500/15 px-1.5 py-0.5 font-mono text-xs text-orange-200">
                      depth {e.depth}
                    </span>
                  )}
                  <span className="ml-auto shrink-0 text-xs text-neutral-600">{e.source}</span>
                </div>
                <p className="mt-1 text-sm leading-snug text-neutral-100">{e.message}</p>
              </article>
            )
          })
        )}
      </div>
    </section>
  )
}

function ButtonToggle({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1 rounded border px-2 py-1 text-xs transition ${
        active
          ? 'border-neutral-600 bg-neutral-800 text-neutral-200'
          : 'border-neutral-800 text-neutral-500 hover:text-neutral-200'
      }`}
    >
      {children}
    </button>
  )
}