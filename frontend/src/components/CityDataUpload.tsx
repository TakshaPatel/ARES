import { useRef, useState } from 'react'
import { AlertTriangle, CheckCircle2, Database, FileJson, MapPin, Upload } from 'lucide-react'
import { loadPresets, loadScenarios, useSimStore } from '../store/useSimStore'
import { parseCityData, SAMPLE_CITY_DATA, type ParsedCity } from '../lib/cityData'
import { CENTER, ZOOM } from '../lib/maptiler'

const MAX_BYTES = 8 * 1024 * 1024

export default function CityDataUpload() {
  const setView = useSimStore((s) => s.setView)
  const setError = useSimStore((s) => s.setError)
  const applyState = useSimStore((s) => s.applyState)
  const resetHistory = useSimStore((s) => s.resetHistory)
  const activeScenario = useSimStore((s) => s.activeScenario)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [parsed, setParsed] = useState<ParsedCity | null>(null)
  const [error, setLocalError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [applied, setApplied] = useState<string | null>(null)

  function ingest(text: string, source: string) {
    try {
      setParsed(parseCityData(text, source))
      setLocalError(null)
      setError(null)
    } catch (e) {
      setParsed(null)
      setLocalError((e as Error).message)
    }
  }

  async function onFile(f: File | undefined) {
    if (!f) return
    if (f.size > MAX_BYTES) {
      setLocalError(`File is ${(f.size / 1048576).toFixed(1)} MB - the limit is 8 MB.`)
      return
    }
    const text = await f.text()
    ingest(text, f.name)
  }

  function onDrop(e: React.DragEvent) {
    e.preventDefault()
    void onFile(e.dataTransfer.files?.[0])
  }

  async function apply() {
    if (!parsed) return
    setBusy(true)
    setApplied(null)
    try {
      if (parsed.kind === 'scenario' && parsed.scenario) {
        const res = await fetch('/api/scenarios/load', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ scenarioId: '', inline: parsed.raw }),
        })
        const body = (await res.json()) as {
          state?: Parameters<typeof applyState>[0]
          error?: string
          name?: string
        }
        if (!res.ok) throw new Error(body.error ?? `Upload failed (${res.status})`)
        if (body.state) {
          applyState(body.state)
          resetHistory()
        }
        void loadScenarios()
        void loadPresets()
        setView({
          center: parsed.view.center,
          zoom: parsed.view.zoom,
          label: parsed.label,
          source: parsed.source,
        })
        setApplied(`Scenario "${body.name ?? parsed.label}" loaded`)
      } else {
        setView({
          center: parsed.view.center ?? CENTER,
          zoom: parsed.view.zoom ?? ZOOM,
          label: parsed.label,
          source: parsed.source,
        })
        setApplied(`View set to ${parsed.label}`)
      }
    } catch (e) {
      setLocalError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const b = parsed?.bounds

  return (
    <div className="h-full w-full overflow-y-auto bg-ares-bg p-4">
      <div className="mx-auto flex max-w-3xl flex-col gap-3">
        <header>
          <h2 className="flex items-center gap-2 text-base font-medium text-neutral-100">
            <Database className="h-4 w-4" />
            Upload City Data
          </h2>
          <p className="mt-1 text-sm leading-relaxed text-neutral-400">
            Load a city or region to drive the exercise. A file with both{' '}
            <code className="text-neutral-200">nodes</code> and <code className="text-neutral-200">connections</code>{' '}
            replaces the active scenario. A lighter file with just a name and a{' '}
            <code className="text-neutral-200">center</code> repositions the map without changing the
            network.
          </p>
        </header>

        <div
          onDragOver={(e) => e.preventDefault()}
          onDrop={onDrop}
          className="flex flex-col items-center gap-3 rounded border border-dashed border-neutral-600 bg-ares-panel/60 p-6 text-center"
        >
          <FileJson className="h-7 w-7 text-neutral-500" />
          <p className="text-sm text-neutral-300">
            Drop a .json city file here
          </p>
          <p className="text-xs text-neutral-500">Max 8 MB</p>
          <div className="flex flex-wrap items-center justify-center gap-2">
            <button
              onClick={() => inputRef.current?.click()}
              className="flex items-center gap-1.5 rounded border border-neutral-500 bg-neutral-800 px-3 py-1.5 text-sm font-medium text-neutral-100 transition hover:bg-neutral-700"
            >
              <Upload className="h-3.5 w-3.5" />
              Choose file
            </button>
            <button
              onClick={() => ingest(SAMPLE_CITY_DATA, 'sample-north-brunswick.json')}
              className="rounded border border-neutral-700 px-3 py-1.5 text-sm text-neutral-400 transition hover:border-neutral-500 hover:text-neutral-100"
            >
              Use sample
            </button>
          </div>
          <input
            ref={inputRef}
            type="file"
            accept=".json,application/json"
            className="hidden"
            onChange={(e) => void onFile(e.target.files?.[0])}
          />
        </div>

        {error && (
          <div className="flex items-start gap-2 rounded border border-red-500/50 bg-red-500/10 p-2.5 text-sm text-red-200">
            <AlertTriangle className="mt-px h-3.5 w-3.5 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {applied && (
          <div className="flex items-center gap-2 rounded border border-emerald-600/50 bg-emerald-600/10 p-2.5 text-sm text-emerald-200">
            <CheckCircle2 className="h-3.5 w-3.5 shrink-0" />
            <span>{applied}</span>
          </div>
        )}

        {parsed && (
          <div className="flex flex-col gap-3 rounded border border-neutral-700 bg-ares-panel/70 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <span
                className={`rounded px-1.5 py-0.5 text-xs font-medium ${
                  parsed.kind === 'scenario'
                    ? 'bg-amber-500/15 text-amber-200'
                    : 'bg-neutral-700 text-neutral-100'
                }`}
              >
                {parsed.kind === 'scenario' ? 'Full scenario' : 'View profile'}
              </span>
              <span className="truncate font-mono text-sm text-neutral-50">{parsed.label}</span>
              <span className="truncate text-xs text-neutral-500">
                {parsed.source}
              </span>
            </div>

            <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs sm:grid-cols-4">
              <div>
                <dt className="text-neutral-500">Nodes</dt>
                <dd className="text-neutral-100">{parsed.nodeCount}</dd>
              </div>
              <div>
                <dt className="text-neutral-500">Connections</dt>
                <dd className="text-neutral-100">{parsed.connectionCount}</dd>
              </div>
              <div>
                <dt className="text-neutral-500">Center</dt>
                <dd className="text-neutral-100">
                  {parsed.view.center
                    ? `${parsed.view.center[1].toFixed(4)}, ${parsed.view.center[0].toFixed(4)}`
                    : '-'}
                </dd>
              </div>
              <div>
                <dt className="text-neutral-500">Zoom</dt>
                <dd className="text-neutral-100">
                  {parsed.view.zoom ? parsed.view.zoom.toFixed(1) : '-'}
                </dd>
              </div>
            </dl>

            {b && (
              <p className="flex items-start gap-1.5 text-sm leading-relaxed text-neutral-400">
                <MapPin className="mt-px h-3 w-3 shrink-0 text-neutral-500" />
                <span>
                  Bounds lat {b.minLat.toFixed(4)} to {b.maxLat.toFixed(4)} · lng{' '}
                  {b.minLng.toFixed(4)} to {b.maxLng.toFixed(4)} (
                  {((b.maxLat - b.minLat) * 111).toFixed(1)} km N-S)
                </span>
              </p>
            )}

            {parsed.blocking.length > 0 && (
              <ul className="flex flex-col gap-0.5 rounded border border-red-500/40 bg-red-500/5 p-2">
                {parsed.blocking.map((b) => (
                  <li key={b} className="flex gap-1.5 text-sm leading-relaxed text-red-200">
                    <span className="shrink-0 text-red-400">✕</span>
                    <span>
                      <span className="font-bold uppercase">Rejected: </span>
                      {b}
                    </span>
                  </li>
                ))}
              </ul>
            )}

            {parsed.warnings.length > 0 && (
              <ul className="flex flex-col gap-0.5">
                {parsed.warnings.map((w) => (
                  <li key={w} className="flex gap-1.5 text-sm text-amber-200/90">
                    <span className="text-amber-500">▸</span>
                    {w}
                  </li>
                ))}
              </ul>
            )}

            {parsed.kind === 'scenario' && (
              <p className="text-sm leading-relaxed text-neutral-500">
                Applying replaces the running scenario ({activeScenario}) and resets the clock. Any
                events already applied are discarded.
              </p>
            )}

            <button
              onClick={() => void apply()}
              disabled={busy || parsed.blocking.length > 0}
              className="self-start rounded border border-neutral-400 bg-neutral-700 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-neutral-600 disabled:opacity-50"
            >
              {busy
                ? 'Applying…'
                : parsed.kind === 'scenario'
                  ? 'Load scenario'
                  : 'Apply starting location'}
            </button>
          </div>
        )}

        <details className="rounded border border-neutral-700 bg-ares-panel/60 p-3">
          <summary className="cursor-pointer text-sm font-medium text-neutral-400">
            Expected file shape
          </summary>
          <pre className="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-all font-mono text-xs leading-relaxed text-neutral-500">
            {SAMPLE_CITY_DATA}
          </pre>
        </details>
      </div>
    </div>
  )
}
