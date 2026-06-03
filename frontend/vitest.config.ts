import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { resolve } from 'node:path'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals:     true,
    setupFiles:  ['./src/test/setup.ts'],
    env: {
      BACKEND_INTERNAL_URL: 'https://backend:8080',
    },
    coverage: {
      provider:         'v8',
      reportsDirectory: './coverage',
      reporter:         ['text', 'json-summary', 'lcov'],
      include: [
        'src/lib/utils.ts',
        'src/lib/sse.ts',
        'src/components/shared/**/*.tsx',
        'src/lib/api.ts',
        'src/lib/auth.ts',
        'src/app/api/health/route.ts',
        'src/app/api/[...path]/route.ts',
        'src/app/api/notifications/stream/route.ts',
        'src/hooks/useBalance.ts',
        'src/hooks/useExchangeRate.ts',
        'src/hooks/useKYCStatus.ts',
        'src/hooks/useSSE.ts',
        'src/components/layout/Footer.tsx',
        'src/components/layout/Header.tsx',
        'src/components/layout/AdminSidebar.tsx',
        'src/components/layout/MobileNav.tsx',
        'src/components/balance/BalanceCard.tsx',
        'src/components/exchange/RateTicker.tsx',
      ],
      thresholds: {
        lines:      80,
        functions:  80,
        branches:   80,
        statements: 80,
      },
    },
  },
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
})
