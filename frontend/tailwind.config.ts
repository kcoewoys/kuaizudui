import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        canvas: '#f6f9ff',
        ink: '#0d1d29',
        muted: '#5c6c7a',
        primary: '#006e2a',
        'primary-soft': '#e3fcef',
        border: '#e1e5e8',
      },
      fontFamily: {
        sans: ['system-ui', '-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', '"Noto Sans SC"', 'sans-serif'],
      },
    },
  },
  plugins: [],
} satisfies Config
