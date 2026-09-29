/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ares: {
          bg: '#070a0f',
          panel: '#0d131b',
          border: '#1e2936',
          accent: '#22d3ee',
          warn: '#f59e0b',
          danger: '#ef4444',
          ok: '#10b981',
        },
      },
      fontFamily: {
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      animation: {
        'pulse-edge': 'pulseEdge 1.2s ease-in-out infinite',
        'dash-flow': 'dashFlow 1s linear infinite',
        'fade-in': 'fadeIn .25s ease-out',
      },
      keyframes: {
        pulseEdge: {
          '0%,100%': { opacity: 0.25, strokeWidth: 1.5 },
          '50%': { opacity: 1, strokeWidth: 3.5 },
        },
        dashFlow: { to: { strokeDashoffset: '-20' } },
        fadeIn: { from: { opacity: 0 }, to: { opacity: 1 } },
      },
    },
  },
  plugins: [],
}
