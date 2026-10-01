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
import { C, TOOLTIP_STYLE } from '../lib/theme'
import { Panel, SectionTitle } from './ui'

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
    <Panel className="p-2.5">
      <div className="flex items-center gap-1.5 text-neutral-500">
        {icon}
        <span className="text-xs">{label}</span>
      </div>
      <div className={`mt-1 font-mono text-2xl font-semibold leading-none tabular-nums ${tone}`}>
        {value}
        {unit && <span className="ml-0.5 text-sm font-normal text-neutral-500">{unit}</span>}
      </div>
      {sub && <div className="mt-1 truncate text-xs text-neutral-500">{sub}</div>}
    </Panel>
  )
}

function tone(v: number, warn: number, bad: number) {
  if (v <= bad) return 'text-red-300'
  if (v <= warn) return 'text-amber-300'
  return 'text-emerald-300'
}

const axisTick = { fontSize: 10, fill: C.tick }

function Legend({ items }: { items: { color: string; label: string; dashed?: boolean }[] }) {
  return (
    <div className="mt-1.5 flex flex-wrap gap-3 text-xs text-neutral-500">
      {items.map((it) => (
        <span key={it.label} className="flex items-center gap-1.5">
          <span
            className="h-0.5 w-4"
            style={{
              backgroundColor: it.dashed
                ? `repeating-linear-gradient(to right, ${it.color} 0 3px, transparent 3px 6px)`
                : it.color,
            }}
          />
          {it.label}
        </span>
      ))}
    </div>
  )
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
      <section className="shrink-0 border-t border-neutral-800 p-4">
        <p className="text-sm text-neutral-500">Awaiting telemetry…</p>
      </section>
    )
  }

  const connectPct =
    state.totalFacilities > 0 ? (state.connectedFacilities / state.totalFacilities) * 100 : 0
  const routeCount = state.diagnostics?.emsRoutes.length ?? 0

  return (
    <section className="border-t border-neutral-800">
      <header className="flex items-center justify-between px-3 py-2">
        <SectionTitle icon={<Activity className="h-3.5 w-3.5" />}>Resilience metrics</SectionTitle>
        <span className="text-xs text-neutral-500">{history.length} samples</span>
      </header>

      <div className="grid grid-cols-2 gap-1.5 px-3 pb-2 sm:grid-cols-3">
        <Kpi
          label="Power grid"
          value={state.powerGridHealth.toFixed(1)}
          unit="%"
          tone={tone(state.powerGridHealth, 60, 30)}
          icon={<Gauge className="h-3.5 w-3.5" />}
          sub={`criticality ${state.diagnostics?.powerCriticality ?? 0}`}
        />
        <Kpi
          label="Comms coverage"
          value={state.commsCoverage.toFixed(1)}
          unit="%"
          tone={tone(state.commsCoverage, 60, 30)}
          icon={<Radio className="h-3.5 w-3.5" />}
          sub={`${state.diagnostics?.meshBackoff.length ?? 0} on mesh fallback`}
        />
        <Kpi
          label="Road access"
          value={state.roadAccessibility.toFixed(1)}
          unit="%"
          tone={tone(state.roadAccessibility, 60, 30)}
          icon={<Truck className="h-3.5 w-3.5" />}
          sub={`${state.diagnostics?.blockedEdges.length ?? 0} segments blocked`}
        />
        <Kpi
          label="Delivery rate"
          value={state.messageDeliveryRate.toFixed(1)}
          unit="%"
          tone={tone(state.messageDeliveryRate, 90, 70)}
          icon={<Activity className="h-3.5 w-3.5" />}
          sub="per-hop packet success"
        />
        <Kpi
          label="Avg latency"
          value={state.averageLatency.toFixed(0)}
          unit="ms"
          tone={
            state.averageLatency > 200
              ? 'text-red-300'
              : state.averageLatency > 100
                ? 'text-amber-300'
                : 'text-emerald-300'
          }
          icon={<Radio className="h-3.5 w-3.5" />}
          sub="to nearest EOC"
        />
        <Kpi
          label="Facilities"
          value={`${state.connectedFacilities}/${state.totalFacilities}`}
          tone={state.isolatedFacilities.length ? 'text-amber-300' : 'text-emerald-300'}
          icon={<Truck className="h-3.5 w-3.5" />}
          sub={`${connectPct.toFixed(0)}% connected · ${routeCount} routes`}
        />
      </div>

      <div className="grid grid-cols-1 gap-2 px-3 pb-3 lg:grid-cols-2">
        <Panel className="p-2.5">
          <h3 className="text-xs text-neutral-500">Infrastructure integrity %</h3>
          <ResponsiveContainer width="100%" height={124}>
            <AreaChart data={chartData} margin={{ top: 6, right: 6, bottom: 0, left: -22 }}>
              <CartesianGrid stroke={C.grid} strokeDasharray="2 4" />
              <XAxis
                dataKey="tick"
                tick={axisTick}
                stroke={C.grid}
                interval="preserveStartEnd"
              />
              <YAxis domain={[0, 100]} tick={axisTick} stroke={C.grid} />
              <Tooltip contentStyle={TOOLTIP_STYLE} isAnimationActive={false} />
              <Area
                type="monotone"
                dataKey="power"
                name="Power"
                stroke={C.power}
                strokeWidth={1.5}
                fill={C.power}
                fillOpacity={0.14}
                isAnimationActive={false}
              />
              <Area
                type="monotone"
                dataKey="comms"
                name="Comms"
                stroke={C.comms}
                strokeWidth={1.5}
                fill={C.comms}
                fillOpacity={0.1}
                isAnimationActive={false}
              />
              <Area
                type="monotone"
                dataKey="road"
                name="Road"
                stroke={C.road}
                strokeWidth={1.5}
                fill={C.road}
                fillOpacity={0.12}
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
          <Legend
            items={[
              { color: C.power, label: 'Power' },
              { color: C.comms, label: 'Comms' },
              { color: C.road, label: 'Road' },
            ]}
          />
        </Panel>

        <Panel className="p-2.5">
          <h3 className="text-xs text-neutral-500">Delivery rate % / latency ms</h3>
          <ResponsiveContainer width="100%" height={124}>
            <LineChart data={chartData} margin={{ top: 6, right: 6, bottom: 0, left: -22 }}>
              <CartesianGrid stroke={C.grid} strokeDasharray="2 4" />
              <XAxis
                dataKey="tick"
                tick={axisTick}
                stroke={C.grid}
                interval="preserveStartEnd"
              />
              <YAxis yAxisId="l" domain={[0, 100]} tick={axisTick} stroke={C.grid} />
              <YAxis yAxisId="r" orientation="right" tick={axisTick} stroke={C.grid} />
              <Tooltip contentStyle={TOOLTIP_STYLE} isAnimationActive={false} />
              <Line
                yAxisId="l"
                type="monotone"
                dataKey="delivery"
                name="Delivery %"
                stroke={C.power}
                strokeWidth={1.5}
                dot={false}
                isAnimationActive={false}
              />
              <Line
                yAxisId="r"
                type="monotone"
                dataKey="latency"
                name="Latency ms"
                stroke={C.road}
                strokeWidth={1.5}
                strokeDasharray="3 3"
                dot={false}
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
          <Legend
            items={[
              { color: C.power, label: 'Delivery %' },
              { color: C.road, label: 'Latency ms', dashed: true },
            ]}
          />
        </Panel>
      </div>
    </section>
  )
}