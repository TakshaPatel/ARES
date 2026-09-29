import { useEffect, useState } from 'react'
import { AlertTriangle, Network, Workflow } from 'lucide-react'
import SystemHeader from './components/SystemHeader'
import MapView from './components/MapView'
import DependencyGraph from './components/DependencyGraph'
import ControlPanel from './components/ControlPanel'
import MetricsDashboard from './components/MetricsDashboard'
import EventLog from './components/EventLog'
import { connect, loadScenarios, useSimStore } from './store/useSimStore'

function ActiveAlerts() {
  const alerts = useSimStore((s) => s.state?.activeAlerts ?? [])
  const lastError = useSimStore((s) => s.lastError)
  const setError = useSimStore((s) => s.setError)

  if (alerts.length === 0 && !lastError) return null

  return (
    <div className="max-h-24 shrink-0 overflow-y-auto border-b border-ares-border bg-red-950/20 px-3 py-1.5">
      {lastError && (
        <div className="flex items-center gap-2 font-mono text-[10px] text-red-300">
          <AlertTriangle className="h-3 w-3 shrink-0" />
          <span className="flex-1">{lastError}</span>
          <button onClick={() => setError(null)} className="text-red-400 hover:text-red-200">
            ✕
          </button>
        </div>
      )}
      {alerts.map((a) => (
        <div key={a} className="flex items-center gap-1.5 font-mono text-[10px] text-amber-200/80">
          <span className="text-amber-500">▸</span>
          <span className="truncate">{a}</span>
        </div>
      ))}
    </div>
  )
}

export default function App() {
  const tab = useSimStore((s) => s.tab)
  const setTab = useSimStore((s) => s.setTab)
  const [tabHover, setTabHover] = useState(false)

  useEffect(() => {
    const disconnect = connect()
    void loadScenarios()
    return disconnect
  }, [])

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-ares-bg font-sans text-slate-200">
      <SystemHeader />

      <div className="flex min-h-0 flex-1">
        <main className="flex w-3/5 min-w-0 flex-col border-r border-ares-border">
          <div className="flex shrink-0 items-center gap-1 border-b border-ares-border bg-ares-panel/60 px-2 py-1.5">
            <TabButton active={tab === 'map'} onClick={() => setTab('map')} icon={<Workflow className="h-3 w-3" />} label="Map View" />
            <TabButton active={tab === 'graph'} onClick={() => setTab('graph')} icon={<Network className="h-3 w-3" />} label="Dependency DAG" />
            <div className="ml-auto font-mono text-[9px] uppercase tracking-wider text-slate-600">
              {tab === 'map' ? 'MapLibre GL · geospatial' : 'React Flow · directed graph'}
            </div>
          </div>
          <div className="min-h-0 flex-1" onMouseEnter={() => setTabHover(true)} onMouseLeave={() => setTabHover(false)}>
            <div className="h-full w-full" style={{ display: tab === 'map' ? 'block' : 'none' }}>
              <MapView />
            </div>
            <div className="h-full w-full" style={{ display: tab === 'graph' ? 'block' : 'none' }}>
              <DependencyGraph />
            </div>
          </div>
        </main>

        <aside className="flex w-2/5 min-w-0 flex-col overflow-hidden">
          <div className="min-h-0 shrink-0 overflow-y-auto">
            <ControlPanel />
          </div>
          <ActiveAlerts />
          <MetricsDashboard />
          <EventLog />
        </aside>
      </div>
      <span className="hidden" data-tab-hover={tabHover} />
    </div>
  )
}

function TabButton({
  active,
  onClick,
  icon,
  label,
}: {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 rounded px-3 py-1.5 font-mono text-[10px] uppercase tracking-[0.15em] transition ${
        active
          ? 'bg-cyan-500/15 text-cyan-300 border border-cyan-500/40'
          : 'border border-transparent text-slate-500 hover:text-slate-300'
      }`}
    >
      {icon}
      {label}
    </button>
  )
}
