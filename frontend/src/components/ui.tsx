import type { ReactNode } from 'react'
import { STATUS_TONE, type StatusTone } from '../lib/theme'

type ButtonVariant = 'ghost' | 'subtle' | 'emphasis' | 'primary' | 'danger' | 'warn'
type ButtonSize = 'sm' | 'md'

const VARIANT: Record<ButtonVariant, string> = {
  ghost: 'border border-neutral-700 bg-neutral-900 text-neutral-200 hover:border-neutral-500 hover:text-white',
  subtle: 'border border-transparent text-neutral-400 hover:bg-neutral-800 hover:text-neutral-100',
  emphasis: 'border border-neutral-500 bg-neutral-700 text-white hover:bg-neutral-600',
  primary: 'border border-emerald-500/40 bg-emerald-500/10 text-emerald-200 hover:bg-emerald-500/20',
  danger: 'border border-red-500/40 bg-red-500/10 text-red-200 hover:bg-red-500/20',
  warn: 'border border-amber-500/40 bg-amber-500/10 text-amber-200 hover:bg-amber-500/20',
}

const SIZE: Record<ButtonSize, string> = {
  sm: 'gap-1.5 px-2 py-1 text-xs',
  md: 'gap-2 px-3 py-2 text-sm',
}

export function Button({
  variant = 'ghost',
  size = 'md',
  icon,
  className = '',
  children,
  ...rest
}: {
  variant?: ButtonVariant
  size?: ButtonSize
  icon?: ReactNode
  className?: string
  children?: ReactNode
} & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      {...rest}
      className={`inline-flex items-center justify-center rounded-md font-medium transition disabled:opacity-40 ${SIZE[size]} ${VARIANT[variant]} ${className}`}
    >
      {icon}
      {children}
    </button>
  )
}

export function SectionTitle({
  icon,
  children,
  count,
  action,
  className = '',
}: {
  icon?: ReactNode
  children: ReactNode
  count?: number
  action?: ReactNode
  className?: string
}) {
  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <h2 className="flex items-center gap-1.5 text-sm font-medium text-neutral-300">
        {icon}
        {children}
      </h2>
      {count !== undefined && (
        <span className="rounded bg-neutral-800 px-1.5 py-0.5 font-mono text-xs tabular-nums text-neutral-500">
          {count}
        </span>
      )}
      {action}
    </div>
  )
}

export function Badge({
  label,
  value,
  tone = 'muted',
}: {
  label: string
  value: string
  tone?: StatusTone
}) {
  return (
    <div
      className={`flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 ${STATUS_TONE[tone]}`}
    >
      <span className="text-xs text-neutral-400">{label}</span>
      <span className="font-mono text-sm font-medium tabular-nums">{value}</span>
    </div>
  )
}

export function Chip({
  active,
  className = '',
  ...rest
}: { active: boolean; className?: string } & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      {...rest}
      className={`rounded px-2 py-1 text-xs transition ${
        active
          ? 'bg-neutral-700 text-white'
          : 'text-neutral-500 hover:bg-neutral-800 hover:text-neutral-200'
      } ${className}`}
    />
  )
}

export function Panel({ className = '', children }: { className?: string; children: ReactNode }) {
  return (
    <div className={`rounded-lg border border-neutral-800 bg-neutral-900/60 ${className}`}>
      {children}
    </div>
  )
}