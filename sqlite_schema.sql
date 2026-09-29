-- ARES :: Adaptive Resilience & Emergency Simulation
-- Embedded persistence schema (SQLite).

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS scenarios (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    hazard_model  TEXT NOT NULL DEFAULT '',
    nodes         INTEGER NOT NULL DEFAULT 0,
    connections   INTEGER NOT NULL DEFAULT 0,
    facilities    INTEGER NOT NULL DEFAULT 0,
    payload       TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS executions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    scenario_id       TEXT NOT NULL,
    started_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at          TIMESTAMP,
    final_tick        INTEGER NOT NULL DEFAULT 0,
    peak_power_loss   REAL NOT NULL DEFAULT 0,
    peak_comms_loss   REAL NOT NULL DEFAULT 0,
    isolated_facility_count INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (scenario_id) REFERENCES scenarios(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS execution_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id  INTEGER NOT NULL,
    tick          INTEGER NOT NULL,
    severity      TEXT NOT NULL,
    source        TEXT NOT NULL,
    node_id       TEXT,
    message       TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (execution_id) REFERENCES executions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS metrics_samples (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id        INTEGER NOT NULL,
    tick                INTEGER NOT NULL,
    power_grid_health   REAL NOT NULL,
    comms_coverage      REAL NOT NULL,
    road_accessibility  REAL NOT NULL,
    message_delivery    REAL NOT NULL,
    average_latency     REAL NOT NULL,
    connected_facilities INTEGER NOT NULL,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (execution_id) REFERENCES executions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_events_execution ON execution_events(execution_id, tick);
CREATE INDEX IF NOT EXISTS idx_metrics_execution ON metrics_samples(execution_id, tick);
CREATE INDEX IF NOT EXISTS idx_executions_scenario ON executions(scenario_id);
