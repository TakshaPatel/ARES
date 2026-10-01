import { useEffect } from 'react'
import { AlertTriangle, Database, FileText, Network, Workflow } from 'lucide-react'
import SystemHeader from './components/SystemHeader'
import MapView from './components/MapView'
import DependencyGraph from './components/DependencyGraph'
import ControlPanel from './components/ControlPanel'
import MetricsDashboard from './components/MetricsDashboard'
import EventLog from './components/EventLog'
import ReportView from './components/ReportView'
import CityDataUpload from './components/CityDataUpload'
import { connect, loadScenarios, useSimStore } from './store/useSimStore'

const NO_ALERTS: string[] = []

const PANE_HINT: Record<string, string> = {
  map: 'MapLibre GL · geospatial',
  graph: 'React Flow · directed graph',
  reports: 'After-action report · exportable',
  upload: 'JSON city scenario / starting location',
}

function ActiveAlerts() {
  const alerts = useSimStore((s) => s.state?.activeAlerts) ?? NO_ALERTS
  const lastError = useSimStore((s) => s.lastError)
  const setError = useSimStore((s) => s.setError)

  if (alerts.length === 0 && !lastError) return null

  return (
    <div className="max-h-24 shrink-0 overflow-y-auto border-b border-neutral-800 bg-neutral-950/60 px-3 py-1.5">
      {lastError && (
        <div className="flex items-center gap-2 text-sm text-red-200">
          <AlertTriangle className="h-3 w-3 shrink-0" />
          <span className="flex-1">{lastError}</span>
          <button onClick={() => setError(null)} className="text-red-400 hover:text-red-200">
            ✕
          </button>
        </div>
      )}
      {alerts.map((a) => (
        <div key={a} className="flex items-center gap-1.5 text-sm text-neutral-300">
          <span className="text-amber-400">▸</span>
          <span className="truncate">{a}</span>
        </div>
      ))}
    </div>
  )
}

export default function App() {
  const tab = useSimStore((s) => s.tab)
  const setTab = useSimStore((s) => s.setTab)

  useEffect(() => {
    const disconnect = connect()
    void loadScenarios()
    return disconnect
  }, [])

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-ares-bg font-sans text-neutral-200">
      <SystemHeader />

      <div className="flex min-h-0 flex-1">
        <main className="flex w-3/5 min-w-0 flex-col border-r border-ares-border">
          <div className="flex shrink-0 items-center gap-1 border-b border-neutral-800 bg-ares-panel px-2 py-1.5">
            <TabButton active={tab === 'map'} onClick={() => setTab('map')} icon={<Workflow className="h-3 w-3" />} label="Map" />
            <TabButton active={tab === 'graph'} onClick={() => setTab('graph')} icon={<Network className="h-3 w-3" />} label="Dependencies" />
            <TabButton active={tab === 'reports'} onClick={() => setTab('reports')} icon={<FileText className="h-3 w-3" />} label="Reports" />
            <TabButton active={tab === 'upload'} onClick={() => setTab('upload')} icon={<Database className="h-3 w-3" />} label="Upload" />
            <div className="ml-auto text-xs text-neutral-500">
              {PANE_HINT[tab]}
            </div>
          </div>
          <div className="min-h-0 flex-1">
            <div className="h-full w-full" style={{ display: tab === 'map' ? 'block' : 'none' }}>
              <MapView active={tab === 'map'} />
            </div>
            <div className="h-full w-full" style={{ display: tab === 'graph' ? 'block' : 'none' }}>
              <DependencyGraph active={tab === 'graph'} />
            </div>
            <div className="h-full w-full" style={{ display: tab === 'reports' ? 'block' : 'none' }}>
              <ReportView />
            </div>
            <div className="h-full w-full" style={{ display: tab === 'upload' ? 'block' : 'none' }}>
              <CityDataUpload />
            </div>
          </div>
        </main>

        <aside className="flex w-2/5 min-w-0 flex-col overflow-hidden">
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
            <ControlPanel />
          </div>
          <ActiveAlerts />
          <MetricsDashboard />
          <EventLog />
        </aside>
      </div>
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
      className={`flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium transition ${
        active
          ? 'bg-neutral-700 text-white border border-neutral-500'
          : 'border border-transparent text-neutral-500 hover:text-neutral-200'
      }`}
    >
      {icon}
      {label}
    </button>
  )
}
