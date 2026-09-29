import { useMemo } from 'react'
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { Activity, Gauge, Radio, Truck } from 'lucide-react'
import { useSimStore } from '../store/useSimStore'

function Kpi({
  label,
  value,
  unit,
  tone,
  icon,
  sub,
}: {
  label: string
  value: string
  unit?: string
  tone: string
  icon: React.ReactNode
  sub?: string
}) {
  return (
    <div className="rounded-md border border-ares-border bg-slate-900/40 p-2.5">
      <div className="flex items-center gap-1.5 text-slate-500">
        {icon}
        <span className="font-mono text-[9px] uppercase tracking-[0.15em]">{label}</span>
      </div>
      <div className={`mt-1 font-mono text-xl font-bold leading-none ${tone}`}>
        {value}
        {unit && <span className="ml-0.5 text-[11px] font-normal text-slate-500">{unit}</span>}
      </div>
      {sub && <div className="mt-0.5 truncate font-mono text-[9px] text-slate-600">{sub}</div>}
    </div>
  )
}

function tone(v: number, warn: number, bad: number) {
  if (v <= bad) return 'text-red-400'
  if (v <= warn) return 'text-amber-400'
  return 'text-emerald-400'
}

const tooltipStyle = {
  backgroundColor: '#0d131b',
  border: '1px solid #1e2936',
  borderRadius: 6,
  fontSize: 11,
  color: '#e2e8f0',
  fontFamily: 'ui-monospace, monospace',
}

export default function MetricsDashboard() {
  const state = useSimStore((s) => s.state)
  const history = useSimStore((s) => s.history)

  const chartData = useMemo(
    () =>
      history.map((h) => ({
        tick: h.tick,
        power: h.powerGridHealth,
        comms: h.commsCoverage,
        road: h.roadAccessibility,
        delivery: h.messageDeliveryRate,
        latency: h.averageLatency,
      })),
    [history],
  )

  if (!state) {
    return (
      <section className="border-t border-ares-border bg-ares-panel/60 p-4">
        <p className="font-mono text-[11px] text-slate-600">Awaiting telemetry…</p>
      </section>
    )
  }

  const powerTone = tone(state.powerGridHealth, 60, 30)
  const commsTone = tone(state.commsCoverage, 60, 30)
  const roadTone = tone(state.roadAccessibility, 60, 30)
  const deliveryTone = tone(state.messageDeliveryRate, 90, 70)
  const latTone = state.averageLatency > 200 ? 'text-red-400' : state.averageLatency > 100 ? 'text-amber-400' : 'text-emerald-400'
  const connectPct =
    state.totalFacilities > 0 ? (state.connectedFacilities / state.totalFacilities) * 100 : 0
  const routeCount = state.diagnostics?.emsRoutes.length ?? 0

  return (
    <section className="border-t border-ares-border bg-ares-panel/60">
      <header className="flex items-center justify-between px-3 py-2">
        <h2 className="flex items-center gap-1.5 font-mono text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">
          <Activity className="h-3.5 w-3.5" />
          Resilience Metrics
        </h2>
        <span className="font-mono text-[10px] text-slate-600">{history.length} samples</span>
      </header>

      <div className="grid grid-cols-2 gap-1.5 px-3 pb-2 sm:grid-cols-3">
        <Kpi
          label="Power Grid"
          value={state.powerGridHealth.toFixed(1)}
          unit="%"
          tone={powerTone}
          icon={<Gauge className="h-3 w-3" />}
          sub={`criticality ${state.diagnostics?.powerCriticality ?? 0}`}
        />
        <Kpi
          label="Comms Coverage"
          value={state.commsCoverage.toFixed(1)}
          unit="%"
          tone={commsTone}
          icon={<Radio className="h-3 w-3" />}
          sub={`${state.diagnostics?.meshBackoff.length ?? 0} on mesh fallback`}
        />
        <Kpi
          label="Road Access"
          value={state.roadAccessibility.toFixed(1)}
          unit="%"
          tone={roadTone}
          icon={<Truck className="h-3 w-3" />}
          sub={`${state.diagnostics?.blockedEdges.length ?? 0} segments blocked`}
        />
        <Kpi
          label="Delivery Rate"
          value={state.messageDeliveryRate.toFixed(1)}
          unit="%"
          tone={deliveryTone}
          icon={<Activity className="h-3 w-3" />}
          sub="per-hop packet success"
        />
        <Kpi
          label="Avg Latency"
          value={state.averageLatency.toFixed(0)}
          unit="ms"
          tone={latTone}
          icon={<Radio className="h-3 w-3" />}
          sub="to nearest EOC"
        />
        <Kpi
          label="Facilities"
          value={`${state.connectedFacilities}/${state.totalFacilities}`}
          tone={state.isolatedFacilities.length ? 'text-amber-400' : 'text-emerald-400'}
          icon={<Truck className="h-3 w-3" />}
          sub={`${connectPct.toFixed(0)}% connected · ${routeCount} routes`}
        />
      </div>

      <div className="grid grid-cols-1 gap-2 px-3 pb-3 lg:grid-cols-2">
        <div className="rounded-md border border-ares-border bg-slate-900/40 p-2">
          <h3 className="mb-1 font-mono text-[9px] uppercase tracking-[0.15em] text-slate-500">
            Infrastructure Integrity %
          </h3>
          <ResponsiveContainer width="100%" height={116}>
            <AreaChart data={chartData} margin={{ top: 2, right: 4, bottom: 0, left: -22 }}>
              <defs>
                <linearGradient id="gPower" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#facc15" stopOpacity={0.5} />
                  <stop offset="100%" stopColor="#facc15" stopOpacity={0.02} />
                </linearGradient>
                <linearGradient id="gComms" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#22d3ee" stopOpacity={0.5} />
                  <stop offset="100%" stopColor="#22d3ee" stopOpacity={0.02} />
                </linearGradient>
                <linearGradient id="gRoad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#94a3b8" stopOpacity={0.4} />
                  <stop offset="100%" stopColor="#94a3b8" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid stroke="#1e2936" strokeDasharray="2 4" />
              <XAxis dataKey="tick" tick={{ fontSize: 8, fill: '#475569' }} stroke="#1e2936" interval="preserveStartEnd" />
              <YAxis domain={[0, 100]} tick={{ fontSize: 8, fill: '#475569' }} stroke="#1e2936" />
              <Tooltip contentStyle={tooltipStyle} isAnimationActive={false} />
              <Area type="monotone" dataKey="power" name="Power" stroke="#facc15" strokeWidth={1.5} fill="url(#gPower)" isAnimationActive={false} />
              <Area type="monotone" dataKey="comms" name="Comms" stroke="#22d3ee" strokeWidth={1.5} fill="url(#gComms)" isAnimationActive={false} />
              <Area type="monotone" dataKey="road" name="Road" stroke="#94a3b8" strokeWidth={1.5} fill="url(#gRoad)" isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
          <div className="mt-1 flex gap-3 font-mono text-[9px] text-slate-500">
            <span className="flex items-center gap-1"><span className="h-0.5 w-3 bg-yellow-400" />Power</span>
            <span className="flex items-center gap-1"><span className="h-0.5 w-3 bg-cyan-400" />Comms</span>
            <span className="flex items-center gap-1"><span className="h-0.5 w-3 bg-slate-400" />Road</span>
          </div>
        </div>

        <div className="rounded-md border border-ares-border bg-slate-900/40 p-2">
          <h3 className="mb-1 font-mono text-[9px] uppercase tracking-[0.15em] text-slate-500">
            Delivery Rate % / Latency ms
          </h3>
          <ResponsiveContainer width="100%" height={116}>
            <LineChart data={chartData} margin={{ top: 2, right: 4, bottom: 0, left: -22 }}>
              <CartesianGrid stroke="#1e2936" strokeDasharray="2 4" />
              <XAxis dataKey="tick" tick={{ fontSize: 8, fill: '#475569' }} stroke="#1e2936" interval="preserveStartEnd" />
              <YAxis yAxisId="l" domain={[0, 100]} tick={{ fontSize: 8, fill: '#475569' }} stroke="#1e2936" />
              <YAxis yAxisId="r" orientation="right" tick={{ fontSize: 8, fill: '#475569' }} stroke="#1e2936" />
              <Tooltip contentStyle={tooltipStyle} isAnimationActive={false} />
              <Line yAxisId="l" type="monotone" dataKey="delivery" name="Delivery %" stroke="#10b981" strokeWidth={1.5} dot={false} isAnimationActive={false} />
              <Line yAxisId="r" type="monotone" dataKey="latency" name="Latency ms" stroke="#f59e0b" strokeWidth={1.5} strokeDasharray="3 3" dot={false} isAnimationActive={false} />
            </LineChart>
          </ResponsiveContainer>
          <div className="mt-1 flex gap-3 font-mono text-[9px] text-slate-500">
            <span className="flex items-center gap-1"><span className="h-0.5 w-3 bg-emerald-400" />Delivery %</span>
            <span className="flex items-center gap-1"><span className="h-0.5 w-3 bg-amber-400" />Latency ms</span>
          </div>
        </div>
      </div>
    </section>
  )
}
