import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('ARES render failure', error, info.componentStack)
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-ares-bg p-6">
        <div className="max-w-xl rounded-md border border-red-500/40 bg-red-950/30 p-4">
          <h1 className="text-sm font-semibold uppercase tracking-wide text-red-300">
            Dashboard render failure
          </h1>
          <p className="mt-2 text-sm text-neutral-400">
            The UI hit an unrecoverable error. The API and WebSocket keep running; reload to
            reconnect.
          </p>
          <pre className="mt-3 max-h-48 overflow-auto whitespace-pre-wrap font-mono text-xs text-red-200/90">
            {this.state.error.message}
          </pre>
          <button
            onClick={() => window.location.reload()}
            className="mt-3 rounded border border-neutral-600 bg-neutral-800 px-3 py-1.5 text-xs font-medium text-neutral-100 transition hover:bg-neutral-700"
          >
            Reload dashboard
          </button>
        </div>
      </div>
    )
  }
}
