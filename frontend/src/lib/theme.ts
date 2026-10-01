import type { EventSeverity, NodeType } from '../types/simulation'

export const C = {
  power: '#d8dade',
  comms: '#e9eaec',
  road: '#8f9296',
  grid: '#2b2d31',
  tick: '#8b8d91',
  ink: '#e9eaec',
  panel: '#16171a',
  panel2: '#1d1e21',
  border: '#2b2d31',
  darkInk: '#1d1e21',
  danger: '#e04a4a',
  dangerFill: '#5c2020',
  warn: '#e0a32e',
  ok: '#4bbf73',
  dead: '#43464b',
  deadLine: '#6b4f3c',
  halo: '#000000',
} as const

export const NODE_COLOR: Record<NodeType, string> = {
  POWER_SUBSTATION: '#e8e2d0',
  CELL_TOWER: '#d8d8d4',
  RADIO_MESH: '#b9bcc0',
  ROAD_INTERSECTION: '#8f9296',
  HOSPITAL: '#f2f2f0',
  FIRE_STATION: '#e8e6e0',
  EMERGENCY_OPS_CENTER: '#ffffff',
}

export const NODE_LETTER: Record<NodeType, string> = {
  POWER_SUBSTATION: 'P',
  CELL_TOWER: 'T',
  RADIO_MESH: 'M',
  ROAD_INTERSECTION: 'R',
  HOSPITAL: 'H',
  FIRE_STATION: 'F',
  EMERGENCY_OPS_CENTER: 'E',
}

export const NODE_EMOJI: Record<NodeType, string> = {
  POWER_SUBSTATION: '⚡',
  CELL_TOWER: '📡',
  RADIO_MESH: '📶',
  ROAD_INTERSECTION: '🚦',
  HOSPITAL: '🏥',
  FIRE_STATION: '🚒',
  EMERGENCY_OPS_CENTER: '🏢',
}

export const SEVERITY_TONE: Record<
  EventSeverity,
  { border: string; text: string; label: string }
> = {
  FAILURE: { border: 'border-l-red-500', text: 'text-red-200', label: 'FAILURE' },
  CASCADE: { border: 'border-l-orange-500', text: 'text-orange-200', label: 'CASCADE' },
  REROUTE: { border: 'border-l-neutral-400', text: 'text-neutral-100', label: 'REROUTE' },
  WARNING: { border: 'border-l-amber-500', text: 'text-amber-200', label: 'WARNING' },
  SUCCESS: { border: 'border-l-emerald-500', text: 'text-emerald-200', label: 'RESTORE' },
  INFO: { border: 'border-l-neutral-600', text: 'text-neutral-400', label: 'INFO' },
}

export const PRESET_TONE: Record<string, string> = {
  CATASTROPHIC: 'border-red-500/40 bg-red-500/10 text-red-200',
  CRITICAL: 'border-orange-500/40 bg-orange-500/10 text-orange-200',
  HIGH: 'border-amber-500/40 bg-amber-500/10 text-amber-200',
  MEDIUM: 'border-neutral-600 bg-neutral-800 text-neutral-300',
}

export type StatusTone = 'ok' | 'warn' | 'danger' | 'muted' | 'accent'

export const STATUS_TONE: Record<StatusTone, string> = {
  ok: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-200',
  warn: 'border-amber-500/40 bg-amber-500/10 text-amber-200',
  danger: 'border-red-500/40 bg-red-500/10 text-red-200',
  muted: 'border-neutral-700 bg-neutral-800/60 text-neutral-300',
  accent: 'border-neutral-500 bg-neutral-700 text-white',
}

export const TOOLTIP_STYLE = {
  backgroundColor: C.panel,
  border: `1px solid ${C.border}`,
  borderRadius: 8,
  fontSize: 12,
  color: C.ink,
} as const