# ARES

**Adaptive Resilience & Emergency Simulation** — a cascading-failure console for municipal utility, communications, road and EMS networks. Inject a substation outage or a flood, then watch the failure propagate through a real dependency graph, one 5-minute tick at a time.

![ARES console](demo_screenshot.png)

---

## Quick start

Requires **Go 1.22+** and **Node 18+**.

```bash
./run.sh
```

That installs frontend deps, builds the UI, and serves everything on <http://localhost:8080>. Pass `--no-build` to skip the rebuild.

For frontend hot reload, run the API and dev server separately:

```bash
go run . -frontend          # API on :8080
cd frontend && npm run dev  # Vite on :5173, proxies /api and /ws
```

## How it works

```
main.go ──► internal/api       REST + WebSocket surface
         ├─► internal/scenario loader, validation, SQLite persistence
         └─► internal/simulation  cascade engine, comms mesh, EMS routing
```

- **`internal/simulation`** is the whole model. Each tick it recomputes power grid health, comms coverage, road accessibility, delivery rate and latency, then propagates outages through the dependency graph to derive cascade depth, isolated facilities and EMS routes. Pure Go, no I/O, fully unit tested.
- **`internal/scenario`** loads and validates scenarios from `internal/scenario/seed/*.json` (embedded at build time), an external directory, or SQLite.
- **`internal/api`** exposes state over REST and a WebSocket that pushes the full state after every change.
- **`frontend`** is a React + TypeScript dashboard: MapLibre geospatial view, React Flow dependency DAG, an exportable after-action report, and a scenario uploader.

### Console tabs

| Tab | Purpose |
| --- | --- |
| **Map** | Satellite/geographic view of the network with live outage overlays |
| **Dependencies** | The same network as a layered DAG — trace an edge backwards to a root cause, forwards to the services that failed |
| **Reports** | Running after-action report, exportable as `.txt` or `.json` |
| **Upload** | Load a different city as JSON, or just a new map centre |

### WebSocket commands

`START` · `PAUSE` · `STEP` · `RESET` · `INJECT_FAILURE` · `INJECT_ROAD_BLOCK` · `APPLY_PRESET` · `APPLY_PRESETS` · `RESTORE_NODE` · `PING`

`APPLY_PRESETS` accepts a list, so several events stack in order and each one re-evaluates the cascade on top of the last.

## Scenario data

The bundled scenario, `north-brunswick-hurricane`, models a major-hurricane wind and rain event over **North Brunswick Township, Middlesex County, New Jersey** — 30 nodes, 60 connections, 8 stacked-event presets.

Real and sourced: substation names and locations (including the 69 kV station on 14th Street), hospital and volunteer fire company names and addresses, road junctions, and the flood-risk framework. Verified against PSE&G and PJM planning filings, NJDEP permit `NJG0306801`, RWJBarnabas Health, township directories, FEMA NFIP, and the US Census.

Modelled, and labelled as such in the data: cell tower and mesh relay placement, the coverage they provide, and all cascade behaviour. North Brunswick and the City of New Brunswick are adjacent but separate municipalities, so the hospitals are reached by mutual aid — that is intentional, not an error.

Because the township sits roughly 15 miles inland on the Raritan, the hazard model is wind, tree-fall and river flooding. There is no storm surge. The rainfall profile is taken from Hurricane Ida (2021).

## Development

```bash
go test ./...              # simulation, scenario and API tests
gofmt -l internal/
go vet ./...
cd frontend && npm run typecheck && npm run build
```

## Configuration

| Flag | Default | Purpose |
| --- | --- | --- |
| `-frontend` | off | Serve the built UI from `-dist` |
| `-dist` | `frontend/dist` | Built frontend directory |
| `-addr` | `:8080` | Listen address |
| `-scenarios` | `scenarios` | Extra directory of scenario JSON files |
| `-db` | `data/ares.db` | SQLite path |

## Stack

Go 1.22 · React 18 · TypeScript 5.6 · Vite 5 · Tailwind 3.4 · Zustand 5 · MapLibre GL 4.7 · React Flow 12.3 · Recharts 2.13
