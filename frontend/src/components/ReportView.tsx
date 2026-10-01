import { useMemo, useState } from 'react'
import { ClipboardList, Download, FileText } from 'lucide-react'
import { useSimStore } from '../store/useSimStore'
import { SEVERITY_TONE } from '../lib/theme'
import type { SimulationState } from '../types/simulation'
import { Button, Chip, SectionTitle } from './ui'

function pct(n: number) {
  return `${(n * 100).toFixed(1)}%`
}

function buildReport(s: SimulationState, viewLabel: string | null): string {
  const nodes = Object.values(s.nodes)
  const down = nodes.filter((n) => !n.operational)
  const conns = Object.values(s.connections)
  const blocked = conns.filter((c) => c.blocked)
  const events = s.diagnostics?.events ?? []
  const lines: string[] = []

  lines.push('ARES INCIDENT REPORT')
  lines.push('='.repeat(64))
  lines.push(`Scenario      : ${viewLabel ?? 'active scenario'}`)
  lines.push(`Tick          : ${s.tick}   (clock ${s.diagnostics?.clock ?? '--'})`)
  lines.push(`Sim running   : ${s.running === true ? 'yes' : 'no'}`)
  lines.push(`Elapsed       : ${s.diagnostics?.elapsedMinutes ?? 0} min`)
  lines.push(`Generated     : ${new Date().toISOString()}`)
  lines.push('')
  lines.push('KEY INDICATORS')
  lines.push('-'.repeat(64))
  lines.push(`Power grid health      ${pct(s.powerGridHealth)}`)
  lines.push(`Comms coverage         ${pct(s.commsCoverage)}`)
  lines.push(`Road accessibility     ${pct(s.roadAccessibility)}`)
  lines.push(`Message delivery       ${pct(s.messageDeliveryRate)}`)
  lines.push(`Average latency        ${s.averageLatency.toFixed(1)} ms`)
  lines.push(`Facilities connected   ${s.connectedFacilities}/${s.totalFacilities}`)
  lines.push('')
  lines.push(`ASSETS OFFLINE (${down.length}/${nodes.length})`)
  lines.push('-'.repeat(64))
  if (down.length === 0) {
    lines.push('  none')
  } else {
    for (const n of [...down].sort((a, b) => b.cascadeDepth - a.cascadeDepth)) {
      lines.push(
        `  [d${n.cascadeDepth}] ${n.id.padEnd(18)} ${n.type.padEnd(22)} ${n.status.padEnd(11)} ${pct(n.health).padStart(7)}  ${n.reason ?? ''}`,
      )
    }
  }
  lines.push('')
  lines.push(`INFRASTRUCTURE (${nodes.length} nodes, ${conns.length} links)`)
  lines.push('-'.repeat(64))
  const byType = new Map<string, { total: number; down: number }>()
  for (const n of nodes) {
    const e = byType.get(n.type) ?? { total: 0, down: 0 }
    e.total++
    if (!n.operational) e.down++
    byType.set(n.type, e)
  }
  for (const [type, e] of [...byType.entries()].sort((a, b) => b[1].total - a[1].total)) {
    lines.push(`  ${type.padEnd(24)} ${e.total - e.down}/${e.total} online`)
  }
  lines.push('')
  lines.push(`BLOCKED LINKS (${blocked.length})`)
  lines.push('-'.repeat(64))
  if (blocked.length === 0) {
    lines.push('  none')
  } else {
    for (const c of blocked) {
      lines.push(`  ${c.id.padEnd(18)} ${c.type.padEnd(14)} ${c.from} -> ${c.to}`)
    }
  }
  const routes = s.diagnostics?.emsRoutes ?? []
  lines.push('')
  lines.push(`EMS ROUTES (${routes.length})`)
  lines.push('-'.repeat(64))
  if (routes.length === 0) {
    lines.push('  none')
  } else {
    for (const r of routes) {
      lines.push(
        `  ${r.id.padEnd(14)} ${r.origin} -> ${r.destination}  ${r.distanceKm.toFixed(1)} km  ETA ${r.etaMinutes} min${r.changed ? '  [REROUTED]' : ''}${r.degraded ? '  [DEGRADED]' : ''}`,
      )
    }
  }
  const alerts = s.activeAlerts ?? []
  lines.push('')
  lines.push(`ACTIVE ALERTS (${alerts.length})`)
  lines.push('-'.repeat(64))
  if (alerts.length === 0) lines.push('  none')
  else for (const a of alerts) lines.push(`  - ${a}`)
  lines.push('')
  lines.push(`EVENT LOG (${events.length} entries, newest first)`)
  lines.push('-'.repeat(64))
  if (events.length === 0) lines.push('  no events recorded')
  else
    for (const e of events) {
      lines.push(
        `  T+${String(e.tick).padStart(4)} ${e.clock} [${e.severity.padEnd(8)}] [${e.category.padEnd(7)}] ${e.message}`,
      )
    }
  lines.push('')
  lines.push('='.repeat(64))
  lines.push('End of report')
  return lines.join('\n')
}

function download(name: string, text: string, type: string) {
  const blob = new Blob([text], { type })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

export default function ReportView() {
  const state = useSimStore((s) => s.state)
  const viewLabel = useSimStore((s) => s.view.label)
  const [filter, setFilter] = useState<string>('ALL')

  const events = state?.diagnostics?.events ?? []
  const filtered = useMemo(
    () => (filter === 'ALL' ? events : events.filter((e) => e.severity === filter)),
    [events, filter],
  )
  const report = useMemo(
    () => (state ? buildReport(state, viewLabel) : ''),
    [state, viewLabel],
  )

  if (!state) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-neutral-500">
        No run data yet
      </div>
    )
  }

  const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
  const kinds = ['ALL', 'FAILURE', 'CASCADE', 'REROUTE', 'WARNING', 'SUCCESS', 'INFO']

  return (
    <div className="flex h-full w-full flex-col bg-ares-bg">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-ares-border bg-ares-panel px-3 py-2">
        <SectionTitle icon={<FileText className="h-3.5 w-3.5" />}>After-action report</SectionTitle>
        <span className="text-xs text-neutral-500">
          tick {state.tick} · {state.diagnostics?.clock ?? '--'} · {events.length} events
        </span>
        <div className="ml-auto flex flex-wrap items-center gap-1">
          {kinds.map((k) => (
            <Chip key={k} active={filter === k} onClick={() => setFilter(k)}>
              {k === 'ALL' ? 'All' : k.toLowerCase()}
            </Chip>
          ))}
        </div>
        <div className="flex gap-1.5">
          <Button
            size="sm"
            variant="emphasis"
            onClick={() => download(`ares-report-${stamp}.txt`, report, 'text/plain;charset=utf-8')}
            icon={<Download className="h-3 w-3" />}
          >
            .txt
          </Button>
          <Button
            size="sm"
            onClick={() => download(`ares-report-${stamp}.json`, JSON.stringify(state, null, 2), 'application/json')}
            icon={<Download className="h-3 w-3" />}
          >
            .json
          </Button>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-0 overflow-hidden lg:grid-cols-2">
        <div className="min-h-0 overflow-y-auto border-r border-ares-border p-3">
          <h3 className="mb-2 flex items-center gap-1.5 text-xs text-neutral-500">
            <ClipboardList className="h-3.5 w-3.5" />
            Narrative log ({filtered.length})
          </h3>
          {filtered.length === 0 ? (
            <p className="text-sm text-neutral-500">No entries for this filter.</p>
          ) : (
            <ol className="flex flex-col">
              {filtered.map((e) => (
                <li
                  key={e.id}
                  className="flex items-start gap-2 border-b border-neutral-800/70 px-1 py-1.5 text-sm leading-snug"
                >
                  <span className="shrink-0 font-mono text-xs tabular-nums text-neutral-600">
                    T+{String(e.tick).padStart(4, '0')}
                  </span>
                  <span className="shrink-0 font-mono text-xs tabular-nums text-neutral-500">
                    {e.clock}
                  </span>
                  <span
                    className={`w-16 shrink-0 text-xs font-semibold uppercase tracking-wide ${
                      SEVERITY_TONE[e.severity]?.text ?? 'text-neutral-300'
                    }`}
                  >
                    {e.severity}
                  </span>
                  <span className="min-w-0 flex-1 text-neutral-200">{e.message}</span>
                </li>
              ))}
            </ol>
          )}
        </div>

        <div className="min-h-0 overflow-y-auto bg-neutral-950/40 p-3">
          <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-neutral-300">
            {report}
          </pre>
        </div>
      </div>
    </div>
  )
}
