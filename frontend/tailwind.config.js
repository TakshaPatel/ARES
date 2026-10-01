/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ares: {
          bg: '#0a0a0b',
          panel: '#16171a',
          panel2: '#1d1e21',
          border: '#2b2d31',
          accent: '#c9ccd1',
          steel: '#6f8ba8',
          warn: '#e0a32e',
          danger: '#e04a4a',
          ok: '#4bbf73',
        },
      },
      fontFamily: {
        sans: [
          'Inter',
          'ui-sans-serif',
          'system-ui',
          '-apple-system',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'sans-serif',
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
}